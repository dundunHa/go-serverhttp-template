package dao

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dundunHa/go-serverhttp-template/internal/db"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// ErrAccountTokenNotFound 表示通过 token 反查 user 时未匹配到映射。
//
// service 层会把 dao 层的该错误向 API 层透传以驱动 webhook PENDING_USER_BINDING /
// verify 400 等业务路径。
var ErrAccountTokenNotFound = errors.New("dao: apple account token not found")

// SubscriptionDAO 暴露 Apple IAP 订阅领域的持久化操作。
//
// 当前 (U3) 仅实现 account-token 部分；后续 U4 会在同一接口上扩展 subscription 与 event
// 相关的写入与查询。集中式 DAO 简化 wiring：cmd/server/main.go 只构造一份。
type SubscriptionDAO interface {
	GetOrCreateAccountToken(ctx context.Context, userID int64) (string, error)
	GetAccountTokenByToken(ctx context.Context, token string) (model.AppleAccountToken, error)
}

type subscriptionDAO struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewSubscriptionDAO 构造一个面向 PostgreSQL 的 SubscriptionDAO。
func NewSubscriptionDAO(pool *pgxpool.Pool) SubscriptionDAO {
	return &subscriptionDAO{
		pool:    pool,
		queries: db.New(pool),
	}
}

// GetOrCreateAccountToken 返回 userID 对应的 appAccountToken UUID。
//
// 同一用户多次调用返回同一个 UUID。实现策略：在事务里先 select；未命中再生成 UUID v4
// 并 insert。借助 apple_account_tokens.UNIQUE(user_id) 约束兜底并发首调写入场景。
func (d *subscriptionDAO) GetOrCreateAccountToken(ctx context.Context, userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("subscription dao: invalid user id %d", userID)
	}

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("subscription dao: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := d.queries.WithTx(tx)
	existing, err := qtx.GetAppleAccountTokenByUser(ctx, userID)
	if err == nil {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return "", fmt.Errorf("subscription dao: commit: %w", commitErr)
		}
		return pgUUIDToString(existing.Token), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("subscription dao: lookup account token: %w", err)
	}

	token, err := newUUIDv4()
	if err != nil {
		return "", fmt.Errorf("subscription dao: generate uuid: %w", err)
	}
	pg, err := uuidStringToPg(token)
	if err != nil {
		return "", fmt.Errorf("subscription dao: encode uuid: %w", err)
	}
	inserted, err := qtx.InsertAppleAccountToken(ctx, db.InsertAppleAccountTokenParams{
		UserID: userID,
		Token:  pg,
	})
	if err != nil {
		return "", fmt.Errorf("subscription dao: insert account token: %w", err)
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return "", fmt.Errorf("subscription dao: commit: %w", commitErr)
	}
	return pgUUIDToString(inserted.Token), nil
}

// GetAccountTokenByToken 通过 UUID 反查映射；未命中或 UUID 无效都返回 ErrAccountTokenNotFound。
//
// 输入既可能是合法 UUID，也可能是来自 Apple payload 的可疑值；DAO 不区分是“格式坏”
// 还是“没绑定”，统一映射到 ErrAccountTokenNotFound 让 service 层用相同的失败路径。
func (d *subscriptionDAO) GetAccountTokenByToken(ctx context.Context, token string) (model.AppleAccountToken, error) {
	if token == "" {
		return model.AppleAccountToken{}, ErrAccountTokenNotFound
	}
	pg, err := uuidStringToPg(token)
	if err != nil {
		return model.AppleAccountToken{}, ErrAccountTokenNotFound
	}
	row, err := d.queries.GetAppleAccountTokenByToken(ctx, pg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.AppleAccountToken{}, ErrAccountTokenNotFound
		}
		return model.AppleAccountToken{}, fmt.Errorf("subscription dao: lookup token: %w", err)
	}
	return model.AppleAccountToken{
		ID:        row.ID,
		UserID:    row.UserID,
		Token:     pgUUIDToString(row.Token),
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

// newUUIDv4 产生 RFC 4122 version 4 (variant 10) 的 UUID 字符串。
// 不引入第三方 UUID 库；crypto/rand 是 stdlib 中安全随机源。
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func uuidStringToPg(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}

func pgUUIDToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

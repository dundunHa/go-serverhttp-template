package dao

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

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

// ErrSubscriptionNotFound 表示按 (original_transaction_id, environment) 查找时未命中。
var ErrSubscriptionNotFound = errors.New("dao: apple subscription not found")

// ErrSubscriptionOwnershipConflict 表示 webhook/verify 试图把已属于其他 user 的
// (original_transaction_id, environment) 写到当前 user。
//
// 该错误是 webhook OWNERSHIP_CONFLICT 路径与 verify 409 路径的唯一区分点。
var ErrSubscriptionOwnershipConflict = errors.New("dao: apple subscription owned by another user")

// SubscriptionDAO 暴露 Apple IAP 订阅领域的持久化操作。
//
// 单元划分：
//   - account-token: U3 完成
//   - 订阅 / 事件 读写: U4
//   - 状态 reducer / webhook 联动: U8 (service 层调用本接口)
//
// 所有写入路径（事件 + 订阅 upsert）都通过 InTx 包裹的事务作用域；service 在同一
// 事务内既写 apple_events 幂等键又 reduce apple_subscriptions，从而保证 atomic
// idempotency。读路径不开事务以减少锁竞争。
type SubscriptionDAO interface {
	GetOrCreateAccountToken(ctx context.Context, userID int64) (string, error)
	GetAccountTokenByToken(ctx context.Context, token string) (model.AppleAccountToken, error)

	GetSubscriptionByOriginalTx(ctx context.Context, originalTxID string, env model.AppleEnvironment) (model.Subscription, error)
	ListSubscriptionsForUserEntitlement(ctx context.Context, userID int64, allowedEnvs []model.AppleEnvironment) ([]model.Subscription, error)

	InTx(ctx context.Context, fn func(SubscriptionTx) error) error
}

// SubscriptionTx 暴露事务作用域的写入操作。仅在 SubscriptionDAO.InTx 回调里使用。
type SubscriptionTx interface {
	InsertAppleEventIfNotExists(ctx context.Context, in model.AppleEventInsert) (created bool, eventID int64, err error)
	UpsertSubscriptionWithOwnershipCheck(ctx context.Context, in model.SubscriptionUpsert) (model.Subscription, error)
	GetSubscriptionByOriginalTx(ctx context.Context, originalTxID string, env model.AppleEnvironment) (model.Subscription, error)
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

// GetSubscriptionByOriginalTx 在事务外按 natural key 查 apple_subscriptions。
func (d *subscriptionDAO) GetSubscriptionByOriginalTx(ctx context.Context, originalTxID string, env model.AppleEnvironment) (model.Subscription, error) {
	row, err := d.queries.GetSubscriptionByOriginalTx(ctx, db.GetSubscriptionByOriginalTxParams{
		OriginalTransactionID: originalTxID,
		Environment:           string(env),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Subscription{}, ErrSubscriptionNotFound
		}
		return model.Subscription{}, fmt.Errorf("subscription dao: get subscription: %w", err)
	}
	return mapSubscriptionRow(row), nil
}

// ListSubscriptionsForUserEntitlement 返回当前 user 在 allowedEnvs 内的订阅，按 entitlement 优先级排序。
//
// 排序由 sqlc 生成的 SQL 完成（见 db/queries/apple_subscriptions.sql）：先按是否仍有
// 有效权益 (status ∈ {ACTIVE,CANCELED} AND effective_end > now)，再按 level DESC，再按
// effective_end DESC，最后 last_event_at DESC。`/users/me` 的 reader 直接拿第一行即可。
func (d *subscriptionDAO) ListSubscriptionsForUserEntitlement(ctx context.Context, userID int64, allowedEnvs []model.AppleEnvironment) ([]model.Subscription, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("subscription dao: invalid user id %d", userID)
	}
	envs := make([]string, 0, len(allowedEnvs))
	for _, e := range allowedEnvs {
		envs = append(envs, string(e))
	}
	rows, err := d.queries.ListSubscriptionsForUserEntitlement(ctx, db.ListSubscriptionsForUserEntitlementParams{
		UserID:  userID,
		Column2: envs,
	})
	if err != nil {
		return nil, fmt.Errorf("subscription dao: list subscriptions: %w", err)
	}
	out := make([]model.Subscription, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapSubscriptionRow(r))
	}
	return out, nil
}

// InTx 在数据库事务内执行 fn，提交或回滚由 fn 的返回值驱动。
//
// service 层通过该方法保证 webhook 事件插入 + 订阅 upsert 的 atomic：fn 内任何 error
// 都会触发 rollback，包括 ErrSubscriptionOwnershipConflict 这种业务错误（事件不会落库）。
func (d *subscriptionDAO) InTx(ctx context.Context, fn func(SubscriptionTx) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("subscription dao: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	q := d.queries.WithTx(tx)
	if err := fn(&subscriptionTxQueries{tx: tx, queries: q}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("subscription dao: commit: %w", err)
	}
	committed = true
	return nil
}

type subscriptionTxQueries struct {
	tx      pgx.Tx
	queries *db.Queries
}

// InsertAppleEventIfNotExists 把通知幂等地写入 apple_events。
//
// 返回 created==false 表示 notification_uuid 已存在（重复投递）；service 应直接 ack 200。
// 返回 (true, id, nil) 表示新事件，service 可继续 reduce subscription。
func (s *subscriptionTxQueries) InsertAppleEventIfNotExists(ctx context.Context, in model.AppleEventInsert) (bool, int64, error) {
	if in.NotificationUUID == "" {
		return false, 0, errors.New("subscription dao: notification uuid required")
	}
	if in.RawJWSSHA256 == "" {
		return false, 0, errors.New("subscription dao: raw_jws_sha256 required")
	}
	id, err := s.queries.InsertAppleEventIfNotExists(ctx, db.InsertAppleEventIfNotExistsParams{
		NotificationUuid:      in.NotificationUUID,
		NotificationType:      in.NotificationType,
		Subtype:               in.Subtype,
		Environment:           string(in.Environment),
		UserID:                int64ToPgInt8(in.UserID),
		AppAccountToken:       optionalUUIDStringToPg(in.AppAccountToken),
		OriginalTransactionID: in.OriginalTransactionID,
		TransactionID:         in.TransactionID,
		WebOrderLineItemID:    in.WebOrderLineItemID,
		ProcessingStatus:      defaultIfEmpty(in.ProcessingStatus, model.EventStatusProcessed),
		ProcessingError:       in.ProcessingError,
		RawJwsSha256:          in.RawJWSSHA256,
		DecodedPayload:        in.DecodedPayload,
		NotificationCreatedAt: optionalTimePg(in.NotificationCreatedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("subscription dao: insert event: %w", err)
	}
	return true, id, nil
}

// UpsertSubscriptionWithOwnershipCheck 在事务内 upsert subscription 并保证 ownership 不漂移。
//
// 流程：
//  1. SELECT 现有 (original_transaction_id, environment) 行；
//  2. 若存在但 user_id 与入参不同 → 返回 ErrSubscriptionOwnershipConflict 触发回滚；
//  3. 否则调用 sqlc UpsertSubscription（ON CONFLICT DO UPDATE 不触碰 user_id 与 app_account_token）。
//
// 这样 verify 路径 409、webhook 路径 OWNERSHIP_CONFLICT 都从同一处错误中分流。
func (s *subscriptionTxQueries) UpsertSubscriptionWithOwnershipCheck(ctx context.Context, in model.SubscriptionUpsert) (model.Subscription, error) {
	if in.UserID <= 0 {
		return model.Subscription{}, fmt.Errorf("subscription dao: invalid user id %d", in.UserID)
	}
	if in.OriginalTransactionID == "" {
		return model.Subscription{}, errors.New("subscription dao: original transaction id required")
	}
	if in.Environment == "" {
		return model.Subscription{}, errors.New("subscription dao: environment required")
	}

	existing, err := s.queries.GetSubscriptionByOriginalTx(ctx, db.GetSubscriptionByOriginalTxParams{
		OriginalTransactionID: in.OriginalTransactionID,
		Environment:           string(in.Environment),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.Subscription{}, fmt.Errorf("subscription dao: lookup before upsert: %w", err)
	}
	if err == nil && existing.UserID != in.UserID {
		return model.Subscription{}, ErrSubscriptionOwnershipConflict
	}

	pg, err := uuidStringToPg(in.AppAccountToken)
	if err != nil {
		return model.Subscription{}, fmt.Errorf("subscription dao: encode app_account_token: %w", err)
	}

	row, err := s.queries.UpsertSubscription(ctx, db.UpsertSubscriptionParams{
		UserID:                    in.UserID,
		AppAccountToken:           pg,
		Environment:               string(in.Environment),
		OriginalTransactionID:     in.OriginalTransactionID,
		LastTransactionID:         in.LastTransactionID,
		WebOrderLineItemID:        in.WebOrderLineItemID,
		PlanID:                    in.PlanID,
		ProviderProductID:         in.ProviderProductID,
		SubscriptionGroupID:       in.SubscriptionGroupID,
		Level:                     int32(in.Level),
		Status:                    defaultIfEmpty(in.Status, model.SubscriptionStatusActive),
		AutoRenewStatus:           defaultIfEmpty(in.AutoRenewStatus, model.AutoRenewStatusUnknown),
		CurrentPeriodStart:        timeToPgTimestamptz(in.CurrentPeriodStart),
		CurrentPeriodEnd:          timeToPgTimestamptz(in.CurrentPeriodEnd),
		GracePeriodExpiresAt:      optionalTimePg(in.GracePeriodExpiresAt),
		LastEventAt:               timeToPgTimestamptz(in.LastEventAt),
		LastNotificationCreatedAt: optionalTimePg(in.LastNotificationCreatedAt),
		LastPayloadHash:           in.LastPayloadHash,
		LastTransactionSnapshot:   in.LastTransactionSnapshot,
	})
	if err != nil {
		return model.Subscription{}, fmt.Errorf("subscription dao: upsert: %w", err)
	}
	return mapSubscriptionRow(row), nil
}

// GetSubscriptionByOriginalTx 在事务内提供一致读，便于 service 在 reducer 中预读现有快照。
func (s *subscriptionTxQueries) GetSubscriptionByOriginalTx(ctx context.Context, originalTxID string, env model.AppleEnvironment) (model.Subscription, error) {
	row, err := s.queries.GetSubscriptionByOriginalTx(ctx, db.GetSubscriptionByOriginalTxParams{
		OriginalTransactionID: originalTxID,
		Environment:           string(env),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Subscription{}, ErrSubscriptionNotFound
		}
		return model.Subscription{}, fmt.Errorf("subscription dao: get subscription in tx: %w", err)
	}
	return mapSubscriptionRow(row), nil
}

func mapSubscriptionRow(row db.AppleSubscription) model.Subscription {
	out := model.Subscription{
		ID:                      row.ID,
		UserID:                  row.UserID,
		AppAccountToken:         pgUUIDToString(row.AppAccountToken),
		Environment:             model.AppleEnvironment(row.Environment),
		OriginalTransactionID:   row.OriginalTransactionID,
		LastTransactionID:       row.LastTransactionID,
		WebOrderLineItemID:      row.WebOrderLineItemID,
		PlanID:                  row.PlanID,
		ProviderProductID:       row.ProviderProductID,
		SubscriptionGroupID:     row.SubscriptionGroupID,
		Level:                   int(row.Level),
		Status:                  row.Status,
		AutoRenewStatus:         row.AutoRenewStatus,
		CurrentPeriodStart:      row.CurrentPeriodStart.Time,
		CurrentPeriodEnd:        row.CurrentPeriodEnd.Time,
		LastEventAt:             row.LastEventAt.Time,
		LastPayloadHash:         row.LastPayloadHash,
		LastTransactionSnapshot: row.LastTransactionSnapshot,
		CreatedAt:               row.CreatedAt.Time,
		UpdatedAt:               row.UpdatedAt.Time,
	}
	if row.GracePeriodExpiresAt.Valid {
		t := row.GracePeriodExpiresAt.Time
		out.GracePeriodExpiresAt = &t
	}
	if row.LastNotificationCreatedAt.Valid {
		t := row.LastNotificationCreatedAt.Time
		out.LastNotificationCreatedAt = &t
	}
	return out
}

// ───────── helpers ─────────

func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func uuidStringToPg(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}

// optionalUUIDStringToPg 允许空字符串（→ NULL）。
func optionalUUIDStringToPg(s string) pgtype.UUID {
	if s == "" {
		return pgtype.UUID{}
	}
	u, err := uuidStringToPg(s)
	if err != nil {
		return pgtype.UUID{}
	}
	return u
}

func pgUUIDToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func int64ToPgInt8(n int64) pgtype.Int8 {
	if n <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: n, Valid: true}
}

func timeToPgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func optionalTimePg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func defaultIfEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

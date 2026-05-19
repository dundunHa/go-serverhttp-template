package payment

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// AppleIAPDAO 是 IAPService 用到的 dao 子集。U6 / U8 都通过同一个接口写入。
type AppleIAPDAO interface {
	InTx(ctx context.Context, fn func(dao.SubscriptionTx) error) error
}

// AppleIAPService 实现 POST /payment/apple/verify 的业务流。
//
// 校验顺序与 plan 一致：fetch -> 类型/撤销/token 校验 -> token 映射 -> 事务 upsert。
type AppleIAPService struct {
	catalog  *Catalog
	verifier AppleTransactionVerifier
	tokens   *TokenService
	dao      AppleIAPDAO
	now      func() time.Time
}

// NewAppleIAPService 构造 service；任一关键依赖为 nil 时 service 调用都返回 ErrNotConfigured。
func NewAppleIAPService(catalog *Catalog, verifier AppleTransactionVerifier, tokens *TokenService, dao AppleIAPDAO) *AppleIAPService {
	return &AppleIAPService{
		catalog:  catalog,
		verifier: verifier,
		tokens:   tokens,
		dao:      dao,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// VerifyTransaction 是 verify endpoint 的服务层入口；返回 SubscriptionInfo 或 typed error。
func (s *AppleIAPService) VerifyTransaction(ctx context.Context, userID int64, transactionID string) (*model.SubscriptionInfo, error) {
	if s == nil || s.catalog == nil || s.verifier == nil || s.tokens == nil || s.dao == nil {
		return nil, ErrNotConfigured
	}
	if userID <= 0 {
		return nil, errors.New("apple iap: invalid user id")
	}
	if strings.TrimSpace(transactionID) == "" {
		return nil, errors.New("apple iap: transaction id required")
	}

	tx, err := s.verifier.FetchTransaction(ctx, transactionID, EnvProduction)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, ErrAppleTransactionNotFound
	}

	if err := s.validateTransaction(tx); err != nil {
		return nil, err
	}

	product, err := s.catalog.Lookup(tx.ProductID, tx.Environment)
	if err != nil {
		return nil, err
	}

	mapped, err := s.tokens.ResolveUserByToken(ctx, tx.AppAccountToken)
	if err != nil {
		if errors.Is(err, ErrAccountTokenNotFound) {
			return nil, ErrAppAccountTokenMismatch
		}
		return nil, err
	}
	if mapped.UserID != userID {
		return nil, ErrAppAccountTokenMismatch
	}

	upsertParams := buildVerifyUpsert(userID, tx, product, s.now())

	var upserted model.Subscription
	if err := s.dao.InTx(ctx, func(qtx dao.SubscriptionTx) error {
		sub, e := qtx.UpsertSubscriptionWithOwnershipCheck(ctx, upsertParams)
		if e != nil {
			return e
		}
		upserted = sub
		return nil
	}); err != nil {
		return nil, err
	}

	info := subscriptionInfoFromRow(upserted, s.now())
	return &info, nil
}

func (s *AppleIAPService) validateTransaction(tx *AppleTransaction) error {
	if !tx.IsAutoRenewableSubscription() {
		return ErrUnsupportedProductType
	}
	if tx.IsRevoked() {
		return ErrTransactionRevoked
	}
	if strings.TrimSpace(tx.AppAccountToken) == "" {
		return ErrEmptyAppAccountToken
	}
	if tx.OriginalTransactionID == "" {
		return errors.New("apple iap: missing original_transaction_id in transaction")
	}
	if tx.Environment == "" {
		return errors.New("apple iap: missing environment in transaction")
	}
	return nil
}

func buildVerifyUpsert(userID int64, tx *AppleTransaction, product Product, now time.Time) model.SubscriptionUpsert {
	status := model.SubscriptionStatusActive
	if tx.IsRevoked() {
		status = model.SubscriptionStatusRevoked
	}
	return model.SubscriptionUpsert{
		UserID:                userID,
		AppAccountToken:       tx.AppAccountToken,
		Environment:           tx.Environment,
		OriginalTransactionID: tx.OriginalTransactionID,
		LastTransactionID:     tx.TransactionID,
		WebOrderLineItemID:    tx.WebOrderLineItemID,
		PlanID:                product.PlanID,
		ProviderProductID:     product.ProductID,
		SubscriptionGroupID:   product.SubscriptionGroupID,
		Level:                 product.Level,
		Status:                status,
		AutoRenewStatus:       model.AutoRenewStatusUnknown,
		CurrentPeriodStart:    tx.PurchaseDate,
		CurrentPeriodEnd:      tx.ExpiresDate,
		LastEventAt:           now,
	}
}

// subscriptionInfoFromRow 把单条 subscription 行映射为 /users/me 用的 SubscriptionInfo。
//
// 这是 verify 路径的小写映射；U7 的 SubscriptionReader 会扩展为多行 entitlement 选择。
func subscriptionInfoFromRow(sub model.Subscription, now time.Time) model.SubscriptionInfo {
	info := model.SubscriptionInfo{
		ProductID:      sub.PlanID,
		SubscribeLevel: sub.Level,
	}
	if !sub.CurrentPeriodEnd.IsZero() {
		info.SubscribeExpiredTime = sub.CurrentPeriodEnd.UTC().Format(time.RFC3339)
	}
	info.Status = APIStatusForSubscription(sub, now)
	if info.Status == "EXPIRED" {
		info.SubscribeLevel = 0
	}
	return info
}

// APIStatusForSubscription 把 DB 内部 status + 时间窗 映射为对外暴露的 API status。
//
// 规则按 plan "## Entitlement Model" 表：
//   - ACTIVE 且仍在有效期 -> "ACTIVE"
//   - CANCELED 但仍在有效期 -> "CANCELED"
//   - ACTIVE / CANCELED 已过期 -> "EXPIRED"
//   - EXPIRED / REVOKED -> "EXPIRED"
//
// effective_end = max(current_period_end, grace_period_expires_at)。
func APIStatusForSubscription(sub model.Subscription, now time.Time) string {
	end := sub.CurrentPeriodEnd
	if sub.GracePeriodExpiresAt != nil && sub.GracePeriodExpiresAt.After(end) {
		end = *sub.GracePeriodExpiresAt
	}
	switch sub.Status {
	case model.SubscriptionStatusActive:
		if end.After(now) {
			return "ACTIVE"
		}
		return "EXPIRED"
	case model.SubscriptionStatusCanceled:
		if end.After(now) {
			return "CANCELED"
		}
		return "EXPIRED"
	case model.SubscriptionStatusExpired, model.SubscriptionStatusRevoked:
		return "EXPIRED"
	default:
		return "EXPIRED"
	}
}

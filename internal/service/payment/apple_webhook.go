package payment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// AppleWebhookService 处理 ASSN V2 通知的解码、幂等记录和订阅 reducer。
type AppleWebhookService struct {
	catalog  *Catalog
	verifier AppleWebhookVerifier
	tokens   *TokenService
	dao      AppleIAPDAO
	now      func() time.Time
}

// NewAppleWebhookService 构造 webhook service。任一关键依赖为 nil 时调用都返回 ErrNotConfigured。
func NewAppleWebhookService(catalog *Catalog, verifier AppleWebhookVerifier, tokens *TokenService, dao AppleIAPDAO) *AppleWebhookService {
	return &AppleWebhookService{
		catalog:  catalog,
		verifier: verifier,
		tokens:   tokens,
		dao:      dao,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// WebhookMaxBodyBytes 暴露 catalog 配置的最大 webhook body 大小，供路由层做 size guard。
func (s *AppleWebhookService) WebhookMaxBodyBytes() int {
	if s == nil || s.catalog == nil {
		return 0
	}
	return s.catalog.WebhookMaxBodyBytes()
}

// HandleSignedPayload 是 POST /webhooks/apple 的服务层入口。返回 nil 表示 200，
// 任意 error 由路由层映射到 HTTP 状态码（参见 api/payment.go mapWebhookError）。
//
// 内部三段：
//  1. 解码 + 校验 JWS；任何失败映射到 ErrAppleAuthRejected 或 ErrInvalidConfig。
//  2. 在事务内幂等写 apple_events，并按 reducer 规则写 apple_subscriptions。
//  3. 返回 nil（200）或 typed error 让路由层决定 4xx/5xx。
func (s *AppleWebhookService) HandleSignedPayload(ctx context.Context, signedPayload string) error {
	if s == nil || s.catalog == nil || s.verifier == nil || s.tokens == nil || s.dao == nil {
		return ErrNotConfigured
	}
	if strings.TrimSpace(signedPayload) == "" {
		return errors.New("apple iap: empty signed payload")
	}

	event, err := s.verifier.DecodeSignedPayload(ctx, signedPayload)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAppleAuthRejected, err)
	}
	if event == nil || event.NotificationUUID == "" {
		return fmt.Errorf("%w: missing notification uuid", ErrAppleAuthRejected)
	}
	if event.Environment == "" {
		return errors.New("apple iap: missing environment in webhook")
	}

	return s.dao.InTx(ctx, func(qtx dao.SubscriptionTx) error {
		return s.processInTx(ctx, qtx, event)
	})
}

func (s *AppleWebhookService) processInTx(ctx context.Context, qtx dao.SubscriptionTx, event *AppleWebhookEvent) error {
	notifTime := event.NotificationCreatedAt
	if notifTime.IsZero() {
		notifTime = s.now()
	}

	insert := model.AppleEventInsert{
		NotificationUUID:      event.NotificationUUID,
		NotificationType:      event.NotificationType,
		Subtype:               event.Subtype,
		Environment:           event.Environment,
		RawJWSSHA256:          event.SignedPayloadSHA256,
		NotificationCreatedAt: &notifTime,
		ProcessingStatus:      model.EventStatusProcessed,
	}
	if event.Transaction != nil {
		insert.AppAccountToken = event.Transaction.AppAccountToken
		insert.OriginalTransactionID = event.Transaction.OriginalTransactionID
		insert.TransactionID = event.Transaction.TransactionID
		insert.WebOrderLineItemID = event.Transaction.WebOrderLineItemID
	}

	classification := s.classifyEvent(ctx, qtx, event)
	insert.ProcessingStatus = classification.status
	insert.UserID = classification.userID
	insert.ProcessingError = classification.errorMessage

	created, _, err := qtx.InsertAppleEventIfNotExists(ctx, insert)
	if err != nil {
		return fmt.Errorf("apple iap webhook: insert event: %w", err)
	}
	if !created {
		return nil
	}

	if classification.status == model.EventStatusProcessed && classification.upsert != nil {
		if _, err := qtx.UpsertSubscriptionWithOwnershipCheck(ctx, *classification.upsert); err != nil {
			return fmt.Errorf("apple iap webhook: upsert subscription: %w", err)
		}
	}
	return nil
}

// eventClassification carries the decisions made before writing apple_events.
type eventClassification struct {
	status       string
	userID       int64
	errorMessage string
	upsert       *model.SubscriptionUpsert
}

func (s *AppleWebhookService) classifyEvent(ctx context.Context, qtx dao.SubscriptionTx, event *AppleWebhookEvent) eventClassification {
	tx := event.Transaction
	if tx == nil {
		if isKnownNotificationType(event.NotificationType) {
			return eventClassification{status: model.EventStatusProcessed}
		}
		return eventClassification{status: model.EventStatusIgnoredUnknownType}
	}
	if !tx.IsAutoRenewableSubscription() {
		return eventClassification{status: model.EventStatusIgnoredUnknownType, errorMessage: "non-subscription transaction"}
	}
	if strings.TrimSpace(tx.AppAccountToken) == "" {
		return eventClassification{status: model.EventStatusPendingUserBinding, errorMessage: "missing appAccountToken"}
	}

	mapped, err := s.tokens.ResolveUserByToken(ctx, tx.AppAccountToken)
	if err != nil {
		if errors.Is(err, ErrAccountTokenNotFound) {
			return eventClassification{status: model.EventStatusPendingUserBinding, errorMessage: "no user binding for appAccountToken"}
		}
		return eventClassification{status: model.EventStatusPermanentFailure, errorMessage: err.Error()}
	}

	product, err := s.catalog.Lookup(tx.ProductID, tx.Environment)
	if err != nil {
		if errors.Is(err, ErrUnknownProduct) {
			return eventClassification{status: model.EventStatusIgnoredUnknownType, userID: mapped.UserID, errorMessage: "product not in catalog"}
		}
		return eventClassification{status: model.EventStatusPermanentFailure, userID: mapped.UserID, errorMessage: err.Error()}
	}

	if existing, err := qtx.GetSubscriptionByOriginalTx(ctx, tx.OriginalTransactionID, tx.Environment); err == nil {
		if existing.UserID != mapped.UserID {
			return eventClassification{status: model.EventStatusOwnershipConflict, userID: existing.UserID, errorMessage: "original_transaction_id owned by another user"}
		}
	} else if !errors.Is(err, dao.ErrSubscriptionNotFound) {
		return eventClassification{status: model.EventStatusPermanentFailure, userID: mapped.UserID, errorMessage: err.Error()}
	}

	if !isKnownNotificationType(event.NotificationType) {
		return eventClassification{status: model.EventStatusIgnoredUnknownType, userID: mapped.UserID}
	}

	upsert := buildWebhookUpsert(mapped.UserID, event, product, s.now())
	return eventClassification{status: model.EventStatusProcessed, userID: mapped.UserID, upsert: &upsert}
}

func isKnownNotificationType(t string) bool {
	switch strings.ToUpper(t) {
	case "SUBSCRIBED", "DID_RENEW", "DID_CHANGE_RENEWAL_STATUS",
		"DID_CHANGE_RENEWAL_PREF", "DID_FAIL_TO_RENEW",
		"GRACE_PERIOD_EXPIRED", "EXPIRED",
		"REFUND", "REVOKE", "PRICE_INCREASE",
		"OFFER_REDEEMED", "RENEWAL_EXTENDED":
		return true
	default:
		return false
	}
}

// buildWebhookUpsert 把通知 + 当前 transaction 转换成 SubscriptionUpsert，按通知类型决定 status / auto_renew。
func buildWebhookUpsert(userID int64, event *AppleWebhookEvent, product Product, now time.Time) model.SubscriptionUpsert {
	tx := event.Transaction
	upsert := model.SubscriptionUpsert{
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
		Status:                model.SubscriptionStatusActive,
		AutoRenewStatus:       model.AutoRenewStatusUnknown,
		CurrentPeriodStart:    tx.PurchaseDate,
		CurrentPeriodEnd:      tx.ExpiresDate,
		LastEventAt:           now,
		LastPayloadHash:       event.SignedPayloadSHA256,
	}
	if !event.NotificationCreatedAt.IsZero() {
		t := event.NotificationCreatedAt
		upsert.LastNotificationCreatedAt = &t
	}
	if tx.IsRevoked() {
		upsert.Status = model.SubscriptionStatusRevoked
		if tx.RevocationDate != nil {
			upsert.LastEventAt = *tx.RevocationDate
		}
	}

	switch strings.ToUpper(event.NotificationType) {
	case "REFUND", "REVOKE":
		upsert.Status = model.SubscriptionStatusRevoked
	case "EXPIRED", "GRACE_PERIOD_EXPIRED":
		upsert.Status = model.SubscriptionStatusExpired
	case "DID_CHANGE_RENEWAL_STATUS":
		if event.RenewalInfo != nil && event.RenewalInfo.AutoRenewStatus == 0 {
			upsert.Status = model.SubscriptionStatusCanceled
			upsert.AutoRenewStatus = model.AutoRenewStatusOff
		} else if event.RenewalInfo != nil && event.RenewalInfo.AutoRenewStatus == 1 {
			upsert.AutoRenewStatus = model.AutoRenewStatusOn
		}
	case "DID_FAIL_TO_RENEW":
		if event.RenewalInfo != nil && event.RenewalInfo.GracePeriodExpiresDate != nil {
			grace := *event.RenewalInfo.GracePeriodExpiresDate
			upsert.GracePeriodExpiresAt = &grace
		}
	case "SUBSCRIBED", "DID_RENEW", "RENEWAL_EXTENDED":
		// fall through with ACTIVE / ON
		if event.RenewalInfo != nil {
			if event.RenewalInfo.AutoRenewStatus == 0 {
				upsert.AutoRenewStatus = model.AutoRenewStatusOff
			} else if event.RenewalInfo.AutoRenewStatus == 1 {
				upsert.AutoRenewStatus = model.AutoRenewStatusOn
			}
		}
	}
	return upsert
}

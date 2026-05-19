package payment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	gopayApple "github.com/go-pay/gopay/apple"
)

// AppleTransactionVerifier 通过 App Store Server API 拉取并验证单条交易。
type AppleTransactionVerifier interface {
	FetchTransaction(ctx context.Context, txID string, env Environment) (*AppleTransaction, error)
}

// AppleWebhookVerifier 解码 + 校验 ASSN V2 signedPayload，输出领域事件。
type AppleWebhookVerifier interface {
	DecodeSignedPayload(ctx context.Context, signedPayload string) (*AppleWebhookEvent, error)
}

// AppleReconciler 提供 Notification History / Subscription Status 复盘能力，覆盖 webhook 漏投。
type AppleReconciler interface {
	GetNotificationHistory(ctx context.Context, req NotificationHistoryRequest) (events []AppleWebhookEvent, nextPageToken string, err error)
	GetSubscriptionStatus(ctx context.Context, originalTransactionID string, env Environment) (*AppleSubscriptionStatus, error)
}

// ErrAppleTransactionNotFound 表示 Apple 返回 4040010 等 not-found 错误码。
var ErrAppleTransactionNotFound = errors.New("apple iap: transaction not found")

// ErrAppleAuthRejected 表示 Apple 返回 401/403 等鉴权失败：禁止 sandbox 回退。
var ErrAppleAuthRejected = errors.New("apple iap: apple api auth rejected")

// gopayVerifier 是 AppleTransactionVerifier / AppleReconciler 的生产实现，
// 同时持有 prod 和 sandbox 两个 gopay client；env routing 与 sandbox fallback
// 由本结构控制，service 层只 see 接口。
type gopayVerifier struct {
	prod                  *gopayApple.Client
	sandbox               *gopayApple.Client
	enableSandboxFallback bool
	bundleID              string
	allowedEnvs           map[Environment]struct{}
}

// NewGopayAppleClients 用 catalog 构造 prod / sandbox 客户端。
//
// 行为：
//   - 总是构造 prod client；任何 catalog 缺失字段都返回 ErrNotConfigured。
//   - sandbox client 仅在以下条件之一为真时构造：
//     1. catalog.AllowedEntitlementEnvironments 包含 Sandbox（dev/staging）；
//     2. catalog.EnableSandboxFallback 为 true（生产用 dev 测试 SKU）。
func NewGopayAppleClients(c *Catalog) (prod *gopayApple.Client, sandbox *gopayApple.Client, err error) {
	if c == nil {
		return nil, nil, ErrNotConfigured
	}
	if c.BundleID() == "" || c.IssuerID() == "" || c.KeyID() == "" || c.PrivateKeyPEM() == "" {
		return nil, nil, ErrNotConfigured
	}

	prod, err = gopayApple.NewClient(c.IssuerID(), c.BundleID(), c.KeyID(), c.PrivateKeyPEM(), true)
	if err != nil {
		return nil, nil, fmt.Errorf("apple iap: build prod client: %w", err)
	}

	sandboxNeeded := c.EnableSandboxFallback()
	if !sandboxNeeded {
		for _, e := range c.AllowedEntitlementEnvironments() {
			if e == EnvSandbox {
				sandboxNeeded = true
				break
			}
		}
	}
	if sandboxNeeded {
		sandbox, err = gopayApple.NewClient(c.IssuerID(), c.BundleID(), c.KeyID(), c.PrivateKeyPEM(), false)
		if err != nil {
			return nil, nil, fmt.Errorf("apple iap: build sandbox client: %w", err)
		}
	}
	return prod, sandbox, nil
}

// NewAppleTransactionVerifier 构造生产用 transaction verifier。catalog 缺配则返回 nil + ErrNotConfigured。
func NewAppleTransactionVerifier(c *Catalog) (AppleTransactionVerifier, error) {
	prod, sandbox, err := NewGopayAppleClients(c)
	if err != nil {
		return nil, err
	}
	v := &gopayVerifier{
		prod:                  prod,
		sandbox:               sandbox,
		enableSandboxFallback: c.EnableSandboxFallback(),
		bundleID:              c.BundleID(),
		allowedEnvs:           map[Environment]struct{}{},
	}
	for _, e := range c.AllowedEntitlementEnvironments() {
		v.allowedEnvs[e] = struct{}{}
	}
	return v, nil
}

// NewAppleReconciler 构造生产用 reconciler；与 verifier 共享 prod/sandbox 客户端。
func NewAppleReconciler(c *Catalog) (AppleReconciler, error) {
	prod, sandbox, err := NewGopayAppleClients(c)
	if err != nil {
		return nil, err
	}
	return &gopayVerifier{
		prod:                  prod,
		sandbox:               sandbox,
		enableSandboxFallback: c.EnableSandboxFallback(),
		bundleID:              c.BundleID(),
	}, nil
}

// FetchTransaction 调用 Apple GetTransactionInfo，并按 fallback 策略尝试 sandbox。
func (v *gopayVerifier) FetchTransaction(ctx context.Context, txID string, env Environment) (*AppleTransaction, error) {
	if v == nil || v.prod == nil {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(txID) == "" {
		return nil, errors.New("apple iap: transaction id required")
	}

	preferProd := env != EnvSandbox
	tx, err := v.fetchOnce(ctx, txID, preferProd)
	if err == nil {
		return tx, nil
	}

	if !shouldFallbackToSandbox(err, preferProd, v.enableSandboxFallback, v.sandbox != nil) {
		return nil, err
	}
	return v.fetchOnce(ctx, txID, false)
}

func (v *gopayVerifier) fetchOnce(ctx context.Context, txID string, useProd bool) (*AppleTransaction, error) {
	client := v.prod
	if !useProd {
		if v.sandbox == nil {
			return nil, ErrSandboxFallbackDisabled
		}
		client = v.sandbox
	}
	rsp, err := client.GetTransactionInfo(ctx, txID)
	if err != nil {
		return nil, classifyAppleError(err)
	}
	if rsp == nil || rsp.SignedTransactionInfo == "" {
		return nil, ErrAppleTransactionNotFound
	}
	decoded, err := rsp.DecodeSignedTransaction()
	if err != nil {
		return nil, fmt.Errorf("apple iap: decode signed transaction: %w", err)
	}
	tx := mapAppleTransaction(decoded)
	if tx.BundleID != "" && v.bundleID != "" && !strings.EqualFold(tx.BundleID, v.bundleID) {
		return nil, fmt.Errorf("apple iap: bundle mismatch: got %s, want %s", tx.BundleID, v.bundleID)
	}
	return &tx, nil
}

// GetNotificationHistory 拉取通知历史并解码每个 SignedPayload 为领域事件，便于 reconcile 流程复用同一个 reducer。
func (v *gopayVerifier) GetNotificationHistory(ctx context.Context, req NotificationHistoryRequest) ([]AppleWebhookEvent, string, error) {
	if v == nil || v.prod == nil {
		return nil, "", ErrNotConfigured
	}
	bm := buildHistoryBodyMap(req)
	rsp, err := v.prod.GetNotificationHistory(ctx, req.PaginationToken, bm)
	if err != nil {
		return nil, "", classifyAppleError(err)
	}
	if rsp == nil {
		return nil, "", nil
	}

	out := make([]AppleWebhookEvent, 0, len(rsp.NotificationHistory))
	for _, n := range rsp.NotificationHistory {
		if n == nil || n.SignedPayload == "" {
			continue
		}
		ev, err := decodeWebhookEvent(n.SignedPayload, v.bundleID)
		if err != nil {
			continue
		}
		out = append(out, *ev)
	}
	return out, rsp.PaginationToken, nil
}

// GetSubscriptionStatus 是 reconcile 的逃生口；此 stub 返回 NotConfigured 直到接入 Apple GetAllSubscriptionStatuses。
//
// 暂未实现的原因：gopay v1.5.118 没有 GetAllSubscriptionStatuses；plan 接受用 raw HTTP 兜底，
// 留到 follow-up 任务，service 此前先依赖 Notification History replay。
func (v *gopayVerifier) GetSubscriptionStatus(ctx context.Context, originalTransactionID string, env Environment) (*AppleSubscriptionStatus, error) {
	_ = ctx
	_ = originalTransactionID
	_ = env
	return nil, errors.New("apple iap: GetSubscriptionStatus not implemented; use GetNotificationHistory replay")
}

// gopayWebhookVerifier 是 AppleWebhookVerifier 的生产实现。
type gopayWebhookVerifier struct {
	bundleID string
}

// NewAppleWebhookVerifier 构造一个解码 + 验证 ASSN V2 signedPayload 的 verifier。catalog 缺配返回 ErrNotConfigured。
func NewAppleWebhookVerifier(c *Catalog) (AppleWebhookVerifier, error) {
	if c == nil || c.BundleID() == "" {
		return nil, ErrNotConfigured
	}
	return &gopayWebhookVerifier{bundleID: c.BundleID()}, nil
}

// DecodeSignedPayload 走 gopay.DecodeSignedPayload (含 x5c 证书链 + 苹果根证校验)，并叠加 bundle id 校验。
func (w *gopayWebhookVerifier) DecodeSignedPayload(ctx context.Context, signedPayload string) (*AppleWebhookEvent, error) {
	_ = ctx
	if w == nil || w.bundleID == "" {
		return nil, ErrNotConfigured
	}
	return decodeWebhookEvent(signedPayload, w.bundleID)
}

// ───────── helpers ─────────

func decodeWebhookEvent(signedPayload, expectBundle string) (*AppleWebhookEvent, error) {
	if !looksLikeCompactJWS(signedPayload) {
		return nil, errors.New("apple iap: signedPayload is not a compact JWS")
	}
	payload, err := gopayApple.DecodeSignedPayload(signedPayload)
	if err != nil {
		return nil, fmt.Errorf("apple iap: decode signed payload: %w", err)
	}
	if payload == nil || payload.NotificationUUID == "" {
		return nil, errors.New("apple iap: missing notification uuid")
	}
	ev := &AppleWebhookEvent{
		NotificationUUID:    payload.NotificationUUID,
		NotificationType:    payload.NotificationType,
		Subtype:             payload.Subtype,
		SignedPayloadSHA256: sha256Hex(signedPayload),
	}
	if payload.Data != nil {
		ev.BundleID = payload.Data.BundleID
		ev.Environment = Environment(payload.Data.Environment)
	}
	if expectBundle != "" && ev.BundleID != "" && !strings.EqualFold(ev.BundleID, expectBundle) {
		return nil, fmt.Errorf("apple iap: bundle mismatch in webhook: got %s, want %s", ev.BundleID, expectBundle)
	}

	if payload.IssuedAt != 0 {
		ev.NotificationCreatedAt = time.Unix(payload.IssuedAt, 0)
	}

	if tx, err := payload.DecodeTransactionInfo(); err == nil && tx != nil {
		mapped := mapAppleTransaction(tx)
		ev.Transaction = &mapped
	}
	if ri, err := payload.DecodeRenewalInfo(); err == nil && ri != nil {
		ev.RenewalInfo = &AppleRenewalInfo{
			AutoRenewStatus:        int(ri.AutoRenewStatus),
			OriginalTransactionID:  ri.OriginalTransactionId,
			AutoRenewProductID:     ri.AutoRenewProductId,
			ExpirationIntent:       int(ri.ExpirationIntent),
			IsInBillingRetryPeriod: ri.IsInBillingRetryPeriod,
		}
		if ri.GracePeriodExpiresDate > 0 {
			t := time.UnixMilli(ri.GracePeriodExpiresDate)
			ev.RenewalInfo.GracePeriodExpiresDate = &t
		}
	}
	return ev, nil
}

func mapAppleTransaction(tx *gopayApple.JWSTransactionDecodedPayload) AppleTransaction {
	out := AppleTransaction{
		TransactionID:         tx.TransactionId,
		OriginalTransactionID: tx.OriginalTransactionId,
		AppAccountToken:       tx.AppAccountToken,
		BundleID:              tx.BundleId,
		Environment:           Environment(tx.Environment),
		ProductID:             tx.ProductId,
		SubscriptionGroupID:   tx.SubscriptionGroupIdentifier,
		Type:                  tx.Type,
		WebOrderLineItemID:    tx.WebOrderLineItemId,
		InAppOwnershipType:    tx.InAppOwnershipType,
		IsUpgraded:            tx.IsUpgraded,
	}
	if tx.PurchaseDate > 0 {
		out.PurchaseDate = time.UnixMilli(tx.PurchaseDate)
	}
	if tx.OriginalPurchaseDate > 0 {
		out.OriginalPurchaseDate = time.UnixMilli(tx.OriginalPurchaseDate)
	}
	if tx.ExpiresDate > 0 {
		out.ExpiresDate = time.UnixMilli(tx.ExpiresDate)
	}
	if tx.RevocationDate > 0 {
		t := time.UnixMilli(tx.RevocationDate)
		out.RevocationDate = &t
	}
	return out
}

func looksLikeCompactJWS(s string) bool {
	if s == "" {
		return false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	return true
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// classifyAppleError 把 gopay/HTTP 错误归类为 service 可分流的 sentinel。
//
// 决策依据：错误信息包含 401/403 → 鉴权失败，禁止 fallback；包含 not found/4040010 → ErrAppleTransactionNotFound。
// gopay v1.5.118 没有公开的 typed error，故只能用字符串前缀做最小启发。
func classifyAppleError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "4040010") || strings.Contains(msg, "transaction id not found") || strings.Contains(msg, "404"):
		return fmt.Errorf("%w: %v", ErrAppleTransactionNotFound, err)
	case strings.Contains(msg, "401") || strings.Contains(msg, "unauthorized"):
		return fmt.Errorf("%w: %v", ErrAppleAuthRejected, err)
	case strings.Contains(msg, "403") || strings.Contains(msg, "forbidden"):
		return fmt.Errorf("%w: %v", ErrAppleAuthRejected, err)
	default:
		return err
	}
}

// shouldFallbackToSandbox 是纯函数决策器，便于 stub 测试。
//
// 规则（与 plan 一致）：
//   - 仅当本次调用是 prod 调用，且 ErrAppleTransactionNotFound，且 enable=true，且 sandbox client 存在，才允许 fallback；
//   - 鉴权失败 / context 超时 / 其他错误一律不 fallback。
func shouldFallbackToSandbox(err error, preferProd, enable, hasSandbox bool) bool {
	if !preferProd || !enable || !hasSandbox {
		return false
	}
	if errors.Is(err, ErrAppleAuthRejected) {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return errors.Is(err, ErrAppleTransactionNotFound)
}

func buildHistoryBodyMap(req NotificationHistoryRequest) map[string]any {
	m := map[string]any{}
	if !req.StartDate.IsZero() {
		m["startDate"] = req.StartDate.UnixMilli()
	}
	if !req.EndDate.IsZero() {
		m["endDate"] = req.EndDate.UnixMilli()
	}
	if req.OriginalTransactionID != "" {
		m["originalTransactionId"] = req.OriginalTransactionID
	}
	return m
}

package payment

import "time"

// AppleTransaction 是从 Apple App Store Server API 拿到的交易在领域内的投影。
//
// 该结构是 verifier 的输出，service / reducer 仅依赖此结构，不感知 gopay 类型。
type AppleTransaction struct {
	TransactionID         string
	OriginalTransactionID string
	AppAccountToken       string
	BundleID              string
	Environment           Environment
	ProductID             string
	SubscriptionGroupID   string
	Type                  string
	ExpiresDate           time.Time
	PurchaseDate          time.Time
	OriginalPurchaseDate  time.Time
	RevocationDate        *time.Time
	WebOrderLineItemID    string
	InAppOwnershipType    string
	IsUpgraded            bool
}

// IsAutoRenewableSubscription 判定 Apple 的 type 字段是否为自动续期订阅。Apple 写法包含空格。
func (t *AppleTransaction) IsAutoRenewableSubscription() bool {
	return t.Type == "Auto-Renewable Subscription"
}

// IsRevoked 表示 Apple 标记此 transaction 为已退款 / 撤销。
func (t *AppleTransaction) IsRevoked() bool {
	return t.RevocationDate != nil && !t.RevocationDate.IsZero()
}

// IsFamilyShared 判定这条交易是否是家庭共享获取，重要的边界场景：
// Apple 的 Set App Account Token endpoint 不支持家庭共享 transaction 的补绑。
func (t *AppleTransaction) IsFamilyShared() bool {
	return t.InAppOwnershipType == "FAMILY_SHARED"
}

// AppleRenewalInfo 是 ASSN V2 通知里 renewal info JWS 的领域投影。
type AppleRenewalInfo struct {
	AutoRenewStatus        int
	OriginalTransactionID  string
	AutoRenewProductID     string
	GracePeriodExpiresDate *time.Time
	ExpirationIntent       int
	IsInBillingRetryPeriod bool
}

// AppleWebhookEvent 是 webhook reducer 处理的标准化事件。
type AppleWebhookEvent struct {
	NotificationUUID      string
	NotificationType      string
	Subtype               string
	BundleID              string
	Environment           Environment
	NotificationCreatedAt time.Time
	Transaction           *AppleTransaction
	RenewalInfo           *AppleRenewalInfo
	SignedPayloadSHA256   string
	DecodedPayload        []byte
}

// AppleSubscriptionStatus 是 Apple Get All Subscription Statuses 的领域投影，仅用于 reconcile。
type AppleSubscriptionStatus struct {
	OriginalTransactionID string
	Environment           Environment
	Status                int
	LastTransaction       *AppleTransaction
	RenewalInfo           *AppleRenewalInfo
}

// NotificationHistoryRequest 是 reconciler GetNotificationHistory 的查询参数。
//
// StartDate / EndDate 至少其中一个必须给定，符合 Apple Notification History 文档。
// OriginalTransactionID 可选，进一步过滤；PaginationToken 由前一次响应回填。
type NotificationHistoryRequest struct {
	StartDate             time.Time
	EndDate               time.Time
	OriginalTransactionID string
	PaginationToken       string
}

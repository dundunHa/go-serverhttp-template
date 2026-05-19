package model

import "time"

// AppleAccountToken 是服务端为每个用户生成并持久化的 Apple appAccountToken 映射记录。
//
// iOS StoreKit 在发起购买时必须把这个 UUID 作为 purchase option 的 appAccountToken
// 一起提交，Apple 后续在 transaction / renewal info 中回传同一 UUID。本服务通过该
// UUID 把 Apple 的交易回写到本地 user_id。
//
// AppleAccountToken 仅服务内部使用，不直接暴露给客户端。
type AppleAccountToken struct {
	ID        int64
	UserID    int64
	Token     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AppleAccountTokenResponse 是 GET /payment/apple/account-token 的响应负载。
//
// iOS 客户端必须把 AppAccountToken 作为 StoreKit purchase option 的 appAccountToken 使用。
// 该 token 只是本服务与 Apple transaction 之间的关联键，不是认证凭证；该 endpoint 仍然要求 Bearer 认证。
type AppleAccountTokenResponse struct {
	AppAccountToken string `json:"app_account_token" doc:"Apple appAccountToken UUID，iOS 在 StoreKit 购买时必须使用该值作为 purchase option" example:"9d8a0f8e-1c4f-46b0-9d93-b3b84f0f9e61" format:"uuid"`
}

// AppleEnvironment 标识 Apple transaction / 通知所属的环境（生产 vs 沙盒）。
type AppleEnvironment string

const (
	AppleEnvProduction AppleEnvironment = "Production"
	AppleEnvSandbox    AppleEnvironment = "Sandbox"
)

// 订阅 DB 内部状态（service 内部 reducer 使用）；与 API 暴露的 status 不同，详见 plan "## Entitlement Model"。
const (
	SubscriptionStatusActive   = "ACTIVE"
	SubscriptionStatusCanceled = "CANCELED"
	SubscriptionStatusExpired  = "EXPIRED"
	SubscriptionStatusRevoked  = "REVOKED"
)

// 自动续期内部状态。
const (
	AutoRenewStatusOn      = "ON"
	AutoRenewStatusOff     = "OFF"
	AutoRenewStatusUnknown = "UNKNOWN"
)

// processing_status：apple_events 的处理结果，覆盖 reducer / 反查所需的全部分支。
const (
	EventStatusProcessed          = "PROCESSED"
	EventStatusIgnoredDuplicate   = "IGNORED_DUPLICATE"
	EventStatusIgnoredUnknownType = "IGNORED_UNKNOWN_TYPE"
	EventStatusPendingUserBinding = "PENDING_USER_BINDING"
	EventStatusOwnershipConflict  = "OWNERSHIP_CONFLICT"
	EventStatusPermanentFailure   = "PERMANENT_FAILURE"
)

// Subscription 是 apple_subscriptions 行的领域投影。reducer / reader 仅依赖此结构。
type Subscription struct {
	ID                        int64
	UserID                    int64
	AppAccountToken           string
	Environment               AppleEnvironment
	OriginalTransactionID     string
	LastTransactionID         string
	WebOrderLineItemID        string
	PlanID                    string
	ProviderProductID         string
	SubscriptionGroupID       string
	Level                     int
	Status                    string
	AutoRenewStatus           string
	CurrentPeriodStart        time.Time
	CurrentPeriodEnd          time.Time
	GracePeriodExpiresAt      *time.Time
	LastEventAt               time.Time
	LastNotificationCreatedAt *time.Time
	LastPayloadHash           string
	LastTransactionSnapshot   []byte
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

// SubscriptionUpsert 是 webhook reducer / verify 写入 apple_subscriptions 时使用的入参。
//
// reducer 在事务中先用 (OriginalTransactionID, Environment) lookup 现有行，把现有 user_id 与
// 入参 UserID 比对：不一致即 ErrSubscriptionOwnershipConflict；一致才执行 upsert。
type SubscriptionUpsert struct {
	UserID                    int64
	AppAccountToken           string
	Environment               AppleEnvironment
	OriginalTransactionID     string
	LastTransactionID         string
	WebOrderLineItemID        string
	PlanID                    string
	ProviderProductID         string
	SubscriptionGroupID       string
	Level                     int
	Status                    string
	AutoRenewStatus           string
	CurrentPeriodStart        time.Time
	CurrentPeriodEnd          time.Time
	GracePeriodExpiresAt      *time.Time
	LastEventAt               time.Time
	LastNotificationCreatedAt *time.Time
	LastPayloadHash           string
	LastTransactionSnapshot   []byte
}

// AppleEventInsert 是 InsertAppleEventIfNotExists 入参；NotificationUUID 是幂等键。
//
// UserID == 0 表示尚未绑定（PENDING_USER_BINDING 路径）；DAO 把零值写为 SQL NULL。
// AppAccountToken 为空字符串表示通知未携带 token；DAO 把空值写为 SQL NULL。
type AppleEventInsert struct {
	NotificationUUID      string
	NotificationType      string
	Subtype               string
	Environment           AppleEnvironment
	UserID                int64
	AppAccountToken       string
	OriginalTransactionID string
	TransactionID         string
	WebOrderLineItemID    string
	ProcessingStatus      string
	ProcessingError       string
	RawJWSSHA256          string
	DecodedPayload        []byte
	NotificationCreatedAt *time.Time
}

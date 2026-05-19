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
	Token     string // 标准 UUID v4 字符串：xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
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

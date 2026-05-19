package model

// User 是领域层的用户实体，主要用于服务与 DAO 之间传递。
type User struct {
	ID   int    `json:"id" doc:"用户在数据库中的自增 ID" example:"1"`
	Name string `json:"name" doc:"用户显示名称" example:"Ada"`
}

// AuthRequest 是 /auth/{provider} 接口的请求体。
type AuthRequest struct {
	Token string `json:"token" doc:"第三方 provider 颁发的 token；guest 场景下使用设备 ID" example:"ya29.a0AfH6SM..." required:"true"`
}

// AuthResponse 是 /auth/{provider} 接口的响应体，返回本服务颁发的 access token 及用户信息。
type AuthResponse struct {
	AccessToken string   `json:"access_token" doc:"本服务颁发的 JWT access token" example:"eyJhbGciOi..."`
	TokenType   string   `json:"token_type" doc:"需要在 Authorization 头中使用的 token 类型" example:"Bearer"`
	ExpiresIn   int64    `json:"expires_in" doc:"access token 的有效期（秒）" example:"3600"`
	User        UserInfo `json:"user" doc:"当前认证用户的身份信息"`
}

// AuthIdentity 是 provider 返回的原始身份描述，仅服务内部使用，不暴露给客户端。
type AuthIdentity struct {
	Provider string
	Subject  string
	Email    string
}

// UserInfo 是认证后返回给客户端的用户身份描述。
type UserInfo struct {
	ID              string `json:"id" doc:"本服务内的用户 ID" example:"1"`
	Email           string `json:"email" doc:"用户邮箱，guest 登录可能为空" example:"ada@example.com"`
	Provider        string `json:"provider" doc:"认证提供方标识" example:"guest" enum:"gmail,apple,guest"`
	ProviderSubject string `json:"-"`
}

// UserSummary 暴露给客户端的最小化用户信息。
type UserSummary struct {
	ID   string `json:"id" doc:"用户 ID" example:"1"`
	Name string `json:"name" doc:"用户显示名称" example:"Ada"`
}

// Credits 用户当前可用的积分余额。
type Credits struct {
	Balance int64 `json:"balance" doc:"当前可用积分余额" example:"0" minimum:"0"`
}

// SubscriptionInfo 用户当前订阅状态。
type SubscriptionInfo struct {
	ProductID            string `json:"product_id" doc:"订阅产品标识（例如 App Store productId）" example:"com.picjoy.pro.monthly"`
	Status               string `json:"status" doc:"订阅状态" example:"ACTIVE" enum:"ACTIVE,EXPIRED,CANCELED,NONE"`
	SubscribeExpiredTime string `json:"subscribe_expired_time" doc:"订阅过期时间（RFC3339）" example:"2026-02-23T12:00:00Z" format:"date-time"`
	SubscribeLevel       int    `json:"subscribe_level" doc:"订阅等级（0 = 免费，其他为付费等级）" example:"1" minimum:"0"`
}

// MeData 是 GET /users/me 接口返回的负载。
type MeData struct {
	Credits          Credits          `json:"credits" doc:"用户积分余额"`
	SubscriptionInfo SubscriptionInfo `json:"subscription_info" doc:"用户订阅状态"`
	User             UserSummary      `json:"user" doc:"用户基础信息"`
}

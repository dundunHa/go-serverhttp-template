package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
	"github.com/dundunHa/go-serverhttp-template/internal/service/auth"
	"github.com/dundunHa/go-serverhttp-template/internal/service/payment"
)

// PaymentTokenService 是 payment 路由所需的最小 token 服务接口。
type PaymentTokenService interface {
	EnsureAccountToken(ctx context.Context, userID int64) (string, error)
}

// PaymentIAPService 是 verify 路由所需的最小服务接口。
type PaymentIAPService interface {
	VerifyTransaction(ctx context.Context, userID int64, transactionID string) (*model.SubscriptionInfo, error)
}

// PaymentDeps 聚合 payment 路由的所有依赖。后续 U8 / U9 会扩展该结构。
type PaymentDeps struct {
	Auth   auth.Service
	Tokens PaymentTokenService
	IAP    PaymentIAPService
}

// RegisterPaymentRoutes 注册 /payment/* 与（后续）/webhooks/apple 路由。
func RegisterPaymentRoutes(api huma.API, deps PaymentDeps) {
	registerPaymentDocMetadata(api)
	registerAccountTokenRoute(api, deps)
	registerVerifyRoute(api, deps)
}

func registerPaymentDocMetadata(api huma.API) {
	openapi := api.OpenAPI()
	for _, t := range openapi.Tags {
		if t.Name == "payment" {
			return
		}
	}
	openapi.Tags = append(openapi.Tags, &huma.Tag{
		Name:        "payment",
		Description: "Apple In-App Purchase 订阅相关接口（账号 token / 校验购买 / Webhook）。",
	})
}

func registerAccountTokenRoute(api huma.API, deps PaymentDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-apple-account-token",
		Method:      http.MethodGet,
		Path:        "/payment/apple/account-token",
		Summary:     "获取 Apple appAccountToken",
		Description: "返回服务端为当前登录用户生成的 Apple appAccountToken UUID。iOS 客户端必须把它作为 StoreKit purchase option 的 appAccountToken 一起提交，Apple 后续在 transaction 中回传同一 UUID 用于本服务的回写。\n\n同一用户多次调用返回同一个 UUID。Token 由服务端持久化，客户端不能提交自选 UUID 进行绑定。",
		Tags:        []string{"payment"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors: []int{
			http.StatusUnauthorized,
			http.StatusInternalServerError,
			http.StatusServiceUnavailable,
		},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization" hidden:"true"`
	}) (*struct {
		Body model.Response[model.AppleAccountTokenResponse]
	}, error) {
		authedUser, err := validateUserBearerToken(ctx, deps.Auth, input.Authorization)
		if err != nil {
			return nil, err
		}
		userID, perr := strconv.ParseInt(authedUser.ID, 10, 64)
		if perr != nil || userID <= 0 {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		if deps.Tokens == nil {
			return nil, huma.Error503ServiceUnavailable("Apple IAP 未配置")
		}
		token, err := deps.Tokens.EnsureAccountToken(ctx, userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("生成 Apple account token 失败")
		}
		return &struct {
			Body model.Response[model.AppleAccountTokenResponse]
		}{
			Body: model.Success(model.AppleAccountTokenResponse{AppAccountToken: token}),
		}, nil
	})
}

// VerifyAppleTransactionRequest 是 POST /payment/apple/verify 的请求体。
type VerifyAppleTransactionRequest struct {
	TransactionID string `json:"transaction_id" doc:"Apple StoreKit purchase 完成时返回的 transactionId" required:"true" minLength:"1" example:"200000123456789"`
}

// VerifyAppleTransactionResponse 是 POST /payment/apple/verify 的响应负载。
type VerifyAppleTransactionResponse struct {
	SubscriptionInfo model.SubscriptionInfo `json:"subscription_info" doc:"刚刚校验完成的订阅状态，已经写入 apple_subscriptions"`
}

func registerVerifyRoute(api huma.API, deps PaymentDeps) {
	huma.Register(api, huma.Operation{
		OperationID: "verify-apple-transaction",
		Method:      http.MethodPost,
		Path:        "/payment/apple/verify",
		Summary:     "校验 Apple IAP 购买并写入订阅状态",
		Description: "客户端在 StoreKit 购买成功后立即调用本接口提交 transactionId。服务端通过 App Store Server API 拉取并验证签名后的 transaction，校验 bundle、产品 catalog、appAccountToken 与当前用户的绑定关系，并幂等写入 apple_subscriptions。\n\n返回值与 GET /users/me 中的 SubscriptionInfo 字段同形：客户端可以据此立即刷新本地订阅 UI，而不需要再发起一次 /users/me。",
		Tags:        []string{"payment"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors: []int{
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusConflict,
			http.StatusInternalServerError,
			http.StatusServiceUnavailable,
		},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization" hidden:"true"`
		Body          VerifyAppleTransactionRequest
	}) (*struct {
		Body model.Response[VerifyAppleTransactionResponse]
	}, error) {
		authedUser, err := validateUserBearerToken(ctx, deps.Auth, input.Authorization)
		if err != nil {
			return nil, err
		}
		userID, perr := strconv.ParseInt(authedUser.ID, 10, 64)
		if perr != nil || userID <= 0 {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		if input.Body.TransactionID == "" {
			return nil, huma.Error400BadRequest("transaction_id 不能为空")
		}
		if deps.IAP == nil {
			return nil, huma.Error503ServiceUnavailable("Apple IAP 未配置")
		}
		info, err := deps.IAP.VerifyTransaction(ctx, userID, input.Body.TransactionID)
		if err != nil {
			return nil, mapVerifyError(err)
		}
		return &struct {
			Body model.Response[VerifyAppleTransactionResponse]
		}{
			Body: model.Success(VerifyAppleTransactionResponse{SubscriptionInfo: *info}),
		}, nil
	})
}

func mapVerifyError(err error) error {
	switch {
	case errors.Is(err, payment.ErrNotConfigured):
		return huma.Error503ServiceUnavailable("Apple IAP 未配置")
	case errors.Is(err, payment.ErrEmptyAppAccountToken),
		errors.Is(err, payment.ErrAppAccountTokenMismatch),
		errors.Is(err, payment.ErrUnknownProduct),
		errors.Is(err, payment.ErrUnsupportedProductType),
		errors.Is(err, payment.ErrTransactionRevoked),
		errors.Is(err, payment.ErrAppleTransactionNotFound),
		errors.Is(err, payment.ErrInvalidConfig):
		return huma.Error400BadRequest(err.Error())
	case errors.Is(err, payment.ErrSubscriptionOwnershipConflict):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, payment.ErrAppleAuthRejected):
		return huma.Error500InternalServerError("Apple API 鉴权失败")
	default:
		return huma.Error500InternalServerError("verify 失败")
	}
}

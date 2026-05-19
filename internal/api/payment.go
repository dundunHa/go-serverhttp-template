package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
	"github.com/dundunHa/go-serverhttp-template/internal/service/auth"
)

// PaymentTokenService 是 payment 路由所需的最小 token 服务接口。
type PaymentTokenService interface {
	EnsureAccountToken(ctx context.Context, userID int64) (string, error)
}

// PaymentDeps 聚合 payment 路由的所有依赖。后续 U6 / U8 / U9 会扩展该结构。
type PaymentDeps struct {
	Auth   auth.Service
	Tokens PaymentTokenService
}

// RegisterPaymentRoutes 注册 /payment/* 与（后续）/webhooks/apple 路由。
func RegisterPaymentRoutes(api huma.API, deps PaymentDeps) {
	registerPaymentDocMetadata(api)
	registerAccountTokenRoute(api, deps)
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

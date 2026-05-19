package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
	"github.com/dundunHa/go-serverhttp-template/internal/service"
	"github.com/dundunHa/go-serverhttp-template/internal/service/auth"
)

type UserDeps struct {
	Users         service.UserService
	Auth          auth.Service
	Subscriptions SubscriptionReader
}

// SubscriptionReader 是 /users/me 用来获取 provider-neutral 订阅状态的依赖。
//
// 生产实现由 internal/service/payment.SubscriptionReader 提供；测试可以注入空实现，
// 此时 loadCurrentUser 返回 SubscriptionInfo{Status: "NONE"}。
type SubscriptionReader interface {
	LoadSubscriptionInfo(ctx context.Context, userID int64) (model.SubscriptionInfo, error)
}

func RegisterUserRoutes(api huma.API, deps UserDeps) {
	registerUserBearerAuth(api)
	registerAPIDocMetadata(api)
	registerUserHelloRoute(api)
	registerUserRoutes(api, deps.Users, deps.Auth, deps.Subscriptions)
	registerUserAuthRoutes(api, deps.Auth)
}

func registerUserBearerAuth(api huma.API) {
	openapi := api.OpenAPI()
	if openapi.Components == nil {
		openapi.Components = &huma.Components{}
	}
	if openapi.Components.SecuritySchemes == nil {
		openapi.Components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}
	openapi.Components.SecuritySchemes["bearerAuth"] = &huma.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
		Description:  "在 Authorization 请求头中携带本服务颁发的 Bearer JWT。可通过 POST /auth/{provider} 获取。",
	}
}

// registerAPIDocMetadata 补充顶层 OpenAPI 描述、标签分组，让 /docs 渲染出来的页面信息更加完整。
func registerAPIDocMetadata(api huma.API) {
	openapi := api.OpenAPI()
	if openapi.Info == nil {
		openapi.Info = &huma.Info{}
	}
	if openapi.Info.Description == "" {
		openapi.Info.Description = "Go Server HTTP Template 的公开 HTTP API。所有业务接口采用统一响应体 `{code, data, msg}`，需身份认证的接口请在 `Authorization` 中携带 `Bearer <access_token>`。"
	}

	hasTag := func(name string) bool {
		for _, t := range openapi.Tags {
			if t.Name == name {
				return true
			}
		}
		return false
	}
	addTag := func(name, description string) {
		if hasTag(name) {
			return
		}
		openapi.Tags = append(openapi.Tags, &huma.Tag{Name: name, Description: description})
	}
	addTag("system", "系统探活、健康检查类接口。")
	addTag("auth", "身份认证与 access token 颁发。")
	addTag("users", "当前登录用户资源。")
}

func registerUserHelloRoute(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-hello",
		Method:      http.MethodGet,
		Path:        "/hello",
		Summary:     "健康检查",
		Description: "返回固定的 hello 消息，用于探活和健康检查。不需要身份认证。",
		Tags:        []string{"system"},
	}, func(ctx context.Context, input *struct{}) (*struct {
		Body model.Response[model.Message]
	}, error) {
		return &struct {
			Body model.Response[model.Message]
		}{
			Body: model.Success(model.Message{Message: "hello"}),
		}, nil
	})
}

func registerUserRoutes(api huma.API, userSvc service.UserService, authSvc auth.Service, subscriptions SubscriptionReader) {
	huma.Register(api, huma.Operation{
		OperationID: "get-current-user",
		Method:      http.MethodGet,
		Path:        "/users/me",
		Summary:     "获取当前登录用户信息",
		Description: "根据 Authorization 请求头中的 Bearer access token 解析出当前用户身份，并返回该用户的基础信息、积分余额以及订阅状态。不接收任何路径或查询参数。",
		Tags:        []string{"users"},
		Security:    []map[string][]string{{"bearerAuth": []string{}}},
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization" hidden:"true"`
	}) (*struct {
		Body model.Response[model.MeData]
	}, error) {
		authedUser, err := validateUserBearerToken(ctx, authSvc, input.Authorization)
		if err != nil {
			return nil, err
		}

		me, err := loadCurrentUser(ctx, userSvc, subscriptions, authedUser)
		if err != nil {
			return nil, err
		}

		return &struct {
			Body model.Response[model.MeData]
		}{
			Body: model.Success(*me),
		}, nil
	})
}

// loadCurrentUser 根据 JWT 中的用户标识，组装 /users/me 的返回数据。
//
// Credits.Balance 仍然保持 0：plan U7 明确不引入 credits/wallet。
// SubscriptionInfo 由注入的 SubscriptionReader 给出；reader 为 nil 时退化为 Status="NONE"。
func loadCurrentUser(ctx context.Context, userSvc service.UserService, subscriptions SubscriptionReader, authedUser *model.UserInfo) (*model.MeData, error) {
	id, err := strconv.Atoi(authedUser.ID)
	if err != nil || id <= 0 {
		return nil, huma.Error401Unauthorized("access token 无效")
	}

	user, err := userSvc.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return nil, huma.Error404NotFound("用户不存在")
		}
		return nil, huma.Error500InternalServerError("获取用户失败")
	}

	subInfo := model.SubscriptionInfo{Status: "NONE"}
	if subscriptions != nil {
		info, err := subscriptions.LoadSubscriptionInfo(ctx, int64(user.ID))
		if err != nil {
			return nil, huma.Error500InternalServerError("获取订阅状态失败")
		}
		subInfo = info
		if subInfo.Status == "" {
			subInfo.Status = "NONE"
		}
	}

	return &model.MeData{
		Credits:          model.Credits{},
		SubscriptionInfo: subInfo,
		User: model.UserSummary{
			ID:   strconv.Itoa(user.ID),
			Name: user.Name,
		},
	}, nil
}

func validateUserBearerToken(ctx context.Context, authSvc auth.Service, authHeader string) (*model.UserInfo, error) {
	if authSvc == nil {
		return nil, huma.Error500InternalServerError("认证服务不可用")
	}
	if authHeader == "" {
		return nil, huma.Error401Unauthorized("缺少 Authorization Bearer token")
	}
	authFields := strings.Fields(authHeader)
	if len(authFields) != 2 || !strings.EqualFold(authFields[0], "Bearer") {
		return nil, huma.Error401Unauthorized("缺少 Authorization Bearer token")
	}
	user, err := authSvc.AuthenticateAccessToken(ctx, authFields[1])
	if err != nil {
		return nil, huma.Error401Unauthorized("access token 无效")
	}
	return user, nil
}

func registerUserAuthRoutes(api huma.API, authSvc auth.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "verify-auth-token",
		Method:      http.MethodPost,
		Path:        "/auth/{provider}",
		Summary:     "校验第三方登录凭证并颁发 access token",
		Description: "校验指定 provider（gmail / apple / guest）的登录凭证，成功后会颁发本服务的 JWT access token，后续业务接口可使用该 token 作为 Bearer 身份。\n\n- gmail：Google ID Token\n- apple：Sign in with Apple identityToken\n- guest：客户端生成的设备 ID",
		Tags:        []string{"auth"},
		Errors: []int{
			http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusNotFound,
			http.StatusUnprocessableEntity,
			http.StatusInternalServerError,
		},
	}, func(ctx context.Context, input *struct {
		Provider string `path:"provider" enum:"gmail,apple,guest" doc:"登录提供方标识" example:"guest"`
		Body     model.AuthRequest
	}) (*struct {
		Body model.Response[model.AuthResponse]
	}, error) {
		if input.Body.Token == "" {
			return nil, huma.Error400BadRequest("token 不能为空")
		}
		user, err := authSvc.Verify(ctx, input.Provider, input.Body.Token)
		if err != nil {
			if errors.Is(err, auth.ErrProviderNotFound) {
				return nil, huma.Error404NotFound("登录提供方不存在")
			}
			if errors.Is(err, auth.ErrIdentityUnavailable) || errors.Is(err, service.ErrAuthIdentityUnsupported) {
				return nil, huma.Error500InternalServerError("身份解析服务不可用")
			}
			return nil, huma.Error401Unauthorized(err.Error())
		}
		accessToken, expiresIn, err := authSvc.IssueAccessToken(ctx, *user)
		if err != nil {
			return nil, huma.Error500InternalServerError("颁发 access token 失败")
		}

		return &struct {
			Body model.Response[model.AuthResponse]
		}{
			Body: model.Success(model.AuthResponse{
				AccessToken: accessToken,
				TokenType:   "Bearer",
				ExpiresIn:   expiresIn,
				User:        *user,
			}),
		}, nil
	})
}

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
	Users service.UserService
	Auth  auth.Service
}

func RegisterUserRoutes(api huma.API, deps UserDeps) {
	registerUserBearerAuth(api)
	registerUserHelloRoute(api)
	registerUserRoutes(api, deps.Users, deps.Auth)
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
	}
}

func registerUserHelloRoute(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-hello",
		Method:      http.MethodGet,
		Path:        "/hello",
		Summary:     "Health-style hello endpoint",
		Tags:        []string{"system"},
	}, func(ctx context.Context, input *struct{}) (*struct {
		Body model.Response[model.Message]
	}, error) {
		return &struct {
			Body model.Response[model.Message]
		}{
			Body: model.Response[model.Message]{
				Data: model.Message{Message: "hello"},
			},
		}, nil
	})
}

func registerUserRoutes(api huma.API, userSvc service.UserService, authSvc auth.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "get-user",
		Method:      http.MethodGet,
		Path:        "/users/{id}",
		Summary:     "Get a user by ID",
		Tags:        []string{"users"},
		Security:    []map[string][]string{{"bearerAuth": []string{}}},
	}, func(ctx context.Context, input *struct {
		ID            string `path:"id" doc:"User ID" example:"1"`
		Authorization string `header:"Authorization" hidden:"true"`
	}) (*struct {
		Body model.Response[model.User]
	}, error) {
		authedUser, err := validateUserBearerToken(ctx, authSvc, input.Authorization)
		if err != nil {
			return nil, err
		}

		id, err := strconv.Atoi(input.ID)
		if err != nil || id <= 0 {
			return nil, huma.Error400BadRequest("id must be a positive integer")
		}
		if authedUser.ID != input.ID {
			return nil, huma.Error403Forbidden("token subject cannot access requested user")
		}

		user, err := userSvc.GetUser(ctx, id)
		if err != nil {
			if errors.Is(err, service.ErrUserNotFound) {
				return nil, huma.Error404NotFound("user not found")
			}
			return nil, huma.Error500InternalServerError("get user failed")
		}

		return &struct {
			Body model.Response[model.User]
		}{
			Body: model.Response[model.User]{Data: *user},
		}, nil
	})
}

func validateUserBearerToken(ctx context.Context, authSvc auth.Service, authHeader string) (*model.UserInfo, error) {
	if authSvc == nil {
		return nil, huma.Error500InternalServerError("auth service unavailable")
	}
	if authHeader == "" {
		return nil, huma.Error401Unauthorized("authorization bearer token required")
	}
	authFields := strings.Fields(authHeader)
	if len(authFields) != 2 || !strings.EqualFold(authFields[0], "Bearer") {
		return nil, huma.Error401Unauthorized("authorization bearer token required")
	}
	user, err := authSvc.AuthenticateAccessToken(ctx, authFields[1])
	if err != nil {
		return nil, huma.Error401Unauthorized("invalid access token")
	}
	return user, nil
}

func registerUserAuthRoutes(api huma.API, authSvc auth.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "verify-auth-token",
		Method:      http.MethodPost,
		Path:        "/auth/{provider}",
		Summary:     "Verify a provider token",
		Tags:        []string{"auth"},
	}, func(ctx context.Context, input *struct {
		Provider string `path:"provider" enum:"gmail,apple,guest"`
		Body     model.AuthRequest
	}) (*struct {
		Body model.Response[model.AuthResponse]
	}, error) {
		if input.Body.Token == "" {
			return nil, huma.Error400BadRequest("token required")
		}
		user, err := authSvc.Verify(ctx, input.Provider, input.Body.Token)
		if err != nil {
			if errors.Is(err, auth.ErrProviderNotFound) {
				return nil, huma.Error404NotFound("provider not found")
			}
			if errors.Is(err, auth.ErrIdentityUnavailable) || errors.Is(err, service.ErrAuthIdentityUnsupported) {
				return nil, huma.Error500InternalServerError("auth identity resolver unavailable")
			}
			return nil, huma.Error401Unauthorized(err.Error())
		}
		accessToken, expiresIn, err := authSvc.IssueAccessToken(ctx, *user)
		if err != nil {
			return nil, huma.Error500InternalServerError("issue access token failed")
		}

		return &struct {
			Body model.Response[model.AuthResponse]
		}{
			Body: model.Response[model.AuthResponse]{
				Data: model.AuthResponse{
					AccessToken: accessToken,
					TokenType:   "Bearer",
					ExpiresIn:   expiresIn,
					User:        *user,
				},
			},
		}, nil
	})
}

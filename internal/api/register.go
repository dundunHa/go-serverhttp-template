package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
	"github.com/dundunHa/go-serverhttp-template/internal/service"
	"github.com/dundunHa/go-serverhttp-template/internal/service/auth"
)

type Deps struct {
	Users service.UserService
	Auth  *auth.AuthService
}

func Register(api huma.API, deps Deps) {
	registerHello(api)
	registerUsers(api, deps.Users)
	registerAuth(api, deps.Auth)
}

func registerHello(api huma.API) {
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

func registerUsers(api huma.API, users service.UserService) {
	huma.Register(api, huma.Operation{
		OperationID: "get-user",
		Method:      http.MethodGet,
		Path:        "/users/{id}",
		Summary:     "Get a user by ID",
		Tags:        []string{"users"},
	}, func(ctx context.Context, input *struct {
		ID string `path:"id" doc:"User ID" example:"1"`
	}) (*struct {
		Body model.Response[model.User]
	}, error) {
		id, err := strconv.Atoi(input.ID)
		if err != nil || id <= 0 {
			return nil, huma.Error400BadRequest("id must be a positive integer")
		}

		user, err := users.GetUser(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
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

func registerAuth(api huma.API, authSvc *auth.AuthService) {
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
		Body model.Response[model.UserInfo]
	}, error) {
		if input.Body.Token == "" {
			return nil, huma.Error400BadRequest("token required")
		}
		user, err := authSvc.Verify(ctx, input.Provider, input.Body.Token)
		if err != nil {
			if errors.Is(err, auth.ErrProviderNotFound) {
				return nil, huma.Error404NotFound("provider not found")
			}
			return nil, huma.Error401Unauthorized(err.Error())
		}

		return &struct {
			Body model.Response[model.UserInfo]
		}{
			Body: model.Response[model.UserInfo]{Data: *user},
		}, nil
	})
}

package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"go-serverhttp-template/internal/service/auth"
)

type AuthHandler struct {
	Svc *auth.AuthService
}

func NewAuthHandler(svc *auth.AuthService) *AuthHandler {
	return &AuthHandler{Svc: svc}
}

func (h *AuthHandler) Register(r chi.Router) {
	type authReq struct {
		Provider string `uri:"provider" json:"-" validate:"required"`
		Token    string `json:"token" validate:"required"`
	}
	r.Post("/{provider}", Adapter(func(ctx context.Context, req authReq) (*auth.UserInfo, *APIError) {
		user, err := h.Svc.Verify(ctx, req.Provider, req.Token)
		if err != nil {
			if errors.Is(err, auth.ErrProviderNotFound) {
				return nil, &APIError{Code: http.StatusNotFound, Message: "provider not found"}
			}
			if errors.Is(err, auth.ErrInvalidToken) || errors.Is(err, auth.ErrAuthFailed) {
				return nil, &APIError{Code: http.StatusUnauthorized, Message: "unauthorized"}
			}
			return nil, &APIError{Code: http.StatusInternalServerError, Message: "internal server error"}
		}
		return user, nil
	}))
}

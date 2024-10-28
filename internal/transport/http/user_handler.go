package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"go-serverhttp-template/internal/dao"
	"go-serverhttp-template/internal/service"
)

type UserHandler struct {
	svc service.UserService
}

func NewUserHandler(s service.UserService) *UserHandler {
	return &UserHandler{svc: s}
}

func (h *UserHandler) Register(r chi.Router) {
	type getReq struct {
		ID int `uri:"id" validate:"required"`
	}
	r.Get("/{id}", Adapter(func(ctx context.Context, req getReq) (*dao.User, *APIError) {
		u, err := h.svc.GetUser(req.ID)
		if err != nil {
			if errors.Is(err, service.ErrUserNotFound) {
				return nil, &APIError{Code: http.StatusNotFound, Message: "user not found"}
			}
			return nil, &APIError{Code: http.StatusInternalServerError, Message: "internal server error"}
		}
		return u, nil
	}))
}

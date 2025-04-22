package httpserver

import (
	"encoding/json"
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
	r.Post("/auth/{provider}", h.Auth)
}

func (h *AuthHandler) Auth(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSON(w, http.StatusBadRequest, AuthResponse{Code: 400, Message: "invalid request"})
		return
	}
	if req.Token == "" {
		WriteJSON(w, http.StatusBadRequest, AuthResponse{Code: 400, Message: "token required"})
		return
	}
	user, err := h.Svc.Verify(r.Context(), provider, req.Token)
	if err != nil {
		WriteJSON(w, http.StatusUnauthorized, AuthResponse{Code: 401, Message: err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, AuthResponse{Code: 0, Message: "ok", Data: user})
}

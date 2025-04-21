package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go-serverhttp-template/internal/service"

	"github.com/go-chi/chi/v5"
)

type UserHandler struct {
	svc service.UserService
}

func NewUserHandler(s service.UserService) *UserHandler {
	return &UserHandler{svc: s}
}

func (h *UserHandler) Register(r chi.Router) {
	r.Get("/{id}", h.Get)
}

func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	u, err := h.svc.GetUser(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(u)
}

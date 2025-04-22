package httpserver

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"go-serverhttp-template/internal/service"
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
		WriteJSON(w, http.StatusNotFound, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	WriteJSON(w, http.StatusOK, u)
}

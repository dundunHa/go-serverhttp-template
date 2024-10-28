package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"go-serverhttp-template/internal/service/auth"
	httpserver "go-serverhttp-template/internal/transport/http"
)

func TestAuthRoute_Path(t *testing.T) {
	t.Parallel()

	mgr := auth.NewProviderManager()
	mgr.Register("guest", auth.NewGuestProvider())

	authSvc := auth.NewAuthService(mgr, nil)
	authHandler := httpserver.NewAuthHandler(authSvc)

	r := chi.NewRouter()
	Register(r, nil, authHandler)

	body, _ := json.Marshal(struct {
		Token string `json:"token"`
	}{Token: "device-1"})
	req := httptest.NewRequest(http.MethodPost, "/auth/guest", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("expected code=0, got %d (message=%q)", resp.Code, resp.Message)
	}
	var ui auth.UserInfo
	if err := json.Unmarshal(resp.Data, &ui); err != nil {
		t.Fatalf("decode userinfo: %v", err)
	}
	if ui.Provider != "guest" || ui.ID != "device-1" {
		t.Fatalf("unexpected userinfo: %+v", ui)
	}
}

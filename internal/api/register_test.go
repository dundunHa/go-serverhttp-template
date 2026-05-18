package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/dundunHa/go-serverhttp-template/internal/service"
	"github.com/dundunHa/go-serverhttp-template/internal/service/auth"
)

func newTestRouter() http.Handler {
	router := chi.NewRouter()
	config := huma.DefaultConfig("Test API", "0.1.0")
	api := humachi.New(router, config)

	mgr := auth.NewProviderManager()
	mgr.Register("guest", auth.NewGuestProvider())
	Register(api, Deps{
		Users: service.NewMemoryUserService(),
		Auth:  auth.NewAuthService(mgr, nil),
	})

	return router
}

func TestRegisterHello(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	rec := httptest.NewRecorder()

	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Data.Message != "hello" {
		t.Fatalf("message = %q, want hello", got.Data.Message)
	}
}

func TestRegisterUsers(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	rec := httptest.NewRecorder()

	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		Data struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Data.ID != 1 || got.Data.Name == "" {
		t.Fatalf("unexpected user: %+v", got.Data)
	}
}

func TestRegisterUsersRejectsInvalidID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users/nope", nil)
	rec := httptest.NewRecorder()

	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestRegisterUsersReturnsNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users/404", nil)
	rec := httptest.NewRecorder()

	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestRegisterAuthGuest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/guest", strings.NewReader(`{"token":"device-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		Data struct {
			ID       string `json:"id"`
			Provider string `json:"provider"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Data.ID != "device-1" || got.Data.Provider != "guest" {
		t.Fatalf("unexpected auth response: %+v", got.Data)
	}
}

func TestRegisterAuthRequiresToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/guest", strings.NewReader(`{"token":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestRegisterAuthRejectsUnknownProvider(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/github", strings.NewReader(`{"token":"abc"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestRegisterOpenAPI(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	newTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"openapi"`) {
		t.Fatalf("openapi response missing openapi field: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"/users/{id}"`) {
		t.Fatalf("openapi response missing users route: %s", rec.Body.String())
	}
}

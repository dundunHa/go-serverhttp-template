package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
	"github.com/dundunHa/go-serverhttp-template/internal/service"
	"github.com/dundunHa/go-serverhttp-template/internal/service/auth"
)

const testJWTSecret = "test-secret"

func newTestAuthService(t testing.TB, identities auth.IdentityResolver) *auth.AuthService {
	t.Helper()
	tokenSvc, err := auth.NewTokenService(auth.TokenConfig{
		Secret:         testJWTSecret,
		Issuer:         "test",
		Audience:       "test-api",
		AccessTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}
	mgr := auth.NewProviderManager()
	mgr.Register("guest", auth.NewGuestProvider())
	return auth.NewAuthService(mgr, identities, tokenSvc)
}

func newUserTestRouter(t testing.TB) http.Handler {
	t.Helper()
	userSvc := service.NewMemoryUserService()

	return newUserTestRouterWithDeps(t, userSvc, newTestAuthService(t, userSvc))
}

func newUserTestRouterWithDeps(t testing.TB, userSvc service.UserService, authSvc auth.Service) http.Handler {
	t.Helper()
	router := chi.NewRouter()
	config := huma.DefaultConfig("Test API", "0.1.0")
	api := humachi.New(router, config)

	RegisterUserRoutes(api, UserDeps{
		Users: userSvc,
		Auth:  authSvc,
	})

	return router
}

func newAuthorizedUserRequest(t testing.TB, method, target string, body io.Reader) *http.Request {
	t.Helper()
	return newAuthorizedUserRequestForUser(t, authTestUser(), method, target, body)
}

func newAuthorizedUserRequestForUser(t testing.TB, user model.UserInfo, method, target string, body io.Reader) *http.Request {
	t.Helper()
	authSvc := newTestAuthService(t, service.NewMemoryUserService())
	token, _, err := authSvc.IssueAccessToken(context.Background(), user)
	if err != nil {
		t.Fatalf("issue test token: %v", err)
	}
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func authTestUser() model.UserInfo {
	return model.UserInfo{
		ID:              "1",
		Provider:        "guest",
		ProviderSubject: "tester",
	}
}

func TestUserRoutesHello(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	rec := httptest.NewRecorder()

	newUserTestRouter(t).ServeHTTP(rec, req)

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

func TestUserRoutesGetUser(t *testing.T) {
	req := newAuthorizedUserRequest(t, http.MethodGet, "/users/1", nil)
	rec := httptest.NewRecorder()

	newUserTestRouter(t).ServeHTTP(rec, req)

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

func TestUserRoutesRejectInvalidUserID(t *testing.T) {
	req := newAuthorizedUserRequest(t, http.MethodGet, "/users/nope", nil)
	rec := httptest.NewRecorder()

	newUserTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUserRoutesReturnNotFound(t *testing.T) {
	req := newAuthorizedUserRequestForUser(t, model.UserInfo{
		ID:              "404",
		Provider:        "guest",
		ProviderSubject: "missing-user",
	}, http.MethodGet, "/users/404", nil)
	rec := httptest.NewRecorder()

	newUserTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestUserRoutesRequireJWT(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	rec := httptest.NewRecorder()

	newUserTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestUserRoutesRejectMismatchedTokenSubject(t *testing.T) {
	authSvc := newTestAuthService(t, service.NewMemoryUserService())
	token, _, err := authSvc.IssueAccessToken(context.Background(), model.UserInfo{
		ID:              "2",
		Provider:        "guest",
		ProviderSubject: "other-device",
	})
	if err != nil {
		t.Fatalf("issue test token: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	newUserTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestUserRoutesAuthGuest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/guest", strings.NewReader(`{"token":"device-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newUserTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got struct {
		Data struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			User        struct {
				ID       string `json:"id"`
				Provider string `json:"provider"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Data.AccessToken == "" || got.Data.TokenType != "Bearer" {
		t.Fatalf("missing token response: %+v", got.Data)
	}
	if got.Data.User.ID != "1" || got.Data.User.Provider != "guest" {
		t.Fatalf("unexpected auth response: %+v", got.Data)
	}
	if strings.Contains(rec.Body.String(), "provider_subject") {
		t.Fatalf("auth response leaked provider subject: %s", rec.Body.String())
	}
}

func TestUserRoutesAuthRequireToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/guest", strings.NewReader(`{"token":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newUserTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUserRoutesAuthRejectUnknownProvider(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/github", strings.NewReader(`{"token":"abc"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newUserTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func TestUserRoutesAuthReturns500WhenIdentityResolverUnavailable(t *testing.T) {
	tokenSvc, err := auth.NewTokenService(auth.TokenConfig{
		Secret:         testJWTSecret,
		AccessTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}
	mgr := auth.NewProviderManager()
	mgr.Register("guest", auth.NewGuestProvider())
	authSvc := auth.NewAuthService(mgr, nil, tokenSvc)

	req := httptest.NewRequest(http.MethodPost, "/auth/guest", strings.NewReader(`{"token":"device-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newUserTestRouterWithDeps(t, service.NewMemoryUserService(), authSvc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestUserRoutesAuthReturns500WhenTokenServiceUnavailable(t *testing.T) {
	userSvc := service.NewMemoryUserService()
	mgr := auth.NewProviderManager()
	mgr.Register("guest", auth.NewGuestProvider())
	authSvc := auth.NewAuthService(mgr, userSvc, nil)

	req := httptest.NewRequest(http.MethodPost, "/auth/guest", strings.NewReader(`{"token":"device-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	newUserTestRouterWithDeps(t, userSvc, authSvc).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestUserRoutesOpenAPI(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	newUserTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"openapi"`) {
		t.Fatalf("openapi response missing openapi field: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"/users/{id}"`) {
		t.Fatalf("openapi response missing users route: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"bearerAuth"`) {
		t.Fatalf("openapi response missing bearer auth scheme: %s", rec.Body.String())
	}
}

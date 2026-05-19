package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
	"github.com/dundunHa/go-serverhttp-template/internal/service"
)

type stubTokenSvc struct {
	mu     sync.Mutex
	tokens map[int64]string
	err    error
	next   int
}

func newStubTokenSvc() *stubTokenSvc {
	return &stubTokenSvc{tokens: map[int64]string{}}
}

func (s *stubTokenSvc) EnsureAccountToken(_ context.Context, userID int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return "", s.err
	}
	if t, ok := s.tokens[userID]; ok {
		return t, nil
	}
	s.next++
	t := fmt.Sprintf("00000000-0000-4000-8000-%012d", s.next)
	s.tokens[userID] = t
	return t, nil
}

func newPaymentTestRouter(t testing.TB, tokens PaymentTokenService) http.Handler {
	t.Helper()
	userSvc := service.NewMemoryUserService()
	authSvc := newTestAuthService(t, userSvc)

	router := chi.NewRouter()
	config := huma.DefaultConfig("Test API", "0.1.0")
	humaAPI := humachi.New(router, config)
	RegisterUserRoutes(humaAPI, UserDeps{Users: userSvc, Auth: authSvc})
	RegisterPaymentRoutes(humaAPI, PaymentDeps{Auth: authSvc, Tokens: tokens})
	return router
}

var paymentUUIDV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestPaymentRoutes_AccountToken_Unauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/payment/apple/account-token", nil)
	newPaymentTestRouter(t, newStubTokenSvc()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestPaymentRoutes_AccountToken_BadBearerScheme(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/payment/apple/account-token", nil)
	req.Header.Set("Authorization", "Token abc")
	newPaymentTestRouter(t, newStubTokenSvc()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestPaymentRoutes_AccountToken_ReturnsUUID(t *testing.T) {
	tokens := newStubTokenSvc()
	rec := httptest.NewRecorder()
	req := newAuthorizedUserRequest(t, http.MethodGet, "/payment/apple/account-token", nil)
	newPaymentTestRouter(t, tokens).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			AppAccountToken string `json:"app_account_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Code != model.CodeSuccess || got.Msg != model.MsgSuccess {
		t.Fatalf("envelope mismatch: code=%d msg=%q", got.Code, got.Msg)
	}
	if !paymentUUIDV4.MatchString(got.Data.AppAccountToken) {
		t.Fatalf("token does not match v4 pattern: %q", got.Data.AppAccountToken)
	}
}

func TestPaymentRoutes_AccountToken_StableForSameUser(t *testing.T) {
	tokens := newStubTokenSvc()

	call := func() string {
		rec := httptest.NewRecorder()
		req := newAuthorizedUserRequest(t, http.MethodGet, "/payment/apple/account-token", nil)
		newPaymentTestRouter(t, tokens).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
		}
		var got struct {
			Data struct {
				AppAccountToken string `json:"app_account_token"`
			} `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		return got.Data.AppAccountToken
	}

	a := call()
	b := call()
	if a == "" || a != b {
		t.Fatalf("token not stable across calls: %q vs %q", a, b)
	}
}

func TestPaymentRoutes_AccountToken_DistinctUsers(t *testing.T) {
	tokens := newStubTokenSvc()

	call := func(user model.UserInfo) string {
		rec := httptest.NewRecorder()
		req := newAuthorizedUserRequestForUser(t, user, http.MethodGet, "/payment/apple/account-token", nil)
		newPaymentTestRouter(t, tokens).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
		}
		var got struct {
			Data struct {
				AppAccountToken string `json:"app_account_token"`
			} `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		return got.Data.AppAccountToken
	}

	a := call(model.UserInfo{ID: "1", Provider: "guest", ProviderSubject: "u1"})
	b := call(model.UserInfo{ID: "2", Provider: "guest", ProviderSubject: "u2"})
	if a == "" || b == "" || a == b {
		t.Fatalf("expected distinct tokens for distinct users, got %q and %q", a, b)
	}
}

func TestPaymentRoutes_AccountToken_ServiceError(t *testing.T) {
	tokens := newStubTokenSvc()
	tokens.err = errors.New("dao broken")
	rec := httptest.NewRecorder()
	req := newAuthorizedUserRequest(t, http.MethodGet, "/payment/apple/account-token", nil)
	newPaymentTestRouter(t, tokens).ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestPaymentRoutes_AccountToken_TokenServiceUnavailable(t *testing.T) {
	userSvc := service.NewMemoryUserService()
	authSvc := newTestAuthService(t, userSvc)

	router := chi.NewRouter()
	config := huma.DefaultConfig("Test API", "0.1.0")
	humaAPI := humachi.New(router, config)
	RegisterUserRoutes(humaAPI, UserDeps{Users: userSvc, Auth: authSvc})
	RegisterPaymentRoutes(humaAPI, PaymentDeps{Auth: authSvc, Tokens: nil})

	rec := httptest.NewRecorder()
	req := newAuthorizedUserRequest(t, http.MethodGet, "/payment/apple/account-token", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestPaymentRoutes_AccountToken_NonNumericSubjectRejected(t *testing.T) {
	tokens := newStubTokenSvc()
	rec := httptest.NewRecorder()
	req := newAuthorizedUserRequestForUser(t, model.UserInfo{
		ID:              "not-a-number",
		Provider:        "guest",
		ProviderSubject: "weird",
	}, http.MethodGet, "/payment/apple/account-token", nil)
	newPaymentTestRouter(t, tokens).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

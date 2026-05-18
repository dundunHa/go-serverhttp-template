package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

type stubResolver struct {
	user *model.UserInfo
}

func (r stubResolver) ResolveAuthIdentity(ctx context.Context, identity model.AuthIdentity) (*model.UserInfo, error) {
	_ = ctx
	user := *r.user
	user.Provider = identity.Provider
	user.ProviderSubject = identity.Subject
	user.Email = identity.Email
	return &user, nil
}

func TestAuthServiceVerifyResolvesProviderIdentity(t *testing.T) {
	mgr := NewProviderManager()
	mgr.Register("guest", NewGuestProvider())
	svc := NewAuthService(mgr, stubResolver{user: &model.UserInfo{ID: "42"}}, nil)

	user, err := svc.Verify(context.Background(), "guest", "device-1")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if user.ID != "42" || user.Provider != "guest" || user.ProviderSubject != "device-1" {
		t.Fatalf("unexpected resolved user: %+v", user)
	}
}

func TestAuthServiceVerifyRejectsUnknownProvider(t *testing.T) {
	svc := NewAuthService(NewProviderManager(), stubResolver{user: &model.UserInfo{ID: "42"}}, nil)

	_, err := svc.Verify(context.Background(), "github", "token")
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("err = %v, want %v", err, ErrProviderNotFound)
	}
}

func TestAuthServiceVerifyRequiresIdentityResolver(t *testing.T) {
	mgr := NewProviderManager()
	mgr.Register("guest", NewGuestProvider())
	svc := NewAuthService(mgr, nil, nil)

	_, err := svc.Verify(context.Background(), "guest", "device-1")
	if !errors.Is(err, ErrIdentityUnavailable) {
		t.Fatalf("err = %v, want %v", err, ErrIdentityUnavailable)
	}
}

func TestAuthServiceIssueAccessTokenRequiresTokenService(t *testing.T) {
	svc := NewAuthService(NewProviderManager(), nil, nil)

	_, _, err := svc.IssueAccessToken(context.Background(), model.UserInfo{ID: "1"})
	if !errors.Is(err, ErrTokenUnavailable) {
		t.Fatalf("err = %v, want %v", err, ErrTokenUnavailable)
	}
}

func TestAuthServiceAuthenticateAccessTokenReturnsUserInfo(t *testing.T) {
	tokenSvc, err := NewTokenService(TokenConfig{
		Secret:         "secret",
		AccessTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}
	svc := NewAuthService(NewProviderManager(), nil, tokenSvc)

	raw, _, err := svc.IssueAccessToken(context.Background(), model.UserInfo{
		ID:              "1",
		Email:           "user@example.com",
		Provider:        "guest",
		ProviderSubject: "device-1",
	})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	user, err := svc.AuthenticateAccessToken(context.Background(), raw)
	if err != nil {
		t.Fatalf("authenticate token: %v", err)
	}
	if user.ID != "1" || user.Provider != "guest" || user.ProviderSubject != "device-1" || user.Email != "user@example.com" {
		t.Fatalf("unexpected user: %+v", user)
	}
}

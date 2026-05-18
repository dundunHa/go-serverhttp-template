package auth

import (
	"context"
	"testing"
	"time"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

func TestTokenServiceIssueAndValidate(t *testing.T) {
	svc, err := NewTokenService(TokenConfig{
		Secret:         "secret",
		Issuer:         "issuer",
		Audience:       "audience",
		AccessTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	token, ttl, err := svc.IssueAccessToken(context.Background(), model.UserInfo{
		ID:              "user-1",
		Email:           "user@example.com",
		Provider:        "guest",
		ProviderSubject: "device-1",
	})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if token == "" || ttl != time.Hour {
		t.Fatalf("unexpected token result: token=%q ttl=%v", token, ttl)
	}

	claims, err := svc.ValidateAccessToken(context.Background(), token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if claims.Subject != "user-1" || claims.Provider != "guest" || claims.ProviderSubject != "device-1" || claims.Email != "user@example.com" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestTokenServiceRejectsInvalidToken(t *testing.T) {
	svc, err := NewTokenService(TokenConfig{
		Secret:         "secret",
		AccessTokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	if _, err := svc.ValidateAccessToken(context.Background(), "not-a-token"); err != ErrInvalidToken {
		t.Fatalf("err = %v, want %v", err, ErrInvalidToken)
	}
}

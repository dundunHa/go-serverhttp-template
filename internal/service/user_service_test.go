package service

import (
	"context"
	"errors"
	"testing"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

func TestMemoryUserService(t *testing.T) {
	svc := NewMemoryUserService()

	user, err := svc.GetUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetUser returned error: %v", err)
	}
	if user.ID != 1 || user.Name == "" {
		t.Fatalf("unexpected user: %+v", user)
	}
}

func TestMemoryUserServiceNotFound(t *testing.T) {
	svc := NewMemoryUserService()

	_, err := svc.GetUser(context.Background(), 404)
	if !errors.Is(err, dao.ErrUserNotFound) {
		t.Fatalf("error = %v, want %v", err, dao.ErrUserNotFound)
	}
}

func TestMemoryUserServiceResolvesAuthIdentityToStableUserID(t *testing.T) {
	svc := NewMemoryUserService()
	identity := model.AuthIdentity{
		Provider: "guest",
		Subject:  "device-1",
		Email:    "guest@example.com",
	}

	first, err := svc.ResolveAuthIdentity(context.Background(), identity)
	if err != nil {
		t.Fatalf("resolve first identity: %v", err)
	}
	second, err := svc.ResolveAuthIdentity(context.Background(), identity)
	if err != nil {
		t.Fatalf("resolve repeated identity: %v", err)
	}

	if first.ID != "1" || second.ID != "1" {
		t.Fatalf("resolved IDs = %q and %q, want both 1", first.ID, second.ID)
	}
	if first.ProviderSubject != "device-1" || first.Provider != "guest" || first.Email != "guest@example.com" {
		t.Fatalf("resolved user mismatch: %+v", first)
	}
}

func TestMemoryUserServiceCreatesUsersForNewAuthIdentities(t *testing.T) {
	svc := NewMemoryUserService()

	if _, err := svc.ResolveAuthIdentity(context.Background(), model.AuthIdentity{Provider: "guest", Subject: "device-1"}); err != nil {
		t.Fatalf("resolve first identity: %v", err)
	}
	next, err := svc.ResolveAuthIdentity(context.Background(), model.AuthIdentity{Provider: "guest", Subject: "device-2"})
	if err != nil {
		t.Fatalf("resolve second identity: %v", err)
	}
	if next.ID != "2" {
		t.Fatalf("second identity ID = %q, want 2", next.ID)
	}

	user, err := svc.GetUser(context.Background(), 2)
	if err != nil {
		t.Fatalf("get generated user: %v", err)
	}
	if user.ID != 2 || user.Name == "" {
		t.Fatalf("generated user mismatch: %+v", user)
	}
}

func TestMemoryUserServiceSeparatesAuthProviders(t *testing.T) {
	svc := NewMemoryUserService()

	guest, err := svc.ResolveAuthIdentity(context.Background(), model.AuthIdentity{Provider: "guest", Subject: "same-subject"})
	if err != nil {
		t.Fatalf("resolve guest: %v", err)
	}
	gmail, err := svc.ResolveAuthIdentity(context.Background(), model.AuthIdentity{Provider: "gmail", Subject: "same-subject"})
	if err != nil {
		t.Fatalf("resolve gmail: %v", err)
	}

	if guest.ID == gmail.ID {
		t.Fatalf("different providers mapped to the same user id: guest=%+v gmail=%+v", guest, gmail)
	}
}

func TestUserServiceResolveAuthIdentityUnsupported(t *testing.T) {
	svc := NewUserService(nil)

	_, err := svc.ResolveAuthIdentity(context.Background(), model.AuthIdentity{
		Provider: "guest",
		Subject:  "device-1",
	})
	if !errors.Is(err, ErrAuthIdentityUnsupported) {
		t.Fatalf("err = %v, want %v", err, ErrAuthIdentityUnsupported)
	}
}

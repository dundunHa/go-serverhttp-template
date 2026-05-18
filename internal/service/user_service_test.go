package service

import (
	"context"
	"errors"
	"testing"

	"github.com/dundunHa/go-serverhttp-template/internal/dao"
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

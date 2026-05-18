package auth

import (
	"context"
	"errors"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// AuthProvider 统一认证接口
type AuthProvider interface {
	VerifyToken(ctx context.Context, token string) (*model.AuthIdentity, error)
}

var (
	ErrInvalidToken        = errors.New("invalid token")
	ErrAuthFailed          = errors.New("authentication failed")
	ErrProviderNotFound    = errors.New("provider not found")
	ErrIdentityUnavailable = errors.New("identity resolver unavailable")
	ErrTokenUnavailable    = errors.New("token service unavailable")
)

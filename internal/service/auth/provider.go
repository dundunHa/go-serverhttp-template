package auth

import (
	"context"
	"errors"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// AuthProvider 统一认证接口
// 只需实现 VerifyToken，返回 UserInfo
type AuthProvider interface {
	VerifyToken(ctx context.Context, token string) (*model.UserInfo, error)
}

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrAuthFailed       = errors.New("authentication failed")
	ErrProviderNotFound = errors.New("provider not found")
)

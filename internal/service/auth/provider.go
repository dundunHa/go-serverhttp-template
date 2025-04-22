package auth

import (
	"context"
	"errors"
)

// AuthProvider 统一认证接口
// 只需实现 VerifyToken，返回 UserInfo
type AuthProvider interface {
	VerifyToken(ctx context.Context, token string) (*UserInfo, error)
}

// UserInfo 统一用户信息
// 只保留最基本字段
type UserInfo struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Provider string `json:"provider"`
}

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrAuthFailed       = errors.New("authentication failed")
	ErrProviderNotFound = errors.New("provider not found")
)

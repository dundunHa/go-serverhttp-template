package auth

import (
	"context"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// GuestProvider 实现 AuthProvider
type GuestProvider struct{}

func NewGuestProvider() *GuestProvider {
	return &GuestProvider{}
}

// VerifyToken 直接将 token 作为设备ID返回
func (p *GuestProvider) VerifyToken(ctx context.Context, token string) (*model.UserInfo, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}
	return &model.UserInfo{
		ID:       token,
		Email:    "",
		Provider: "guest",
	}, nil
}

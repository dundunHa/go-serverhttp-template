package auth

import (
	"context"

	"github.com/go-redis/redis"
)

type AuthService struct {
	mgr         *ProviderManager
	redisClient *redis.Client
}

func NewAuthService(mgr *ProviderManager, redisClient *redis.Client) *AuthService {
	return &AuthService{
		mgr:         mgr,
		redisClient: redisClient,
	}
}

// Verify 统一认证入口
func (s *AuthService) Verify(ctx context.Context, provider, token string) (*UserInfo, error) {
	p, ok := s.mgr.Get(provider)
	if !ok {
		return nil, ErrProviderNotFound
	}
	return p.VerifyToken(ctx, token)
}

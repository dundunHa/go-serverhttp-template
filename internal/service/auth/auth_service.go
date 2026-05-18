package auth

import (
	"context"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

type Service interface {
	Verify(ctx context.Context, provider, token string) (*model.UserInfo, error)
	IssueAccessToken(ctx context.Context, user model.UserInfo) (string, int64, error)
	AuthenticateAccessToken(ctx context.Context, token string) (*model.UserInfo, error)
}

type AuthService struct {
	mgr        *ProviderManager
	identities IdentityResolver
	tokens     *TokenService
}

func NewAuthService(mgr *ProviderManager, identities IdentityResolver, tokens *TokenService) *AuthService {
	return &AuthService{
		mgr:        mgr,
		identities: identities,
		tokens:     tokens,
	}
}

// Verify 统一认证入口
func (s *AuthService) Verify(ctx context.Context, provider, token string) (*model.UserInfo, error) {
	p, ok := s.mgr.Get(provider)
	if !ok {
		return nil, ErrProviderNotFound
	}
	identity, err := p.VerifyToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if s.identities == nil {
		return nil, ErrIdentityUnavailable
	}
	return s.identities.ResolveAuthIdentity(ctx, *identity)
}

func (s *AuthService) IssueAccessToken(ctx context.Context, user model.UserInfo) (string, int64, error) {
	if s.tokens == nil {
		return "", 0, ErrTokenUnavailable
	}
	token, ttl, err := s.tokens.IssueAccessToken(ctx, user)
	if err != nil {
		return "", 0, err
	}
	return token, int64(ttl.Seconds()), nil
}

func (s *AuthService) ValidateAccessToken(ctx context.Context, token string) (*TokenClaims, error) {
	if s.tokens == nil {
		return nil, ErrTokenUnavailable
	}
	return s.tokens.ValidateAccessToken(ctx, token)
}

func (s *AuthService) AuthenticateAccessToken(ctx context.Context, token string) (*model.UserInfo, error) {
	claims, err := s.ValidateAccessToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return &model.UserInfo{
		ID:              claims.Subject,
		Email:           claims.Email,
		Provider:        claims.Provider,
		ProviderSubject: claims.ProviderSubject,
	}, nil
}

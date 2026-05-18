package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

type TokenConfig struct {
	Secret         string
	Issuer         string
	Audience       string
	AccessTokenTTL time.Duration
}

type TokenClaims struct {
	Provider        string `json:"provider"`
	ProviderSubject string `json:"provider_subject,omitempty"`
	Email           string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret []byte
	issuer string
	aud    string
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenService(cfg TokenConfig) (*TokenService, error) {
	if cfg.Secret == "" {
		return nil, errors.New("jwt secret required")
	}
	if cfg.AccessTokenTTL <= 0 {
		return nil, errors.New("jwt access token ttl must be positive")
	}
	return &TokenService{
		secret: []byte(cfg.Secret),
		issuer: cfg.Issuer,
		aud:    cfg.Audience,
		ttl:    cfg.AccessTokenTTL,
		now:    time.Now,
	}, nil
}

func (s *TokenService) IssueAccessToken(ctx context.Context, user model.UserInfo) (string, time.Duration, error) {
	_ = ctx
	now := s.now().UTC()
	claims := TokenClaims{
		Provider:        user.Provider,
		ProviderSubject: user.ProviderSubject,
		Email:           user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
	}
	if s.aud != "" {
		claims.Audience = jwt.ClaimStrings{s.aud}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := token.SignedString(s.secret)
	if err != nil {
		return "", 0, fmt.Errorf("sign jwt: %w", err)
	}
	return raw, s.ttl, nil
}

func (s *TokenService) ValidateAccessToken(ctx context.Context, raw string) (*TokenClaims, error) {
	_ = ctx
	claims := &TokenClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	})
	if err != nil || token == nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	if s.issuer != "" && !claims.VerifyIssuer(s.issuer, true) {
		return nil, ErrInvalidToken
	}
	if s.aud != "" && !claims.VerifyAudience(s.aud, true) {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

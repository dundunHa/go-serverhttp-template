package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/lestrrat-go/jwx/jwk"

	"go-serverhttp-template/internal/config"
)

// AppleProvider 实现 AuthProvider
type AppleProvider struct {
	clientID        string
	jwksURL         string
	keySet          jwk.Set
	mu              sync.RWMutex
	lastFetch       time.Time
	refreshInterval time.Duration
}

// NewAppleProvider 创建新的AppleProvider实例
func NewAppleProvider(cfg config.AppleConfig) *AppleProvider {
	return &AppleProvider{
		clientID:        cfg.ClientID,
		jwksURL:         cfg.JwksURL,
		refreshInterval: cfg.RefreshInterval,
	}
}

// refreshKeys 拉取并解析 Apple JWKs
func (p *AppleProvider) refreshKeys(ctx context.Context) error {
	p.mu.RLock()
	if time.Since(p.lastFetch) < p.refreshInterval {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	set, err := jwk.Fetch(ctx, p.jwksURL)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}

	p.mu.Lock()
	p.keySet = set
	p.lastFetch = time.Now()
	p.mu.Unlock()
	return nil
}

// VerifyToken 校验 Apple ID Token
func (p *AppleProvider) VerifyToken(ctx context.Context, token string) (*UserInfo, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}

	if err := p.refreshKeys(ctx); err != nil {
		return nil, fmt.Errorf("refresh keys: %w", err)
	}

	type appleClaims struct {
		Email string `json:"email"`
		jwt.RegisteredClaims
	}

	parsed, err := jwt.ParseWithClaims(token, &appleClaims{}, func(t *jwt.Token) (interface{}, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok {
			return nil, ErrInvalidToken
		}

		p.mu.RLock()
		key, found := p.keySet.LookupKeyID(kid)
		p.mu.RUnlock()

		if !found {
			return nil, ErrAuthFailed
		}

		var pubkey interface{}
		if err := key.Raw(&pubkey); err != nil {
			return nil, fmt.Errorf("get public key: %w", err)
		}
		return pubkey, nil
	})

	if err != nil || !parsed.Valid {
		return nil, ErrAuthFailed
	}

	claims := parsed.Claims.(*appleClaims)

	if claims.Issuer != "https://appleid.apple.com" {
		return nil, ErrAuthFailed
	}

	if !claims.VerifyAudience(p.clientID, true) {
		return nil, ErrAuthFailed
	}

	return &UserInfo{
		ID:       claims.Subject,
		Email:    claims.Email,
		Provider: "apple",
	}, nil
}

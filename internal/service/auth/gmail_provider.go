package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/dundunHa/go-serverhttp-template/internal/config"
	"github.com/dundunHa/go-serverhttp-template/internal/model"
)

// GmailProvider 实现 AuthProvider
type GmailProvider struct {
	clientID   string
	httpClient *http.Client
}

func NewGmailProvider(cfg config.GmailConfig) *GmailProvider {
	return &GmailProvider{
		clientID:   cfg.ClientID,
		httpClient: http.DefaultClient,
	}
}

// VerifyToken 校验 Google ID Token
func (p *GmailProvider) VerifyToken(ctx context.Context, token string) (*model.AuthIdentity, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}
	// 调用 Google tokeninfo API
	tokenInfoURL := "https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenInfoURL, nil)
	if err != nil {
		return nil, ErrAuthFailed
	}
	resp, err := p.client().Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil, ErrAuthFailed
	}
	defer resp.Body.Close()
	var data struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Aud   string `json:"aud"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, ErrAuthFailed
	}
	if data.Aud != p.clientID {
		return nil, ErrAuthFailed
	}
	return &model.AuthIdentity{
		Provider: "gmail",
		Subject:  data.Sub,
		Email:    data.Email,
	}, nil
}

func (p *GmailProvider) client() *http.Client {
	if p.httpClient != nil {
		return p.httpClient
	}
	return http.DefaultClient
}

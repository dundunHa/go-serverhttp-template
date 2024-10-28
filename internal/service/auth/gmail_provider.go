package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// GmailProvider 实现 AuthProvider
type GmailProvider struct {
	clientID string
	client   *http.Client
}

func NewGmailProvider(clientID string) *GmailProvider {
	return NewGmailProviderWithClient(clientID, &http.Client{Timeout: 5 * time.Second})
}

func NewGmailProviderWithClient(clientID string, client *http.Client) *GmailProvider {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &GmailProvider{clientID: clientID, client: client}
}

// VerifyToken 校验 Google ID Token
func (p *GmailProvider) VerifyToken(ctx context.Context, token string) (*UserInfo, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}
	// 调用 Google tokeninfo API
	url := "https://oauth2.googleapis.com/tokeninfo?id_token=" + token
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := p.client.Do(req)
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
	return &UserInfo{
		ID:       data.Sub,
		Email:    data.Email,
		Provider: "gmail",
	}, nil
}

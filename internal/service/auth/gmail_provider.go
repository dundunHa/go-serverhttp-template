package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
)

// GmailProvider 实现 AuthProvider
type GmailProvider struct {
	clientID string
}

func NewGmailProvider() *GmailProvider {
	return &GmailProvider{
		clientID: os.Getenv("GMAIL_CLIENT_ID"),
	}
}

// VerifyToken 校验 Google ID Token
func (p *GmailProvider) VerifyToken(ctx context.Context, token string) (*UserInfo, error) {
	if token == "" {
		return nil, ErrInvalidToken
	}
	// 调用 Google tokeninfo API
	url := "https://oauth2.googleapis.com/tokeninfo?id_token=" + token
	resp, err := http.Get(url)
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

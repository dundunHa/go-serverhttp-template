package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/lestrrat-go/jwx/jwk"

	"go-serverhttp-template/internal/config"
)

// TestAppleProvider_VerifyToken 覆盖 AppleProvider.VerifyToken 的各种场景
func TestAppleProvider_VerifyToken(t *testing.T) {
	// 1. 生成一对 RSA 密钥
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("生成 RSA 私钥失败: %v", err)
	}
	// 2. 从公钥构造 JWK，并设置 kid 再加入 Set
	pubJWK, err := jwk.New(&privKey.PublicKey)
	if err != nil {
		t.Fatalf("构造 JWK 失败: %v", err)
	}
	kid := "test-kid"
	if err := pubJWK.Set(jwk.KeyIDKey, kid); err != nil {
		t.Fatalf("设置 kid 失败: %v", err)
	}
	keySet := jwk.NewSet()
	keySet.Add(pubJWK)

	// 3. 初始化 AppleProvider，并跳过 refreshKeys
	cfg := config.AppleConfig{
		ClientID:        "client123",
		JwksURL:         "https://unused.example.com",
		RefreshInterval: time.Hour,
	}
	prov := NewAppleProvider(cfg)
	prov.mu.Lock()
	prov.keySet = keySet
	prov.lastFetch = time.Now()
	prov.mu.Unlock()

	// 4. helper：生成带不同 Issuer/Audience 的 token
	makeToken := func(issuer string, aud []string) string {
		claims := struct {
			Email string `json:"email"`
			jwt.RegisteredClaims
		}{
			Email: "test@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    issuer,
				Audience:  aud,
				Subject:   "sub123",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = kid
		signed, err := tok.SignedString(privKey)
		if err != nil {
			t.Fatalf("签名 token 失败: %v", err)
		}
		return signed
	}

	tests := []struct {
		name    string
		token   string
		wantErr error
	}{
		{"空 token", "", ErrInvalidToken},
		{"格式不对的 token", "not.a.jwt", ErrAuthFailed},
		{"Issuer 不匹配", makeToken("wrong-issuer", []string{"client123"}), ErrAuthFailed},
		{"Audience 不匹配", makeToken("https://appleid.apple.com", []string{"wrong"}), ErrAuthFailed},
		{"正常流程", makeToken("https://appleid.apple.com", []string{"client123"}), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ui, err := prov.VerifyToken(context.Background(), tt.token)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("期望错误 %v，实际 %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("意外错误: %v", err)
			}
			// 校验返回的 UserInfo
			if ui.Provider != "apple" || ui.ID != "sub123" || ui.Email != "test@example.com" {
				t.Errorf("返回值不正确: %+v", ui)
			}
		})
	}
}

// TestAppleProvider_VerifyToken_Success 单独测试正常流程
func TestAppleProvider_VerifyToken_Success(t *testing.T) {
	// 1. 生成一对 RSA 密钥
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("生成 RSA 私钥失败: %v", err)
	}
	// 2. 构造 JWK Set 并赋值给 provider
	pubJWK, _ := jwk.New(&privKey.PublicKey)
	const kid = "success-kid"
	_ = pubJWK.Set(jwk.KeyIDKey, kid)
	set := jwk.NewSet()
	set.Add(pubJWK)

	// 3. 初始化 provider
	cfg := config.AppleConfig{ClientID: "client-success", RefreshInterval: time.Hour}
	prov := NewAppleProvider(cfg)
	prov.mu.Lock()
	prov.keySet = set
	prov.lastFetch = time.Now()
	prov.mu.Unlock()

	// 4. 签发一个合法的 token
	claims := struct {
		Email string `json:"email"`
		jwt.RegisteredClaims
	}{
		Email: "ok@apple.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://appleid.apple.com",
			Audience:  []string{"client-success"},
			Subject:   "user-123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(privKey)
	if err != nil {
		t.Fatalf("签名 token 失败: %v", err)
	}

	// 5. 调用 VerifyToken 并断言
	ui, err := prov.VerifyToken(context.Background(), signed)
	if err != nil {
		t.Fatalf("期望正常通过，实际出错: %v", err)
	}
	if ui.Provider != "apple" || ui.ID != "user-123" || ui.Email != "ok@apple.com" {
		t.Errorf("返回值不正确: %+v", ui)
	}
}

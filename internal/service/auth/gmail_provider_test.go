package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeRT 用于模拟 HTTP 请求的 RoundTripper
type fakeRT struct {
	resp *http.Response
	err  error
}

func (f *fakeRT) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.resp, f.err
}

// TestGmailProvider_VerifyToken 覆盖 GmailProvider.VerifyToken 的各种场景
func TestGmailProvider_VerifyToken(t *testing.T) {
	tests := []struct {
		name      string
		rt        *fakeRT
		token     string
		wantErr   error
		wantID    string
		wantEmail string
	}{
		{"空 token", nil, "", ErrInvalidToken, "", ""},
		{"HTTP 调用失败", &fakeRT{nil, errors.New("net error")}, "tok", ErrAuthFailed, "", ""},
		{"非 200 状态码", &fakeRT{
			resp: &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(""))},
			err:  nil,
		}, "tok", ErrAuthFailed, "", ""},
		{"JSON 解码失败", &fakeRT{
			resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("not json"))},
			err:  nil,
		}, "tok", ErrAuthFailed, "", ""},
		{"aud 字段不匹配", &fakeRT{
			resp: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"sub":"uid","email":"u@e.com","aud":"wrong"}`)),
			},
			err: nil,
		}, "tok", ErrAuthFailed, "", ""},
		{"正常流程", &fakeRT{
			resp: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"sub":"uid","email":"u@e.com","aud":"client123"}`)),
			},
			err: nil,
		}, "tok", nil, "uid", "u@e.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p *GmailProvider
			if tt.rt != nil {
				p = NewGmailProviderWithClient("client123", &http.Client{Transport: tt.rt})
			} else {
				p = NewGmailProviderWithClient("client123", nil)
			}
			ui, err := p.VerifyToken(context.Background(), tt.token)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("期望错误 %v，实际 %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("意外错误: %v", err)
			}
			// 验证返回值
			if ui.Provider != "gmail" || ui.ID != tt.wantID || ui.Email != tt.wantEmail {
				t.Errorf("返回值不正确: %+v", ui)
			}
		})
	}
}

// TestGmailProvider_VerifyToken_Success 单独测试正常流程
func TestGmailProvider_VerifyToken_Success(t *testing.T) {
	// 1. 模拟一个 200 且内容合法的 Response
	body := `{"sub":"g123","email":"ok@gmail.com","aud":"gmail-success"}`
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	rt := &fakeRT{resp: resp, err: nil}

	// 2. 构造 provider 并注入 client
	p := NewGmailProviderWithClient("gmail-success", &http.Client{Transport: rt})

	// 3. 调用 VerifyToken 并断言
	ui, err := p.VerifyToken(context.Background(), "any-token")
	if err != nil {
		t.Fatalf("期望正常通过，实际出错: %v", err)
	}
	if ui.Provider != "gmail" || ui.ID != "g123" || ui.Email != "ok@gmail.com" {
		t.Errorf("返回值不正确: %+v", ui)
	}
}

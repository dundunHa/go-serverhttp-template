package payment

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestShouldFallbackToSandbox(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cancelErr := cancelled.Err()

	cases := []struct {
		name       string
		err        error
		preferProd bool
		enable     bool
		hasSandbox bool
		want       bool
	}{
		{name: "not_found_with_fallback", err: ErrAppleTransactionNotFound, preferProd: true, enable: true, hasSandbox: true, want: true},
		{name: "not_found_no_sandbox_client", err: ErrAppleTransactionNotFound, preferProd: true, enable: true, hasSandbox: false, want: false},
		{name: "not_found_disabled", err: ErrAppleTransactionNotFound, preferProd: true, enable: false, hasSandbox: true, want: false},
		{name: "auth_rejected_never_falls_back", err: ErrAppleAuthRejected, preferProd: true, enable: true, hasSandbox: true, want: false},
		{name: "non_prod_call_never_falls_back", err: ErrAppleTransactionNotFound, preferProd: false, enable: true, hasSandbox: true, want: false},
		{name: "context_cancelled_never_falls_back", err: cancelErr, preferProd: true, enable: true, hasSandbox: true, want: false},
		{name: "unrelated_error_never_falls_back", err: errors.New("network reset"), preferProd: true, enable: true, hasSandbox: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldFallbackToSandbox(tc.err, tc.preferProd, tc.enable, tc.hasSandbox)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClassifyAppleError(t *testing.T) {
	cases := []struct {
		name        string
		input       error
		wantSentinel error
		wantNil     bool
	}{
		{name: "nil_passthrough", input: nil, wantNil: true},
		{name: "not_found_4040010", input: errors.New("apple status 4040010"), wantSentinel: ErrAppleTransactionNotFound},
		{name: "not_found_404", input: errors.New("apple returned 404 not found"), wantSentinel: ErrAppleTransactionNotFound},
		{name: "auth_401", input: errors.New("apple status 401 unauthorized"), wantSentinel: ErrAppleAuthRejected},
		{name: "auth_403", input: errors.New("apple status 403 forbidden"), wantSentinel: ErrAppleAuthRejected},
		{name: "context_canceled_passthrough", input: context.Canceled, wantSentinel: context.Canceled},
		{name: "deadline_passthrough", input: context.DeadlineExceeded, wantSentinel: context.DeadlineExceeded},
		{name: "unknown_passthrough", input: errors.New("network reset"), wantSentinel: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyAppleError(tc.input)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil error")
			}
			if tc.wantSentinel != nil && !errors.Is(got, tc.wantSentinel) {
				t.Fatalf("expected to wrap %v, got %v", tc.wantSentinel, got)
			}
		})
	}
}

func TestLooksLikeCompactJWS(t *testing.T) {
	cases := map[string]bool{
		"":              false,
		"abc":           false,
		"abc.def":       false,
		"abc.def.ghi":   true,
		"abc..ghi":      false,
		"a.b.c.d":       false,
		strings.Repeat("a", 10) + "." + strings.Repeat("b", 10) + "." + strings.Repeat("c", 10): true,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := looksLikeCompactJWS(in); got != want {
				t.Fatalf("looksLikeCompactJWS(%q) = %v, want %v", in, got, want)
			}
		})
	}
}

func TestSHA256Hex(t *testing.T) {
	h := sha256Hex("apple")
	if len(h) != 64 {
		t.Fatalf("sha256 hex must be 64 chars, got %d", len(h))
	}
	if h2 := sha256Hex("apple"); h != h2 {
		t.Fatalf("sha256 not stable: %s vs %s", h, h2)
	}
	if hb := sha256Hex("Apple"); h == hb {
		t.Fatalf("sha256 should distinguish casing")
	}
}

func TestNewAppleVerifiers_NotConfigured(t *testing.T) {
	if _, err := NewAppleTransactionVerifier(nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil catalog: got %v, want ErrNotConfigured", err)
	}
	if _, err := NewAppleWebhookVerifier(nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil catalog webhook: got %v, want ErrNotConfigured", err)
	}
	if _, err := NewAppleReconciler(nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("nil catalog reconciler: got %v, want ErrNotConfigured", err)
	}
}

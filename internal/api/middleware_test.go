package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chiMw "github.com/go-chi/chi/v5/middleware"

	logpkg "github.com/dundunHa/go-serverhttp-template/pkg/log"
)

func TestRedactSensitiveJSON(t *testing.T) {
	got := redactSensitiveJSON(`{"token":"provider-token","data":{"access_token":"jwt","user":{"id":"1"}}}`)

	if strings.Contains(got, "provider-token") || strings.Contains(got, "jwt") {
		t.Fatalf("sensitive values were not redacted: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") || !strings.Contains(got, `"id":"1"`) {
		t.Fatalf("unexpected redacted JSON: %s", got)
	}
}

func TestRedactSensitiveJSON_AppleIAPFields(t *testing.T) {
	cases := []struct {
		name string
		in   string
		hide []string
		keep []string
	}{
		{
			name: "webhook_signed_payload",
			in:   `{"signedPayload":"eyJhbGciOiJF.aaa.bbb","other":"keep"}`,
			hide: []string{"eyJhbGciOiJF.aaa.bbb"},
			keep: []string{"keep"},
		},
		{
			name: "snake_case_signed_payload",
			in:   `{"signed_payload":"eyJaaa","unrelated":"value"}`,
			hide: []string{"eyJaaa"},
			keep: []string{"value"},
		},
		{
			name: "app_account_token_both_cases",
			in:   `{"appAccountToken":"uuid-abc","data":{"app_account_token":"uuid-xyz"}}`,
			hide: []string{"uuid-abc", "uuid-xyz"},
		},
		{
			name: "private_key_and_p8",
			in:   `{"private_key":"-----BEGIN PRIVATE KEY-----","p8":"file-contents","p8_path":"/path"}`,
			hide: []string{"BEGIN PRIVATE KEY", "file-contents", "/path"},
		},
		{
			name: "apple_iap_private_key_alias",
			in:   `{"apple_iap_private_key":"pem-blob"}`,
			hide: []string{"pem-blob"},
		},
		{
			name: "raw_jws_and_decoded_payload",
			in:   `{"raw_jws":"abc.def.ghi","decoded_payload":{"transactionId":"200000123"}}`,
			hide: []string{"abc.def.ghi", "200000123"},
		},
		{
			name: "key_id_and_issuer_id",
			in:   `{"key_id":"ABC123","issuer_id":"57246542"}`,
			hide: []string{"ABC123", "57246542"},
		},
		{
			name: "nested_in_arrays",
			in:   `{"events":[{"signedPayload":"abc"},{"signedPayload":"def"}]}`,
			hide: []string{"abc", "def"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactSensitiveJSON(tc.in)
			for _, secret := range tc.hide {
				if strings.Contains(got, secret) {
					t.Fatalf("expected %q redacted, got %s", secret, got)
				}
			}
			if !strings.Contains(got, "[REDACTED]") {
				t.Fatalf("expected [REDACTED] marker in %s", got)
			}
			for _, plain := range tc.keep {
				if !strings.Contains(got, plain) {
					t.Fatalf("expected %q preserved, got %s", plain, got)
				}
			}
		})
	}
}

func TestRedactSensitiveJSON_NonJSONUnchanged(t *testing.T) {
	in := "not-json"
	if got := redactSensitiveJSON(in); got != in {
		t.Fatalf("non-json should pass through unchanged, got %q", got)
	}
	if got := redactSensitiveJSON(""); got != "" {
		t.Fatalf("empty input should pass through, got %q", got)
	}
}

func TestLoggingInjectsModuleIntoHandlerContext(t *testing.T) {
	var logs bytes.Buffer
	root := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := chiMw.RequestID(InjectRootLogger(root)(Logging()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logpkg.FromContext(r.Context()).InfoContext(r.Context(), "handler log")
		w.WriteHeader(http.StatusNoContent)
	}))))

	req := httptest.NewRequest(http.MethodGet, "/users/1", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	for _, entry := range jsonLogEntries(t, logs.String()) {
		if entry["msg"] == "handler log" {
			if entry["module"] != "users" {
				t.Fatalf("handler log module = %v, want users; entry=%#v", entry["module"], entry)
			}
			if entry["request_id"] == "" || entry["trace_id"] == "" {
				t.Fatalf("handler log missing request IDs: %#v", entry)
			}
			return
		}
	}
	t.Fatalf("handler log was not emitted: %s", logs.String())
}

func TestLoggingRestoresRequestBodyAndRedactsLogs(t *testing.T) {
	var logs bytes.Buffer
	root := slog.New(slog.NewJSONHandler(&logs, nil))
	var bodySeen string
	handler := chiMw.RequestID(InjectRootLogger(root)(Logging()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		bodySeen = string(bodyBytes)
		w.WriteHeader(http.StatusAccepted)
	}))))

	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/auth/guest", strings.NewReader(`{"token":"secret-token"}`)),
	)

	if bodySeen != `{"token":"secret-token"}` {
		t.Fatalf("handler saw body %q", bodySeen)
	}
	for _, entry := range jsonLogEntries(t, logs.String()) {
		if entry["msg"] == "Started Request" {
			if entry["module"] != "auth" {
				t.Fatalf("request log module = %v, want auth; entry=%#v", entry["module"], entry)
			}
			body, ok := entry["body"].(string)
			if !ok {
				t.Fatalf("request log body is not a string: %#v", entry)
			}
			if strings.Contains(body, "secret-token") {
				t.Fatalf("sensitive request body should be redacted: %#v", entry)
			}
			if !strings.Contains(body, "[REDACTED]") {
				t.Fatalf("request body missing redaction marker: %#v", entry)
			}
			return
		}
	}
	t.Fatalf("request log was not emitted: %s", logs.String())
}

func TestLoggingDoesNotLogLargeRequestOrResponseBodies(t *testing.T) {
	var logs bytes.Buffer
	root := slog.New(slog.NewJSONHandler(&logs, nil))
	largeRequestBody := `{"token":"` + strings.Repeat("secret", maxBodyLogSize) + `"}`
	var bodySeen string
	handler := chiMw.RequestID(InjectRootLogger(root)(Logging()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		bodySeen = string(bodyBytes)
		_, _ = w.Write([]byte(`{"access_token":"` + strings.Repeat("response-secret", maxBodyLogSize) + `"}`))
	}))))

	handler.ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/auth/guest", strings.NewReader(largeRequestBody)),
	)

	if bodySeen != largeRequestBody {
		t.Fatalf("handler body was not restored")
	}
	for _, entry := range jsonLogEntries(t, logs.String()) {
		if body, ok := entry["body"].(string); ok {
			if body != truncatedBodyLogValue {
				t.Fatalf("large request body log = %q, want %q", body, truncatedBodyLogValue)
			}
		}
		if responseBody, ok := entry["response_body"].(string); ok {
			if responseBody != truncatedBodyLogValue {
				t.Fatalf("large response body log = %q, want %q", responseBody, truncatedBodyLogValue)
			}
		}
	}
	if strings.Contains(logs.String(), "response-secret") || strings.Contains(logs.String(), "secretsecret") {
		t.Fatalf("large sensitive body leaked into logs: %s", logs.String())
	}
}

func jsonLogEntries(t testing.TB, raw string) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(raw))
	var entries []map[string]any
	for {
		var entry map[string]any
		if err := decoder.Decode(&entry); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode JSON log: %v; raw=%s", err, raw)
		}
		entries = append(entries, entry)
	}
	return entries
}

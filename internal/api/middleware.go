package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/cors"

	chiMw "github.com/go-chi/chi/v5/middleware"

	logpkg "github.com/dundunHa/go-serverhttp-template/pkg/log"
)

const maxBodyLogSize = 8 * 1024
const truncatedBodyLogValue = "[TRUNCATED]"

type responseCapture struct {
	chiMw.WrapResponseWriter
	body      *bytes.Buffer
	truncated bool
}

func newResponseCapture(w http.ResponseWriter, protoMajor int) *responseCapture {
	return &responseCapture{
		WrapResponseWriter: chiMw.NewWrapResponseWriter(w, protoMajor),
		body:               &bytes.Buffer{},
	}
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	rc.captureBody(b)
	return rc.WrapResponseWriter.Write(b)
}

func (rc *responseCapture) captureBody(b []byte) {
	if rc.body.Len() > maxBodyLogSize {
		rc.truncated = true
		return
	}
	remaining := maxBodyLogSize + 1 - rc.body.Len()
	if len(b) > remaining {
		rc.truncated = true
		_, _ = rc.body.Write(b[:remaining])
		return
	}
	_, _ = rc.body.Write(b)
}

func (rc *responseCapture) logBody() string {
	if rc.truncated || rc.body.Len() > maxBodyLogSize {
		return truncatedBodyLogValue
	}
	return redactSensitiveJSON(rc.body.String())
}

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logpkg.FromContext(r.Context()).ErrorContext(
					r.Context(),
					"panic recovered",
					"panic", err,
					"stack", string(debug.Stack()),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    http.StatusInternalServerError,
					"message": "internal server error",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func CORS() func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})
}

func InjectRootLogger(root *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(logpkg.NewContext(r.Context(), root))
			next.ServeHTTP(w, r)
		})
	}
}

func Logging() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rootLogger := logpkg.FromContext(r.Context())
			requestID := requestID(r)
			logger := rootLogger.With(
				"module", moduleName(r.URL.Path),
				"trace_id", headerOrDefault(r, "X-Trace-ID", requestID),
				"request_id", requestID,
				"client_ip", clientIP(r),
			)
			r = r.WithContext(logpkg.NewContext(r.Context(), logger))

			start := time.Now()
			requestBody := readRequestBodyForLog(r)
			logger.InfoContext(
				r.Context(),
				"Started Request",
				"phase", "start",
				"method", r.Method,
				"uri", r.RequestURI,
				"query", r.URL.Query(),
				"body", requestBody,
			)

			ww := newResponseCapture(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			logger.InfoContext(
				r.Context(),
				"Completed Request",
				"phase", "completed",
				"status", ww.Status(),
				"duration", time.Since(start),
				"response_body", ww.logBody(),
			)
		})
	}
}

func moduleName(path string) string {
	switch {
	case path == "/hello", strings.HasPrefix(path, "/docs"), strings.HasPrefix(path, "/openapi"):
		return "system"
	case strings.HasPrefix(path, "/auth"):
		return "auth"
	case strings.HasPrefix(path, "/users"):
		return "users"
	default:
		return "http"
	}
}

func headerOrDefault(r *http.Request, name string, fallback string) string {
	if value := r.Header.Get(name); value != "" {
		return value
	}
	return fallback
}

func requestID(r *http.Request) string {
	if value := chiMw.GetReqID(r.Context()); value != "" {
		return value
	}
	return r.Header.Get(chiMw.RequestIDHeader)
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.Split(forwarded, ",")[0]
	}
	return r.RemoteAddr
}

func readRequestBodyForLog(r *http.Request) string {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return ""
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxBodyLogSize+1))
	if err != nil {
		logpkg.FromContext(r.Context()).ErrorContext(r.Context(), "read request body failed", "err", err)
		return ""
	}
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(bodyBytes), r.Body))
	if len(bodyBytes) > maxBodyLogSize {
		return truncatedBodyLogValue
	}
	return redactSensitiveJSON(string(bodyBytes))
}

func redactSensitiveJSON(value string) string {
	if value == "" {
		return ""
	}

	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return value
	}
	redactSensitiveValue(decoded)

	redacted, err := json.Marshal(decoded)
	if err != nil {
		return value
	}
	return string(redacted)
}

func redactSensitiveValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, field := range typed {
			if isSensitiveField(key) {
				typed[key] = "[REDACTED]"
				continue
			}
			redactSensitiveValue(field)
		}
	case []any:
		for _, item := range typed {
			redactSensitiveValue(item)
		}
	}
}

func isSensitiveField(field string) bool {
	switch strings.ToLower(field) {
	case "token", "access_token", "refresh_token", "id_token", "password", "secret":
		return true
	default:
		return false
	}
}

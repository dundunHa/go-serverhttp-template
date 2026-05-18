package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	chiMw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const maxBodyLogSize = 8 * 1024

type responseCapture struct {
	chiMw.WrapResponseWriter
	body *bytes.Buffer
}

func newResponseCapture(w http.ResponseWriter, protoMajor int) *responseCapture {
	return &responseCapture{
		WrapResponseWriter: chiMw.NewWrapResponseWriter(w, protoMajor),
		body:               &bytes.Buffer{},
	}
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	rc.body.Write(b)
	return rc.WrapResponseWriter.Write(b)
}

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Error().Interface("panic", err).Bytes("stack", debug.Stack()).Msg("panic recovered")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
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

func InjectRootLogger(root *zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(root.WithContext(r.Context()))
			next.ServeHTTP(w, r)
		})
	}
}

func Logging(module string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rootLogger := zerolog.Ctx(r.Context())
			logger := rootLogger.With().
				Str("module", module).
				Str("trace_id", headerOrNewID(r, "X-Trace-ID")).
				Str("request_id", headerOrNewID(r, "X-Request-ID")).
				Str("client_ip", clientIP(r)).
				Logger()
			r = r.WithContext(logger.WithContext(r.Context()))

			start := time.Now()
			requestBody := readSmallRequestBody(r)
			logger.Info().
				Str("phase", "start").
				Str("method", r.Method).
				Str("uri", r.RequestURI).
				Interface("query", r.URL.Query()).
				Str("body", requestBody).
				Msg("Started Request")

			ww := newResponseCapture(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			logger.Info().
				Str("phase", "completed").
				Int("status", ww.Status()).
				Dur("duration", time.Since(start)).
				Str("response_body", truncate(ww.body.String(), maxBodyLogSize)).
				Msg("Completed Request")
		})
	}
}

func headerOrNewID(r *http.Request, name string) string {
	if value := r.Header.Get(name); value != "" {
		return value
	}
	return uuid.New().String()
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

func readSmallRequestBody(r *http.Request) string {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return ""
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		zerolog.Ctx(r.Context()).Error().Err(err).Msg("read request body failed")
		return ""
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	return truncate(string(bodyBytes), maxBodyLogSize)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "... (truncated)"
}

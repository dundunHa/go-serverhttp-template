package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	chiMw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const maxBodyLogSize = 8 * 1024

type ResponseCapture struct {
	chiMw.WrapResponseWriter
	body *bytes.Buffer
}

func NewResponseCapture(w http.ResponseWriter, protoMajor int) *ResponseCapture {
	return &ResponseCapture{
		WrapResponseWriter: chiMw.NewWrapResponseWriter(w, protoMajor),
		body:               &bytes.Buffer{},
	}
}

func (rc *ResponseCapture) Write(b []byte) (int, error) {
	rc.body.Write(b)
	return rc.WrapResponseWriter.Write(b)
}

// InjectRootLogger 注入根 Logger 到上下文
func InjectRootLogger(root *zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(root.WithContext(r.Context()))
			next.ServeHTTP(w, r)
		})
	}
}

// LoggingMiddleware 记录请求开始与完成日志
func LoggingMiddleware(module string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从 Context 获取根 Logger
			rootLogger := zerolog.Ctx(r.Context())
			baseLogger := rootLogger.With().Str("module", module).Logger()

			start := time.Now()

			// 生成 trace_id 与 request_id
			traceID := r.Header.Get("X-Trace-ID")
			if traceID == "" {
				traceID = uuid.New().String()
			}
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			clientIP := r.RemoteAddr
			if ip := r.Header.Get("X-Real-IP"); ip != "" {
				clientIP = ip
			} else if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				clientIP = strings.Split(fwd, ",")[0]
			}
			userID, _ := r.Context().Value("userID").(string)

			logger := baseLogger.With().
				Str("trace_id", traceID).
				Str("request_id", requestID).
				Str("user_id", userID).
				Str("client_ip", clientIP).
				Logger()
			r = r.WithContext(logger.WithContext(r.Context()))

			method := r.Method
			uri := r.RequestURI
			query := r.URL.Query()

			// 读取请求体
			var requestBody string
			if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
				bodyBytes, err := io.ReadAll(r.Body)
				if err != nil {
					logger.Error().Err(err).Msg("Error reading request body")
				} else {
					r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
					if len(bodyBytes) > maxBodyLogSize {
						requestBody = string(bodyBytes[:maxBodyLogSize]) + "... (truncated)"
					} else {
						requestBody = string(bodyBytes)
					}
				}
			}

			logger.Info().
				Str("phase", "start").
				Str("method", method).
				Str("uri", uri).
				Interface("query", query).
				Str("body", requestBody).
				Msg("Started Request")
			ww := NewResponseCapture(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)
			duration := time.Since(start)
			statusCode := ww.Status()

			respFull := ww.body.String()
			var respBody string
			if len(respFull) > maxBodyLogSize {
				respBody = respFull[:maxBodyLogSize] + "... (truncated)"
			} else {
				respBody = respFull
			}
			logger.Info().
				Str("phase", "completed").
				Int("status", statusCode).
				Dur("duration", duration).
				Str("response_body", respBody).
				Msg("Completed Request")
		})
	}
}

package api

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const maxBodyLogSize = 8 * 1024 // 8 KB

type ResponseCapture struct {
	middleware.WrapResponseWriter
	body *bytes.Buffer
}

func NewResponseCapture(w http.ResponseWriter, protoMajor int) *ResponseCapture {
	return &ResponseCapture{
		WrapResponseWriter: middleware.NewWrapResponseWriter(w, protoMajor),
		body:               &bytes.Buffer{},
	}
}

func (rc *ResponseCapture) Write(b []byte) (int, error) {
	rc.body.Write(b) // Capture response body
	return rc.WrapResponseWriter.Write(b)
}

// LoggingMiddleware 支持注入模块名，方便在路由初始化时添加模块信息
func LoggingMiddleware(module string) func(http.Handler) http.Handler {
	// 初始化基础 logger，附加模块信息
	baseLogger := log.Logger.With().Str("module", module).Logger()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// 提取或生成 traceID 和 requestID
			traceID := r.Header.Get("X-Trace-ID")
			if traceID == "" {
				traceID = uuid.New().String()
			}
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}
			// 提取客户端 IP
			clientIP := r.RemoteAddr
			if ip := r.Header.Get("X-Real-IP"); ip != "" {
				clientIP = ip
			} else if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				clientIP = strings.Split(fwd, ",")[0]
			}
			// 提取 userID（由 BasicAuth 注入）
			userID, _ := r.Context().Value("userID").(string)

			// 构建 per-request Logger 并注入 Context，使用传入的 baseLogger 作为基础
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

			// 读取并截断响应体
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

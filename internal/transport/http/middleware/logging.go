package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	chiMw "github.com/go-chi/chi/v5/middleware"
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
	if rc.body.Len() < maxBodyLogSize {
		remain := maxBodyLogSize - rc.body.Len()
		if len(b) > remain {
			rc.body.Write(b[:remain])
		} else {
			rc.body.Write(b)
		}
	}
	return rc.WrapResponseWriter.Write(b)
}

func isJSONRequest(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return false
	}
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "application/json")
}

func redactJSONFields(v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, vv := range t {
			lk := strings.ToLower(k)
			switch lk {
			case "token", "password", "authorization", "access_token", "refresh_token", "id_token":
				t[k] = "***"
				continue
			}
			redactJSONFields(vv)
		}
	case []interface{}:
		for i := range t {
			redactJSONFields(t[i])
		}
	}
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
func LoggingMiddleware(module string, logBody bool) func(http.Handler) http.Handler {
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

			w.Header().Set("X-Trace-ID", traceID)
			w.Header().Set("X-Request-ID", requestID)

			clientIP := r.RemoteAddr
			if ip := r.Header.Get("X-Real-IP"); ip != "" {
				clientIP = ip
			} else if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				clientIP = strings.Split(fwd, ",")[0]
			}
			userID, _ := UserID(r.Context())

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
			if logBody && (method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch) {
				if r.ContentLength > maxBodyLogSize || r.ContentLength < 0 {
					requestBody = "[omitted]"
				} else if !isJSONRequest(r) {
					requestBody = "[omitted]"
				} else {
					bodyBytes, err := io.ReadAll(r.Body)
					if err != nil {
						logger.Error().Err(err).Msg("Error reading request body")
					} else {
						r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
						var payload interface{}
						if err := json.Unmarshal(bodyBytes, &payload); err != nil {
							requestBody = "[omitted]"
						} else {
							redactJSONFields(payload)
							if redacted, err := json.Marshal(payload); err == nil {
								requestBody = string(redacted)
							} else {
								requestBody = "[omitted]"
							}
						}
					}
				}
			}

			startEvt := logger.Info().
				Str("phase", "start").
				Str("method", method).
				Str("uri", uri).
				Interface("query", query)
			if logBody {
				startEvt = startEvt.Str("body", requestBody)
			}
			startEvt.Msg("Started Request")

			var (
				statusFn   func() int
				respBodyFn func() string
				ww         http.ResponseWriter
			)
			if logBody {
				rc := NewResponseCapture(w, r.ProtoMajor)
				statusFn = rc.Status
				respBodyFn = func() string { return rc.body.String() }
				ww = rc
			} else {
				rc := chiMw.NewWrapResponseWriter(w, r.ProtoMajor)
				statusFn = rc.Status
				ww = rc
			}

			next.ServeHTTP(ww, r)
			duration := time.Since(start)
			statusCode := statusFn()

			doneEvt := logger.Info().
				Str("phase", "completed").
				Int("status", statusCode).
				Dur("duration", duration)
			if logBody && respBodyFn != nil {
				respFull := respBodyFn()
				doneEvt = doneEvt.Str("response_body", respFull)
			}
			doneEvt.Msg("Completed Request")
		})
	}
}

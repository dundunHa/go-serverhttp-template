package api

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/middleware"
	"go.uber.org/zap"
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

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		method := r.Method
		uri := r.RequestURI
		query := r.URL.Query()

		var requestBody string
		if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				log.Println("Error reading body", zap.Error(err))
			} else {
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				if len(bodyBytes) > maxBodyLogSize {
					requestBody = string(bodyBytes[:maxBodyLogSize]) + "... (truncated)"
				} else {
					requestBody = string(bodyBytes)
				}
			}
		}
		log.Println("Started Request",
			zap.String("method", method),
			zap.String("uri", uri),
			zap.Any("query", query),
			zap.String("body", requestBody),
		)
		ww := NewResponseCapture(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)
		duration := time.Since(start)
		statusCode := ww.Status()

		log.Println("Completed Request",
			zap.String("method", method),
			zap.String("uri", uri),
			zap.Int("status", statusCode),
			zap.Duration("duration", duration),
		)
	})
}

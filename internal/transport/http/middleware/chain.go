package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/go-chi/cors"
	"github.com/rs/zerolog/log"

	httpserver "go-serverhttp-template/internal/transport/http"
)

// CORS 中间件，允许所有来源（可根据需要调整）
func CORS() func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
	})
}

// ErrorHandler 统一错误处理，捕获 panic 并返回统一格式
func ErrorHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error().Interface("panic", rec).Bytes("stack", debug.Stack()).Msg("panic recovered")
				httpserver.WriteError(w, &httpserver.APIError{Code: 500, Message: "内部错误"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

package router

import (
	"github.com/go-chi/chi"

	"go-serverhttp-template/internal/api"
	"go-serverhttp-template/internal/middleware"

	"net/http"

	"github.com/rs/zerolog"
)

// 新增：注入根Logger的中间件
func InjectRootLogger(root *zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := root.WithContext(r.Context())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func Register(r *chi.Mux, logger *zerolog.Logger) {
	r.Use(InjectRootLogger(logger))

	r.Route("/hello", func(g chi.Router) {
		g.Use(middleware.LoggingMiddleware("hello"))
		g.Get("/", api.GetHelloHandler())
	})
}

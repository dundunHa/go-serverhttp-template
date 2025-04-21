package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"go-serverhttp-template/internal/api"
	httpmiddleware "go-serverhttp-template/internal/transport/http/middleware"
)

// Register 注册所有业务路由
func Register(r chi.Router, logger *zerolog.Logger) {
	// 注入根 Logger
	r.Use(httpmiddleware.InjectRootLogger(logger))
	// Hello 模块路由示例
	r.Route("/hello", func(g chi.Router) {
		g.Use(httpmiddleware.LoggingMiddleware("hello"))
		g.Get("/", api.GetHelloHandler())
	})
}

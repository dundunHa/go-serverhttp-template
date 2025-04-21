package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"go-serverhttp-template/internal/api"
	httpserver "go-serverhttp-template/internal/transport/http"
	httpmiddleware "go-serverhttp-template/internal/transport/http/middleware"
)

func Register(r chi.Router, logger *zerolog.Logger, userHandler *httpserver.UserHandler) {
	r.Use(httpmiddleware.InjectRootLogger(logger))
	r.Route("/hello", func(g chi.Router) {
		g.Use(httpmiddleware.LoggingMiddleware("hello"))
		g.Get("/", api.GetHelloHandler())
	})
	r.Route("/users", func(g chi.Router) {
		userHandler.Register(g)
	})
}

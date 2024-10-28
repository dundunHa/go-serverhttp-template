package router

import (
	"github.com/go-chi/chi/v5"

	httpserver "go-serverhttp-template/internal/transport/http"
)

func Register(r chi.Router, userHandler *httpserver.UserHandler, authHandler *httpserver.AuthHandler) {
	r.Route("/hello", func(g chi.Router) {
		g.Get("/", httpserver.GetHelloHandler())
	})
	if userHandler != nil {
		r.Route("/users", func(g chi.Router) {
			userHandler.Register(g)
		})
	}
	if authHandler != nil {
		r.Route("/auth", func(g chi.Router) {
			authHandler.Register(g)
		})
	}
}

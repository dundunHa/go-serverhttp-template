package api

import (
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
)

func Register(r *chi.Mux) {
	r.Use(LoggingMiddleware)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}))

	r.Route("/", func(g chi.Router) {
		g.Use(BasicAuth)

		g.Get("/status", GetHello)
	})
}

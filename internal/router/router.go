package router

import (
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"

	"go-serverhttp-template/internal/api"
	"go-serverhttp-template/internal/middleware"
)

func Register(r *chi.Mux) {
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.With(middleware.LoggingMiddleware("hello")).
		Route("/", func(g chi.Router) {
			// g.Use(BasicAuth)
			g.Get("/status", api.GetHello)
		})
}

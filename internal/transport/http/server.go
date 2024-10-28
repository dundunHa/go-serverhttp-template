package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Server struct {
	mux *chi.Mux
}

func NewServer() *Server {
	mux := chi.NewRouter()
	return &Server{mux: mux}
}

// Use 注册全局中间件
func (s *Server) Use(mwf ...func(http.Handler) http.Handler) {
	for _, m := range mwf {
		s.mux.Use(m)
	}
}

// WithRoutes 注册业务路由
func (s *Server) WithRoutes(register func(r chi.Router)) {
	register(s.mux)
}

// Handler 返回 http.Handler
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Router 返回 chi.Router，用于挂载更多路由
func (s *Server) Router() chi.Router {
	return s.mux
}

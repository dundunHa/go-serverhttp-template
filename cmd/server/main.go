package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"go-serverhttp-template/internal/config"
	"go-serverhttp-template/internal/router"
	httpserver "go-serverhttp-template/internal/transport/http"
	"go-serverhttp-template/internal/transport/http/middleware"
	logpkg "go-serverhttp-template/pkg/log"
)

func main() {
	conf := config.LoadConfig()

	logpkg.InitLogger(conf.Log)
	log.Info().Msg("Logger initialized")

	srv := httpserver.NewServer()
	srv.Use(
		middleware.Recovery,
		middleware.CORS(),
		middleware.ErrorHandler,
	)
	// 注册业务路由
	baseLogger := log.Logger.With().Str("module", "http").Logger()
	srv.WithRoutes(func(r chi.Router) {
		router.Register(r, &baseLogger)
	})
	// 挂载 Swagger 文档路由
	httpserver.MountSwagger(srv.Router())

	// 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", conf.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info().Str("addr", addr).Msg("Starting HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed unexpectedly")
		}
	}()

	// 优雅关机
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutdown signal received, commencing graceful shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server.SetKeepAlivesEnabled(false)

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Graceful shutdown failed")
	} else {
		log.Info().Msg("Server gracefully stopped")
	}

	// TODO: 清理其他资源（如数据库连接等）
}

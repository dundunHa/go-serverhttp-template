package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/rs/zerolog/log"

	"go-serverhttp-template/internal/config"
	"go-serverhttp-template/internal/router"
	logpkg "go-serverhttp-template/pkg/log"
)

func main() {
	conf := config.LoadConfig()

	// 初始化日志系统
	logpkg.InitLogger(conf.Log)
	log.Info().Msg("Logger initialized")

	r := chi.NewRouter()
	baseLogger := log.Logger.With().Str("module", "http").Logger()
	router.Register(r, &baseLogger)

	// 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", conf.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info().Str("addr", addr).Msg("Starting HTTP server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed unexpectedly")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutdown signal received, commencing graceful shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	srv.SetKeepAlivesEnabled(false)

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Graceful shutdown failed")
	} else {
		log.Info().Msg("Server gracefully stopped")
	}

	// TODO: 清理其他资源（如数据库连接等）
}

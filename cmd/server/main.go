package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"go-serverhttp-template/internal/comfyui"
	"go-serverhttp-template/internal/config"
	"go-serverhttp-template/internal/dao"
	"go-serverhttp-template/internal/router"
	"go-serverhttp-template/internal/service"
	"go-serverhttp-template/internal/service/auth"
	"go-serverhttp-template/internal/storage"
	httpserver "go-serverhttp-template/internal/transport/http"
	"go-serverhttp-template/internal/transport/http/middleware"
	"go-serverhttp-template/pkg/cache"
	logpkg "go-serverhttp-template/pkg/log"
)

func main() {
	conf, err := config.LoadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("load config failed")
	}

	initLogger(conf.Log)
	initCache(conf.Cache)

	db, err := storage.InitDB()
	if err != nil {
		log.Fatal().Err(err).Msg("init db failed")
	}
	userDAO := initUserDAO(db)
	userSvc := initUserService(userDAO)
	mgr := auth.NewProviderManager()
	mgr.Register("gmail", auth.NewGmailProvider())
	mgr.Register("apple", auth.NewAppleProvider(conf.Auth.Apple))
	mgr.Register("guest", auth.NewGuestProvider())
	authSvc := auth.NewAuthService(mgr, nil)
	authHandler := httpserver.NewAuthHandler(authSvc)
	userHandler := initUserHandler(userSvc)

	// 初始化 ComfyUI 客户端
	comfyUIClient := comfyui.NewClient(conf.ComfyUI)
	comfyUIHandler := httpserver.NewComfyUIHandler(comfyUIClient)

	srv := newHTTPServer(conf.Server.Port, userHandler, authHandler, comfyUIHandler)
	startServer(srv)

	waitForShutdown(srv, 10*time.Second)
}

func initLogger(cfg logpkg.Config) {
	logpkg.InitLogger(cfg)
	log.Info().Msg("Logger initialized")
}
func initCache(cfg config.CacheConfig) {
	cacheCfg := cache.Config{
		Addrs:        cfg.Addrs,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
	cache.Init(cacheCfg)
	log.Info().Msg("Cache initialized")
}

func initUserDAO(db *sql.DB) dao.UserDAO {
	return dao.NewUserDAO(db)
}

func initUserService(userDAO dao.UserDAO) service.UserService {
	return service.NewUserService(userDAO)
}

func initUserHandler(userSvc service.UserService) *httpserver.UserHandler {
	return httpserver.NewUserHandler(userSvc)
}

// 构建一个带中间件和路由的 HTTP Server
func newHTTPServer(port int, userHandler *httpserver.UserHandler, authHandler *httpserver.AuthHandler, comfyUIHandler *httpserver.ComfyUIHandler) *http.Server {
	svc := httpserver.NewServer()
	svc.Use(
		middleware.Recovery,
		middleware.CORS(),
		middleware.ErrorHandler,
	)

	baseLogger := log.Logger.With().Str("module", "http").Logger()
	svc.WithRoutes(func(r chi.Router) {
		router.Register(r, &baseLogger, userHandler, authHandler, comfyUIHandler)
	})
	httpserver.MountSwagger(svc.Router())

	addr := fmt.Sprintf(":%d", port)
	return &http.Server{
		Addr:         addr,
		Handler:      svc.Handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// 并发启动 HTTP 服务
func startServer(srv *http.Server) {
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("Starting HTTP server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed unexpectedly")
		}
	}()
}

// 捕捉 SIGINT/SIGTERM 并优雅关机
func waitForShutdown(srv *http.Server, timeout time.Duration) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutdown signal received, commencing graceful shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	srv.SetKeepAlivesEnabled(false)
	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Graceful shutdown failed")
	} else {
		log.Info().Msg("Server gracefully stopped")
	}
}

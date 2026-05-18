package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	chiMw "github.com/go-chi/chi/v5/middleware"

	"github.com/dundunHa/go-serverhttp-template/internal/api"
	"github.com/dundunHa/go-serverhttp-template/internal/config"
	"github.com/dundunHa/go-serverhttp-template/internal/dao"
	"github.com/dundunHa/go-serverhttp-template/internal/service"
	"github.com/dundunHa/go-serverhttp-template/internal/service/auth"
	"github.com/dundunHa/go-serverhttp-template/internal/storage"
	"github.com/dundunHa/go-serverhttp-template/pkg/cache"
	logpkg "github.com/dundunHa/go-serverhttp-template/pkg/log"
)

func main() {
	conf, err := config.LoadConfig()
	if err != nil {
		slog.Error("load config failed", "err", err)
		os.Exit(1)
	}

	initLogger(conf.AppEnv, conf.Log)
	initCache(conf.Cache)

	db, err := storage.InitDB(conf.DB.DSN)
	if err != nil && !errors.Is(err, storage.ErrMissingDSN) {
		slog.Error("init db failed", "err", err)
		os.Exit(1)
	}
	if errors.Is(err, storage.ErrMissingDSN) {
		slog.Warn("DB_DSN is empty; using in-memory demo user service")
	}
	userSvc := initUserService(db)
	mgr := auth.NewProviderManager()
	mgr.Register("gmail", auth.NewGmailProvider(conf.Auth.Gmail))
	mgr.Register("apple", auth.NewAppleProvider(conf.Auth.Apple))
	mgr.Register("guest", auth.NewGuestProvider())
	tokenSvc, err := auth.NewTokenService(auth.TokenConfig{
		Secret:         conf.Auth.JWT.Secret,
		Issuer:         conf.Auth.JWT.Issuer,
		Audience:       conf.Auth.JWT.Audience,
		AccessTokenTTL: conf.Auth.JWT.AccessTokenTTL,
	})
	if err != nil {
		slog.Error("init jwt service failed", "err", err)
		os.Exit(1)
	}
	authSvc := auth.NewAuthService(mgr, userSvc, tokenSvc)

	srv := newHTTPServer(conf.Server.Port, userSvc, authSvc)
	startServer(srv)

	waitForShutdown(srv, 10*time.Second)
}

func initLogger(appEnv string, cfg logpkg.Config) {
	logpkg.InitLogger(appEnv, cfg)
	slog.Info("Logger initialized", "app_env", appEnv)
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
	slog.Info("Cache initialized")
}

func initUserService(db *sql.DB) service.UserService {
	if db == nil {
		return service.NewMemoryUserService()
	}
	return service.NewUserService(dao.NewUserDAO(db))
}

// 构建一个带中间件和路由的 HTTP Server
func newHTTPServer(port int, userSvc service.UserService, authSvc auth.Service) *http.Server {
	r := chi.NewRouter()
	r.Use(
		chiMw.RequestID,
		api.Recovery,
		api.CORS(),
	)

	r.Use(api.InjectRootLogger(slog.Default()))
	r.Use(api.Logging())

	humaConfig := huma.DefaultConfig("Go Server HTTP Template API", "0.1.0")
	humaConfig.OpenAPIPath = "/openapi"
	humaConfig.DocsPath = "/docs"
	humaAPI := humachi.New(r, humaConfig)
	api.RegisterUserRoutes(humaAPI, api.UserDeps{
		Users: userSvc,
		Auth:  authSvc,
	})

	addr := fmt.Sprintf(":%d", port)
	return &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// 并发启动 HTTP 服务
func startServer(srv *http.Server) {
	go func() {
		slog.Info("Starting HTTP server", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed unexpectedly", "err", err)
			os.Exit(1)
		}
	}()
}

// 捕捉 SIGINT/SIGTERM 并优雅关机
func waitForShutdown(srv *http.Server, timeout time.Duration) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutdown signal received, commencing graceful shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	srv.SetKeepAlivesEnabled(false)
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Graceful shutdown failed", "err", err)
	} else {
		slog.Info("Server gracefully stopped")
	}
}

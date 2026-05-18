package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/dundunHa/go-serverhttp-template/internal/api"
	"github.com/dundunHa/go-serverhttp-template/internal/comfyui"
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
		log.Fatal().Err(err).Msg("load config failed")
	}

	initLogger(conf.Log)
	initCache(conf.Cache)

	db, err := storage.InitDB(conf.DB.DSN)
	if err != nil && !errors.Is(err, storage.ErrMissingDSN) {
		log.Fatal().Err(err).Msg("init db failed")
	}
	if errors.Is(err, storage.ErrMissingDSN) {
		log.Warn().Msg("DB_DSN is empty; using in-memory demo user service")
	}
	userSvc := initUserService(db)
	mgr := auth.NewProviderManager()
	mgr.Register("gmail", auth.NewGmailProvider())
	mgr.Register("apple", auth.NewAppleProvider(conf.Auth.Apple))
	mgr.Register("guest", auth.NewGuestProvider())
	authSvc := auth.NewAuthService(mgr, nil)

	// 初始化 ComfyUI 客户端
	comfyUIClient := comfyui.NewClient(conf.ComfyUI)
	comfyUIHandler := api.NewComfyUIHandler(comfyUIClient)

	srv := newHTTPServer(conf.Server.Port, userSvc, authSvc, comfyUIHandler)
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

func initUserService(db *sql.DB) service.UserService {
	if db == nil {
		return service.NewMemoryUserService()
	}
	return service.NewUserService(dao.NewUserDAO(db))
}

// 构建一个带中间件和路由的 HTTP Server
func newHTTPServer(port int, userSvc service.UserService, authSvc *auth.AuthService, comfyUIHandler *api.ComfyUIHandler) *http.Server {
	r := chi.NewRouter()
	r.Use(
		api.Recovery,
		api.CORS(),
	)

	baseLogger := log.Logger
	r.Use(api.InjectRootLogger(&baseLogger))
	r.Use(api.Logging("http"))

	humaConfig := huma.DefaultConfig("Go Server HTTP Template API", "0.1.0")
	humaConfig.OpenAPIPath = "/openapi"
	humaConfig.DocsPath = "/docs"
	humaAPI := humachi.New(r, humaConfig)
	api.Register(humaAPI, api.Deps{
		Users: userSvc,
		Auth:  authSvc,
	})

	r.Route("/api/v1/comfyui", func(g chi.Router) {
		comfyUIHandler.Register(g)
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

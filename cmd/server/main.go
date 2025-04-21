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

	"go-serverhttp-template/internal/config"
	"go-serverhttp-template/internal/dao"
	"go-serverhttp-template/internal/router"
	"go-serverhttp-template/internal/service"
	httpserver "go-serverhttp-template/internal/transport/http"
	"go-serverhttp-template/internal/transport/http/middleware"
	"go-serverhttp-template/pkg/cache"
	logpkg "go-serverhttp-template/pkg/log"
)

func main() {
	// 1. 加载配置
	conf := config.LoadConfig()

	// 2. 初始化各个组件
	initLogger(conf.Log)
	initCache(conf.Cache)

	// 3a. 初始化 DB/DAO/Service/Handler
	db, err := initDB(conf)
	if err != nil {
		log.Fatal().Err(err).Msg("init db failed")
	}
	userDAO := initUserDAO(db)
	userSvc := initUserService(userDAO)
	userHandler := initUserHandler(userSvc)

	// 3b. 构建并启动 HTTP 服务
	srv := newHTTPServer(conf.Server.Port, userHandler)
	startServer(srv)

	// 4. 等待系统信号并优雅关机
	waitForShutdown(srv, 10*time.Second)
}

// 初始化日志
func initLogger(cfg logpkg.Config) {
	logpkg.InitLogger(cfg)
	log.Info().Msg("Logger initialized")
}

// 初始化 Redis 缓存
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

// 新增：初始化数据库连接
func initDB(cfg *config.Config) (*sql.DB, error) {
	// 这里可根据实际情况选择 database/sql 或 gorm
	return sql.Open("postgres", cfg.DB.Mysql.Addr)
}

// 新增：初始化 DAO
func initUserDAO(db *sql.DB) dao.UserDAO {
	return dao.NewUserDAO(db)
}

// 新增：初始化 Service
func initUserService(userDAO dao.UserDAO) service.UserService {
	return service.NewUserService(userDAO)
}

// 新增：初始化 Handler
func initUserHandler(userSvc service.UserService) *httpserver.UserHandler {
	return httpserver.NewUserHandler(userSvc)
}

// 构建一个带中间件和路由的 HTTP Server
func newHTTPServer(port int, userHandler *httpserver.UserHandler) *http.Server {
	svc := httpserver.NewServer()
	svc.Use(
		middleware.Recovery,
		middleware.CORS(),
		middleware.ErrorHandler,
	)

	baseLogger := log.Logger.With().Str("module", "http").Logger()
	svc.WithRoutes(func(r chi.Router) {
		router.Register(r, &baseLogger, userHandler)
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

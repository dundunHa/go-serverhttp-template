package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"go-serverhttp-template/internal/config"
	"go-serverhttp-template/internal/dao"
	"go-serverhttp-template/internal/router"
	"go-serverhttp-template/internal/service"
	"go-serverhttp-template/internal/service/auth"
	"go-serverhttp-template/internal/storage"
	grpcserver "go-serverhttp-template/internal/transport/grpc"
	httpserver "go-serverhttp-template/internal/transport/http"
	"go-serverhttp-template/internal/transport/http/middleware"
	"go-serverhttp-template/pkg/cache"
	logpkg "go-serverhttp-template/pkg/log"
)

type stopFunc func(context.Context) error

func main() {
	conf, err := config.LoadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("load config failed")
	}

	initLogger(conf.Log)
	logStartupWarnings(conf.Warnings())
	initCache(conf.Cache)

	var userHandler *httpserver.UserHandler
	db := initDBIfConfigured(conf.DB)
	if db != nil {
		userDAO := initUserDAO(db)
		userSvc := initUserService(userDAO)
		userHandler = initUserHandler(userSvc)
	}

	authHandler := initAuthHandler(conf)

	serverStops := make([]stopFunc, 0, 2)
	resourceStops := make([]stopFunc, 0, 2)

	resourceStops = append(resourceStops, func(ctx context.Context) error {
		_ = ctx
		return cache.Close()
	})
	if db != nil {
		resourceStops = append(resourceStops, func(ctx context.Context) error {
			_ = ctx
			return db.Close()
		})
	}

	switch conf.Mode {
	case config.ModeHTTP:
		srv := newHTTPServer(conf.HTTP.Port, conf.HTTP.LogBody, userHandler, authHandler)
		startHTTPServer(srv)
		serverStops = append(serverStops, srv.Shutdown)
	case config.ModeGRPC:
		stop, err := startGRPCServer(conf.GRPC.Port)
		if err != nil {
			log.Fatal().Err(err).Msg("start grpc server failed")
		}
		serverStops = append(serverStops, stop)
	case config.ModeBoth:
		srv := newHTTPServer(conf.HTTP.Port, conf.HTTP.LogBody, userHandler, authHandler)
		startHTTPServer(srv)
		serverStops = append(serverStops, srv.Shutdown)

		stop, err := startGRPCServer(conf.GRPC.Port)
		if err != nil {
			log.Fatal().Err(err).Msg("start grpc server failed")
		}
		serverStops = append(serverStops, stop)
	default:
		log.Fatal().Str("mode", string(conf.Mode)).Msg("invalid mode; expected http|grpc|both")
	}

	waitForShutdown(10*time.Second, serverStops, resourceStops)
}

func initLogger(cfg logpkg.Config) {
	logpkg.InitLogger(cfg)
	log.Info().Msg("Logger initialized")
}

func logStartupWarnings(warns []string) {
	for _, w := range warns {
		log.Warn().Msg(w)
	}
}

func initCache(cfg config.CacheConfig) {
	if len(cfg.Addrs) == 0 {
		log.Warn().Msg("cache disabled: missing cache.addrs")
		return
	}
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
func newHTTPServer(port int, logBody bool, userHandler *httpserver.UserHandler, authHandler *httpserver.AuthHandler) *http.Server {
	svc := httpserver.NewServer()
	baseLogger := log.Logger.With().Str("module", "http").Logger()
	svc.Use(
		middleware.CORS(),
		middleware.ErrorHandler,
		middleware.InjectRootLogger(&baseLogger),
		middleware.LoggingMiddleware("http", logBody),
	)
	svc.WithRoutes(func(r chi.Router) {
		router.Register(r, userHandler, authHandler)
	})

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
func startHTTPServer(srv *http.Server) {
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("Starting HTTP server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed unexpectedly")
		}
	}()
}

// 捕捉 SIGINT/SIGTERM 并优雅关机
func waitForShutdown(timeout time.Duration, serverStops, resourceStops []stopFunc) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutdown signal received, commencing graceful shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	shutdown(ctx, serverStops, resourceStops)
	log.Info().Msg("Shutdown completed")
}

func shutdown(ctx context.Context, serverStops, resourceStops []stopFunc) {
	log.Info().Msg("Stopping servers")
	runStops(ctx, serverStops)
	log.Info().Msg("Closing resources")
	runStops(ctx, resourceStops)
}

func runStops(ctx context.Context, stops []stopFunc) {
	var wg sync.WaitGroup
	for _, stop := range stops {
		if stop == nil {
			continue
		}
		wg.Add(1)
		go func(s stopFunc) {
			defer wg.Done()
			if err := s(ctx); err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				log.Error().Err(err).Msg("shutdown hook failed")
			}
		}(stop)
	}
	wg.Wait()
}

func initDBIfConfigured(cfg config.DBConfig) *sql.DB {
	if cfg.DSN == "" {
		return nil
	}
	db, err := storage.InitDB(cfg.DSN, storage.Options{
		MaxOpenConns:    cfg.MaxOpenConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
	})
	if err != nil {
		log.Warn().Err(err).Msg("init db failed; disabling db-dependent routes")
		return nil
	}
	return db
}

func initAuthHandler(conf *config.Config) *httpserver.AuthHandler {
	mgr := auth.NewProviderManager()
	mgr.Register("guest", auth.NewGuestProvider())
	if conf.Auth.Gmail.ClientID != "" {
		mgr.Register("gmail", auth.NewGmailProvider(conf.Auth.Gmail.ClientID))
	}
	if conf.Auth.Apple.ClientID != "" {
		mgr.Register("apple", auth.NewAppleProvider(conf.Auth.Apple))
	}
	authSvc := auth.NewAuthService(mgr, nil)
	return httpserver.NewAuthHandler(authSvc)
}

func startGRPCServer(port int) (func(context.Context) error, error) {
	addr := fmt.Sprintf(":%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	baseLogger := log.Logger.With().Str("module", "grpc").Logger()
	srv := grpcserver.NewServer(&baseLogger)
	grpcserver.RegisterServices(srv, grpcserver.Deps{Logger: &baseLogger})

	go func() {
		log.Info().Str("addr", addr).Msg("Starting gRPC server")
		if err := srv.Serve(lis); err != nil {
			log.Error().Err(err).Msg("gRPC server stopped with error")
		}
	}()

	return func(ctx context.Context) error {
		ch := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(ch)
		}()

		select {
		case <-ch:
		case <-ctx.Done():
			srv.Stop()
		}
		return lis.Close()
	}, nil
}

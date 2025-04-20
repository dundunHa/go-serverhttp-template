package main

import (
	"fmt"
	"net/http"

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
	router.Register(r)

	// 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", conf.Server.Port)
	log.Info().Str("addr", addr).Msg("Starting HTTP server")
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal().Err(err).Msg("Server failed to start")
	}
}

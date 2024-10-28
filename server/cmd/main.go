package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi"

	"go-serverhttp-template/server/api"
	"go-serverhttp-template/server/config"
)

func main() {
	conf := config.LoadConfig()

	r := chi.NewRouter()
	api.Register(r)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", conf.Server.Port), r); err != nil {
		log.Println("Server failed to start:", err)
	}
}

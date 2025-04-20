package api

import (
	"net/http"

	"github.com/rs/zerolog"
)

func GetHello(w http.ResponseWriter, r *http.Request) {
	logger := zerolog.Ctx(r.Context())
	logger.Info().Msg("Handling Hello API")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Hello, World!")); err != nil {
		logger.Error().Err(err).Msg("Write response failed")
	} else {
		logger.Info().Msg("Hello, World!")
	}
}

func GetHelloHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zerolog.Ctx(r.Context())
		logger.Info().Msg("Handling Hello API")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("Hello, World!")); err != nil {
			logger.Error().Err(err).Msg("Write response failed")
		} else {
			logger.Info().Msg("Hello, World!")
		}
	}
}

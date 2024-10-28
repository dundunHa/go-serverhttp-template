package httpserver

import (
	"net/http"

	"github.com/rs/zerolog"
)

func GetHelloHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zerolog.Ctx(r.Context())
		logger.Info().Msg("Handling Hello API")
		WriteSuccess(w, map[string]string{"message": "Hello, World!"})
	}
}

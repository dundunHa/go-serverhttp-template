package httpserver

import (
	"net/http"
)

func WriteSuccess(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "ok",
		"data":    data,
	})
}

func WriteError(w http.ResponseWriter, err *APIError) {
	WriteJSON(w, http.StatusOK, err)
}

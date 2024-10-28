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
	status := http.StatusInternalServerError
	if err != nil && err.Code >= 400 && err.Code < 600 {
		status = err.Code
	}
	WriteJSON(w, status, err)
}

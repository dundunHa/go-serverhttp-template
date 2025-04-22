package httpserver

import (
	"encoding/json"
	"log"
	"net/http"
)

// WriteJSON writes the status code and encodes data as JSON to ResponseWriter
// If encoding fails, it logs the error since the header might have been written
func WriteJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("WriteJSON encode failed: %v", err)
	}
}

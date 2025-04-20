package common

import (
	"encoding/json"
	"log"
	"net/http"
)

func JSON(w http.ResponseWriter, code int, data interface{}, msg string) {
	var resp struct {
		Msg  string      `json:"msg"`
		Code int         `json:"code"`
		Data interface{} `json:"data"`
	}
	resp.Code = code
	resp.Data = data
	resp.Msg = msg
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(resp)
	if err != nil {
		log.Println("Wire Response Error", err.Error())
	}
}

func Success(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, data, "")
}

func Error(w http.ResponseWriter, err error) {
	JSON(w, http.StatusInternalServerError, nil, err.Error())
}

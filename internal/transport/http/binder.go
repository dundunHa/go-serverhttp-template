package httpserver

import (
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

type APIError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// BindRequest 自动绑定 path/query/body 并校验
func BindRequest(r *http.Request, dest interface{}) *APIError {
	destVal := reflect.ValueOf(dest).Elem()
	destType := destVal.Type()
	// 1. path 参数
	for i := 0; i < destType.NumField(); i++ {
		field := destType.Field(i)
		if name := field.Tag.Get("uri"); name != "" {
			if val := chi.URLParam(r, name); val != "" {
				destVal.Field(i).SetString(val)
			}
		}
	}
	// 2. query 参数
	if err := parseQuery(r, dest); err != nil {
		return &APIError{Code: 400, Message: err.Error()}
	}
	// 3. body
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
			return &APIError{Code: 400, Message: "invalid JSON body"}
		}
	}
	// 4. validator 校验
	if err := validate.Struct(dest); err != nil {
		return &APIError{Code: 400, Message: err.Error()}
	}
	return nil
}

// parseQuery 解析 query 参数到 struct（简单实现，仅支持 string 字段）
func parseQuery(r *http.Request, dest interface{}) error {
	values := r.URL.Query()
	destVal := reflect.ValueOf(dest).Elem()
	destType := destVal.Type()
	for i := 0; i < destType.NumField(); i++ {
		field := destType.Field(i)
		if name := field.Tag.Get("query"); name != "" {
			if val := values.Get(name); val != "" {
				destVal.Field(i).SetString(val)
			}
		}
	}
	return nil
}

package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

type APIError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func setFieldFromString(v reflect.Value, raw string) error {
	if !v.CanSet() {
		return nil
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(i)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(u)
		return nil
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		v.SetBool(b)
		return nil
	default:
		return fmt.Errorf("unsupported field kind %s", v.Kind())
	}
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
				if err := setFieldFromString(destVal.Field(i), val); err != nil {
					return &APIError{Code: 400, Message: fmt.Sprintf("invalid uri param %q", name)}
				}
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

// parseQuery 解析 query 参数到 struct（支持 string/int/uint/bool）
func parseQuery(r *http.Request, dest interface{}) error {
	values := r.URL.Query()
	destVal := reflect.ValueOf(dest).Elem()
	destType := destVal.Type()
	for i := 0; i < destType.NumField(); i++ {
		field := destType.Field(i)
		if name := field.Tag.Get("query"); name != "" {
			if val := values.Get(name); val != "" {
				if err := setFieldFromString(destVal.Field(i), val); err != nil {
					return fmt.Errorf("invalid query param %q", name)
				}
			}
		}
	}
	return nil
}

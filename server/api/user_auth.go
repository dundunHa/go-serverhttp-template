package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"go-serverhttp-template/server/constants"
)

func BasicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" || strings.HasPrefix(r.URL.Path, "/internal") {
			next.ServeHTTP(w, r)
			return
		}
		userID := r.URL.Query().Get("user_id")
		var bodyByte []byte
		var err error
		if userID == "" {
			var authInfo struct {
				UserID string `json:"user_id"`
			}
			bodyByte, err = io.ReadAll(r.Body)
			if err != nil {
				Error(w, constants.ErrRequestFailed)
				return
			}
			err = json.Unmarshal(bodyByte, &authInfo)
			if err != nil {
				Error(w, constants.ErrParamsInvalid)
				return
			}
			userID = authInfo.UserID
		}

		err = checkUser()
		if err != nil {
			Error(w, constants.ErrAuthenticationFailed)
			return
		}
		// 将请求体重新放回 r.Body 中供后续使用
		r.Body = io.NopCloser(bytes.NewBuffer(bodyByte))
		next.ServeHTTP(w, r)
	})
}

func checkUser() error {
	return nil
}

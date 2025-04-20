package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"go-serverhttp-template/internal/common"
	"go-serverhttp-template/internal/constants"
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
				common.Error(w, constants.ErrRequestFailed)
				return
			}
			err = json.Unmarshal(bodyByte, &authInfo)
			if err != nil {
				common.Error(w, constants.ErrParamsInvalid)
				return
			}
			userID = authInfo.UserID
		}

		err = checkUser(userID)
		if err != nil {
			common.Error(w, constants.ErrAuthenticationFailed)
			return
		}
		if bodyByte != nil {
			r.Body = io.NopCloser(bytes.NewBuffer(bodyByte))
		}
		next.ServeHTTP(w, r)
	})
}

// TODO: 检查用户是否存在
func checkUser(userID string) error {
	return nil
}

package constants

import "errors"

var (
	ErrRequestFailed        = errors.New("request failed")
	ErrParamsInvalid        = errors.New("params invalid")
	ErrAuthenticationFailed = errors.New("authentication failed")
)

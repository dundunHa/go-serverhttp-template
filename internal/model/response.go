package model

type Response[T any] struct {
	Data T `json:"data"`
}

type Message struct {
	Message string `json:"message"`
}

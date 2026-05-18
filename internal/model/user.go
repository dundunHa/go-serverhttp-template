package model

type User struct {
	ID   int    `json:"id" example:"1"`
	Name string `json:"name" example:"Ada"`
}

type AuthRequest struct {
	Token string `json:"token" doc:"Provider token or guest device ID"`
}

type UserInfo struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Provider string `json:"provider"`
}

package model

type User struct {
	ID   int    `json:"id" example:"1"`
	Name string `json:"name" example:"Ada"`
}

type AuthRequest struct {
	Token string `json:"token" doc:"Provider token or guest device ID"`
}

type AuthResponse struct {
	AccessToken string   `json:"access_token"`
	TokenType   string   `json:"token_type" example:"Bearer"`
	ExpiresIn   int64    `json:"expires_in" doc:"Access token lifetime in seconds"`
	User        UserInfo `json:"user"`
}

type AuthIdentity struct {
	Provider string
	Subject  string
	Email    string
}

type UserInfo struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	Provider        string `json:"provider"`
	ProviderSubject string `json:"-"`
}

package httpserver

type AuthRequest struct {
	Provider string `json:"provider"`
	Token    string `json:"token"`
}

type AuthResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

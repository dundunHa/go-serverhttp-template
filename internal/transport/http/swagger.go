package httpserver

import (
	"net/http"
)

// MountSwagger 挂载 Swagger 文档路由（实际项目中可用 swaggo/http-swagger）
func MountSwagger(mux http.Handler) {
	// 示例：实际可用 httpSwagger.Handler
	// r.Get("/swagger/*", httpSwagger.Handler(...))
}

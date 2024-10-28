package grpcserver

import (
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// Deps 汇总 gRPC 层需要的依赖，便于统一注入与测试。
// 目前模板不包含任何 proto/service，实现方可按需扩展。
type Deps struct {
	Logger *zerolog.Logger
}

// RegisterServices 收敛所有 gRPC 服务注册逻辑（pb.RegisterXxxServer）。
// 作为脚手架扩展点：当前不注册任何实际服务。
func RegisterServices(reg grpc.ServiceRegistrar, deps Deps) {
	_ = reg
	_ = deps
}

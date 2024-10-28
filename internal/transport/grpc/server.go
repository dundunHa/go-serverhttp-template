package grpcserver

import (
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// NewServer 创建 gRPC Server 实例。
// 注意：当前脚手架不内置 proto/service，仅提供启动与注册骨架。
func NewServer(logger *zerolog.Logger) *grpc.Server {
	_ = logger
	return grpc.NewServer()
}

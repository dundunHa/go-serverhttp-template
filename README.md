## go-serverhttp-template

面向新 Go 服务的脚手架：默认提供分层结构、HTTP 服务骨架，并预留 gRPC 扩展点（当前不包含任何 proto/service）。

## 快速开始

- 使用示例配置（可选）：`cp configs/config.toml.example configs/config.toml`
- 仅 HTTP（默认）：`go run ./cmd/server`
- 启用 gRPC（无实际服务，仅启动监听）：`APP_MODE=grpc APP_GRPC_PORT=9090 go run ./cmd/server`
- 同时启用：`APP_MODE=both go run ./cmd/server`

### 配置优先级

1. 环境变量（推荐；默认前缀 `APP_`，可通过 `CONFIG_PREFIX` 自定义，置空代表无前缀）
2. TOML 配置文件（默认读取 `configs/config.toml`；可用 `CONFIG_FILE` 指定路径）

示例：
- `APP_HTTP_PORT=8080`
- `APP_DB_DSN="postgres://user:pass@localhost:5432/db?sslmode=disable"`
- `APP_DB_MAX_OPEN_CONNS=10`
- `APP_HTTP_LOG_BODY=true`（默认 `false`）

缺少 DB/Apple/Gmail 等配置时，服务会在启动时给出 warning，并禁用相关能力（不阻塞启动）。

## 目录结构

```
.
├── .cursor/           # 脚手架相关配置及规则
│   └── rules/         # 规则文件目录
├── cmd/               # 应用入口
│   └── server/        # 服务入口
│       └── main.go
├── internal/          # 私有应用代码
│   ├── transport/     # 传输层
│   │   └── http/      # HTTP 服务相关
│   │   └── grpc/      # gRPC 服务骨架（仅注册入口，无 proto）
│   ├── storage/       # 数据持久层
│   ├── config/        # 配置加载
│   ├── router/        # 路由定义
│   ├── model/         # 领域模型/DTO
│   └── service/       # 业务逻辑层
├── pkg/               # 对外暴露的公共库
├── Dockerfile         # Docker 镜像构建脚本
├── .golangci.yml      # Lint 配置文件
├── go.mod             # Go 模块定义
├── go.sum             # Go 模块校验
├── README.md          # 项目说明
├── LICENSE            # 许可证文件
└── .gitignore         # Git 忽略规则
```

# Go HTTP Template

轻量 Go HTTP 脚手架，默认使用 `chi + Huma`：`chi` 负责路由和中间件，`Huma` 负责类型化 handler、请求校验和 OpenAPI/Docs 自动生成。

## 目录结构

```
.
├── .cursor/           # 脚手架相关配置及规则
│   └── rules/         # 规则文件目录
├── cmd/               # 应用入口
│   └── server/        # 服务入口
│       └── main.go
├── internal/          # 私有应用代码
│   ├── config/        # 配置加载
│   ├── api/           # Huma API 注册和 HTTP contract
│   ├── model/         # 领域模型/DTO
│   ├── service/       # 业务逻辑层
│   ├── dao/           # 数据访问
│   └── storage/       # 数据库连接
├── pkg/               # 对外暴露的公共库
├── Dockerfile         # Docker 镜像构建脚本
├── .golangci.yml      # Lint 配置文件
├── go.mod             # Go 模块定义
├── go.sum             # Go 模块校验
├── README.md          # 项目说明
├── LICENSE            # 许可证文件
└── .gitignore         # Git 忽略规则
``` 

## 新增 API 的默认路径

优先按这条简单路径扩展：

```
request
  -> internal/api       # 路由、输入校验、HTTP status、response mapping
  -> internal/service   # 业务逻辑
  -> internal/model     # 请求/响应/业务 DTO
  -> internal/dao       # 需要数据库时再进入数据访问
```

不要为普通 CRUD 提前拆 `domain/app/transport` 多层目录。只有出现跨协议复用、复杂事务、状态机或多个 worker 共享业务逻辑时，再提升结构。

特殊 HTTP 能力也优先放在 `internal/api` 下。不要再新增 `internal/transport/http`。

## API 文档

Huma 自动生成 OpenAPI 和交互文档：

- `GET /openapi.json`
- `GET /openapi.yaml`
- `GET /docs`

`openapi.json` 不手写维护。需要给前端或 SDK 使用时，在 CI 里从运行时代码导出或比对快照。

## 本地运行

```bash
go test ./...
go run ./cmd/server
```

默认不要求数据库。未设置 `DB_DSN` 时，`/users/1` 使用内存 demo 数据，认证身份也映射到同一套内存用户服务，方便本地验证和 agent 开发。这个内存映射只适合 demo；生产环境应替换为持久化的 provider identity 到系统 user id 映射。

默认认证流程：

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/auth/guest \
  -H 'Content-Type: application/json' \
  -d '{"token":"device-1"}' | jq -r '.data.access_token')

curl http://localhost:8080/users/1 -H "Authorization: Bearer $TOKEN"
```

`/auth/{provider}` 响应里的 `user.id` 是系统内 user id，不是 Gmail/Apple subject 或 guest device ID。受保护的 `/users/{id}` 会要求 bearer token 的系统 user id 与 path id 一致。

常用环境变量：

```bash
APP_ENV=dev
SERVER_PORT=8080
DB_DSN=postgres://user:pass@localhost:5432/app?sslmode=disable
LOG_LEVEL=info
AUTH_GMAIL_CLIENT_ID=
AUTH_APPLE_CLIENT_ID=
AUTH_JWT_SECRET=dev-secret-change-me
AUTH_JWT_ISSUER=go-serverhttp-template
AUTH_JWT_AUDIENCE=go-serverhttp-template-api
AUTH_JWT_ACCESS_TOKEN_TTL=15m
```

日志使用 Go 标准库 `log/slog`。`APP_ENV=dev` 时以 text 格式输出到控制台，`APP_ENV=prod` 时以 JSON 格式输出到控制台。

# go-serverhttp-template

## 日志设计规范

### 1. 日志上下文与依赖注入
- 所有业务代码（Handler、Service、Repo 等）都通过依赖注入方式接收 `*zerolog.Logger`，避免直接使用全局 log。
- 在 main.go 初始化全局 logger，并为每个模块分配独立字段（如 module/component），逐层传递。

### 2. Handler 层注入示例
```go
func Register(r *chi.Mux, logger *zerolog.Logger) {
    helloLogger := logger.With().Str("component", "hello").Logger()
    r.Get("/status", api.GetHelloHandler(&helloLogger))
}

// api/hello.go
func GetHelloHandler(logger *zerolog.Logger) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        logger.Info().Msg("Handling Hello API")
        // ...
    }
}
```

### 3. Service 层注入示例
```go
type UserService struct {
    logger *zerolog.Logger
}
func NewUserService(logger *zerolog.Logger) *UserService {
    return &UserService{logger: logger}
}
```

### 4. 测试友好
- 单元测试时可传入 mock logger 或 zerolog.Testing，便于断言日志内容。

### 5. 日志字段规范
- 统一添加 trace_id、user_id、module、component 等业务字段，便于日志检索与追踪。
- 日志输出格式、等级、路径等通过配置文件或环境变量管理。

### 6. 推荐实践
- 通过中间件将 logger 注入到 context，必要时可用 `zerolog.Ctx(r.Context())` 获取。
- 禁止在业务代码中直接使用 `log.Logger` 全局变量。
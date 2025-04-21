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
│   ├── storage/       # 数据持久层
│   ├── constants/     # 常量定义
│   ├── config/        # 配置加载
│   ├── api/           # API 定义
│   ├── common/        # 通用组件
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
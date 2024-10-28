# 通用用户认证模块技术方案

## 1. 目标与定位
本模块为通用用户认证器，仅支持 Gmail、Apple、Guest 三种认证方式，返回基础用户信息（ID、Email、Provider），不涉及持久化、刷新 token、用户管理等复杂逻辑。

## 2. 目录结构
```
internal/service/auth/
├── provider.go         # AuthProvider 接口与 UserInfo 定义
├── gmail_provider.go   # GmailProvider 实现
├── apple_provider.go   # AppleProvider 实现
├── guest_provider.go   # GuestProvider 实现
├── manager.go          # ProviderManager 注册/获取 provider
├── auth_service.go     # AuthService 统一认证入口
```

## 3. 配置管理
- 配置优先级：环境变量（推荐，统一前缀 `APP_`）> TOML 配置文件。
- 推荐环境变量：
  - `APP_AUTH_GMAIL_CLIENT_ID`
  - `APP_AUTH_APPLE_CLIENT_ID`
- 兼容旧环境变量（会在启动期 warning）：
  - `GMAIL_CLIENT_ID`
  - `SERVER_AUTH_APPLE_CLIENT_ID`

## 4. 核心接口
- `AuthProvider`：统一认证接口，`VerifyToken(ctx, token) (*UserInfo, error)`
- `UserInfo`：只包含 `ID`、`Email`、`Provider` 字段
- `ProviderManager`：注册/获取 provider
- `AuthService`：统一认证入口，`Verify(ctx, provider, token)`

## 5. Provider 实现
- Gmail：调用 Google tokeninfo API 校验 id_token
- Apple：解码 id_token payload 并校验 aud 字段（实际应校验签名，简化处理）
- Guest：直接将 token 作为用户 ID 返回

## 6. HTTP 层
- 新增 `internal/transport/http/auth_handler.go`、`auth_types.go`
- 路由：`POST /auth/{provider}`，body: `{ "token": "..." }`
- 响应：
  ```json
  { "code": 0, "message": "ok", "data": { "id": "...", "email": "...", "provider": "..." } }
  ```
- 错误响应遵循统一格式

## 7. 日志
- 关键流程可用 zap 结构化日志（如需扩展）

## 8. 测试
- Provider、Service 层可用 table-driven 单元测试
- HTTP 层可用 httptest

## 9. 其他
- 删除所有冗余 provider/types/manager/init 相关旧代码
- 保持最小可用实现，便于后续扩展

---

本方案已完全落地于当前代码结构，后续如需扩展其他认证方式，仅需实现 AuthProvider 并注册到 ProviderManager。 

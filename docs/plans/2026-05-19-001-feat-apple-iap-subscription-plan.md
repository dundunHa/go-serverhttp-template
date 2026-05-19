---
title: "feat: Apple IAP subscription marker"
type: feat
status: completed
date: 2026-05-19
origin:
  - ~/.gstack/projects/go-serverhttp-template/lxp-main-design-20260518-203148-apple-iap.md
  - ~/.gstack/projects/go-serverhttp-template/lxp-main-eng-review-test-plan-20260518-203148.md
review:
  skill: compound-engineering:ce-doc-review
  mode: fixed-plan
---

# feat: Apple IAP subscription marker

## Summary

为 `go-serverhttp-template` 增加 Apple In-App Purchase 自动续订订阅处理。首版只交付“订阅标识”和 `/users/me` 订阅状态，不做 credits、wallet、ledger、order table 或多支付 provider。

本计划修复了原工程规划中的高风险前提错误：Apple `appAccountToken` 是 UUID，不是本服务的 int64 user id；sandbox 交易不能在生产授予权益；`/users/me` 不能只查 `ACTIVE` 行；webhook public endpoint 需要资源边界和日志脱敏；真实 Postgres 集成测试不能污染默认 `go test ./...`。

首版落地目标：

- 登录用户可以获取服务端生成的 Apple `appAccountToken` UUID，并在 iOS StoreKit purchase option 中使用。
- 登录用户调用 `POST /payment/apple/verify` 后，服务端校验 Apple transaction，写入或更新 `apple_subscriptions`。
- Apple Server Notifications V2 调用 `POST /webhooks/apple` 后，服务端校验 JWS、按 `notification_uuid` 幂等处理，并更新订阅状态。
- `GET /users/me` 返回 provider-neutral 的 `SubscriptionInfo`，且保留 `Credits.Balance = 0` 的现有契约。

## Review Fixes Applied

| Finding | Fix in this plan |
| --- | --- |
| `appAccountToken` 被当成 int64 user id | 新增 `apple_account_tokens` 表和 `GET /payment/apple/account-token`，服务端生成 UUID，verify/webhook 只通过 UUID 映射 user_id |
| sandbox 可激活生产权益 | 新增 `APPLE_IAP_ENTITLEMENT_ENVIRONMENTS` 与 `APPLE_IAP_ENABLE_SANDBOX_FALLBACK`，`/users/me` 必须按 allowed environments 过滤 |
| `/users/me` 过期状态和 CANCELED/grace 语义不一致 | 明确 entitlement 计算：`ACTIVE/CANCELED` 且有效期未过仍有权益；无订阅为 `NONE`，终止/过期/REVOKED 映射为 `EXPIRED` |
| 公共 webhook 缺少滥用边界 | webhook handler 先做 body size、`signedPayload` 长度和 JWS 基础格式快筛，再进入证书/JWS 验证 |
| `signedPayload` 会进请求日志 | 新增日志脱敏实施单元，扩展 `isSensitiveField`，覆盖 IAP payload/key/token 字段 |
| raw JWS 留存无边界 | 默认不保存完整 raw JWS，只保存 `raw_jws_sha256` 和最小 decoded payload；完整 raw payload 仅本地调试开关，生产禁用 |
| service 单元过厚 | 拆成 C1 verify/read path 和 C2 webhook/status/reconcile path |
| 默认测试拖入真实 Postgres | 集成/E2E 测试使用 `integration` build tag + `INTEGRATION_DB_DSN`；默认 `go test ./...` 只跑 unit/httptest |
| migration 没有安装路径 | 增加 `make migrate` 或等价顺序执行 `db/migrations/*.sql`，更新 README 和 smoke 前置条件 |
| `slog.FromContext` 不存在 | 使用现有 `logpkg.FromContext(ctx)` |
| webhook retry 不是完整恢复机制 | 增加最小 reconciliation/backfill 能力和 runbook，不能把未实现 metric/alert 写成已闭环 |

## Goal

添加 Apple IAP 自动续订订阅能力，输出仅限：

- 创建或更新 `apple_subscriptions` 订阅状态行。
- 插入 `apple_events` 幂等事件行。
- 创建或读取 `apple_account_tokens` 用户到 Apple account token UUID 的映射。
- 让 `/users/me` 从订阅读侧计算 `SubscriptionInfo`。

## Hard Constraints

- 不引入 credits、wallet、ledger、order table。
- 不支持 consumable products；只处理 auto-renewable subscription。
- 不让 LLM、客户端或 Apple payload 直接写正式 user 权益；权益由服务端校验后计算。
- 继续使用现有栈：`chi + huma/v2`、`pgx/v5 + sqlc`、`envconfig`、`log/slog`、`api -> service -> dao -> db`。
- 继续使用统一响应 envelope：`model.Response[T]{code, data, msg}`。
- `POST /payment/apple/verify` 必须复用现有 bearer JWT auth。
- `POST /webhooks/apple` 是 public endpoint，但必须通过 Apple JWS 校验、bundle/environment 校验、body limit 和日志脱敏保护。
- 默认测试命令 `go test ./...` 不能依赖本地 Postgres。

## Out of Scope

| Deferred | Rationale |
| --- | --- |
| Credits / wallet / ledger | 用户明确排除；`MeData.Credits.Balance` 保持 0 |
| Consumable IAP | 本期只做 auto-renewable subscriptions |
| Product catalog 管理后台 | 首版静态 env JSON 足够；后续再做 DB-backed product catalog |
| 多 provider：Creem / PayPal | 未来 provider 复用 provider-neutral entitlement contract，但不在本 PR 实现 |
| Subscription admin UI | 本期只提供最小 runbook / reconciliation 能力 |
| Legacy `verifyReceipt` | StoreKit 2 + App Store Server API 覆盖首版需求 |
| Docker 镜像结构调整 | 仍只有当前 server 构建；不新增部署 artifact |

## Current Code Facts

已核对目标仓库 `/Users/lxp/Library/CloudStorage/Dropbox/code-space/mygitspace/go-serverhttp-template`：

- `internal/api/user.go` 的 `UserDeps` 当前只有 `Users` 和 `Auth`，`loadCurrentUser` 只返回 zero `SubscriptionInfo`。
- `cmd/server/main.go` 当前只构造 `userSvc` 和 `authSvc`，只调用 `api.RegisterUserRoutes`。
- `internal/api/middleware.go` 会记录 POST request body，当前敏感字段只覆盖 `token/access_token/refresh_token/id_token/password/secret`。
- `pkg/log/log.go` 的上下文 logger 是 `logpkg.FromContext(ctx)`，不是 `slog.FromContext(ctx)`。
- `sqlc.yaml` 使用 `db/migrations` 和 `db/queries` 生成到 `internal/db`，`Makefile` 有 `make sqlc`，没有 `make migrate`。
- `.github/workflows/ci.yml` 当前只跑 `go test ./...` 和 `go build`，没有 Postgres service。
- README 的首次运行只执行 `db/migrations/001_init.sql`。

## External Apple Facts

- `appAccountToken` 是 app/server 创建并传给 StoreKit 的 UUID；Apple 在 transaction / renewal info 里回传同一个 UUID。它不是本服务 int64 user id。
- App Store Server Notifications V2 失败后 Apple 会重试，但重试不是完整恢复机制；生产需要 Notification History / Subscription Status reconciliation 路径覆盖 missed webhook。
- `Set App Account Token` 可以为部分 App Store 外购买补 app account token，但 Family Sharing transaction 不支持该补绑端点。

References:

- [Apple App Store Server API appAccountToken](https://developer.apple.com/documentation/appstoreserverapi/appaccounttoken)
- [Apple Set App Account Token](https://developer.apple.com/documentation/appstoreserverapi/set-app-account-token)
- [Apple Responding to App Store Server Notifications](https://developer.apple.com/documentation/appstoreservernotifications/responding-to-app-store-server-notifications)
- [Apple Get Notification History](https://developer.apple.com/documentation/appstoreserverapi/get-notification-history)

## Requirements

- R1. 服务端必须为每个用户生成稳定 Apple `appAccountToken` UUID，并通过 authenticated endpoint 返回给 iOS。
- R2. iOS 发起 purchase 前必须使用服务端返回的 UUID 作为 StoreKit `appAccountToken`。
- R3. verify/webhook 必须强制要求 Apple transaction 里存在 `appAccountToken`，并只通过 UUID 映射本服务 `user_id`。
- R4. 空 `appAccountToken` 必须返回 typed error；不能自动设置为 user id。
- R5. `original_transaction_id + environment` 是 Apple subscription natural key。
- R6. 同一 `original_transaction_id + environment` 已绑定用户 A 时，用户 B verify 必须返回 409 ownership conflict；webhook 遇到冲突记录事件并 200 ack。
- R7. production entitlement 不能由 sandbox transaction 授予；`/users/me` 必须按 allowed entitlement environments 过滤。
- R8. verify 的 sandbox fallback 必须可配置；生产默认关闭，dev/staging 可开启。
- R9. `SubscriptionInfo.ProductID` 对外返回服务端规范化 plan id，不返回 Apple raw product id。
- R10. `SubscribeLevel` 是 provider-neutral 权益优先级；`/users/me` 在多条有效订阅中返回最高 level，同 level 返回有效期更晚的订阅。
- R11. `CANCELED` 表示 auto-renew off，但有效期未过仍有权益；`REVOKED` 在 API 映射为 `EXPIRED`。
- R12. webhook 事件插入和 subscription 更新必须在同一个 DB transaction 内完成。
- R13. webhook 必须按 `notification_uuid` 幂等；重复投递不重复执行业务逻辑。
- R14. webhook 必须定义乱序归约键，不能只依赖 `notification_uuid`。
- R15. webhook public body 必须有 size limit、JWS 格式快筛和日志脱敏。
- R16. 默认不保存完整 raw JWS；只保存 hash 和最小 decoded fields。生产禁用完整 raw payload 留存。
- R17. `.p8` 私钥只能通过生产 secret/KMS 注入，不能进入 README 示例、日志或仓库。
- R18. `ErrNotConfigured` 判定必须覆盖 bundle_id、issuer_id、key_id、private_key/p8_path、products、entitlement environments。
- R19. 集成测试必须有 `integration` build tag 或 `INTEGRATION_DB_DSN` gate；默认 `go test ./...` 不依赖 Postgres。
- R20. missed webhook 必须有最小 reconciliation/backfill runbook 和代码入口，不能只依赖 Apple retry。

## Public Contract

### `GET /payment/apple/account-token` (auth: bearer)

返回当前用户的 Apple `appAccountToken` UUID。iOS 客户端必须在 StoreKit purchase option 中使用这个 UUID。

Response:

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "app_account_token": "9d8a0f8e-1c4f-46b0-9d93-b3b84f0f9e61"
  }
}
```

Rules:

- 同一用户多次调用返回同一个 UUID。
- UUID 由服务端生成并持久化；客户端不能提交自选 UUID 进行绑定。
- token 是用户和 Apple transaction 的关联键，不是认证凭证；仍必须通过 bearer auth 访问该 endpoint。

### `POST /payment/apple/verify` (auth: bearer)

Request:

```json
{ "transaction_id": "200000123456789" }
```

Response:

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "subscription_info": {
      "product_id": "pro_monthly",
      "status": "ACTIVE",
      "subscribe_expired_time": "2026-06-18T20:00:00Z",
      "subscribe_level": 1
    }
  }
}
```

Errors:

- `400`: empty transaction id, missing appAccountToken, token mismatch, bundle mismatch, unsupported product, non-subscription product, revoked transaction.
- `401`: missing/bad bearer token.
- `409`: original transaction already belongs to another user.
- `503`: IAP service not configured.
- `500`: Apple API or DB internal failure.

### `POST /webhooks/apple` (public, JWS validated)

Request:

```json
{ "signedPayload": "eyJhbGciOiJF..." }
```

Response:

```json
{ "code": 200, "msg": "success", "data": { "success": true, "message": "ok" } }
```

Error / ack policy:

- Invalid JSON / oversized body / malformed signedPayload: `400`.
- JWS verify failed or bundle mismatch: `401`.
- Service not configured / transient DB / Apple API dependency failure: `500`, so Apple retries.
- Valid Apple event but no known `appAccountToken` mapping: insert `apple_events` with `processing_status='PENDING_USER_BINDING'`, return `200`, and recover through authenticated verify or reconciliation.
- Duplicate `notification_uuid`: return `200` with no business mutation.
- Ownership conflict: record event as `OWNERSHIP_CONFLICT`, structured log with safe fields, return `200`.

### `GET /users/me` (existing)

`SubscriptionInfo` remains the public contract:

```go
type SubscriptionInfo struct {
    ProductID            string `json:"product_id"`
    Status               string `json:"status"` // ACTIVE, EXPIRED, CANCELED, NONE
    SubscribeExpiredTime string `json:"subscribe_expired_time"`
    SubscribeLevel       int    `json:"subscribe_level"`
}
```

Contract:

- `product_id` returns canonical service plan id from `APPLE_IAP_PRODUCTS[*].plan_id`, not Apple raw SKU.
- `subscribe_level` is provider-neutral entitlement priority.
- `Status="ACTIVE"` means entitlement active and auto-renew not known off.
- `Status="CANCELED"` means auto-renew is off but entitlement remains active until `subscribe_expired_time`.
- `Status="EXPIRED"` means no current entitlement, including DB `EXPIRED` and `REVOKED`.
- `Status="NONE"` means the user has no subscription row at all.
- Existing `Credits.Balance` remains `0`.

## Architecture

```
internal/
├── api/
│   ├── payment.go              # Huma routes: account-token, verify, webhook
│   ├── payment_test.go
│   ├── middleware.go           # redact IAP sensitive fields
│   └── user.go                 # /users/me reads SubscriptionReader
├── service/
│   ├── payment/
│   │   ├── apple_iap.go        # verify path orchestrator
│   │   ├── apple_webhook.go    # webhook dispatch + reducer
│   │   ├── apple_reconcile.go  # notification history / status reconciliation
│   │   ├── apple_verifier.go   # App Store Server API interface + gopay impl
│   │   ├── catalog.go          # static plan catalog
│   │   ├── token.go            # appAccountToken service
│   │   ├── errors.go
│   │   └── *_test.go
│   └── user_service.go
├── dao/
│   ├── subscription.go         # SubscriptionDAO + AppleEventDAO + AccountTokenDAO
│   └── subscription_test.go    # integration-gated
├── db/
│   └── sqlc generated
└── model/
    ├── subscription.go
    └── user.go                 # update SubscriptionInfo doc strings only
```

### Verify Data Flow

```
iOS client
  ├─ GET /payment/apple/account-token (Bearer)
  │    -> service creates/loads user UUID token
  │
  ├─ StoreKit purchase(.appAccountToken(uuid))
  │
  └─ POST /payment/apple/verify {transaction_id} (Bearer)
       -> auth user id
       -> App Store Server API GetTransactionInfo
       -> decode signed transaction
       -> check bundle_id, environment, product catalog, appAccountToken
       -> token UUID maps to same user id
       -> upsert apple_subscriptions
       -> return SubscriptionInfo
```

### Webhook Data Flow

```
Apple ASSN V2
  -> POST /webhooks/apple {signedPayload}
      -> body limit + JWS compact-shape check
      -> DecodeSignedPayload / x5c + signature validation
      -> bundle/environment/catalog checks
      -> insert apple_events(notification_uuid) in tx
         -> duplicate: commit + 200
      -> decode transaction + appAccountToken
      -> token UUID maps user_id?
         -> yes: reduce event into apple_subscriptions in same tx
         -> no: mark PENDING_USER_BINDING, commit + 200
      -> return 200
```

## Entitlement Model

### Provider-neutral catalog

`APPLE_IAP_PRODUCTS` stores provider SKU and canonical service plan id:

```json
[
  {
    "plan_id": "pro_monthly",
    "product_id": "com.app.pro.monthly",
    "level": 1,
    "environment": "Production",
    "subscription_group_id": "21456789"
  }
]
```

Rules:

- `(product_id, environment)` must be unique.
- `plan_id` is returned to `/users/me`.
- `level` drives entitlement priority across future providers.
- Production config must not include sandbox product rows in `APPLE_IAP_ENTITLEMENT_ENVIRONMENTS`.

### DB status vs API status

DB status:

```text
ACTIVE | CANCELED | EXPIRED | REVOKED
```

API status:

```text
ACTIVE | CANCELED | EXPIRED | NONE
```

Mapping:

| DB state | Time condition | API status | Entitlement |
| --- | --- | --- | --- |
| no row | n/a | `NONE` | no |
| `ACTIVE` | effective end > now | `ACTIVE` | yes |
| `CANCELED` | effective end > now | `CANCELED` | yes until end |
| `ACTIVE` / `CANCELED` | effective end <= now | `EXPIRED` | no |
| `EXPIRED` | any | `EXPIRED` | no |
| `REVOKED` | any | `EXPIRED` | no |

`effective end = max(current_period_end, grace_period_expires_at)` when grace exists.

### `/users/me` selection rule

1. Consider only rows whose `environment` is in `APPLE_IAP_ENTITLEMENT_ENVIRONMENTS`.
2. If any row has active entitlement, return:
   - highest `level`;
   - tie-breaker: latest `effective end`;
   - tie-breaker: latest `last_event_at`.
3. If no row has active entitlement but any subscription row exists, return latest terminal row as `EXPIRED`.
4. If no row exists, return `NONE`.

## Configuration

New env vars in `internal/config/config.go`:

```bash
APPLE_IAP_BUNDLE_ID=
APPLE_IAP_ISSUER_ID=
APPLE_IAP_KEY_ID=
APPLE_IAP_PRIVATE_KEY=
APPLE_IAP_P8_PATH=
APPLE_IAP_PRODUCTS='[{"plan_id":"pro_monthly","product_id":"com.app.pro.monthly","level":1,"environment":"Production"}]'
APPLE_IAP_ENTITLEMENT_ENVIRONMENTS=Production
APPLE_IAP_ENABLE_SANDBOX_FALLBACK=false
APPLE_IAP_WEBHOOK_MAX_BODY_BYTES=65536
APPLE_IAP_STORE_RAW_PAYLOADS=false
APPLE_IAP_APPLE_API_TIMEOUT=10s
```

Not configured if any of these are missing or invalid:

- `bundle_id`
- `issuer_id`
- `key_id`
- `private_key` or `p8_path`
- at least one valid product entry
- at least one entitlement environment

Route behavior when not configured:

- verify/account-token: `503`.
- webhook: `500` so Apple retries after transient deploy/config issue.

Key management:

- Production must inject `.p8` through secret/KMS. `APPLE_IAP_P8_PATH` is local-dev only.
- Do not log private key content, p8 path content, signedPayload, appAccountToken, decoded payload, or raw JWS.
- README may document variable names but must not include real key material.
- Rotation runbook: deploy new key ID/private key pair, verify App Store Server API call, then revoke old key in App Store Connect.

## Library

Use `github.com/go-pay/gopay/apple`, pinned explicitly. Current available latest from `go list -m -versions github.com/go-pay/gopay` is `v1.5.118`; implementation should pin a concrete version instead of saying “latest”.

The service layer depends on interfaces:

```go
type AppleTransactionVerifier interface {
    FetchTransaction(ctx context.Context, txID string, env AppleEnvironment) (*AppleTransaction, error)
}

type AppleWebhookVerifier interface {
    DecodeSignedPayload(ctx context.Context, signedPayload string) (*AppleWebhookEvent, error)
}

type AppleReconciler interface {
    GetNotificationHistory(ctx context.Context, req NotificationHistoryRequest) ([]AppleWebhookEvent, string, error)
    GetSubscriptionStatus(ctx context.Context, originalTransactionID string, env AppleEnvironment) (*AppleSubscriptionStatus, error)
}
```

If gopay lacks a required reconciliation method, keep that behind the interface and implement the small raw HTTP App Store Server API call with the same JWT client config.

## Database

New migration: `db/migrations/002_apple_iap.sql`.

### `apple_account_tokens`

```sql
CREATE TABLE IF NOT EXISTS apple_account_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id),
    UNIQUE (token)
);
```

The service generates UUID v4 using stdlib `crypto/rand` and stores it as string/UUID. No need to expose `uuid.UUID` through public domain models.

### `apple_subscriptions`

```sql
CREATE TABLE IF NOT EXISTS apple_subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_account_token UUID NOT NULL REFERENCES apple_account_tokens(token),
    environment TEXT NOT NULL,                    -- Sandbox | Production
    original_transaction_id TEXT NOT NULL,
    last_transaction_id TEXT NOT NULL DEFAULT '',
    web_order_line_item_id TEXT NOT NULL DEFAULT '',
    plan_id TEXT NOT NULL,
    provider_product_id TEXT NOT NULL,
    subscription_group_id TEXT NOT NULL DEFAULT '',
    level INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'ACTIVE',        -- ACTIVE | CANCELED | EXPIRED | REVOKED
    auto_renew_status TEXT NOT NULL DEFAULT 'UNKNOWN', -- ON | OFF | UNKNOWN
    current_period_start TIMESTAMPTZ NOT NULL,
    current_period_end TIMESTAMPTZ NOT NULL,
    grace_period_expires_at TIMESTAMPTZ,
    last_event_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_notification_created_at TIMESTAMPTZ,
    last_payload_hash TEXT NOT NULL DEFAULT '',
    last_transaction_snapshot JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (original_transaction_id, environment)
);

CREATE INDEX IF NOT EXISTS apple_subscriptions_user_env_idx
    ON apple_subscriptions(user_id, environment);
CREATE INDEX IF NOT EXISTS apple_subscriptions_user_entitlement_idx
    ON apple_subscriptions(user_id, level DESC, current_period_end DESC)
    WHERE status IN ('ACTIVE', 'CANCELED');
CREATE INDEX IF NOT EXISTS apple_subscriptions_status_period_idx
    ON apple_subscriptions(status, current_period_end);
```

`last_transaction_snapshot` is a minimized decoded snapshot, not full raw JWS. It may include transaction identifiers, product id, dates, environment, and revocation fields. It must not include full signed payload.

### `apple_events`

```sql
CREATE TABLE IF NOT EXISTS apple_events (
    id BIGSERIAL PRIMARY KEY,
    notification_uuid TEXT NOT NULL UNIQUE,
    notification_type TEXT NOT NULL,
    subtype TEXT NOT NULL DEFAULT '',
    environment TEXT NOT NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    app_account_token UUID,
    original_transaction_id TEXT NOT NULL DEFAULT '',
    transaction_id TEXT NOT NULL DEFAULT '',
    web_order_line_item_id TEXT NOT NULL DEFAULT '',
    processing_status TEXT NOT NULL DEFAULT 'PROCESSED',
    processing_error TEXT NOT NULL DEFAULT '',
    raw_jws_sha256 TEXT NOT NULL,
    decoded_payload JSONB,
    notification_created_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS apple_events_transaction_id_idx ON apple_events(transaction_id);
CREATE INDEX IF NOT EXISTS apple_events_original_tx_idx ON apple_events(original_transaction_id, environment);
CREATE INDEX IF NOT EXISTS apple_events_status_idx ON apple_events(processing_status, created_at);
```

Allowed `processing_status` values:

```text
PROCESSED
IGNORED_DUPLICATE
IGNORED_UNKNOWN_TYPE
PENDING_USER_BINDING
OWNERSHIP_CONFLICT
PERMANENT_FAILURE
```

No `source = VERIFY (future)` column in this PR. `apple_events` is webhook idempotency and audit state only.

### Query shape

Representative sqlc queries:

```sql
-- name: GetOrCreateAppleAccountToken :one
-- implemented by DAO transaction: select by user_id; if missing insert generated UUID

-- name: GetAppleAccountTokenByToken :one
SELECT id, user_id, token
FROM apple_account_tokens
WHERE token = $1;

-- name: GetSubscriptionByOriginalTx :one
SELECT *
FROM apple_subscriptions
WHERE original_transaction_id = $1 AND environment = $2;

-- name: ListSubscriptionsForUserEntitlement :many
SELECT *
FROM apple_subscriptions
WHERE user_id = $1
  AND environment = ANY($2::text[])
ORDER BY
  CASE WHEN status IN ('ACTIVE','CANCELED')
            AND COALESCE(grace_period_expires_at, current_period_end) > now()
       THEN 0 ELSE 1 END,
  level DESC,
  COALESCE(grace_period_expires_at, current_period_end) DESC,
  last_event_at DESC;

-- name: InsertAppleEventIfNotExists :one
INSERT INTO apple_events (
    notification_uuid, notification_type, subtype, environment,
    user_id, app_account_token, original_transaction_id, transaction_id,
    web_order_line_item_id, processing_status, processing_error,
    raw_jws_sha256, decoded_payload, notification_created_at
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
)
ON CONFLICT (notification_uuid) DO NOTHING
RETURNING id;
```

DAO maps `pgx.ErrNoRows` from `InsertAppleEventIfNotExists` to `created=false, nil`.

## Event Reduction

### Idempotency and ordering

Two separate protections:

- Duplicate delivery: `apple_events.notification_uuid UNIQUE`.
- Out-of-order state changes: subscription reducer compares event effective keys before mutating `apple_subscriptions`.

Reducer inputs should keep:

- `notification_created_at`
- `transaction_id`
- `original_transaction_id`
- `web_order_line_item_id`
- `purchase_date`
- `expires_date`
- `revocation_date`
- `grace_period_expires_date`

Ordering rules:

- Renewal/subscribe updates apply when `expires_date` extends effective end or transaction ordering is newer.
- Expiration does not override a newer renewal whose effective end is later.
- Revocation/refund wins for the specific transaction/original transaction unless a later reversal/status check confirms recovery.
- Unknown notification types are recorded and acked, not treated as fatal.
- Ambiguous state should call App Store Server API subscription status before downgrading entitlement.

### Notification mapping

| Apple notification | DB effect |
| --- | --- |
| `SUBSCRIBED`, `DID_RENEW` | upsert subscription as `ACTIVE`, update period and product catalog fields |
| `DID_CHANGE_RENEWAL_STATUS` with auto-renew off | set `auto_renew_status='OFF'`, `status='CANCELED'`, keep entitlement until effective end |
| `DID_FAIL_TO_RENEW` with grace | keep `status='ACTIVE'`, set `grace_period_expires_at` |
| `GRACE_PERIOD_EXPIRED`, `EXPIRED` | set `EXPIRED` only if event is not older than current effective end |
| `REFUND`, `REVOKE` | set `REVOKED` |
| unknown | insert event as `IGNORED_UNKNOWN_TYPE`, return 200 |

## Sensitive IAP Data Handling

- Request logs must redact `signedPayload`, `signed_payload`, `raw_jws`, `raw_payload`, `decoded_payload`, `appAccountToken`, `app_account_token`, `private_key`, `apple_iap_private_key`, `p8`, `key_id`, and `issuer_id` where they appear in JSON bodies.
- Service logs should use safe identifiers: `notification_uuid`, `transaction_id`, `original_transaction_id`, `environment`, `notification_type`, `processing_status`, and hashed token/JWS where needed.
- Default DB storage:
  - store `raw_jws_sha256`;
  - store minimized `decoded_payload`;
  - do not store full raw JWS.
- `APPLE_IAP_STORE_RAW_PAYLOADS=true` may exist for local dev only. Production config must fail if this is true.
- Add a future retention task for `decoded_payload`; until then keep decoded payload minimal enough for long-term retention.

## API Wiring

### User routes

Add a narrow dependency to `api.UserDeps`:

```go
type SubscriptionReader interface {
    LoadSubscriptionInfo(ctx context.Context, userID int64) (model.SubscriptionInfo, error)
}

type UserDeps struct {
    Users         service.UserService
    Auth          auth.Service
    Subscriptions SubscriptionReader
}
```

`loadCurrentUser` receives `SubscriptionReader`. If it is nil, return zero subscription only in tests that explicitly choose nil; production wiring must pass the real service.

### Payment routes

Add:

```go
type PaymentDeps struct {
    Auth auth.Service
    IAP  payment.AppleIAPService
}

func RegisterPaymentRoutes(api huma.API, deps PaymentDeps)
```

`cmd/server/main.go` constructs:

1. `userDAO`, `userSvc`.
2. `subscriptionDAO`, `accountTokenDAO`, `eventDAO`.
3. `payment.AppleIAPService`.
4. `authSvc`.
5. register user routes with `Subscriptions`.
6. register payment routes with `Auth` and `IAP`.

## Implementation Units

### U1. Migration, Queries, and Local Migration Command

**Goal:** Add Apple IAP tables and generated sqlc bindings without breaking default test path.

**Files:**

- Add: `db/migrations/002_apple_iap.sql`
- Add: `db/queries/apple_account_tokens.sql`
- Add: `db/queries/apple_subscriptions.sql`
- Add: `db/queries/apple_events.sql`
- Modify generated: `internal/db/*`
- Modify: `Makefile`
- Modify: `README.md`

**Approach:**

- Add the three new tables above.
- Add `make migrate` that applies `db/migrations/*.sql` in lexical order to `DB_DSN`.
- Update README first-run instructions from only `001_init.sql` to `make migrate`.
- Run `make sqlc`.

**Tests:**

- `make sqlc` produces deterministic generated code.
- `make migrate DB_DSN=...` applies both migrations on empty DB.
- Re-running migration is idempotent due `IF NOT EXISTS`.

### U2. Config, Catalog, and Key Guards

**Goal:** Parse IAP config and fail closed before API handling.

**Files:**

- Modify: `internal/config/config.go`
- Add: `internal/service/payment/catalog.go`
- Add: `internal/service/payment/config_test.go`

**Approach:**

- Add `AppleIAPConfig`.
- Parse `APPLE_IAP_PRODUCTS` JSON into product catalog entries.
- Validate required fields: bundle, issuer, key id, private key or p8 path, products, entitlement environments.
- Fail production if `APPLE_IAP_STORE_RAW_PAYLOADS=true`.
- Default `APPLE_IAP_ENABLE_SANDBOX_FALLBACK=false`; tests/dev may set true.

**Tests:**

- Missing issuer/key_id/private key/products => `ErrNotConfigured`.
- Duplicate `(product_id, environment)` rejected.
- Production with sandbox-only entitlement env rejected unless explicitly allowed for non-prod.

### U3. Account Token Mapping

**Goal:** Provide server-generated UUID for StoreKit `appAccountToken`.

**Files:**

- Add: `internal/model/subscription.go`
- Add: `internal/dao/subscription.go`
- Add: `internal/service/payment/token.go`
- Modify: `internal/api/payment.go`

**Approach:**

- DAO `GetOrCreateAccountToken(ctx, userID)` selects existing token or inserts a generated UUID in a transaction.
- API `GET /payment/apple/account-token` uses bearer auth and returns the token.
- UUID generation uses stdlib `crypto/rand`; no need to add a UUID domain dependency unless implementation chooses to.

**Tests:**

- Same user receives stable token.
- Different users receive different tokens.
- Missing bearer returns 401.
- Token format is UUID.

### U4. Subscription and Event DAO

**Goal:** Implement idempotent event insert and ownership-safe subscription upsert.

**Files:**

- Modify: `internal/dao/subscription.go`
- Add: `internal/dao/subscription_integration_test.go` with `//go:build integration`

**Approach:**

- `InsertAppleEventIfNotExists` returns `(created bool, eventID int64, error)`.
- `UpsertSubscriptionWithOwnershipCheck`:
  - new original tx => insert;
  - existing same user => reducer update;
  - existing different user => `ErrSubscriptionOwnershipConflict`.
- All webhook business mutation accepts an explicit `pgx.Tx` so service can keep event insert + subscription update atomic.

**Tests:**

- Fresh event created.
- Duplicate event returns `created=false`.
- New subscription inserts.
- Same user renew updates.
- Different user conflict.
- Transaction rollback removes event row when subscription mutation fails.

### U5. Apple Verifiers

**Goal:** Isolate gopay / Apple Server API behind testable interfaces.

**Files:**

- Add: `internal/service/payment/apple_verifier.go`
- Add: `internal/service/payment/apple_webhook_verifier.go`
- Add: `internal/service/payment/apple_reconcile.go`
- Modify: `go.mod`, `go.sum`

**Approach:**

- Add `github.com/go-pay/gopay` pinned to `v1.5.118` unless compatibility testing selects another explicit version.
- Transaction verifier:
  - production client;
  - sandbox client only if fallback enabled.
- Webhook verifier:
  - decode and verify signedPayload;
  - check bundle id;
  - return normalized internal event type.
- Reconciler:
  - provide interface for Notification History and Subscription Status;
  - raw HTTP fallback if gopay lacks a method.

**Tests:**

- Unit tests use stubs, not real Apple.
- Fallback only on Apple not-found/env mismatch errors and only when enabled.
- 401/403 config errors do not fallback.
- context cancellation does not fallback.

### U6. Verify Subscription Marker

**Goal:** Implement authenticated purchase verification and immediate entitlement readback.

**Files:**

- Add: `internal/service/payment/apple_iap.go`
- Add: `internal/service/payment/apple_iap_test.go`
- Modify: `internal/api/payment.go`

**Approach:**

- Authenticate bearer.
- Fetch Apple transaction by `transaction_id`.
- Validate:
  - bundle id matches;
  - environment accepted for verify;
  - product exists in catalog and is subscription;
  - `appAccountToken` exists;
  - token maps to the authenticated user;
  - transaction is not revoked.
- Upsert subscription.
- Return `LoadSubscriptionInfo`.

**Tests:**

- Empty txID => 400.
- Missing appAccountToken => 400.
- appAccountToken maps to different user => 400 mismatch.
- Existing original tx same user => 200 update.
- Existing original tx different user => 409.
- Sandbox transaction in prod entitlement config does not grant `/users/me` ACTIVE.

### U7. `/users/me` Subscription Reader

**Goal:** Populate existing user contract from provider-neutral entitlement logic.

**Files:**

- Modify: `internal/api/user.go`
- Modify: `internal/model/user.go`
- Add: `internal/service/payment/subscription_reader.go`
- Modify: `internal/api/user_test.go`

**Approach:**

- Add `SubscriptionReader` dependency.
- `LoadSubscriptionInfo` uses allowed entitlement environments and catalog/level fields.
- Selection rule follows `/users/me` contract above.
- No active entitlement but existing row returns `EXPIRED`.
- Revoked maps to `EXPIRED`.

**Mandatory regression test:**

- Existing no-subscription user still returns `Credits.Balance = 0`.
- Existing no-subscription user returns `SubscriptionInfo.Status = "NONE"`.

**Additional tests:**

- Active row returns `ACTIVE`.
- Canceled row with future end returns `CANCELED` and non-zero level.
- Expired active row returns `EXPIRED`.
- Revoked row returns `EXPIRED`.
- Higher level shorter period wins over lower level longer period.

### U8. Webhook Idempotency and State Reducer

**Goal:** Process ASSN V2 notifications safely.

**Files:**

- Add: `internal/service/payment/apple_webhook.go`
- Add: `internal/service/payment/apple_webhook_test.go`
- Modify: `internal/api/payment.go`

**Approach:**

- API handler enforces:
  - `MaxBytesReader` body limit;
  - `signedPayload` present and below configured length;
  - compact JWS has 3 segments before expensive verify.
- Service:
  - decode + verify JWS;
  - check bundle id and catalog;
  - begin tx;
  - insert event idempotency row;
  - duplicate => commit 200;
  - map appAccountToken to user if present;
  - if no mapping, mark `PENDING_USER_BINDING`, commit 200;
  - reduce known notification types into subscription row;
  - unknown types are logged and acked.

**Tests:**

- Invalid JWS => 401, no DB mutation.
- Oversized body => 400.
- Duplicate notification_uuid => no mutation.
- SUBSCRIBED/DID_RENEW => upsert/extend.
- EXPIRED after newer DID_RENEW => ignored.
- DID_CHANGE_RENEWAL_STATUS off => CANCELED, still entitled until end.
- REFUND/REVOKE => REVOKED.
- Missing token => pending event, 200.
- Ownership conflict => event recorded, 200.
- Not configured => 500.

### U9. Reconciliation / Missed Webhook Recovery

**Goal:** Avoid permanent drift when webhook delivery or JWS verification fails beyond Apple retry behavior.

**Files:**

- Add: `internal/service/payment/apple_reconcile.go`
- Add: `internal/service/payment/apple_reconcile_test.go`
- Add optional: `cmd/apple-iap-reconcile/main.go`
- Add: `docs/runbooks/apple-iap-reconcile.md`

**Approach:**

- Provide service method to replay Notification History for a time range / pagination token.
- Each replayed notification goes through the same webhook reducer and `notification_uuid` idempotency.
- Provide status reconciliation by `original_transaction_id` for support/debug paths.
- If optional CLI is implemented, it reads normal config and takes `--from`, `--to`, `--environment`, `--pagination-token`.
- If CLI is not implemented in the first PR, runbook must show `go test`/small one-off invocation is not acceptable as the operational path; leave the PR blocked until a callable path exists.

**Tests:**

- Replayed duplicate notification is idempotent.
- Replayed renewal fixes stale expired row.
- Pagination token persisted or emitted in output.

### U10. Logging Redaction and Security Tests

**Goal:** Prevent Apple payload/key/token leakage through request logs.

**Files:**

- Modify: `internal/api/middleware.go`
- Add/modify: `internal/api/middleware_test.go`

**Approach:**

- Extend `isSensitiveField` for:
  - `signedPayload`, `signed_payload`
  - `raw_jws`, `raw_payload`, `decoded_payload`
  - `appAccountToken`, `app_account_token`
  - `private_key`, `apple_iap_private_key`
  - `p8`, `issuer_id`, `key_id`
- Ensure nested JSON values are redacted.

**Tests:**

- Request body with `signedPayload` logs `[REDACTED]`.
- Response body with token-like fields logs `[REDACTED]`.
- Non-JSON body behavior unchanged.

### U11. Integration Tests and CI Shape

**Goal:** Keep default CI stable while preserving DB transaction coverage.

**Files:**

- Add integration tests with `//go:build integration`
- Modify: `Makefile`
- Optional modify: `.github/workflows/ci.yml`

**Approach:**

- Default:
  - `go test ./...` runs unit and httptest tests only.
- Integration:
  - `make test-integration INTEGRATION_DB_DSN=postgres://...`
  - command expands to `go test -tags=integration ./...`.
- If adding CI Postgres service is acceptable, add separate `integration` job rather than changing the default `test` job semantics.

**Tests:**

- `make test` passes without DB.
- `make test-integration` fails fast with clear message if `INTEGRATION_DB_DSN` is missing.
- Integration setup applies `make migrate` before tests.

## Test Plan

This supersedes the gstack test artifact for implementation.

### Default unit / httptest

- `GET /payment/apple/account-token`
  - missing bearer => 401
  - valid bearer => stable UUID
- `POST /payment/apple/verify`
  - missing bearer => 401
  - empty `transaction_id` => 400
  - missing appAccountToken in Apple transaction => 400
  - token maps to different user => 400
  - unsupported product => 400
  - Apple 401/403 => no sandbox fallback
  - Apple not found + fallback enabled => sandbox retry
  - fallback disabled => no sandbox retry
  - not configured => 503
- `POST /webhooks/apple`
  - oversized body => 400
  - invalid JWS => 401
  - bundle mismatch => 401
  - duplicate notification_uuid => 200 idempotent
  - missing token mapping => 200 + `PENDING_USER_BINDING`
  - unknown notification type => 200 + `IGNORED_UNKNOWN_TYPE`
  - not configured => 500
- `GET /users/me`
  - no subscription => `Status="NONE"`, `Credits.Balance=0`
  - active subscription => populated `ACTIVE`
  - canceled but not expired => `CANCELED`, still level > 0
  - revoked => `EXPIRED`
  - expired row => `EXPIRED`
  - sandbox row in prod allowed env => not active
  - high-level short period beats low-level long period
- Logging middleware
  - redacts `signedPayload`
  - redacts `appAccountToken`
  - redacts private key fields

### Integration-gated

Run with:

```bash
make migrate DB_DSN="$INTEGRATION_DB_DSN"
make test-integration INTEGRATION_DB_DSN="$INTEGRATION_DB_DSN"
```

Coverage:

- account token unique constraints and concurrent get-or-create.
- subscription ownership conflict under concurrent verify.
- webhook event insert + subscription update atomic rollback.
- duplicate `notification_uuid` race.
- `/users/me` entitlement query against real Postgres ordering.
- migration idempotency.

### Manual smoke

Preconditions:

- real App Store Connect API key configured;
- sandbox fallback enabled only in dev/staging;
- iOS sandbox transaction created with server-provided appAccountToken.

Flow:

1. `make migrate DB_DSN=...`
2. `go run ./cmd/server`
3. authenticate guest/apple user
4. `GET /payment/apple/account-token`
5. iOS purchase using returned UUID
6. `POST /payment/apple/verify`
7. `GET /users/me` shows expected status/level
8. replay sandbox webhook fixture or use Apple sandbox notification

## Failure Modes

| Failure | Handling | Test |
| --- | --- | --- |
| App Store API timeout | return 500 on verify; webhook reducer returns 500 only for transient dependency | unit |
| App Store API 401/403 | no fallback; config/auth error | unit |
| Sandbox tx in prod | does not grant entitlement | unit + integration |
| Empty appAccountToken | verify 400; webhook pending or ignored according to event | unit |
| Token maps to other user | verify 400; existing original tx conflict 409 | unit + integration |
| Duplicate webhook | event conflict => 200 no mutation | integration |
| DB failure after event insert | tx rollback, webhook 500 | integration |
| JWS root rotation / verifier stale | 401 to Apple; operational alert/reconcile required | residual + runbook |
| Missed webhook after retry window | reconciliation via Notification History / Subscription Status | unit for replay + runbook |
| raw payload leakage | default no raw JWS storage + logging redaction | unit |

Residual risk:

- This repo has no metrics framework today. This PR should add structured logs and reconciliation. Metrics/alert integration can be follow-up only if deployment platform already supplies log-based alerts; otherwise launch should be blocked on an alert path for webhook unauthorized/transient failure spikes.

## Parallelization

| Step | Modules touched | Depends on |
| --- | --- | --- |
| A. Migration + sqlc + migrate command | `db/`, `internal/db`, `Makefile`, `README` | none |
| B. Config + catalog + go.mod | `internal/config`, `internal/service/payment`, `go.mod` | none |
| C1. Token + verify/read service | `dao`, `service/payment`, `api/payment` | A, B |
| C2. Webhook reducer + reconciliation | `service/payment`, `api/payment` | A, B, C1 models |
| D. `/users/me` wiring | `api/user.go`, `cmd/server` | C1 |
| E. Security logging | `api/middleware.go` | none |
| F. Integration tests | tests across dao/service/api | A-D |

Lanes:

- Lane 1: A -> C1 -> D
- Lane 2: B -> C1
- Lane 3: E independent
- Lane 4: C2 after A/B and shared models
- Join before final smoke: C1, C2, D, E, F

Do not run multiple workers against the same files unless write sets are split. `api/payment.go`, `service/payment/*`, and `dao/subscription.go` should be edited serially or by a single worker.

## Implementation Order

1. Add migrations, queries, `make migrate`, README migration update.
2. Add config/catalog parsing and gopay dependency.
3. Add model enums and account-token DAO/service/API.
4. Add subscription/event DAO with integration-gated tests.
5. Add Apple verifier interfaces and stub-first unit tests.
6. Implement verify path and `POST /payment/apple/verify`.
7. Implement `SubscriptionReader` and `/users/me` wiring.
8. Extend log redaction and middleware tests.
9. Implement webhook handler and reducer.
10. Implement reconciliation/backfill service and runbook/CLI.
11. Run default tests.
12. Run integration tests against local Postgres.
13. Run manual sandbox smoke if credentials and transaction are available.

## Decisions Locked

| ID | Decision | Choice | Rationale |
| --- | --- | --- | --- |
| D1 | Scope | Subscription marker only | User excluded credits/wallet/ledger/order table |
| D2 | appAccountToken | Server-generated UUID mapping | Apple requires UUID; int64 user id is invalid |
| D3 | Product contract | `/users/me.product_id` returns canonical `plan_id` | Avoid Apple-specific public contract |
| D4 | Environment entitlement | Production grants only configured `Production` rows | Prevent sandbox privilege bypass |
| D5 | Webhook tx boundary | event insert + subscription update in one pgx tx | Atomic idempotency and mutation |
| D6 | Empty token | Reject verify; webhook pending if valid event cannot bind | No TOFU / no auto uid binding |
| D7 | CANCELED semantics | Still entitled until effective end | Auto-renew off is not immediate expiry |
| D8 | Raw payload | Store hash + minimized decoded payload by default | Avoid long-term JWS/PII leakage |
| D9 | Tests | Integration gated, default go test DB-free | Preserve current CI/dev experience |
| D10 | Reconciliation | Minimal Notification History / status replay path | Apple retry alone is not enough |

## Follow-up Candidates

| ID | What | Why deferred |
| --- | --- | --- |
| T1 | DB-backed product catalog | Static env is enough until product changes need admin UI |
| T2 | Full admin dashboard for subscription events | Useful for support, not required for marker correctness |
| T3 | Platform metrics/alert package | Current repo has no metrics framework; use structured logs + deploy alert first |
| T4 | PayPal / Creem provider | Provider-neutral contract now avoids Apple lock-in |
| T5 | Legacy receipt fallback | StoreKit 2 Server API is the chosen v1 path |
| T6 | Raw payload retention job | Default minimizes payload; retention can follow once storage policy is set |

## Review Coverage

`ce-doc-review` personas used:

- coherence
- feasibility
- security-lens
- scope-guardian
- adversarial-document-reviewer
- product-lens

High-priority findings from all reviewers have been incorporated into this plan. Remaining non-blocking risks are captured under Failure Modes and Follow-up Candidates.

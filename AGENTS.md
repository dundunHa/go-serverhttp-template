# Repository Guidelines

## Project Structure

- `cmd/server/`: main entrypoint (`cmd/server/main.go`) for HTTP/gRPC startup.
- `internal/`: private application code (API handlers, router, services, storage, middleware).
- `internal/transport/http/`: HTTP server, handlers, and middleware.
- `internal/transport/grpc/`: gRPC server skeleton (registration entrypoint only; no proto/services by default).
- `pkg/`: reusable/public packages (e.g., logging, cache).
- `configs/`: TOML config examples (see `configs/config.toml.example`).
- `.github/workflows/`: CI workflow definitions (currently commented out in `ci.yml`).

## Build, Test, and Development Commands

- Run locally: `go run ./cmd/server` (requires env vars; see below).
- Run with gRPC only (no services yet): `APP_MODE=grpc go run ./cmd/server`
- Run all tests: `go test ./...`
- Run a focused package: `go test ./internal/service/auth -run TestName`
- Lint (recommended): `golangci-lint run` (config: `.golangci.yml`)
- Format: `gofmt -w .` (optionally `goimports -w .` if installed)
- Build container: `docker build -t go-serverhttp-template .`

## Configuration & Security

Config precedence:

1. Environment variables (preferred; default prefix `APP_`, customizable via `CONFIG_PREFIX`, empty = no prefix)
2. TOML config file (default `configs/config.toml`, or `CONFIG_FILE=/path/to/config.toml`)

Common keys:

- `APP_MODE`: `http` | `grpc` | `both` (or `MODE` if `CONFIG_PREFIX=` is empty)
- `APP_HTTP_PORT` / `APP_GRPC_PORT` (or `HTTP_PORT` / `GRPC_PORT` without prefix)
- `APP_DB_DSN` (Postgres DSN; missing will disable DB-dependent routes with a startup warning)
- `APP_HTTP_LOG_BODY` (default `false`)

Keep secrets (DB DSN, auth credentials) out of the repo; use local env files or your secret manager.

## Coding Style & Naming Conventions

- Follow standard Go style: tabs for indentation, `gofmt`-formatted code.
- Package naming: short, lowercase; avoid underscores.
- Keep `internal/` APIs private; only export from `pkg/` when you intend reuse.

## Testing Guidelines

- Prefer table-driven tests and deterministic behavior.
- Name tests `*_test.go` with `TestXxx` functions (examples live under `internal/service/auth/`).

## Commit & Pull Request Guidelines

- Commit messages follow Conventional Commits (seen in history): `feat: ...`, `fix: ...`, `chore: ...`.
- PRs should include: purpose/summary, how to run/test, config changes (new env vars), and linked issue/ticket.

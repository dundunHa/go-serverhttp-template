.PHONY: fmt test test-integration dev sqlc migrate

GOFMT_TARGET_DIRS ?= cmd internal pkg
GO_TEST_TARGETS ?= ./...
GO_TEST_FLAGS ?= -timeout=60s
GO_RUN_TARGET ?= ./cmd/server
SQLC_VERSION ?= v1.31.1
MIGRATIONS_DIR ?= db/migrations

fmt:
	gofmt -w $(GOFMT_TARGET_DIRS)
	golangci-lint run --fix ./...

test:
	go test $(GO_TEST_FLAGS) $(GO_TEST_TARGETS)

test-integration:
	@# 跑带 //go:build integration tag 的测试。要求 INTEGRATION_DB_DSN 指向已应用全部迁移的 Postgres 实例。
	@if [ -z "$$INTEGRATION_DB_DSN" ]; then \
		echo "error: INTEGRATION_DB_DSN is required (example: INTEGRATION_DB_DSN=postgres://user:pass@host:5432/app_test?sslmode=disable)" >&2; \
		echo "       run 'make migrate DB_DSN=\"\$$INTEGRATION_DB_DSN\"' first to create the schema." >&2; \
		exit 1; \
	fi
	INTEGRATION_DB_DSN=$$INTEGRATION_DB_DSN go test $(GO_TEST_FLAGS) -tags=integration $(GO_TEST_TARGETS)

dev:
	go run $(GO_RUN_TARGET)

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

migrate:
	@# Applies db/migrations/*.sql in lexical order to $$DB_DSN. Re-running is idempotent
	@# because each migration uses CREATE TABLE / CREATE INDEX IF NOT EXISTS.
	@if [ -z "$$DB_DSN" ]; then \
		echo "error: DB_DSN is required (example: DB_DSN=postgres://user:pass@host:5432/app?sslmode=disable)" >&2; \
		exit 1; \
	fi
	@if ! command -v psql >/dev/null 2>&1; then \
		echo "error: psql not found in PATH" >&2; \
		exit 1; \
	fi
	@files=$$(ls $(MIGRATIONS_DIR)/*.sql 2>/dev/null | sort); \
	if [ -z "$$files" ]; then \
		echo "error: no migration files in $(MIGRATIONS_DIR)" >&2; \
		exit 1; \
	fi; \
	for f in $$files; do \
		echo "applying $$f"; \
		psql "$$DB_DSN" -v ON_ERROR_STOP=1 -f "$$f" || exit $$?; \
	done

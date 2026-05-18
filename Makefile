.PHONY: fmt test dev sqlc

GOFMT_TARGET_DIRS ?= cmd internal pkg
GO_TEST_TARGETS ?= ./...
GO_TEST_FLAGS ?= -timeout=60s
GO_RUN_TARGET ?= ./cmd/server
SQLC_VERSION ?= v1.31.1

fmt:
	gofmt -w $(GOFMT_TARGET_DIRS)
	golangci-lint run --fix ./...

test:
	go test $(GO_TEST_FLAGS) $(GO_TEST_TARGETS)

dev:
	go run $(GO_RUN_TARGET)

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

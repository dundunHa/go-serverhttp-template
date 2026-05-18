.PHONY: fmt test

GOFMT_TARGET_DIRS ?= cmd internal pkg
GO_TEST_TARGETS ?= ./...
GO_TEST_FLAGS ?= -timeout=60s

fmt:
	gofmt -w $(GOFMT_TARGET_DIRS)
	golangci-lint run --fix ./...

test:
	go test $(GO_TEST_FLAGS) $(GO_TEST_TARGETS)

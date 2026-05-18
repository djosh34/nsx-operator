SHELL := /bin/bash

BIN_DIR := $(CURDIR)/.bin

GOFUMPT_VERSION := v0.10.0
GOLANGCI_LINT_VERSION := v2.12.2

GOFUMPT := $(BIN_DIR)/gofumpt
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint

.PHONY: check lint test test-coverage

check: lint test test-coverage

lint: $(GOFUMPT) $(GOLANGCI_LINT)
	$(GOFUMPT) -w .
	$(GOLANGCI_LINT) run ./...

test:
	go test ./...

test-coverage:
	go test -cover ./...

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

$(GOFUMPT): go.mod | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

$(GOLANGCI_LINT): | $(BIN_DIR)
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(BIN_DIR) $(GOLANGCI_LINT_VERSION)

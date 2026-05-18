SHELL := /bin/bash

BIN_DIR := $(CURDIR)/.bin

GOFUMPT_VERSION := v0.10.0
GOLANGCI_LINT_VERSION := v2.12.2
SETUP_ENVTEST_VERSION := v0.24.1
ENVTEST_K8S_VERSION := 1.32.x

GOFUMPT := $(BIN_DIR)/gofumpt
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
SETUP_ENVTEST := $(BIN_DIR)/setup-envtest

.PHONY: check lint test test-coverage

check: lint test test-coverage

lint: $(GOFUMPT) $(GOLANGCI_LINT)
	$(GOFUMPT) -w .
	$(GOLANGCI_LINT) run ./...

test: $(SETUP_ENVTEST)
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./...

test-coverage: $(SETUP_ENVTEST)
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -cover ./...

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

$(GOFUMPT): go.mod | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

$(GOLANGCI_LINT): | $(BIN_DIR)
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(BIN_DIR) $(GOLANGCI_LINT_VERSION)

$(SETUP_ENVTEST): go.mod | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)

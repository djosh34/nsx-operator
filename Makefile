SHELL := /bin/bash

BIN_DIR := $(CURDIR)/.bin

GOFUMPT_VERSION := v0.10.0
GOLANGCI_LINT_VERSION := v2.12.2
SETUP_ENVTEST_VERSION := v0.24.1
ENVTEST_K8S_VERSION := 1.32.x
COVERAGE_THRESHOLD := 80.0
COVERAGE_PROFILE := coverage.out

GOFUMPT := $(BIN_DIR)/gofumpt
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
SETUP_ENVTEST := $(BIN_DIR)/setup-envtest

.PHONY: check fmt vet lint test test-race test-contract test-e2e test-large-chaos test-coverage

check: fmt vet lint test test-race test-contract test-e2e test-large-chaos test-coverage

fmt: $(GOFUMPT)
	$(GOFUMPT) -w .

vet:
	go vet ./...

lint: fmt $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

test: $(SETUP_ENVTEST)
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./...

test-race: $(SETUP_ENVTEST)
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -race ./...

test-contract: $(SETUP_ENVTEST)
	go test ./internal/nsxclient -run 'Test(MockAPIRouteInventoryIsSupportedAndContracted|TypedClientContractsAgainstMockAPI|SharedRateLimitedClientConcurrencyAgainstMockAPI)' -count=1
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./internal/stateoperator -run 'TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI' -count=1

test-e2e: $(SETUP_ENVTEST)
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./api/v1alpha ./internal/kubeapi ./internal/startup ./internal/stateoperator ./cmd/nsx-operator -count=1

test-large-chaos: $(SETUP_ENVTEST)
	GOMEMLIMIT=768MiB KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -tags largechaos ./internal/nsxclient ./internal/stateoperator -run 'Test(Large|Chaos|GroupReconcileManageUnavailableSetsUnknownConditionsAndDoNotRequeue)' -count=1 -timeout 20m

test-coverage: $(SETUP_ENVTEST)
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -coverprofile=$(COVERAGE_PROFILE) -covermode=atomic ./...
	@total="$$(go tool cover -func=$(COVERAGE_PROFILE) | awk '/^total:/ {gsub(/%/,"",$$3); print $$3}')"; \
	awk -v total="$$total" -v threshold="$(COVERAGE_THRESHOLD)" 'BEGIN { \
		if (total + 0 < threshold + 0) { \
			printf "coverage %.1f%% below %.1f%% threshold\n", total, threshold; \
			exit 1; \
		} \
		printf "coverage %.1f%% meets %.1f%% threshold\n", total, threshold; \
	}'

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

$(GOFUMPT): go.mod | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

$(GOLANGCI_LINT): | $(BIN_DIR)
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(BIN_DIR) $(GOLANGCI_LINT_VERSION)

$(SETUP_ENVTEST): go.mod | $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)

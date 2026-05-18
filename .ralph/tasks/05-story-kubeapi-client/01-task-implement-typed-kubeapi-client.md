## Task: Implement Typed Kubernetes CRD Client <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/kubeapi` as a typed client-go wrapper for `NSXNetworkCloud` and `NSXGroup` CRs without using the official Kubernetes code generator. Callers should use typed objects and status structs, not raw JSON status patches.

In scope: clients for List, Get, Create, Update, Apply, UpdateStatus, Delete, and Watch for both CRDs; typed `ListOptions`, `FieldFilter`, and `filterBy(field, value)` abstraction mapping to field selectors such as `spec.networkCloudFQDN=<fqdn>`; required resourceVersion on Update, status-only update behavior, required apply field manager, and typed object handling. Out of scope: official code generation and raw `[]byte` status patch APIs.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Tests use a real or fake Kubernetes API path sufficient to prove field selectors, apply, watch, and status updates.
- [x] Tests prove UpdateStatus cannot mutate spec.
</acceptance_criteria>

<plan>
.ralph/tasks/05-story-kubeapi-client/01-task-implement-typed-kubeapi-client_plans/2026-05-19-typed-kubeapi-client-plan.md
</plan>

<verification>
Concrete envtest-backed evidence:

- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/kubeapi -count=1 -v` passed.
  - `TestNetworkCloudClientCreatesGetsAndListsByFieldSelector` logged typed NetworkCloud create/get/list with `FieldNetworkCloudFQDN` returning `cloud-a`.
  - `TestGroupClientCreatesGetsAndListsBySelectableFields` logged typed Group lists returning `[group-b]` for `FieldGroupID`, `[group-a]` for `FieldGroupMode`, and `[group-a]` for `FieldNetworkCloudFQDN`.
  - `TestUpdateRequiresResourceVersionAndPersistsFetchedObjectChanges` proved empty `resourceVersion` update is rejected locally and fetched-object updates persist for both resources.
  - `TestStatusUpdateStoresStatusAndPreservesSpec` logged typed `UpdateStatus` storing `Synced` while preserving group `display_name`, and also verified NetworkCloud status update preserved spec.
  - `TestApplyRequiresFieldManagerAndUsesServerSideApply` proved empty apply field manager fails locally, then server-side apply created and updated Group spec and created NetworkCloud spec using `application/apply-patch+yaml`.
  - `TestDeleteRemovesTypedObject` proved typed delete makes later typed get return Kubernetes not-found for both resources.
  - `TestWatchEmitsTypedEventsForFieldSelector` proved watches with typed field filters emit typed `*NSXGroup` and `*NSXNetworkCloud` added events.
  - `TestInvalidResourceSpecificFiltersFailBeforeRequest` proved resource-specific invalid filters fail with validation errors.
  - `TestConstructorValidationNilLoggerAndStructuredLogs` proved nil config fails, nil logger is accepted, and zap observer sees structured resource fields.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test -race ./internal/kubeapi -count=1` passed.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test -coverprofile=/tmp/kubeapi.cover ./internal/kubeapi -count=1 && go tool cover -func=/tmp/kubeapi.cover` passed with `internal/kubeapi` coverage `80.6%`.
- `make check` passed: gofumpt, golangci-lint (`0 issues`), `go test ./...`, and `go test -cover ./...`.
- `make test` passed: all packages `ok`.
- `make test-coverage` passed with package coverage: `api/v1alpha 100.0%`, `cmd/nsx-operator 81.6%`, `internal/buildinfo 100.0%`, `internal/config 82.9%`, `internal/httpratelimit 87.8%`, `internal/kubeapi 80.6%`, `internal/logging 96.2%`, `internal/nsxclient 80.3%`, `internal/startup 82.8%`.
- Final boundary review: `internal/kubeapi` owns raw resource names and selector rendering, exposes typed resource clients and typed status update methods only, has no raw `[]byte`/JSON status patch API, and adds no duplicate CR DTO layer.
</verification>

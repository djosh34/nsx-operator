## Task: Add Global And Per-NetworkCloud NSX Write Disable Controls <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add explicit controls that can disable every non-GET write call to NSX before it reaches any NSX manager, while preserving read/list behavior. The operator needs a global configuration field that disables all NSX write calls, and each NetworkCloud custom resource also needs its own field that enables or disables NSX write calls for that specific NetworkCloud.

The global configuration field is a hard override for all non-GET calls to NSX. When global writes are disabled, no per-NetworkCloud setting may re-enable NSX writes. When global writes are enabled, the per-NetworkCloud field decides whether that NetworkCloud may perform non-GET NSX calls. "Write calls" means every NSX HTTP method other than GET, including but not limited to POST, PUT, PATCH, and DELETE. GET/list calls to NSX must continue so observe/status behavior can still run.

In scope: add the global config field; add the NetworkCloud CRD/API field; wire the setting through reconciliation and NSX client call sites so writes are blocked before the HTTP request is sent; preserve existing observe/list/get behavior; make status/logging explain when writes are skipped because of global or per-resource configuration; keep zap structured logging; update tests and generated manifests as required by the repo conventions.

Out of scope: changing the semantics of existing PATCH/delete write behavior beyond adding the write gate; adding unrelated fields; changing unrelated reconciliation behavior; adding metrics, except where an existing metric has to remain correct when writes are skipped.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] A global config field exists and, when disabled, prevents all non-GET NSX HTTP calls for every manager and every NetworkCloud.
- [x] A per-NetworkCloud CRD/API field exists and, when disabled, prevents all non-GET NSX HTTP calls for only that NetworkCloud while still allowing GET/list calls.
- [x] The global field overrides the per-NetworkCloud field; per-resource writes cannot be re-enabled when global writes are disabled.
- [x] Tests prove POST, PUT, PATCH, and DELETE NSX calls are blocked when writes are disabled, and GET/list calls still occur.
- [x] Tests prove blocked writes do not reach the sibling `../nsx-t-mockapi` service or equivalent request recorder.
- [x] Structured zap logs include manager/resource/function context and whether writes were skipped because of global config or per-NetworkCloud config.
- [x] Existing normal, contract, and relevant e2e test gates pass, including `make test`, `make test-contract`, and any CRD/API generation verification used by this repo.
</acceptance_criteria>

<verification_evidence>
- Config/API:
  - `go test ./internal/config -run TestLoadNSXWritesEnabledDefaultsTrueAndAllowsFalse -count=1` passed.
  - `KUBEBUILDER_ASSETS="$(./.bin/setup-envtest use 1.32.x -p path)" go test ./api/v1alpha -run TestCRDsInstallStatusSubresourceSelectableFieldsAndSchema -count=1` passed; this verifies `spec.writesEnabled: false` is stored and non-boolean values are rejected by the CRD schema.
- Client write gate:
  - `go test ./internal/nsxclient -run 'TestClientWriteControlBlocksNonGETAndAllowsReadRequests|TestClientAddsBasicAuthToReadAndWriteRequests' -count=1` passed.
  - `TestClientWriteControlBlocksNonGETAndAllowsReadRequests` exercises public POST, PUT, PATCH, DELETE, GET, and list methods. The recording transport only saw:
    - `GET /policy/api/v1/infra/domains/default/groups/app`
    - `GET /policy/api/v1/infra/domains/default/groups`
  - The same test asserts structured zap fields on the skipped-write info log: `method`, `url`, `path`, `writeDisabledReason`, `networkCloudName`, and `networkCloudFQDN`.
- Startup/global override:
  - `go test ./internal/startup -run TestWriteControlForCloudAppliesGlobalOverrideAndPerCloudDisable -count=1` passed.
  - This proves global disabled overrides cloud-enabled, global enabled respects cloud disabled, and omitted cloud setting remains enabled.
- Reconcile/status:
  - `go test ./internal/stateoperator -run 'TestGroupReconcileManageWriteDisabled|TestGroupReconcileManageDeleteWriteDisabled|TestApplyManagerPlanWriteDisabled' -count=1` passed.
  - Status conditions use reason `NSXWritesDisabled`; apply/delete conditions are `False`, `Synced` is `Unknown`, and delete keeps the finalizer.
- Mock API/request-recorder non-reachability:
  - `go test ./internal/stateoperator -run TestDisabledNSXWritesDoNotReachMockAPIRecorderWhileReadsStillDo -count=1` passed.
  - The test uses the existing mock API harness/request recorder. Disabled POST, PUT, PATCH, and DELETE returned `WriteDisabledError` and the recorder stayed empty. A subsequent `ListGroups` GET reached the recorder as `GET /policy/api/v1/infra/domains/default/groups`.
- Required gates on final tree:
  - `make check` passed. It included gofumpt, `go vet ./...`, `golangci-lint run ./...`, `go test ./...`, `go test -race ./...`, contract tests, focused lifecycle/mock API tests, largechaos tests, and coverage.
  - `make test` passed.
  - `make test-contract` passed.
  - `make test-coverage` passed with total coverage `83.4%`, above the `80.0%` threshold.
  - New write-control functions reported by `go tool cover -func=coverage.out`:
    - `api/v1alpha.NSXNetworkCloudSpec.NSXWritesEnabled`: `100.0%`
    - `internal/nsxclient.(*Client).requireWriteEnabled`: `100.0%`
    - `internal/nsxclient.(*Client).requestURL`: `100.0%`
    - `internal/startup.writeControlForCloud`: `100.0%`
    - `internal/stateoperator.isWriteDisabled`: `100.0%`
    - `internal/stateoperator.writeDisabledConfigName`: `100.0%`
- Boundary review:
  - Final improve-code-boundaries review kept policy centralized at the `internal/nsxclient` HTTP boundary, kept global/per-cloud decision composition in `internal/startup`, and limited stateoperator changes to status classification and sweep continuation after a typed `WriteDisabledError`.
  - No duplicated write-enable checks were added to individual route methods or reconciler call sites.
</verification_evidence>

<plan>
.ralph/tasks/22-story-nsx-write-disable-controls/01-task-add-nsx-write-disable-controls_plans/plan-20260519-nsx-write-disable-controls.md
NOW EXECUTE
</plan>

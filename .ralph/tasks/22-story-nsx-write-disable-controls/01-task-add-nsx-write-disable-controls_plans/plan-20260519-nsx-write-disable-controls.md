# Plan: NSX Write Disable Controls

Task: `.ralph/tasks/22-story-nsx-write-disable-controls/01-task-add-nsx-write-disable-controls.md`

## Current Design Reading

- `internal/nsxclient.Client` is the narrowest NSX HTTP boundary. Typed route methods call `Client.do` for single requests and `listAllTyped` for GET list pages. Enforcing the write gate before `httpClient.Do` in this package blocks every POST, PUT, PATCH, DELETE call before any NSX manager or mock API can observe it, without duplicating checks across reconciler code.
- `internal/startup.NewManager` already builds a per-`NSXNetworkCloud` manager client from global config plus the cloud CR. This is the right place to merge global and per-cloud settings into one `nsxclient.Options` value.
- `internal/stateoperator` plans managed writes/deletes separately from GET/list observation. The gate should not remove planned status work, but status/logs must explain when a planned NSX write was skipped by config.
- API and CRD definitions are hand-maintained in `api/v1alpha/types.go` and `config/crd/bases/*.yaml`; CRD behavior is verified through envtest in `api/v1alpha/crd_integration_test.go`.

## Interface Design

- Add global config field `nsx.writesEnabled` parsed into `config.NSXConfig.WritesEnabled bool`.
  - Default to `true` when omitted so existing configs preserve behavior.
  - Accept only YAML booleans through the existing yaml decoder.
  - Log it in startup debug output as `nsx_writes_enabled`.
- Add per-cloud API field `spec.writesEnabled` to `api/v1alpha.NSXNetworkCloudSpec`.
  - Use `*bool` in Go with JSON tag `writesEnabled,omitempty` so old CRs deserialize as unset.
  - Add helper method or function with one public semantic: unset means enabled, false means disabled.
  - Add CRD schema property `writesEnabled` with `type: boolean`, `default: true`, and a short description.
  - Include an additional printer column `WritesEnabled` if it stays readable; do not make it selectable unless there is a real query need.
- Add `nsxclient.WriteControl` or equivalent deep-module type in `internal/nsxclient` with:
  - `Enabled bool`
  - `Reason string` values such as `global_config`, `network_cloud`
  - `NetworkCloudName string`
  - `NetworkCloudFQDN string`
- Add `nsxclient.Options.WriteControl`.
  - If omitted, writes remain enabled.
  - `Client.do` checks `method != http.MethodGet` before `newRequest`/`httpClient.Do`, logs an info event, and returns a typed sentinel error such as `nsxclient.WriteDisabledError`.
  - `listAllTyped` remains GET-only and does not check the gate.
- In `startup.NewManager`, compute effective writes as:
  - global false -> disabled reason `global_config`
  - global true and cloud `spec.writesEnabled == false` -> disabled reason `network_cloud`
  - otherwise enabled
  - Pass manager/resource context into `nsxclient.Options.WriteControl` so logs include `networkCloudName`, normalized `networkCloudFQDN`, and reason.
- In `stateoperator`, classify `nsxclient.WriteDisabledError` separately from NSX/network failures.
  - Manage apply should set `Applying=False`, `Synced=Unknown`, reason `NSXWritesDisabled`, message explaining whether global config or NetworkCloud config skipped the write.
  - Manage delete should set `Deleting=False`, `Synced=Unknown`, reason `NSXWritesDisabled`, with a delete-specific message.
  - Default manager sweep should continue updating Kubernetes statuses and cloud status after a skipped write when possible; no NSX write should be attempted after the first blocked operation in that apply path.

## Improve-Code-Boundaries Use

- Keep policy at the `nsxclient` HTTP boundary, not scattered across every route method or reconciler call site.
- Keep effective decision construction in startup, where global config and per-cloud resource data are both already available.
- Avoid adding a parallel "dry-run manager client" wrapper unless tests prove the client-boundary approach cannot produce the required status/log context.
- Do not duplicate global/per-cloud booleans through every manager plan type. Plans should remain about desired state; write admission belongs to the client boundary and error/status classification.

## TDD Execution Plan

Use vertical red-green slices. For each slice: write one behavior test, run the focused failing test to confirm RED, implement only enough code for GREEN, then run the focused package tests before moving on.

1. Config slice
   - RED: add `TestLoadNSXWritesEnabledDefaultsTrueAndAllowsFalse` in `internal/config/config_test.go`.
   - GREEN: add raw/config fields and defaulting.
   - Verify: `go test ./internal/config -run TestLoadNSXWritesEnabledDefaultsTrueAndAllowsFalse -count=1`.

2. API/CRD slice
   - RED: extend `api/v1alpha/crd_integration_test.go` to create one cloud with `writesEnabled: false`, assert it is stored as false, and assert invalid non-boolean values are rejected.
   - GREEN: update `api/v1alpha/types.go`, `api/v1alpha/deepcopy.go` if needed, and `config/crd/bases/nsx.ing.com_nsxnetworkclouds.yaml`.
   - Verify through make/envtest path: `make test-e2e` or focused `KUBEBUILDER_ASSETS="$(./.bin/setup-envtest use 1.32.x -p path)" go test ./api/v1alpha -run TestCRDsInstallStatusSubresourceSelectableFieldsAndSchema -count=1`.

3. NSX client gate slice
   - RED: add one table-driven behavior test in `internal/nsxclient/client_test.go` proving POST, PUT, PATCH, DELETE are blocked and GET/list still reaches the recording transport when writes are disabled.
   - Include an observer logger assertion for structured fields: method, URL/path, write disable reason, network cloud name/FQDN.
   - GREEN: implement `WriteDisabledError`, write control options/defaulting, `Client.do` gate before request send, and structured zap info log.
   - Verify: `go test ./internal/nsxclient -run 'Test.*Write.*Disabled|TestClientAddsBasicAuth' -count=1`.

4. Mock API non-reachability slice
   - RED: add a mock API backed test, preferably in `internal/stateoperator/manager_pipeline_write_semantics_test.go` or `internal/nsxclient/contract_test.go`, where a disabled client attempts POST/PUT/PATCH/DELETE and the request recorder remains empty while GET/list still records.
   - GREEN: reuse the client gate; do not add a second check.
   - Verify: focused `go test` for the package/test.

5. Startup wiring slice
   - RED: add `internal/startup/manager_test.go` coverage proving global false overrides a cloud with `writesEnabled: true`, and global true respects cloud false.
   - If `NewManager` is too hard to observe directly, extract a small unexported `writeControlForCloud(config.NSXConfig, nsxv1alpha.NSXNetworkCloud)` function and test it through startup package tests.
   - GREEN: wire `Options.WriteControl` in the `managerClientFactory`.
   - Verify: `go test ./internal/startup -run 'Test.*Write' -count=1`.

6. Reconciler/status slice
   - RED: add behavior tests in `internal/stateoperator/operator_test.go`:
     - manage apply blocked by global config/classified `WriteDisabledError` updates status and does not requeue as an unclassified error.
     - manage delete blocked by per-cloud config updates status and does not remove the finalizer.
   - GREEN: add status classification for `WriteDisabledError` in apply/delete failure handling.
   - Verify: focused stateoperator tests.

7. Manager sweep slice
   - RED: add or extend manager pipeline tests proving a sweep with planned managed writes/deletes keeps GET/list observation but skips NSX writes via the disabled client and records skipped-write statuses/logs.
   - GREEN: likely classification only; if `ApplyManagerPlan` currently aborts too early for multiple status updates, adjust narrowly without changing plan semantics.
   - Verify: focused stateoperator tests.

8. Boundary refactor pass
   - Run the improve-code-boundaries review against the changed code.
   - Remove any duplicated write-enable logic or bool plumbing found outside startup/client/status classification.
   - Run focused tests after each refactor step.

9. Required final checks
   - `make check`
   - `make test`
   - `make test-coverage`
   - Confirm total coverage is at least 80% and new write-gate code is covered at 80%+ by focused package coverage evidence.
   - Also run `make test-contract` explicitly if not relying on `make check` output in the task evidence.

## Manual Verification Plan

- Use sibling `../nsx-t-mockapi` through existing testcontainers/mock API helpers where available.
- Record concrete evidence in the task file:
  - focused test command output showing blocked POST/PUT/PATCH/DELETE and allowed GET/list;
  - recorder/mock API request list proving disabled writes did not reach the mock service;
  - sample structured zap JSON log line with manager/resource/function context and disable reason;
  - final `make check`, `make test`, and `make test-coverage` outputs.

## Files Expected To Change

- `internal/config/config.go`
- `internal/config/config_test.go`
- `api/v1alpha/types.go`
- `api/v1alpha/deepcopy.go` if the pointer bool requires generated-style copy handling
- `api/v1alpha/crd_integration_test.go`
- `config/crd/bases/nsx.ing.com_nsxnetworkclouds.yaml`
- `hack/compose/nsx-operator-config.yaml`
- `internal/nsxclient/client.go`
- `internal/nsxclient/errors.go` or a new small file in `internal/nsxclient`
- `internal/nsxclient/client_test.go`
- `internal/nsxclient/contract_test.go` if mock API non-reachability best fits there
- `internal/startup/manager.go`
- `internal/startup/manager_test.go`
- `internal/stateoperator/reconciler.go`
- `internal/stateoperator/operator_test.go`
- `internal/stateoperator/manager_pipeline_write_semantics_test.go`
- `.ralph/tasks/22-story-nsx-write-disable-controls/01-task-add-nsx-write-disable-controls.md`

NOW EXECUTE

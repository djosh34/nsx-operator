# Final Acceptance Evidence Matrix

Task: `.ralph/tasks/18-story-acceptance-hardening/01-task-verify-full-acceptance-criteria.md`

Plan: `.ralph/tasks/18-story-acceptance-hardening/01-task-verify-full-acceptance-criteria_plans/01-plan-final-acceptance-evidence.md`

Evidence directory: `.ralph/tasks/18-story-acceptance-hardening/evidence/20260519T032638Z/`

## Command Summary

| Command or check | Evidence | Result |
| --- | --- | --- |
| Environment capture | `environment.txt` | Captured UTC/local date, host, git commit/status, Go version, Docker status, envtest version target, mockapi path/commit/status. |
| `make test` | `make-test.log` | PASS. |
| CRD/envtest verification | `crd-verification.log` | PASS. Envtest Kubernetes API server version `1.32.0`; both CRDs Established; schemas/status subresources/selectable fields exercised through the API server. |
| Mockapi contracts and lifecycle | `mockapi-contract.log` | PASS. NSX typed client contracts, shared host+port limiter behavior, Observe/Manage lifecycle, and selected expression writes verified against `../nsx-t-mockapi`. |
| Runtime/controller/pipeline evidence | `runtime-verification.log`, `runtime-verification-envtest-correction.log` | PASS after corrected envtest invocation. The first runtime log contains a command construction failure for two envtest-dependent tests; `runtime-verification-envtest-correction.log` reruns those tests with `KUBEBUILDER_ASSETS` and passes. |
| Large/chaos scenarios | `large-chaos.log`, `large-chaos-verbose.log` | PASS. Verbose evidence includes 2000 remote groups, 10000 real CRs, 5000 managed writes, 5000 observe deletes, and limiter/unavailable chaos evidence. |
| Explicit race run | `race.log` | PASS for `go test -race ./...` with envtest assets. |
| `make test-coverage` | `make-test-coverage.log` | PASS. Total coverage `82.7%`, threshold `80.0%`. |
| `make check` | `make-check.log` | PASS. Includes fmt, vet, lint, test, race, contract, e2e, large/chaos, and coverage. |

## Acceptance Mapping

| Acceptance item | Evidence |
| --- | --- |
| CRDs install on Kubernetes >= 1.32 | `crd-verification.log`: envtest Kubernetes API server version `1.32.0`; `CRD nsxnetworkclouds.nsx.ing.com is Established`; `CRD nsxgroups.nsx.ing.com is Established`. |
| API version is `nsx.ing.com/v1alpha` | `crd-verification.log`: dynamic clients use `GroupName` and `Version` from the public API package and successfully create/list `nsxnetworkclouds` and `nsxgroups`; `api/v1alpha/types_test.go` is included in `make-test.log` and verifies JSON/public API shape. |
| CRDs include schemas | `crd-verification.log`: both CRDs expose status schema and reject invalid mode, invalid `segment_path`, missing `cidrs`, and missing `networkCloudFQDN` through the API server. |
| CRDs include status subresources | `crd-verification.log`: status subresource updates for `cloud-a` and `group-a` kept specs unchanged and stored conditions. |
| CRDs include selectable fields | `crd-verification.log`: field selectors for `spec.networkCloudFQDN`, `spec.networkCloudId`, `spec.groupID`, and `spec.mode` returned the expected objects. |
| `domainId` absent from CRD specs | `make-test.log`: `api/v1alpha` package passed; `TestJSONShapeUsesPublicAPIFieldNames` rejects `domainId` in both public specs. |
| `NSXGroup` spec/status and `NSXNetworkCloud` status match design | `crd-verification.log`, `make-test.log`, and `make-test-coverage.log`: API shape, status conditions, validation, and status-only updates pass through public package and envtest coverage. |
| Each `cidrs` item maps one-to-one unchanged to NSX `IPAddressExpression.ip_addresses` | `mockapi-contract.log`: `TestManagedWriteUsesSelectedPatchEndpointsAndPreservesUnrelatedMockAPIExpressions`; `make-test.log`: stateoperator write semantics and manager pipeline tests passed. |
| Observe import/update/delete works | `mockapi-contract.log`: `TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI`; `runtime-verification.log`: `TestProcessManagerSnapshotImportsRemoteOnlyGroupsAsObserveUpserts` and `TestProcessManagerSnapshotObserveGroupsMirrorRemoteAndDeleteWhenMissing`. |
| Manage patch/delete uses documented PATCH endpoints | `mockapi-contract.log`: nsxclient route contracts include `policy.groups.patch`, `policy.groups.ip_address_expressions.patch`, and stateoperator selected-expression write tests. |
| Manage delete waits for confirmed absence | `runtime-verification-envtest-correction.log`: `TestDefaultManagerSweepRemovesManagedFinalizerAfterConfirmedRemoteAbsence`; `runtime-verification.log`: `TestProcessManagerSnapshotDeletingManageGroupPlansFinalizerRemovalAfterRemoteAbsence`. |
| controller-runtime manager and `Reconcile` registered | `runtime-verification.log`: `TestNewManagerRegistersControllersAndPeriodicSweeper` and reconcile tests for both CRDs passed. |
| `Start` ticker is non-overlapping and skips elapsed ticks | `runtime-verification.log`: `TestStartSkipsElapsedTicksAfterLongSweep` and `TestStartDoesNotOverlapSweeps` passed. |
| One goroutine per cloud per sweep | `runtime-verification.log`: `TestStartImmediatelySweepsAllNetworkClouds` and `TestStartWaitsForHealthyCloudWhenAnotherCloudFails` are covered by package tests in `make-test.log`; scheduler evidence confirms per-cloud sweep behavior. |
| Gather/process/apply pipeline exists | `runtime-verification.log`: gather, process, apply, default sweep, and operation ordering tests passed. |
| Process stage has zero client calls | Boundary pass plus `runtime-verification.log`: `ProcessManagerSnapshot` tests operate on a pure `ManagerSnapshot` and do not require Kubernetes or NSX clients; client calls are isolated in `GatherManagerSnapshot` and `ApplyManagerPlan`. |
| Kube client exposes field filters | `runtime-verification.log`: kube client field selector tests passed; `crd-verification.log` confirms API server selectable field behavior. |
| HTTP limiter is generic per host+port and blocks | `mockapi-contract.log`: shared limiter blocks same logical host+port while allowing different port; `runtime-verification.log`: httpratelimit package tests passed. |
| NSX client uses global Basic Auth | `runtime-verification.log`: `TestClientAddsBasicAuthToReadAndWriteRequests`; `mockapi-contract.log` validates authenticated mockapi client contracts. |
| NSX client stream-decodes list results | `runtime-verification.log`: `TestDecodeListResultsStreamsTypedPointers`. |
| NSX client paginates | `runtime-verification.log`: `TestListMethodsFollowPaginationUntilCursorIsEmpty`; `runtime-verification.log` also covers NSX pagination in snapshot gather. |
| NSX client does not auto-refetch/reapply | `runtime-verification.log`: `TestWriteStatusErrorsAreTypedAndNotRetried`; reconciliation tests record no speculative requeue/refetch behavior. |
| Zap logs JSONL to stderr | `runtime-verification.log`: logging and `cmd/nsx-operator` JSONL stderr tests passed; credential material is not logged. |
| `go test -race ./...` passes | `race.log`; also repeated inside `make-check.log`. |
| 80%+ coverage | `make-test-coverage.log` and `make-check.log`: total coverage `82.7% meets 80.0% threshold`; package coverage for touched runtime packages is at or above 80%. No production code was added in this verification task. |
| Large e2e and chaos scenarios pass | `large-chaos.log`, `large-chaos-verbose.log`, and `make-check.log`. |

## Boundary Pass

No production code changed during this task. The final `$improve-code-boundaries` pass inspected the relevant surfaces and found the existing split still clean:

- `GatherManagerSnapshot` owns Kubernetes/NSX reads.
- `ProcessManagerSnapshot` is a pure planning function over `ManagerSnapshot`.
- `ApplyManagerPlan` owns Kubernetes/NSX writes.
- Startup builds clients/logger/manager and keeps bootstrap concerns out of state processing.
- NSX client, HTTP limiter, kube API, and logging remain in separate modules with small public interfaces.

No follow-up task is required from this verification run.

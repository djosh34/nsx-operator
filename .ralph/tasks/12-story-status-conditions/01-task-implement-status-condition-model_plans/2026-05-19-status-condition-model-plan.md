## Plan: Status Condition Model

Task: `.ralph/tasks/12-story-status-conditions/01-task-implement-status-condition-model.md`

### Current State

- `api/v1alpha` already defines both status structs as `[]metav1.Condition` only and the CRD schemas expose only `status.conditions`.
- The missing behavior is the condition model itself: `internal/stateoperator/manager_pipeline.go` currently builds conditions through private ad hoc functions, resets `lastTransitionTime` on every status plan, and often writes `ObservedGeneration: 0`.
- The sweep pipeline also omits several condition types from the task scope: `Realized`, explicit `Applying=False`, `Deleting=False`, and `Reachable/Swept=True` on successful manager sweeps.
- Existing pipeline tests cover some status outcomes but do not prove transition-time preservation, generation handling, synced derivation, or Unknown condition rules.

### Public Interface And Boundary Design

Use `$improve-code-boundaries` by moving status rule construction out of `manager_pipeline.go` into a focused internal condition model package:

```go
package statuscondition

type ConditionUpdate struct {
	Type    string
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

func BuildNetworkCloudStatus(previous nsxv1alpha.NSXNetworkCloudStatus, observedGeneration int64, now time.Time, updates ...ConditionUpdate) nsxv1alpha.NSXNetworkCloudStatus
func BuildGroupStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, now time.Time, updates ...ConditionUpdate) nsxv1alpha.NSXGroupStatus

func Reachable(status metav1.ConditionStatus, reason string, message string) ConditionUpdate
func Swept(status metav1.ConditionStatus, reason string, message string) ConditionUpdate
func RemotePresent(status metav1.ConditionStatus, reason string, message string) ConditionUpdate
func SpecMatchesRemote(status metav1.ConditionStatus, reason string, message string) ConditionUpdate
func UnsupportedExpression(status metav1.ConditionStatus, reason string, message string) ConditionUpdate
func Realized(status metav1.ConditionStatus, reason string, message string) ConditionUpdate
func Applying(status metav1.ConditionStatus, reason string, message string) ConditionUpdate
func Deleting(status metav1.ConditionStatus, reason string, message string) ConditionUpdate
func Synced(remotePresent metav1.ConditionStatus, specMatchesRemote metav1.ConditionStatus, unsupportedExpression metav1.ConditionStatus, realized metav1.ConditionStatus, reason string, message string) ConditionUpdate
```

Rules for the package:

- The package owns type validation, ordering, merge behavior, generation assignment, and transition-time preservation.
- `lastTransitionTime` changes only when a condition type is new or its `Status` changes. Reason/message changes without a status transition keep the previous transition time.
- `ObservedGeneration` is always set to the object generation passed by the caller.
- Status condition order is deterministic:
  - Network cloud: `Reachable`, `Swept`
  - Group: `RemotePresent`, `SpecMatchesRemote`, `UnsupportedExpression`, `Realized`, `Synced`, `Applying`, `Deleting`
- `Synced` is derived only from input condition statuses:
  - `True` when `RemotePresent=True`, `SpecMatchesRemote=True`, `UnsupportedExpression=False`, and `Realized=True`.
  - `False` when any known input makes sync impossible, including `RemotePresent=False`, `SpecMatchesRemote=False`, `UnsupportedExpression=True`, or `Realized=False`.
  - `Unknown` when no known-false input exists but one or more required inputs is `Unknown`.
- `BuildGroupStatus` and `BuildNetworkCloudStatus` reject unknown condition types by returning no silent success. Prefer returning `(status, error)` if implementation needs validation; if using panic would be the alternative, change the interface to include an error and update the call sites to check it. Do not ignore errors.

### Stateoperator Integration

- Replace `condition`, `missingManageStatus`, `applyingManageStatus`, `matchingManageStatus`, and `syncedRemoteStatus` with calls into the condition model.
- Feed previous status into status builders:
  - `snapshot.Cloud.Status` for cloud status plans.
  - `localBinding.Group.Status` for existing group status plans.
  - empty status for remote-only Observe upserts.
- Set `ObservedGeneration` from `snapshot.Cloud.Generation` or `localBinding.Group.Generation`; for new Observe upserts use the generation available at creation time, which is `0` before the API server assigns a generation.
- On successful gather, plan cloud status with `Reachable=True` and `Swept=True`.
- On gather failure, plan cloud status with `Reachable=False` and `Swept=False`, and no child group mass updates.
- For Observe groups with remote present, set:
  - `RemotePresent=True`
  - `SpecMatchesRemote=True` after the local spec is brought to the remote projection
  - `UnsupportedExpression=True/False` from the remote projection report
  - `Realized=True/False/Unknown` from remote NSX state
  - derived `Synced`
  - `Applying=False`
  - `Deleting=False`
- For Manage groups:
  - Remote missing: `RemotePresent=False`, `SpecMatchesRemote=False` or `Unknown` if comparison is impossible, `UnsupportedExpression=False`, `Realized=Unknown`, derived `Synced=False`, `Applying=True`, `Deleting=False`.
  - Remote drifted and write planned: `RemotePresent=True`, `SpecMatchesRemote=False`, `UnsupportedExpression` from remote, `Realized` from remote when known, derived `Synced=False`, `Applying=True`, `Deleting=False`.
  - Remote matched success: `RemotePresent=True`, `SpecMatchesRemote=True`, `UnsupportedExpression=False`, `Realized=True`, derived `Synced=True`, `Applying=False`, `Deleting=False`.
  - Remote matched in progress: `RemotePresent=True`, `SpecMatchesRemote=True`, `UnsupportedExpression=False`, `Realized=Unknown`, derived `Synced=Unknown` if the condition model determines only realization is unknown, otherwise `False` if the task/design requires conservative unsynced. If this conflicts with the design during implementation, switch the task marker back to `TO BE VERIFIED`.
  - Remote matched failure: `RemotePresent=True`, `SpecMatchesRemote=True`, `UnsupportedExpression=False`, `Realized=False`, derived `Synced=False`, `Applying=False`, `Deleting=False`.

### TDD Execution Plan Using `$tdd`

Use vertical red-green cycles. Do not write all tests first.

1. [x] Tracer bullet: condition merge preserves transition time.
   - RED: add a behavior test for `BuildGroupStatus` with previous `RemotePresent=True` at `oldTime`; rebuild it as `RemotePresent=True` at `newTime` and assert `LastTransitionTime` remains `oldTime`, reason/message/generation update, and no other fields are added.
   - GREEN: implement the smallest condition builder and one helper needed to pass.

2. [x] Transition on status change.
   - RED: extend with one test where previous `RemotePresent=True` becomes `RemotePresent=False`; assert `LastTransitionTime` becomes `newTime`.
   - GREEN: implement status-change detection.

3. [x] Synced derivation truth table.
   - RED: table-test `Synced` through the public helper for True, False, and Unknown cases, including unsupported expression and unknown realization.
   - GREEN: implement derivation using condition statuses, not message strings.

4. [x] Deterministic condition sets for all named helpers.
   - RED: test group status builder with all group condition helpers and cloud status builder with both cloud helpers; assert condition order, type names, observedGeneration, and Unknown statuses are preserved.
   - GREEN: complete helper constructors and deterministic ordering.

5. [x] Pipeline gather failure uses the model.
   - RED: update the existing gather-failure test to include previous cloud status and assert `Reachable=False`, `Swept=False`, observedGeneration, transition-time preservation, and no group status updates.
   - GREEN: wire cloud failure status through `statuscondition`.

6. [x] Pipeline successful gather updates cloud status.
   - RED: add a `ProcessManagerSnapshot` success test that asserts `CloudStatus` sets `Reachable=True` and `Swept=True`.
   - GREEN: plan successful cloud status.

7. [x] Observe status contains the full group condition set.
   - RED: update remote-only/existing Observe tests to assert `RemotePresent`, `SpecMatchesRemote`, `UnsupportedExpression`, `Realized`, `Synced`, `Applying`, and `Deleting`, including an unsupported-expression case.
   - GREEN: replace observe status construction with condition-model calls.

8. [x] Manage status contains True/False/Unknown rules.
   - RED: add or update manage tests for missing, drifted, matched success, matched in-progress, and matched failure. Assert each relevant condition status, especially Unknown cases.
   - GREEN: replace manage status construction with condition-model calls and remote realization mapping.

9. [x] CRD/object verification proves status conditions only.
   - RED: strengthen the CRD integration test to inspect the CRD schemas and representative stored objects, asserting `status` has only the `conditions` property and rejecting or ignoring non-condition status fields through the Kubernetes API server.
   - GREEN: adjust CRD schema only if verification exposes drift.

10. [x] Boundary/refactor pass.
   - Remove the old private condition constructors and duplicated status helpers from `manager_pipeline.go`.
   - Keep remote projection/comparison in stateoperator and condition mechanics in `internal/statuscondition`.
   - Run focused tests after each refactor step.

### Required Verification

- `go test ./internal/statuscondition`
- `go test ./internal/stateoperator`
- `go test ./api/v1alpha`
- `make check`
- `make test`
- `make test-coverage`
- Coverage for new code must be at least 80%, and global `make test-coverage` must also report at least 80%.
- Manual verification evidence must be appended to the task file before marking `<passes>true</passes>`, including the exact commands and relevant output proving conditions-only CRD/object status.
- Final `$improve-code-boundaries` review must scan for condition construction outside the condition model, duplicate status DTOs, string-derived business logic, ignored errors, and any non-condition status fields.

NOW EXECUTE

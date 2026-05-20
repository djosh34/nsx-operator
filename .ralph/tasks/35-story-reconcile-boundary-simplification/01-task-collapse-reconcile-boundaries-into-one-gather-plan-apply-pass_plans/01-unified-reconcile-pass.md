# Unified Reconcile Pass Plan

Task: `.ralph/tasks/35-story-reconcile-boundary-simplification/01-task-collapse-reconcile-boundaries-into-one-gather-plan-apply-pass.md`

## Current State Read

- `internal/stateoperator/manager_pipeline.go` already has the closest useful shape: `GatherManagerSnapshot`, `ProcessManagerSnapshot`, `ApplyManagerPlan`, and separate Kubernetes/NSX write plans.
- `internal/stateoperator/reconciler.go` does not reconcile; controller-runtime events only log.
- `internal/stateoperator/operator.go` owns sweep cloud iteration and calls a per-cloud sweep function, so the event path and periodic path are not forced through the same pass boundary.
- `internal/stateoperator/manager_kube_writes.go` has the desired batched Kubernetes write boundary, but `ManagerPlan` still carries legacy slices (`ObserveUpserts`, `ManagedWrites`, `GroupStatuses`, etc.) that duplicate the typed write sets.
- Existing tests already cover many preserve-behavior cases: observe import, observe drift repair, observe deletion, manage apply, manage deletion, finalizers, status conditions, large-count behavior, mockapi lifecycle behavior, and duplicate no-GET behavior inside the current batch applier.

## Skills Applied

- `$tdd`: execute as vertical red/green/refactor slices. Each new test must fail first and verify observable behavior through public package APIs or real HTTP/Kubernetes surfaces.
- `$improve-code-boundaries`: remove the split boundary instead of adding a facade. The cleanup target is duplicate plan shapes and event/sweep paths that can bypass the shared pass.

## Target Boundary

Introduce one production reconcile pass runner used by controller-runtime event reconcilers and the periodic operator:

```go
type ReconcileTriggerKind string

const (
	ReconcileTriggerSweep        ReconcileTriggerKind = "sweep"
	ReconcileTriggerNetworkCloud ReconcileTriggerKind = "networkCloud"
	ReconcileTriggerGroup        ReconcileTriggerKind = "group"
)

type ReconcileTrigger struct {
	Kind ReconcileTriggerKind
	Name string
	Sweep SweepContext
}

type ReconcilePassRunner interface {
	RunReconcilePass(ctx context.Context, trigger ReconcileTrigger) error
}
```

The default implementation should:

- Gather Kubernetes state exactly once per pass into an in-memory snapshot:
  - one `NSXNetworkCloud` list
  - one `NSXGroup` list
  - no controller-runtime `Get` during production pass execution
- Narrow the pass in memory:
  - sweep trigger processes every gathered cloud
  - network cloud event processes the gathered cloud with matching name
  - group event finds the gathered group by name, then processes that group's gathered cloud
  - missing event objects are logged as no-op and return success
- For each selected cloud, gather NSX manager groups once, build a `ManagerSnapshot` from the already gathered local groups, process it, then apply through `ApplyManagerPlan`.
- Log with structured zap fields for trigger, gather counts, selected cloud counts, per-cloud local/remote counts, write bucket counts, skipped unchanged statuses, apply counts, dependency ordering, and pass completion.

## TDD Execution Slices

### 1. Controller Events Use The Shared Runner

- [x] RED: Add tests in `internal/stateoperator/operator_test.go` proving `NetworkCloudReconciler.Reconcile` calls a supplied `ReconcilePassRunner` with `ReconcileTriggerNetworkCloud` and the request name.
- [x] GREEN: Add `Runner ReconcilePassRunner` to `NetworkCloudReconciler`; keep nil-runner behavior as log-only for isolated tests if needed.
- [x] RED: Add the same behavior test for `GroupReconciler` with `ReconcileTriggerGroup`.
- [x] GREEN: Add runner support to `GroupReconciler`.
- [x] RED: Add tests proving runner errors are returned and context cancellation is still returned before runner invocation.
- [x] GREEN: Wire error propagation and debug/info logging.

### 2. Startup Injects The Same Production Runner Into Events And Periodic Sweep

- [x] RED: Add startup test proving the registered `NetworkCloudReconciler`, `GroupReconciler`, and operator receive the same production pass runner dependencies when `NewManager` builds normally.
- [x] GREEN: Build one default pass runner in `internal/startup/manager.go` and pass it to `stateoperator.New` plus both reconcilers.
- [x] Keep existing test override seams (`SweepCloud`, fake managers) working where they are explicitly test-only, but production construction must not create separate reconcile paths.

### 3. One-Pass Kubernetes Gather Snapshot

- [x] RED: Add a pass-runner test with an instrumented HTTP or fake typed client proving a sweep pass performs one cloud list and one group list, then applies selected cloud plans without any Kubernetes `Get`.
- [x] GREEN: Add `GatherReconcilePassSnapshot` and `ProcessReconcilePass` style functions in a new focused file such as `internal/stateoperator/reconcile_pass.go`.
- [x] RED: Add event-scope tests:
  - network cloud event with existing cloud processes only that cloud
  - group event with existing group processes only the group's cloud
  - missing cloud/group event logs no-op and does not call NSX manager construction
- [x] GREEN: Implement in-memory narrowing from the single gathered cloud/group lists.
- [x] RED: Add duplicate-query regression coverage that fails if the same Kubernetes resource is fetched/listed again after gather during the same pass.
- [x] GREEN: Ensure manager snapshot construction consumes gathered groups directly and does not call `kubeClient.Groups().List` from inside per-cloud processing.

### 4. Periodic Operator Runs The Same Pass

- [x] RED: Update operator tests so `NSXStateOperator.Start`/`runSweep` uses `RunReconcilePass` with `ReconcileTriggerSweep` instead of independently listing clouds and invoking per-cloud sweep logic.
- [x] GREEN: Replace production `runSweep` implementation with the shared runner call.
- [x] Preserve `SweepContext.ID` in logs and trigger fields.
- [x] Keep or shrink `CloudSweepFunc` only as a compatibility/test injection if needed; production default should not use it.

### 5. Preserve Manager Behaviors Through The New Pass

- [x] RED/GREEN one behavior at a time, reusing existing tests where possible and adding event-trigger variants where coverage is missing:
  - observe import creates local observe group and status after create
  - observe drift repair updates spec and status without losing unrelated finalizers
  - observe missing remote deletes local observe group
  - manage missing remote creates/puts NSX group and updates applying status
  - manage drift patches NSX group and updates applying status
  - manage deletion deletes remote first, then removes finalizer only after remote absence
  - unsupported expression status is preserved
  - network cloud gather failure writes cloud status only
- [x] Use `../nsx-t-mockapi` through existing `internal/testsupport/mockapi` tests for NSX-facing behavior.

### 6. Collapse Duplicate Plan Shapes

Boundary cleanup after the shared pass is green:

- [ ] RED: Adjust tests to assert behavior through `ManagerPlan.KubeWrites` and `ManagerPlan.NSXWrites`, not through duplicated legacy slices.
- [ ] GREEN: Remove `legacyManagerKubeWrites` and `legacyManagerNSXWrites`; `ApplyManagerPlan` must require explicit typed write sets produced by processing.
- [ ] Refactor `ManagerPlan` to keep only:
  - `KubeWrites ManagerKubeWritePlan`
  - `NSXWrites ManagerNSXWritePlan`
  - a compact metrics/log summary if tests and metrics need counts
  - private status write decisions for debug logging
- [ ] Remove or privatize exported plan fields that exist only for tests.
- [ ] Split `manager_pipeline.go` if it remains over 1500 lines after the above:
  - remote normalization
  - snapshot processing
  - NSX apply/status construction
  - metrics/log summaries

### 7. Batch Apply Boundary Hardening

- [x] RED: Add a test that a pass submits at most one `ApplyManagerKubeWrites` call per selected cloud and never calls typed Kubernetes write methods directly from reconcile processing.
- [x] GREEN: Make `DefaultReconcilePassRunner` own the only production apply call.
- [x] RED: Add tests for dependency ordering inside the write plan where status/finalizer writes use prior create/update/status resource versions and no follow-up `Get`.
- [x] GREEN: Keep ordering in `ApplyManagerKubeWrites`; add structured debug fields for dependency source operation/name/resourceVersion.

### 8. Verification Commands

Run all required checks only after implementation is complete:

- [x] `make check`
- [x] `make test`
- [x] `make test-coverage`
- [x] Confirm total coverage is at least 80%.
- [x] Confirm new/changed code is covered at 80%+ using focused package coverage output where practical.
- [x] Final `$improve-code-boundaries` pass: inspect for duplicate local state projections, too-public test-only types, single-use helper clutter, and mixed event/sweep/apply responsibilities. Fix any newly introduced mud before marking the task done.

## Completion Updates

- [x] Record concrete verification evidence in the task file.
- [x] Set `<passes>true</passes>` in the task only after all required checks pass.
- [ ] Run `/bin/bash .ralph/task_switch.sh`.
- [ ] Add all files, including `.ralph` updates and generated coverage files unless ignored.
- [ ] Commit with message prefix `task finished 01-task-collapse-reconcile-boundaries-into-one-gather-plan-apply-pass:`.
- [ ] Push.

NOW EXECUTE

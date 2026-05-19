# Implement Finalizer And Object Lifecycle Rules Plan

Plan path: `.ralph/tasks/13-story-object-lifecycle/01-task-implement-finalizer-and-lifecycle-rules_plans/2026-05-19-finalizer-lifecycle-plan.md`

## Current Reading

- Task file: `.ralph/tasks/13-story-object-lifecycle/01-task-implement-finalizer-and-lifecycle-rules.md`.
- Required skills read: `$tdd` and `$improve-code-boundaries`.
- Relevant code:
  - `internal/stateoperator/reconciler.go` owns event-driven NSXGroup reconcile behavior and already defines `GroupFinalizer = "nsx.ing.com/finalizer"`.
  - `internal/stateoperator/manager_pipeline.go` owns manager sweep gather/plan/apply behavior and already has plan fields for observe upserts/deletes, managed deletes, finalizer removals, and status updates.
  - `internal/stateoperator/operator.go` owns global/cloud sweep orchestration.
  - `internal/startup/manager.go` wires controller-runtime controllers and the default sweep.
  - `internal/nsxclient/contract_test.go` already has a process-based pattern for building/running the sibling `../nsx-t-mockapi`.
- Observed partial implementation from previous stories:
  - Observe imports/upserts from manager sweep include the group finalizer.
  - Manage reconcile adds the group finalizer before writing NSX when the group is not deleting.
  - Observe deletion removes the group finalizer without constructing an NSX manager client.
  - Manage deletion via reconcile submits an NSX delete and keeps the finalizer.
  - Manager sweep removes managed finalizers only after a successful list where the remote group is absent.
- Likely gaps to prove/fix:
  - Existing Observe NSXGroup CRs that the controller manages but did not create need the same finalizer while not deleting.
  - Cloud sweeps should re-check that the NSXNetworkCloud still exists immediately before sweeping, so a cloud deleted after the global list is not swept.
  - The task acceptance requires concrete/manual-style E2E evidence, including Observe versus Manage deletion and cloud deletion behavior, not only unit-level fake-client assertions.

## Boundary Design

- Keep lifecycle decisions inside `internal/stateoperator`; do not move Kubernetes deletion semantics into `internal/kubeapi` or startup wiring.
- Keep NSXGroup finalizer mutation behind one small controller-facing helper in `reconciler.go` if it remains only reconcile behavior. If the same finalizer logic is needed by sweep apply code, extract a minimal helper in `stateoperator` rather than duplicating slice edits.
- Keep manager sweep planning as pure as possible:
  - `ProcessManagerSnapshot` should continue to decide from a successful gathered snapshot whether managed finalizers can be removed.
  - `ApplyManagerPlan` should continue to apply the resulting operations in order: Kubernetes group upserts, NSX writes/deletes, statuses, managed finalizer removals, observe CR deletes, cloud status.
- Keep stale cloud handling in sweep orchestration:
  - `runSweep` can list cloud names.
  - `runCloudSweep` should validate the named cloud still exists before invoking `sweepCloud`.
  - Missing cloud should be debug logged and skipped, not treated as an NSX or Kubernetes failure.
- Do not add finalizers to `NSXNetworkCloud` unless execution proves an existing repo convention requires it. The task explicitly says this is out of scope.
- Do not add owner references from `NSXGroup` to `NSXNetworkCloud`; cloud deletion must not make Kubernetes garbage-collect child group CRs.
- Boundary smell to watch with `$improve-code-boundaries`: avoid adding a second lifecycle pipeline or DTO just for deletion. The current deep module is `ManagerPlan`; extend or reuse it only where it preserves the pure snapshot-to-plan boundary.

## Public Interface And Behavior

- No CRD schema or public Go API change is planned.
- The observable Kubernetes behavior should be:
  - Every NSXGroup the operator creates or actively manages has `metadata.finalizers` containing `nsx.ing.com/finalizer`.
  - Observe deletion removes only `nsx.ing.com/finalizer` and performs no NSX delete.
  - Manage deletion submits an NSX delete when the remote group is still present and keeps `nsx.ing.com/finalizer`.
  - A later successful manager list that no longer includes that managed remote group removes `nsx.ing.com/finalizer`.
  - Deleting an NSXNetworkCloud stops subsequent/default manager sweeps for that cloud after the cloud object disappears.
  - Deleting an NSXNetworkCloud does not delete child NSXGroup CRs and does not require a cloud finalizer.

## TDD Execution Plan

Execute as vertical red-green-refactor cycles. Do not write a batch of tests first.

1. [x] RED: add/adjust one behavior test in `internal/stateoperator/operator_test.go` proving an existing non-deleting Observe NSXGroup gets `GroupFinalizer` and does not construct or call an NSX manager client.
2. [x] GREEN: update `GroupReconciler.Reconcile` to ensure the group finalizer for non-deleting Observe groups using the existing finalizer helper path, then return without NSX mutation.
3. [x] RED: add one behavior test in `internal/stateoperator/operator_test.go` proving a cloud deleted after global list but before cloud sweep is skipped and `SweepCloud` is not called for that missing cloud.
4. [x] GREEN: make `runCloudSweep` re-fetch the cloud by name before invoking `sweepCloud`; skip and debug-log NotFound, return/log real get errors as cloud sweep failures.
5. [x] RED: add or extend an envtest/mockapi E2E test proving Observe and Manage deletion differ:
   - Seed mockapi with a remote group.
   - Create an Observe group with the finalizer, delete it, and verify the Kubernetes finalizer is removed while the mockapi group remains present.
   - Create a Manage group, let reconcile/sweep submit/delete as needed, delete it, and verify the finalizer remains until a successful future mockapi-backed list shows the group absent.
6. [x] GREEN: implement only the minimum code needed for that E2E behavior. If direct reconcile deletion plus later sweep confirmation already satisfies it, do not redesign it.
7. [x] RED: add or extend E2E evidence proving deleting an NSXNetworkCloud does not delete child NSXGroup CRs and stops further manager sweeps for that cloud.
8. [x] GREEN: implement only missing behavior. Do not add owner references or cloud finalizers.
9. [x] Refactor after green:
   - Remove duplicate finalizer slice code if the reconcile changes create duplication.
   - Keep status and finalizer ordering clear: status updates before finalizer removal for managed deletion confirmation.
   - Add structured zap debug/info logs for lifecycle branches and stale cloud skip.

## Concrete Verification

Run these before completion and record exact output/evidence in the task file or linked artifact:

1. [x] `make check`
2. [x] `make test`
3. [x] `make test-coverage`
4. [x] Confirm `make test-coverage` reports 80%+ total coverage and that new lifecycle code paths have meaningful behavior coverage.
5. [x] Run focused E2E tests that exercise envtest plus sibling mockapi, for example:
   - `KUBEBUILDER_ASSETS="$$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run 'Lifecycle|Finalizer|CloudDeletion|DefaultManagerSweep' -count=1 -v`
   - If a new package/test name is created, replace the regex with the actual test names.
6. [x] Capture evidence in the task file:
   - test command outputs,
   - observed finalizer state transitions,
   - mockapi remote group presence/absence after Observe and Manage deletion,
   - cloud deletion showing the child NSXGroup CR remains,
   - logs or counters proving no sweep occurs after the cloud object disappears.

## Completion Steps

- [x] Design remained valid; no switch back to `TO BE VERIFIED` was needed.
- [x] When all checks pass, do a final `$improve-code-boundaries` pass:
  - verify lifecycle code did not add duplicate type shapes or stringly state,
  - verify finalizer logic lives in the stateoperator boundary,
  - verify no cloud owner-reference/finalizer leak was introduced.
- [x] Update `.ralph/tasks/13-story-object-lifecycle/01-task-implement-finalizer-and-lifecycle-rules.md` with concrete evidence and set `<passes>true</passes>`.
- [x] Run `/bin/bash .ralph/task_switch.sh`.
- [x] `git add` all changed files, including `.ralph`.
- [x] Commit with `task finished 01-task-implement-finalizer-and-lifecycle-rules: implement NSXGroup lifecycle finalizers`.
- [x] Push.

DONE

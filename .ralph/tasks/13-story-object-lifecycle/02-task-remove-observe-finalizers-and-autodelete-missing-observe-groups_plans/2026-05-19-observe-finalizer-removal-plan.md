# Remove Observe Finalizers And Autodelete Missing Observe Groups Plan

Plan path: `.ralph/tasks/13-story-object-lifecycle/02-task-remove-observe-finalizers-and-autodelete-missing-observe-groups_plans/2026-05-19-observe-finalizer-removal-plan.md`

## Current Reading

- Task file: `.ralph/tasks/13-story-object-lifecycle/02-task-remove-observe-finalizers-and-autodelete-missing-observe-groups.md`.
- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- Relevant code:
  - `internal/stateoperator/reconciler.go` owns event-driven `NSXGroup` reconcile behavior and defines `GroupFinalizer = "nsx.ing.com/finalizer"`.
  - `internal/stateoperator/manager_pipeline.go` owns manager sweep gather/plan/apply behavior for Observe imports, drift repair, missing-remote deletes, managed writes/deletes, status updates, and managed finalizer removals.
  - `internal/stateoperator/operator.go` owns global/cloud sweep orchestration and already skips clouds that disappeared between global list and per-cloud sweep.
  - `internal/stateoperator/operator_test.go` and `internal/stateoperator/manager_pipeline_test.go` contain the focused unit/envtest coverage from the previous lifecycle task.
- Current behavior that must change:
  - `GroupReconciler.Reconcile` adds `nsx.ing.com/finalizer` to non-deleting Observe groups.
  - `observeGroupFromRemote` creates manager-sweep imported Observe groups with `Finalizers: []string{GroupFinalizer}`.
  - Existing tests assert Observe groups receive the finalizer.
  - `ApplyManagerPlan` deletes missing Observe CRs, but the sweep plan does not explicitly remove a legacy Observe finalizer before deleting the CR.
- Current behavior to preserve:
  - Manage-mode reconcile still adds `nsx.ing.com/finalizer` before NSX writes.
  - Manage-mode deletion still submits NSX delete and keeps the finalizer until a later successful sweep confirms remote absence.
  - Observe-mode deletion must not construct or call an NSX manager client.
  - Missing Observe groups are detected only after a successful manager gather/list; gather failure must not auto-delete local CRs.

## Boundary Design

- Keep lifecycle decisions inside `internal/stateoperator`; do not move Kubernetes lifecycle behavior into `startup`, `kubeapi`, or `nsxclient`.
- Keep the public CRD and Go API unchanged. This task changes operator behavior, not schema.
- Keep `ManagerPlan` as the sweep boundary. Add one explicit plan field only if needed, e.g. `ObserveFinalizerRemovals []string`, instead of hiding finalizer mutation in `kubeAPIAdapter.ApplyGroup` or adding a second lifecycle pipeline.
- Reuse `ManagerKubeApplier.RemoveGroupFinalizer` for Observe finalizer migration and missing-remote deletion. Do not create a duplicate Kubernetes client method.
- Preserve the existing helper shape:
  - `ensureGroupFinalizer` remains Manage-only.
  - Add or reuse a small `removeGroupFinalizer` helper in `reconciler.go` for Observe reconciliation, so deletion and non-deletion Observe paths do not duplicate slice edits.
- Do not add finalizers to `NSXNetworkCloud`.
- Do not add owner references from `NSXGroup` to `NSXNetworkCloud`.
- Do not remove unknown third-party finalizers from a CR unless execution proves the task requires it. The operator owns `nsx.ing.com/finalizer`; newly imported Observe groups must have an empty finalizer list.
- Boundary smell to watch with `$improve-code-boundaries`: avoid making `kubeapi` understand Observe versus Manage. `kubeapi` should remain a typed transport wrapper; `stateoperator` should decide lifecycle intent.

## Public Interface And Behavior

- No CRD schema change.
- Observable Kubernetes behavior after execution:
  - A newly created or reconciled Observe-mode `NSXGroup` does not receive `nsx.ing.com/finalizer`.
  - A manager-sweep imported Observe-mode `NSXGroup` is created with no finalizers.
  - An existing Observe-mode `NSXGroup` that still has `nsx.ing.com/finalizer` has that finalizer removed by reconcile without constructing an NSX manager client.
  - A manager sweep also plans/removes `nsx.ing.com/finalizer` from existing Observe groups it sees, so legacy Observe objects migrate during verified sweeps as well as event reconcile.
  - Deleting an Observe-mode `NSXGroup` from Kubernetes completes without an operator finalizer and without any NSX delete.
  - If a successful manager sweep verifies the backing NSX group is absent, the operator deletes the Observe-mode Kubernetes CR. If the legacy operator finalizer is still present, the sweep removes it as part of the same plan so the CR can actually disappear.
  - Manage-mode finalizer and delete behavior remains as implemented by the previous lifecycle task.

## TDD Execution Plan

Execute as vertical red-green-refactor cycles. Do not batch all tests before implementation.

1. [x] RED: update `TestGroupReconcileObserveDoesNotMutateNSXOrRequeue` or add a replacement behavior test in `internal/stateoperator/operator_test.go` proving a non-deleting Observe `NSXGroup` with no finalizers remains without `nsx.ing.com/finalizer` and does not construct an NSX manager client.
2. [x] GREEN: remove the Observe non-deleting call to `ensureGroupFinalizer` in `GroupReconciler.Reconcile`; return without NSX mutation.
3. [x] RED: add one focused behavior test proving a non-deleting legacy Observe `NSXGroup` with `nsx.ing.com/finalizer` has that finalizer removed by reconcile while unrelated finalizers are preserved and no NSX manager client is constructed.
4. [x] GREEN: add a small `removeGroupFinalizer` helper in `reconciler.go`, call it for every Observe reconcile path, and log successful removal with structured zap fields including component, reconcile key, group name, cloud FQDN, group ID, and mode.
5. [x] RED: update `TestProcessManagerSnapshotImportsRemoteOnlyGroupsAsObserveUpserts` in `internal/stateoperator/manager_pipeline_test.go` so remote-only Observe imports are expected to have no finalizers.
6. [x] GREEN: change `observeGroupFromRemote` to omit finalizers.
7. [x] RED: add or update a manager pipeline behavior test proving an existing Observe group with `nsx.ing.com/finalizer` plans an Observe finalizer removal when the remote group is present and does not plan any NSX write/delete.
8. [x] GREEN: extend `ManagerPlan` with `ObserveFinalizerRemovals []string`, populate it for Observe locals that contain `GroupFinalizer`, and apply it through `ManagerKubeApplier.RemoveGroupFinalizer`.
9. [x] RED: add or update a manager pipeline behavior test proving a missing Observe group with a legacy finalizer plans finalizer removal before CR deletion, and `ApplyManagerPlan` executes `remove-finalizer` before `delete-group-cr`.
10. [x] GREEN: apply Observe finalizer removals before `ObserveDeletes` in `ApplyManagerPlan`.
11. [x] RED: update typed envtest coverage in `TestDefaultManagerSweepAppliesObserveUpsertStatusAndDeleteThroughTypedKubeAPI` to prove imported/repaired Observe groups do not have `nsx.ing.com/finalizer`, and a missing Observe CR is actually gone after sweep.
12. [x] GREEN: adjust only the sweep apply behavior needed for the envtest to pass; do not change Manage lifecycle semantics.
13. [x] RED: update `TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI` or add a focused envtest/mockapi test proving:
   - Observe CR created from remote import has no finalizer.
   - deleting the Observe CR from Kubernetes completes.
   - the backing mockapi NSX group remains present.
   - Manage deletion still submits NSX delete and waits for sweep confirmation as before.
14. [x] GREEN: implement only the missing behavior.
15. [x] RED: add envtest/mockapi coverage for the new acceptance criterion: delete the backing mockapi NSX group outside the operator, run a verified manager sweep, and prove the Observe CR is automatically deleted from kube-api without any NSX delete attempt by the operator.
16. [x] GREEN: rely on successful `GatherManagerSnapshot` plus `ObserveDeletes` for absent remote objects. If the CR can remain terminating due to a legacy finalizer, fix only by using the Observe finalizer removal plan.
17. [x] RED/GREEN: update any previous lifecycle tests that still assert Observe finalizers are added. Keep Manage finalizer/delete tests intact.
18. [x] Refactor after green:
   - Remove duplicate finalizer slice edits if introduced.
   - Keep `ManagerPlan` ordering obvious: Observe upserts, managed writes/deletes, statuses, Observe finalizer removals, managed finalizer removals, Observe CR deletes, cloud status.
   - Add structured zap debug/info logging in `defaultManagerSweep` for Observe finalizer removal count/names and Observe auto-delete count/names, without logging unbounded full objects.

## Concrete Verification

Run these before completion and record exact output/evidence in the task file:

1. [x] Focused unit tests as they are added, one red-green cycle at a time, using exact `go test` commands for the touched package and test names.
2. [x] Focused envtest/mockapi verification, for example:
   - `KUBEBUILDER_ASSETS="$$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run 'TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI|TestDefaultManagerSweepAppliesObserveUpsertStatusAndDeleteThroughTypedKubeAPI|Test.*Observe.*Missing.*MockAPI' -count=1 -v`
   - Replace the regex with actual final test names if they differ.
3. [x] `make check`
4. [x] `make test`
5. [x] `make test-coverage`
6. [x] Confirm `make test-coverage` reports at least 80% statement coverage for every package and that new lifecycle branches are covered by behavior tests, not brittle string/file assertions.

## Completion Steps

- [x] Final `$improve-code-boundaries` pass:
  - verify Observe lifecycle logic remains in `internal/stateoperator`,
  - verify no duplicate Observe/Manage DTO shapes were introduced,
  - verify no cloud finalizer or owner-reference behavior leaked in,
  - verify `kubeapi` still remains mode-agnostic transport.
- [x] Update `.ralph/tasks/13-story-object-lifecycle/02-task-remove-observe-finalizers-and-autodelete-missing-observe-groups.md` with concrete verification evidence and set `<passes>true</passes>`.
- [x] Run `/bin/bash .ralph/task_switch.sh`.
- [x] `git add` all changed files, including `.ralph`.
- [x] Commit with `task finished 02-task-remove-observe-finalizers-and-autodelete-missing-observe-groups: remove observe finalizers and auto-delete missing observe groups`.
- [x] Push.

NOW EXECUTE

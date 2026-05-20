Plan path: `.ralph/tasks/bugs/controller-reconcilers-bypass-batched-kubeapi_plans/01-controller-reconcile-batch-boundary-verification.md`

# Controller Reconcile Batch Boundary Verification

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- Current task file: `.ralph/tasks/bugs/controller-reconcilers-bypass-batched-kubeapi.md`.
- The bug task describes older controller-runtime reconcilers that directly read and wrote Kubernetes resources from per-object `Reconcile` calls.
- Current code inspected before planning:
  - `internal/stateoperator/reconciler.go` `NetworkCloudReconciler.Reconcile` checks context cancellation and logs an observed event. It does not call `Client.Get`, `Client.List`, `Client.Update`, `Client.Patch`, or `Client.Status().Update`.
  - `internal/stateoperator/reconciler.go` `GroupReconciler.Reconcile` checks context cancellation and logs an observed event. It does not call `Client.Get`, `Client.List`, `Client.Update`, `Client.Patch`, `Client.Status().Update`, or construct an NSX manager client.
  - `internal/startup/manager.go` still registers both controller-runtime reconcilers, but only injects legacy fields that are now unused by reconcile behavior.
  - `internal/stateoperator/operator.go` owns the actual sweep path. It lists `NSXNetworkCloud` objects once in `runSweep`, delegates each cloud to `defaultManagerSweep`, and uses the typed `kubeapi.Client` for manager work.
  - `internal/stateoperator/manager_pipeline.go` `GatherManagerSnapshot` gathers local groups through `kubeapi.Groups().List` and remote groups through `ManagerClient.ListGroups`.
  - `internal/stateoperator/manager_pipeline.go` `ProcessManagerSnapshot` builds `ManagerPlan.KubeWrites` using `ManagerKubeWritePlan`.
  - `internal/stateoperator/manager_kube_writes.go` `kubeAPIAdapter.ApplyManagerKubeWrites` applies writes through typed batch APIs: `CreateBatch`, `UpdateBatch`, `UpdateStatusBatch`, `PatchFinalizersBatch`, `DeleteBatch`, and `NetworkClouds().UpdateStatusBatch`.
- Initial audit command:
  - `rg -n "\.Update\(|\.Patch\(|Status\(\)\.Update|Client\.Get|Client\.List|\.Get\(ctx|\.List\(ctx" internal/stateoperator internal/startup --glob '*.go'`
  - Production hits are currently limited to the global cloud list in `operator.go`, manager gather `ListGroups`, and test/helper code. No production controller `Reconcile` write path appears in the audit.

## Public Interface And Type Design

- Keep controller-runtime reconcilers as lightweight event observers:
  - `NetworkCloudReconciler.Reconcile(ctx, req) (reconcile.Result, error)`
  - `GroupReconciler.Reconcile(ctx, req) (reconcile.Result, error)`
- Do not restore direct per-object Kubernetes reads/writes to either controller reconciler.
- Do not add a second per-event queue, per-object gatherer, or per-object manager apply path unless execution proves controller events must trigger immediate work. The current deep boundary is the existing sweep:
  - gather through `NSXStateOperator.runSweep` and `GatherManagerSnapshot`
  - process through `ProcessManagerSnapshot`
  - apply through `ApplyManagerPlan` and `ManagerKubeApplier.ApplyManagerKubeWrites`
- If controller events need to trigger faster work in the future, the interface should schedule or request a shared sweep, not embed object-specific Kubernetes or NSX mutations inside `Reconcile`.
- Consider removing unused injected fields from `NetworkCloudReconciler` and `GroupReconciler` only after tests prove startup wiring and public tests do not need them:
  - `NetworkCloudReconciler.Client`
  - `GroupReconciler.Client`
  - `GroupReconciler.ManagerClientFactory`
  - `GroupReconciler.Clock`
  This is a boundary cleanup candidate, not required if it creates broad churn.

## Boundary Cleanup From `$improve-code-boundaries`

- Primary smell to prevent: wrong place-ism. Per-event controller reconcilers should not own manager gather/process/apply semantics or typed kube batch request construction.
- Primary desired boundary:
  - `reconciler.go` observes controller-runtime events and logs them.
  - `operator.go` owns sweep scheduling and cloud gathering.
  - `manager_pipeline.go` owns in-memory decision making from gathered state.
  - `manager_kube_writes.go` owns typed Kubernetes batch apply and resource-version propagation.
  - `internal/kubeapi` remains a typed Kubernetes API client and must not learn stateoperator reconcile decisions.
- Avoid one-shared-shape drift by reusing `kubeapi.BatchKey`, `ManagerKubeWritePlan`, `ManagerNSXWritePlan`, `GroupStatusPlan`, and `CloudStatusPlan` instead of adding a second controller-specific request DTO.
- Avoid mixed responsibilities by keeping tests for controller events separate from manager sweep behavior:
  - Controller tests should prove no client/NSX mutation happens during `Reconcile`.
  - Manager pipeline tests should prove finalizer/status/NSX failure/status behaviors are represented in batch plans and applied through batch APIs.
- Avoid overengineering. Because current reconcilers are already event-only, execution should prefer focused verification and small boundary cleanup over inventing new scheduling machinery.
- Avoid too-public fields if execution touches reconciler structs. Fields that are no longer read by production reconcile behavior should be removed instead of left as misleading extension points, provided startup/tests can be updated cleanly.

## Behaviors To Prove With TDD

- `NetworkCloudReconciler` returns an empty result for a normal event, emits structured debug logging, and does not need or use a controller-runtime client.
- `GroupReconciler` returns an empty result for a normal event, emits structured debug logging, and does not construct an NSX manager client or mutate Kubernetes.
- Both reconcilers return context cancellation errors.
- A production code audit shows no direct `client.Update`, `client.Patch`, or `Status().Update` calls in controller-runtime reconcile write paths.
- Observe finalizer removal is planned as a `GroupFinalizerPatchRequest` through `ManagerKubeWritePlan`, not as direct `client.Update`.
- Manage finalizer addition behavior remains intentionally absent from direct controller reconcile paths. If managed finalizers are required elsewhere, they must be planned through the shared manager sweep/batch path before implementation continues.
- Manage finalizer removal is planned as a `GroupFinalizerPatchRequest` through `ManagerKubeWritePlan`, including dependency on status results where needed.
- Manage apply status is planned as `GroupStatusUpdateRequest` through `ManagerKubeWritePlan`, using gathered resource versions.
- Manage delete status is planned as `GroupStatusUpdateRequest` through `ManagerKubeWritePlan`, using gathered resource versions.
- Classified NSX failure status is covered in the manager sweep path. If no current failure-classification path exists, execution must identify the missing production behavior and either add it through the shared plan or switch this plan back to `TO BE VERIFIED`.
- A real or instrumented manager sweep proves typed batch methods are used and no per-object controller-runtime direct writes happen during controller `Reconcile`.

## TDD Execution Plan

Follow vertical red-green cycles. Do not write all tests first.

1. [x] RED/REPRO: Run `go test ./internal/stateoperator -run 'TestNetworkCloudReconcile|TestGroupReconcile' -count=1 -v` and record whether current controller event behavior already proves no direct writes.
2. [x] GREEN: If the tests fail or are too weak, update one controller test at a time through the public `Reconcile` interface. Use a failing client/manager factory test double that fails on any unexpected client or NSX access.
3. [x] RED/REPRO: Run the direct-write audit command and record production hits:
   - `rg -n "\.Update\(|\.Patch\(|Status\(\)\.Update|Client\.Get|Client\.List|\.Get\(ctx|\.List\(ctx" internal/stateoperator internal/startup --glob '*.go'`
4. [x] GREEN: If production controller `Reconcile` paths still have direct Kubernetes writes, remove them by delegating behavior to the shared sweep/batch path. If only dead injected fields remain, either remove them or document why they are harmless.
5. [x] RED: Add or identify one focused manager process test for observe finalizer removal proving `ProcessManagerSnapshot` produces `KubeWrites.GroupFinalizerPatches` or a dependency entry, not a direct write.
6. [x] GREEN: Implement only the minimum needed to pass. If current code already passes, do not churn production code.
7. [x] RED: Add or identify one focused manager process test for managed finalizer removal after remote absence proving finalizer removal is represented in `ManagerKubeWritePlan`.
8. [x] GREEN: Implement only the missing planning/apply behavior through `ManagerKubeWritePlan`.
9. [x] RED: Add or identify one focused manager process test for manage apply status proving a drifted/missing managed group creates a `GroupStatusUpdateRequest` with the gathered resource version.
10. [x] GREEN: Keep status planning inside `ProcessManagerSnapshot`; do not move it into controller `Reconcile`.
11. [x] RED: Add or identify one focused manager process test for manage delete status proving deletion status is planned through `GroupStatusUpdateRequest` with the gathered resource version.
12. [x] GREEN: Keep delete-status planning in the shared manager plan and batch apply boundary.
13. [x] RED: Add or identify one focused test for classified NSX failure status. Prefer an integration-style test through the manager sweep using the existing mock NSX manager/client surfaces. The test must verify externally visible status behavior, not internal helper names.
14. [x] GREEN: If classified failure status is missing, add it to the manager sweep path and batch status update plan. If adding it changes status types/enums or the public status contract, switch this plan back to `TO BE VERIFIED` and quit immediately.
15. [x] REFACTOR: Run the `$improve-code-boundaries` review on touched code:
   - remove misleading unused reconciler fields if safe
   - avoid adding controller-specific DTOs
   - keep batch request construction in manager planning
   - keep batch apply in `manager_kube_writes.go`
16. [x] VERIFY focused command: `go test ./internal/stateoperator -run Reconcile -count=1`.
17. [x] VERIFY package command: `go test ./internal/stateoperator -count=1`.
18. [x] VERIFY full command: `go test ./...`.
19. [x] VERIFY required gates:
   - `make check`
   - `make test`
   - `make test-coverage`
20. [x] VERIFY coverage:
   - Confirm new/changed code is 80%+ covered.
   - Confirm `make test-coverage` reports 80%+ total coverage.
21. [x] MANUAL EVIDENCE: Record in the task file:
   - audit command and result showing no production direct reconcile writes
   - focused and full test commands
   - coverage output
   - evidence for observe finalizer removal, manage finalizer removal, manage apply status, manage delete status, and classified NSX failure status
   - any mockapi/testcontainer evidence if the classified failure verification uses `../nsx-t-mockapi`
22. [x] DONE: Only after every required check passes, set `<passes>true</passes>`, run `/bin/bash .ralph/task_switch.sh`, add all files including `.ralph`, commit with `task finished controller-reconcilers-bypass-batched-kubeapi: ...`, push, then quit immediately.

## Design Tripwires

- If execution finds controller-runtime events must immediately mutate Kubernetes to preserve correctness, switch this plan back to `TO BE VERIFIED`. The new design must still route writes through a gathered/bucketed batch boundary.
- If manage finalizer addition is truly missing as a required production behavior, switch back to `TO BE VERIFIED` before changing types. The current acceptance text asks for "manage finalizer addition", but the inspected code primarily shows managed finalizer removal/status behavior in the sweep path.
- If classified NSX failure status requires a new CRD enum, condition reason contract, or status schema change, switch back to `TO BE VERIFIED` and document the required type/interface change.
- If removing unused reconciler fields causes wide startup/test churn, leave that cleanup out of the bug fix and document the remaining harmless fields as follow-up boundary smell.
- If any production `Reconcile` path needs `client.Update`, `client.Patch`, or `Status().Update`, stop and redesign. That violates the hard task requirement.
- If a new test would only assert that a string exists in a source file, do not add it. Use command audit evidence for source inspection and behavior tests through public interfaces for runtime behavior.

NOW EXECUTE

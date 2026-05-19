Plan path: `.ralph/tasks/33-story-reconcile-write-efficiency/03-task-remove-get-then-update-from-reconcile-loops_plans/01-remove-get-then-update-plan.md`

# Remove Get Then Update From Reconcile Loops

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- Current task file: `.ralph/tasks/33-story-reconcile-write-efficiency/03-task-remove-get-then-update-from-reconcile-loops.md`.
- The previous task already added generic typed kube-api batch methods in `internal/kubeapi/batch_methods.go`:
  - `Groups().CreateBatch`, `UpdateBatch`, `ApplyBatch`, `UpdateStatusBatch`, `PatchFinalizersBatch`, `DeleteBatch`.
  - Equivalent `NetworkClouds()` batch methods.
- The manager sweep in `internal/stateoperator/manager_pipeline.go` already has most of the required gather/process/apply shape:
  - `GatherManagerSnapshot` lists local `NSXGroup` resources for a network cloud and lists all NSX-T groups.
  - `ProcessManagerSnapshot` compares gathered state and builds a `ManagerPlan`.
  - `ApplyManagerPlan` applies Kubernetes writes and NSX writes.
- The main violation is the current `kubeAPIAdapter`:
  - `ApplyGroup` performs `Groups().Get` before create/update.
  - `UpdateGroupStatus` performs `Groups().Get` before `UpdateStatus`.
  - `RemoveGroupFinalizer` performs `Groups().Get` before update.
  - `UpdateCloudStatus` performs `NetworkClouds().Get` before `UpdateStatus`.
- Direct controller-runtime `GroupReconciler` uses the object loaded at the beginning of `Reconcile`; it must be audited and protected, but it does not need a fresh per-item `Get` immediately before status/finalizer writes. The scalable 10,000+ resource path is the manager sweep.

## Interface And Type Design

- No CRD schema change.
- No user-facing semantics change.
- Keep `kubeapi` batch method signatures from the previous task. Do not invent a second batch executor or a second request shape.
- Replace the shallow per-item `ManagerKubeApplier` interface with a deeper batch-oriented boundary:

```go
type ManagerKubeWritePlan struct {
	GroupCreates          map[kubeapi.BatchKey]kubeapi.GroupCreateRequest
	GroupUpdates          map[kubeapi.BatchKey]kubeapi.GroupUpdateRequest
	GroupDeletes          map[kubeapi.BatchKey]kubeapi.GroupDeleteRequest
	GroupStatusUpdates    map[kubeapi.BatchKey]kubeapi.GroupStatusUpdateRequest
	GroupFinalizerPatches map[kubeapi.BatchKey]kubeapi.GroupFinalizerPatchRequest
	CloudStatusUpdates    map[kubeapi.BatchKey]kubeapi.NetworkCloudStatusUpdateRequest

	GroupStatusesAfterGroupWrite    map[kubeapi.BatchKey]GroupStatusAfterGroupWrite
	GroupFinalizersAfterGroupWrite  map[kubeapi.BatchKey]GroupFinalizerAfterGroupWrite
	GroupFinalizersAfterStatusWrite map[kubeapi.BatchKey]GroupFinalizerAfterStatusWrite
}

type ManagerKubeApplier interface {
	ApplyManagerKubeWrites(ctx context.Context, writes ManagerKubeWritePlan) error
}
```

- Exact helper type names can change during implementation, but the plan shape must not:
  - The process pass owns typed batch request maps.
  - The apply pass executes those maps through `internal/kubeapi` batch APIs.
  - Any same-sweep write dependency uses the resource version returned by an earlier write result, not a fresh `Get`.
- Keep the existing semantic plan fields only where they clarify NSX-T writes, metrics, or tests. Kubernetes writes should be driven by `ManagerPlan.KubeWrites`, not by loops over `ObserveUpserts`, `GroupStatuses`, finalizer name slices, and delete name slices inside the adapter.
- Add resource-version carrying fields where semantic tests still need them:
  - `GroupStatusPlan` should include the gathered resource version when the status target existed in the gather pass.
  - `CloudStatusPlan` should include the gathered cloud resource version.
  - Finalizer removal plans should carry the gathered finalizer list and resource version, not just a name.

## Gather Pass

- Keep `GatherManagerSnapshot` as the only kube-api read source for manager sweep group writes.
- The cloud object passed into `defaultManagerSweep` is already produced by the operator's cloud list pass. Treat it as gathered state and use its `ResourceVersion` for cloud status updates.
- Continue listing groups by normalized `spec.networkCloudFQDN`; this is the required full local state for that manager.
- Continue listing NSX-T manager groups once through `ManagerClient.ListGroups`.
- Add info/debug logs around gathered counts:
  - Info: network cloud name/FQDN, local group count, remote group count.
  - Debug: list filters, unsupported remote groups, gather failures.

## Process Pass

- `ProcessManagerSnapshot` must build `ManagerPlan.KubeWrites` directly.
- For observe remote-only imports:
  - Add a `GroupCreateRequest` for the new `NSXGroup`.
  - Add a pending status entry keyed to the create request. The exact `UpdateStatusBatch` request is completed in apply using the create result's resource version.
- For observe existing drift:
  - Build an `NSXGroup` update object by copying the gathered local object, replacing `Spec` with `observeSpecFromRemote`, and preserving unrelated metadata/finalizers.
  - Add a `GroupUpdateRequest` with the gathered resource version.
  - If status also changes, add a pending status entry keyed to the update request so apply uses the update result's resource version.
- For observe already-correct resources:
  - Add no spec write.
  - Add no status write when `statuscondition.CompareGroupStatus` says the only would-be change is an unchanged status with preserved `lastTransitionTime`.
- For observe finalizer removal:
  - If no earlier group write/status write will change the resource version, add `GroupFinalizerPatchRequest` immediately using gathered resource version and the desired finalizer array.
  - If there is an earlier same-object group or status write, record a pending finalizer patch so apply fills the resource version from the last successful same-object write result.
- For observe deletes:
  - Add `GroupDeleteRequest`; no per-item get is needed.
- For managed group statuses:
  - Build `GroupStatusUpdateRequest` from the gathered group resource version when the group has no earlier Kubernetes write in the same apply.
  - If a finalizer removal follows the status update, patch finalizers with the status update result's resource version.
- For managed finalizer removals after confirmed remote absence:
  - Build finalizer patches from gathered finalizers/resource versions, or from status write results when status is written first.
- For cloud status:
  - Build `NetworkCloudStatusUpdateRequest` from the gathered cloud object's resource version.
- For NSX-T managed writes and deletes:
  - Keep `ManagedWrites` and `ManagedDeletes` as semantic NSX operations. They are not kube-api writes.
- Add structured debug logging during process:
  - Per resource decision: create/update/delete/status/finalizer/skipped.
  - Include `networkCloudFQDN`, `groupID`, Kubernetes name, mode, resource version, and decision reason.
- Add structured info logging for process summary:
  - Counts for creates, updates, deletes, status writes, finalizer patches, NSX writes, NSX deletes, skipped resources.

## Apply Pass

- Implement `kubeAPIAdapter.ApplyManagerKubeWrites` using the typed batch APIs:
  - `Groups().CreateBatch`
  - `Groups().UpdateBatch`
  - `Groups().UpdateStatusBatch`
  - `Groups().PatchFinalizersBatch`
  - `Groups().DeleteBatch`
  - `NetworkClouds().UpdateStatusBatch`
- Execute Kubernetes writes in dependency-safe phases:
  1. Group creates and updates.
  2. Fill and execute status updates that depend on create/update results.
  3. Execute status updates that were ready from gathered resource versions.
  4. Fill and execute finalizer patches that depend on group write or status write results.
  5. Execute finalizer patches that were ready from gathered resource versions.
  6. Execute group deletes.
  7. Execute cloud status updates.
- Preserve existing high-level order relative to NSX-T writes:
  - Kubernetes observe create/update work first.
  - NSX-T managed writes/deletes next.
  - Kubernetes statuses/finalizers/deletes/cloud status after that, unless implementation proves status for observe creates must immediately follow the create to get the correct resource version. If that ordering conflict appears, switch this plan back to `TO BE VERIFIED` and document the ordering contract.
- Do not call `Groups().Get`, `NetworkClouds().Get`, controller-runtime `Client.Get`, or any other per-resource read inside apply.
- Aggregate batch errors with resource names and batch keys. Never ignore per-item errors.
- Use info logs for batch phase start/completion counts and debug logs for individual result/resource-version propagation.

## Boundary Cleanup From `$improve-code-boundaries`

- Avoid smell 3, wrong place-ism:
  - Kube batch request construction belongs in the stateoperator process pass because it owns reconcile decisions and gathered resource versions.
  - `internal/kubeapi` remains the typed REST client and must not learn reconcile semantics.
- Avoid smell 5, one shared shape:
  - Use the existing `kubeapi.BatchKey` and typed request structs. Do not add another generic key or local DTO that restates the same operation/resource/name fields.
- Avoid smell 8, too much in one file:
  - `manager_pipeline.go` is already large. If the implementation becomes unwieldy, split Kubernetes write planning/apply helpers into a focused file such as `internal/stateoperator/manager_kube_writes.go` in the same package.
- Avoid smell 10, helper churn:
  - Only add helper functions where they remove repeated typed-map construction or resource-version propagation. Do not fragment simple one-call transformations.
- Avoid smell 13, functions with wrong overlap:
  - There should be one place that builds manager kube write maps and one place that applies them. Do not keep the old per-item adapter methods next to the new batch path.
- Avoid smell 14, too public:
  - Keep dependency marker structs private unless tests outside the package require exported names. Export only the types already part of stateoperator's test-facing API.

## Behaviors To Prove With TDD

- Manager process planning creates typed kube batch request maps for observe imports, observe updates, observe deletes, status updates, finalizer patches, and cloud status updates.
- Existing local observe updates use the gathered object's resource version in `GroupUpdateRequest`; no get is needed to update.
- Existing status-only changes use gathered group/cloud resource versions in status batch requests.
- Observe imports produce status updates from create results without a follow-up get.
- Finalizer patches after a same-object update/status use the latest previous write result's resource version, not a fresh get.
- Already-correct group status produces no status batch request, including when `lastTransitionTime` would otherwise be the only changed value.
- Typed kube API request counts in a real manager sweep are full-list plus batched writes:
  - No `GET /apis/.../nsxgroups/{name}` during manager apply.
  - No `GET /apis/.../nsxnetworkclouds/{name}` during manager apply.
  - Expected `LIST` calls remain present for gather and test verification.
- A 10,000+ resource manager sweep does not perform per-item get-before-write. The evidence should record list counts and write counts.
- Existing direct `GroupReconciler` manage apply/delete and finalizer flows still use the initially gathered object and do not introduce a fresh get-before-update.
- Logs include structured info summaries and debug per-resource decisions for gather, process, batch construction, skipped unchanged resources, batch phases, and results.

## TDD Execution Plan

 RED: Add a focused process-pass test showing an observe remote-only import produces a `GroupCreateRequest` plus a pending group status tied to that create, not a semantic `ObserveUpserts` loop item only.
 GREEN: Add `ManagerKubeWritePlan` to `ManagerPlan` and populate create/status-after-create requests for remote-only observe imports.
 RED: Add a process-pass test showing an existing drifted observe group produces a `GroupUpdateRequest` with the gathered resource version and preserved unrelated finalizers.
 GREEN: Populate observe update requests from copied gathered objects. Keep existing semantic tests green.
 RED: Add process-pass tests for status-only group and cloud updates proving `UpdateStatusBatch` requests carry gathered resource versions.
 GREEN: Add resource-version fields to status plans and build typed group/cloud status update maps directly during processing.
 RED: Add process-pass tests for finalizer removal:
   - immediate patch uses gathered resource version when no earlier same-object write exists.
   - pending patch waits for a prior same-object update/status result when one exists.
 GREEN: Add finalizer patch request construction and pending dependency records.
 RED: Add an adapter/apply test with a fake typed kube API transport that fails the test on any group/cloud `GET` during `ApplyManagerPlan`, while allowing list/setup calls outside apply.
 GREEN: Replace the per-item `kubeAPIAdapter` methods with `ApplyManagerKubeWrites` that calls typed batch APIs and propagates resource versions from create/update/status results.
 RED: Add adapter/apply tests proving create-result resource versions are used for import status updates and status-result resource versions are used for subsequent finalizer patches.
 GREEN: Complete dependency resolution and batch result aggregation. Do not catch and discard item errors.
 RED: Add manager sweep integration test against envtest typed kube API and mock NSX manager with mixed observe/create/update/delete/status/finalizer work. Assert request counts show list plus batched writes and zero apply-phase gets.
 GREEN: Wire `defaultManagerSweep` to the batch applier and adjust logging/metrics without changing CRD semantics.
 RED: Add a 10,000+ resource manager sweep verification test or script-backed integration test that records typed kube API call counts. Keep it runnable under normal test commands if practical; if too slow for full suite, make a targeted test command and record evidence in the task file.
 GREEN: Optimize map sizing, avoid quadratic scans, and make the 10,000+ scenario pass without per-item get calls.
 RED: Add or update direct `GroupReconciler` tests/audit instrumentation proving direct reconcile writes do not introduce fresh get-before-update behavior.
 GREEN: Keep direct reconciler behavior intact or switch its finalizer/status writes to patch/status update with the already-loaded object if needed.
 RED: Add zap observer assertions for gather/process/apply summary logs and debug per-resource skip/write decisions.
 GREEN: Add missing structured zap fields and ensure errors are wrapped and returned.
 REFACTOR: Run the `$improve-code-boundaries` review on touched code. Split manager kube write planning/apply into a focused file if `manager_pipeline.go` gets muddier, remove obsolete per-item adapter methods, and keep `kubeapi` free of reconcile semantics.
 VERIFY focused tests after each green step, then run:
   - `go test ./internal/stateoperator ./internal/kubeapi -count=1`
   - `go test -race ./internal/stateoperator -run 'Manager|Sweep|Reconcile' -count=1`
   - `go test ./...`
 VERIFY required gates:
   - `make check`
   - `make test`
   - `make test-coverage`
 MANUAL EVIDENCE: Record in the task file:
   - Exact commands and outputs.
   - Typed kube API call counts for normal mixed sweep.
   - 10,000+ resource call counts.
   - NSX-T mockapi evidence from `../nsx-t-mockapi` or the existing testcontainer/mockapi harness.
   - Coverage percentage proving new code and total `make test-coverage` are 80%+.
 DONE: Only after all checks pass and coverage is 80%+, set `<passes>true</passes>`, run `/bin/bash .ralph/task_switch.sh`, commit all files with `task finished 03-task-remove-get-then-update-from-reconcile-loops: ...`, push, then quit immediately.

## Design Tripwires

- If status updates for just-created observe groups cannot be done in the same sweep without a fresh get or an unsafe ordering change, switch this plan back to `TO BE VERIFIED`, document whether status must be deferred to the next sweep or whether a status patch/apply API is needed, and quit immediately.
- If server-side apply is required for observe upserts but would incorrectly own/remove finalizers, switch back to `TO BE VERIFIED` and document the create/update contract before implementation.
- If the process pass cannot directly prepare typed batch request maps without making dependency handling unclear, switch back to `TO BE VERIFIED` and propose a smaller typed dependency model.
- If the 10,000+ resource test is too slow for `make test`, keep a focused large verification command, but still run and record it before marking the task complete.
- If any code path needs a fresh per-resource kube-api get immediately before patch/update to preserve semantics, switch back to `TO BE VERIFIED`; that violates the task's hard requirement.
- If implementation requires a CRD schema or user-facing semantics change, switch back to `TO BE VERIFIED` and quit immediately.

NOW EXECUTE

Plan path: `.ralph/tasks/bugs/manager-sweep-uses-multiple-kube-batch-calls_plans/01-one-gather-one-process-one-apply-plan.md`

# Manager Sweep One Gather One Process One Apply

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- Current task file: `.ralph/tasks/bugs/manager-sweep-uses-multiple-kube-batch-calls.md`.
- The bug task requires the full manager loop to be one initial gather, one processing sweep over gathered state, and one bounded apply boundary with no post-gather re-gets or unchanged-resource writes.
- Current broken evidence inspected:
  - `internal/stateoperator/operator.go` lists all `NSXNetworkCloud` objects in `runSweep`, then calls `Get` again in `runCloudSweep`.
  - `internal/stateoperator/manager_pipeline.go` `defaultManagerSweep` calls `managerClientFactory` once in `GatherManagerSnapshot`, then can call it again for apply.
  - `internal/stateoperator/manager_pipeline.go` `ApplyManagerPlan` calls `ApplyManagerKubeWrites` twice by splitting `managerKubeObjectWrites` and `managerKubePostObjectWrites`.
  - `internal/stateoperator/manager_kube_writes.go` `ApplyManagerKubeWrites` performs several typed kube batch calls by operation.
  - `internal/stateoperator/reconciler.go` still has direct `Get`, `List`, `Update`, `Status().Update`, finalizer mutation, and NSX writes outside the manager sweep boundary.
- Existing good boundary to preserve:
  - `BindingKey{NetworkCloudFQDN, GroupID}` already uniquely combines cloud and group identity.
  - `BuildBindings` already builds `LocalByKey` and `RemoteByKey` maps and detects duplicate combined keys.
  - `ProcessManagerSnapshot` already does in-memory comparison from `ManagerSnapshot` data and uses status comparison helpers to avoid unchanged status writes.

## Public Interface And Type Design

Keep the public planning API centered on gathered snapshots and one write-set boundary. Do not expose another adapter layer around the current split paths.

1. Replace the split kube applier call shape with one pass-level method:

```go
type ManagerKubeApplier interface {
	ApplyManagerKubeWrites(ctx context.Context, writes ManagerKubeWritePlan) (*ManagerKubeApplyResult, error)
}

type ManagerKubeApplyResult struct {
	GroupCreates          map[kubeapi.BatchKey]*nsxv1alpha.NSXGroup
	GroupUpdates          map[kubeapi.BatchKey]*nsxv1alpha.NSXGroup
	GroupStatusUpdates    map[kubeapi.BatchKey]*nsxv1alpha.NSXGroup
	GroupFinalizerPatches map[kubeapi.BatchKey]*nsxv1alpha.NSXGroup
	GroupDeletes          map[kubeapi.BatchKey]struct{}
	CloudStatusUpdates    map[kubeapi.BatchKey]*nsxv1alpha.NSXNetworkCloud
}
```

The existing interface name can remain, but it must be called at most once per manager pass. The result type is internal to `stateoperator` unless tests or package boundaries prove it must be exported.

2. Collapse the pre/post write helpers:

- Remove `managerKubeObjectWrites`, `managerKubePostObjectWrites`, `groupFinalizersAfterGeneratedStatuses`, and `groupFinalizersAfterDirectStatuses` if the unified applier makes them unnecessary.
- Keep `ManagerKubeWritePlan` as the single typed write set if it remains the simplest shape.
- Preserve `GroupStatusesAfterGroupWrite`, `GroupFinalizersAfterGroupWrite`, and `GroupFinalizersAfterStatusWrite` as dependency descriptors inside the one write set, not as a reason to split the pass.
- The unified applier may internally execute dependent Kubernetes operations in stages because Kubernetes resource versions from creates/updates/status writes are real ordering dependencies. That internal staging must still happen behind one pass-level apply boundary and must not perform re-gets.

3. Add one NSX manager write boundary for the pass:

```go
type ManagerNSXWritePlan struct {
	Patches map[BindingKey]ManagedGroupPatch
	Puts    map[BindingKey]ManagedGroupPut
	Deletes map[BindingKey]ManagedGroupDelete
}
```

- If the existing NSX Manager API surface cannot provide a true multi-resource HTTP call, keep the production `nsxclient` methods as transport primitives but move all classification into the plan and expose one `ApplyManagerNSXWrites(ctx, plan)` boundary from `stateoperator`.
- Use `Patch` for existing managed resources whose group/expression fields can be patched.
- Use `Put` only for new managed resources or cases where patching is impossible. If `nsxclient` lacks a put/upsert route, add the typed route and request DTO in `internal/nsxclient` instead of overloading "add expression" helpers as semantic put operations.
- Enforce that one combined `BindingKey` cannot be present in both patch and put maps. If a future design needs a different key, switch this plan back to `TO BE VERIFIED` before coding.
- Deletes remain separate from patch/put because the task separately requires delete behavior and unchanged resources to have zero writes.

4. Replace the per-cloud refresh with gathered cloud objects:

- `runSweep` should pass the listed `NSXNetworkCloud` item into the cloud sweep directly.
- `runCloudSweep` must not call `client.Get` after the global list.
- If deletion-race handling is needed, use the listed object's deletion timestamp/resource version from the gather, and tolerate not-found only during the one apply boundary where Kubernetes returns it.

5. Turn controller-runtime reconcilers into event observers for this bug scope:

- `NetworkCloudReconciler.Reconcile` may log the event key and return without reading or writing the object.
- `GroupReconciler.Reconcile` may log the event key and return without `Get`, `List`, direct finalizer writes, status writes, or NSX writes.
- If startup currently depends on those direct reconcilers for correctness, replace that dependency with manager sweep behavior before removing it. Do not leave a bypass that writes unchanged resources outside the gathered manager pass.

## Boundary Cleanup From `$improve-code-boundaries`

- The main boundary smell is split ownership of reconciliation:
  - `operator.go` owns one list but then re-gets individual clouds.
  - `manager_pipeline.go` owns gather/process/apply but splits apply into two kube calls and constructs a second NSX client.
  - `manager_kube_writes.go` owns dependency ordering but leaks it as pre/post write phases.
  - `reconciler.go` bypasses the manager pipeline entirely.
- Flatten this by making `stateoperator` own one pass model:
  - gather once,
  - build indexed snapshot once,
  - process maps keyed by `BindingKey`,
  - submit one `ManagerKubeWritePlan` and one `ManagerNSXWritePlan`.
- Keep `internal/kubeapi` as typed Kubernetes REST transport. Do not move manager semantics into kubeapi.
- Keep `internal/nsxclient` as typed NSX transport. Do not move manager classification or no-patch/no-put decisions into nsxclient.
- Prefer deleting helper functions that only exist to split the old phases. A smaller apply file is better than preserving "object writes" and "post object writes" naming.
- Avoid "one shared shape" between Kubernetes and NSX write plans. Kubernetes writes need resource-version dependency descriptors; NSX writes need patch/put/delete classification.

## Behaviors To Prove

- The manager sweep lists `NSXNetworkCloud` objects once and does not `Get` each cloud afterward.
- `GatherManagerSnapshot` lists local `NSXGroup` objects once for the cloud and lists remote NSX groups once for the cloud.
- After gather, processing calls no Kubernetes client methods and no NSX Manager methods.
- `BuildBindings` uses the combined `BindingKey` and does not confuse same `GroupID` values from different clouds.
- `ApplyManagerPlan` calls the Kubernetes applier at most once for a non-empty write set.
- The unified Kubernetes apply boundary handles create plus status, update plus status, status plus finalizer, delete, and cloud status in one pass without a re-get.
- The same resource is never present in both NSX patch and put buckets.
- Unchanged resources produce no Kubernetes writes, no NSX writes, no status updates, no finalizer patches, and no deletes.
- Large-count manager input remains bounded: one cloud list, one local group list per cloud, one remote group list per cloud, one pass-level kube apply call, and no per-resource re-get.
- Controller-runtime reconcilers do not perform direct writes or direct NSX manager writes outside the manager sweep path.

## TDD Execution Plan

Follow vertical red-green cycles. Do not write all tests first.

1. [ ] RED: Add a focused `internal/stateoperator` test for `runSweep` using a counting controller-runtime client. It should prove one `List` for `NSXNetworkCloud` and zero post-list `Get` calls for clouds.
2. [ ] GREEN: Remove the cloud refresh from `runCloudSweep` and make the listed cloud object the gathered cloud state.
3. [ ] RED: Add a manager pipeline test proving `ApplyManagerPlan` calls `ApplyManagerKubeWrites` exactly once for a mixed write set containing create, update, direct status, status-after-write, finalizer-after-status, delete, and cloud status.
4. [ ] GREEN: Remove the pre/post split in `ApplyManagerPlan` and make the applier handle dependency ordering within one call.
5. [ ] RED: Add a kube applier test proving dependent status/finalizer requests use resource versions from prior batch results and no `Groups().Get` or `NetworkClouds().Get` occurs.
6. [ ] GREEN: Refactor `ApplyManagerKubeWrites` into one internal staged implementation that returns `ManagerKubeApplyResult` and never re-queries.
7. [ ] RED: Add a processing test with two clouds that both have a same-named or same-ID group candidate and prove `BuildBindings` compares with the combined `BindingKey`.
8. [ ] GREEN: Tighten the planning code only if the test exposes a gap; otherwise keep the existing `BindingKey` behavior and record it as preserved evidence.
9. [ ] RED: Add a test proving unchanged local/remote state produces an empty `ManagerKubeWritePlan`, no NSX patch/put/delete plan, and no status/finalizer/delete write.
10. [ ] GREEN: Fix any status/finalizer/result-accounting branch that still writes unchanged resources.
11. [ ] RED: Add NSX write classification tests for existing-drifted resource -> patch, missing managed resource -> put, patch-impossible case -> put, delete -> delete, and same `BindingKey` cannot be both patch and put.
12. [ ] GREEN: Introduce `ManagerNSXWritePlan` and one `ApplyManagerNSXWrites` boundary. Add or wire `nsxclient` put/upsert routes only if needed for a real missing-resource behavior.
13. [ ] RED: Add full loop instrumentation test around kube and NSX test doubles proving one initial gather, zero post-gather Kubernetes/NSX re-get/re-list calls, one processing pass, one pass-level kube apply boundary, and bounded NSX patch/put/delete apply.
14. [ ] GREEN: Wire `defaultManagerSweep` through the one gathered client/snapshot and one apply boundary; do not construct a second manager client after gather unless the gathered client itself is retained for apply without another list/get.
15. [ ] RED: Add reconciler tests proving `NetworkCloudReconciler` and `GroupReconciler` do not `Get`, `List`, `Update`, `Status().Update`, or call NSX manager on events.
16. [ ] GREEN: Shrink reconcilers to event logging/requeue-neutral observers, or route them through the same pass boundary if an event-triggered pass is already available without violating one-gather semantics.
17. [ ] RED: Add large-count regression test with many groups proving call counts stay bounded and the apply path does not fall back to per-resource Kubernetes re-gets or hidden per-resource writes for unchanged resources.
18. [ ] GREEN: Fix any loop or helper that scales call counts with unchanged resource count except the allowed initial gathered lists and planned mutating operations.
19. [ ] REFACTOR: Run the `$improve-code-boundaries` review on touched code. Delete old split-phase helpers and avoid new DTO duplication between snapshot, plan, and transport request layers.
20. [ ] VERIFY focused tests:
    - `go test ./internal/stateoperator -run 'Manager|Sweep|Reconcile' -count=1`
21. [ ] VERIFY required repo gates:
    - `make check`
    - `make test`
    - `make test-coverage`
22. [ ] VERIFY coverage:
    - Confirm new/changed code is at least 80% covered.
    - Confirm aggregate `make test-coverage` remains at least 80%.
23. [ ] RECORD manual verification evidence in the task file or a linked artifact:
    - exact test commands,
    - call-count evidence from test doubles/instrumentation,
    - any mockapi evidence if NSX transport routes changed,
    - final boundary review notes.
24. [ ] COMPLETE only after all gates pass:
    - set `<passes>true</passes>` in the task file,
    - run `/bin/bash .ralph/task_switch.sh`,
    - add all files including `.ralph`,
    - commit with `task finished manager-sweep-uses-multiple-kube-batch-calls: ...`,
    - push,
    - quit immediately.

## Switch-Back Conditions

- If a true NSX Manager multi-resource patch/put endpoint does not exist and the task owner requires literal one HTTP patch and one HTTP put across all resources, switch this plan back to `TO BE VERIFIED` and document the NSX API mismatch before coding.
- If controller-runtime reconcilers must still perform direct writes for a user-visible latency requirement, switch this plan back to `TO BE VERIFIED` and redesign the public event-triggered pass interface before coding.
- If Kubernetes status/finalizer dependencies cannot be represented without a post-write resource version and a second apply boundary, switch this plan back to `TO BE VERIFIED` and record the exact API constraint.
- If execution requires changing CRD schema, enum semantics, or public custom resource fields, switch this plan back to `TO BE VERIFIED` before coding.

NOW EXECUTE

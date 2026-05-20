## Bug: Controller Reconcilers Bypass Batched Kube API <status>done</status> <passes>true</passes> <priority>ultra high</priority>

<plan>
.ralph/tasks/bugs/controller-reconcilers-bypass-batched-kubeapi_plans/01-controller-reconcile-batch-boundary-verification.md
</plan>

<description>
The completed task `.ralph/tasks/33-story-reconcile-write-efficiency/03-task-remove-get-then-update-from-reconcile-loops.md` proves batching for the periodic manager sweep, but not for all controller-runtime reconcile loops.

Verified code evidence:
- `internal/startup/manager.go` registers `NetworkCloudReconciler` and `GroupReconciler` as controller-runtime reconcilers.
- `internal/stateoperator/reconciler.go` `NetworkCloudReconciler.Reconcile` calls `r.Client.Get` directly for one `NSXNetworkCloud`.
- `internal/stateoperator/reconciler.go` `GroupReconciler.Reconcile` calls `r.Client.Get` directly for one `NSXGroup`.
- `GroupReconciler` also calls direct controller-runtime client writes through `ensureGroupFinalizer` (`r.Client.Update`), `removeGroupFinalizer` (`r.Client.Update`), and `updateGroupStatus` (`r.Client.Status().Update`).
- `GroupReconciler.findNetworkCloud` does a controller-runtime `List` during individual group reconcile instead of using a single gathered pass shared with the write plan.

This violates the PO requirement that all reconcile loops use the radical gather/process/bucket/apply pattern and that Kubernetes writes go through batch APIs. The controller reconcilers still implement per-event, direct client behavior outside the typed `internal/kubeapi` batch API.
</description>

<mandatory_manual_verification>
Manually reproduce or inspect the broken behavior, fix it, then manually verify with concrete evidence that all controller-runtime reconcile paths use the same gathered/bucketed batched write model as the manager sweep.

Required verification evidence:
- A code audit showing no production `Reconcile` path writes Kubernetes resources via direct `client.Update`, `client.Patch`, or `Status().Update`.
- Tests or instrumentation proving `NetworkCloudReconciler` and `GroupReconciler` do not perform per-object direct writes and instead enqueue/build batch work through the typed kube API batch layer or a single shared reconcile pass.
- Focused tests for observe finalizer removal, manage finalizer addition, manage apply status, manage delete status, and classified NSX failure status.
- `go test ./internal/stateoperator -run Reconcile -count=1`.
- `go test ./...`.
- Relevant race or integration tests if the fix changes shared sweep/reconcile execution.
</mandatory_manual_verification>

<acceptance_criteria>
- [x] I reproduced or inspected the broken behavior enough to understand the failure.
- [x] I fixed the bug.
- [x] `NetworkCloudReconciler` and `GroupReconciler` no longer bypass the batched kube API write path.
- [x] All controller reconcile Kubernetes writes are prepared as gathered/bucketed batch requests or are delegated to one shared pass that does so.
- [x] Direct per-event `client.Update`, `client.Patch`, and `Status().Update` calls are absent from production reconcile write paths.
- [x] I manually verified with concrete calls, commands, logs, screenshots, external service status, or other evidence that the bug no longer occurs.
- [x] The verification evidence is recorded in the task or linked artifact.
</acceptance_criteria>

<verification_evidence>
Implementation summary:
- Removed legacy `Client`, `ManagerClientFactory`, and `Clock` fields from controller-runtime observer reconcilers. Startup now wires only loggers into `NetworkCloudReconciler` and `GroupReconciler`, so the event observers do not carry direct Kubernetes or NSX mutation surfaces.
- Added managed finalizer addition planning through `ManagerKubeWritePlan.GroupFinalizerPatches` using the typed `kubeapi.GroupFinalizerPatchRequest` batch path. No controller-runtime direct write path was added.

Production direct-write audit:
```bash
rg -n "\.Update\(|\.Patch\(|Status\(\)\.Update|Client\.Get|Client\.List|\.Get\(ctx|\.List\(ctx" internal/stateoperator internal/startup --glob '*.go' --glob '!**/*_test.go'
```
Result:
```text
internal/stateoperator/operator.go:157:	err := o.client.List(ctx, &clouds)
internal/stateoperator/manager_pipeline.go:428:	remoteGroups, err := managerClient.ListGroups(ctx)
```
Audit interpretation: no production controller-runtime `Reconcile` path performs direct `client.Update`, `client.Patch`, or `Status().Update`. The remaining production hits are the shared sweep's network cloud gather and the manager remote group gather.

Focused behavior evidence:
- `go test ./internal/stateoperator -run Reconcile -count=1` passed. This covers `NetworkCloudReconciler` and `GroupReconciler` observing events without direct client/NSX mutation and returning context cancellation errors.
- `go test ./internal/stateoperator -run 'TestProcessManagerSnapshot(ObserveGroupWithLegacyFinalizerPlansFinalizerRemovalOnly|ManageGroupPlansFinalizerAdditionThroughBatchPatch|ManageGroupsWriteMissingAndDriftedAndOnlyStatusMatching|DeletingManageGroupPlansFinalizerRemovalAfterRemoteAbsence|DeletingManageGroupPlansDeleteWhileRemoteExists)' -count=1 -v` passed.
  - Observe finalizer removal: `TestProcessManagerSnapshotObserveGroupWithLegacyFinalizerPlansFinalizerRemovalOnly` proves finalizer removal is planned through `GroupFinalizersAfterStatusWrite`.
  - Manage finalizer addition: `TestProcessManagerSnapshotManageGroupPlansFinalizerAdditionThroughBatchPatch` proves finalizer addition is planned through `GroupFinalizerPatches`.
  - Manage apply status: `TestProcessManagerSnapshotManageGroupsWriteMissingAndDriftedAndOnlyStatusMatching` proves drifted/missing managed groups plan status updates and classified NSX puts/patches through shared manager plans.
  - Manage delete status: `TestProcessManagerSnapshotDeletingManageGroupPlansFinalizerRemovalAfterRemoteAbsence` and `TestProcessManagerSnapshotDeletingManageGroupPlansDeleteWhileRemoteExists` prove delete and confirmed-absent statuses are planned through shared manager status writes.
- `go test ./internal/stateoperator -run TestProcessManagerSnapshotRemoteOnlyUnsupportedExpressionMarksUnsynced -count=1 -v` passed. This proves classified unsupported NSX expression status is planned by the manager processor.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run TestDefaultManagerSweepLogsUnsupportedRemoteReason -count=1 -v` passed. This proves the envtest-backed default sweep records the classified unsupported status and structured log through the shared sweep path.

Required gates:
- `make check` passed. It ran formatting, vet, lint, project lint, `go test ./...`, `go test -race ./...`, mockapi contract tests, e2e tests, large chaos tests, and coverage. Coverage reported `coverage 87.0% meets 80.0% threshold`.
- `make test` passed as a standalone gate with `KUBEBUILDER_ASSETS` configured by `setup-envtest`.
- `make test-coverage` passed as a standalone gate. Total coverage: 87.0%. `internal/stateoperator` coverage: 89.7%.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -count=1` passed.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./...` passed.
- `git diff --check` passed.
</verification_evidence>

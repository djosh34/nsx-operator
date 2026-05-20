## Bug: Controller Reconcilers Bypass Batched Kube API <status>not_started</status> <passes>false</passes> <priority>ultra high</priority>

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
- [ ] I reproduced or inspected the broken behavior enough to understand the failure.
- [ ] I fixed the bug.
- [ ] `NetworkCloudReconciler` and `GroupReconciler` no longer bypass the batched kube API write path.
- [ ] All controller reconcile Kubernetes writes are prepared as gathered/bucketed batch requests or are delegated to one shared pass that does so.
- [ ] Direct per-event `client.Update`, `client.Patch`, and `Status().Update` calls are absent from production reconcile write paths.
- [ ] I manually verified with concrete calls, commands, logs, screenshots, external service status, or other evidence that the bug no longer occurs.
- [ ] The verification evidence is recorded in the task or linked artifact.
</acceptance_criteria>

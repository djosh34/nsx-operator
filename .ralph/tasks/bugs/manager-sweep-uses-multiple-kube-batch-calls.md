## Bug: Manager Sweep Must Be One Gather One Process One Apply With No Regets Or Write Churn <status>done</status> <passes>true</passes> <priority>ultra high</priority>

<description>
The manager sweep uses typed batch APIs in places, but it does not satisfy the PO's hard end-to-end batching contract.

Hard requirement for the entire loop path:
- There is exactly one initial gather phase for the pass.
- The gather phase is the only place allowed to read required state.
- The gather phase may issue one Kubernetes API gather for all relevant local `NSXNetworkCloud`/`NSXGroup` state and one NSX Manager general list-groups call for all relevant remote groups.
- After that gather completes, there must be zero Kubernetes or NSX Manager re-get/re-list calls anywhere in the entire loop path.
- All required state for compare, status decisions, patch planning, put planning, delete planning, finalizer decisions, and result accounting must come from the state pulled during the initial gather.
- There is exactly one processing sweep over the gathered state.
- The processing sweep must build maps keyed by the special combined identity key for local/remote comparison. That key must uniquely combine the network cloud identity and the group identity so hash-map comparison cannot confuse same-named groups from different clouds.
- The processing sweep must use those maps as the hash compare source of truth. It must not call back out to Kubernetes or NSX Manager while processing.
- There is one patch call maximum for the pass. That patch call must carry all resources that need patching.
- There is one put call maximum for the pass. Put is allowed only for new resources or for cases where patching is impossible.
- The same resource must never be both patched and put in the same pass.
- A resource that was already classified for patch must not also be classified for put, and a resource that was already classified for put must not also be classified for patch.
- The vast majority of resources must produce zero patch/put calls because their desired state and status have not changed.
- Already-correct resources must not be secretly updated anywhere else in the code path, including status update helpers, finalizer helpers, result accounting, controller-runtime reconciler paths, or post-apply cleanup paths.
- The implementation fails this bug if unchanged resources receive any patch, put, update, status update, finalizer write, delete, or other write through another branch of the same loop.

Verified code evidence:
- `internal/stateoperator/manager_pipeline.go` `ApplyManagerPlan` calls `ApplyManagerKubeWrites` once for pre-object writes and again for post-object writes.
- `internal/stateoperator/manager_kube_writes.go` `ApplyManagerKubeWrites` then calls separate typed batch methods for creates, updates, status updates, finalizer patches, deletes, and network-cloud status updates:
  - `Groups().CreateBatch`
  - `Groups().UpdateBatch`
  - `Groups().UpdateStatusBatch`
  - `Groups().PatchFinalizersBatch`
  - `Groups().DeleteBatch`
  - `NetworkClouds().UpdateStatusBatch`

This is batched by operation, but it is not one maximum batch call for the pass. The current design also spreads dependency ordering across `managerKubeObjectWrites`, `managerKubePostObjectWrites`, and post-result resource-version plumbing instead of one obvious bucketed apply boundary.

Additional known related failures from the batching investigation:
- The state operator sweep can query the same `NSXNetworkCloud` twice in one pass: `List` in `runSweep`, then `Get` in `runCloudSweep`.
- Controller-runtime `NetworkCloudReconciler` and `GroupReconciler` still have direct `Get`/`List`/`Update`/`Status().Update` behavior outside the gathered manager sweep path.
- Those paths must not become escape hatches that re-read or write unchanged resources after the manager gather/process/apply pass.
</description>

<mandatory_manual_verification>
Manually reproduce or inspect the broken behavior, fix it, then manually verify with concrete evidence that the whole manager sweep path performs one gather, one processing sweep, and one bounded apply phase with no re-gets and no unchanged-resource writes.

Required verification evidence:
- Instrumentation or test doubles around Kubernetes and NSX Manager clients showing exactly one initial gather of required local state and one NSX Manager general list-groups call for the pass.
- Instrumentation proving zero Kubernetes or NSX Manager `Get`/re-list calls after the initial gather, across the full loop path.
- Tests proving processing uses the gathered maps keyed by the combined network-cloud/group identity key and does all hash comparison in memory.
- Tests proving one maximum patch call for all resources that need patching.
- Tests proving one maximum put call total, used only for new resources or where patch is impossible.
- Tests proving no resource is both patched and put in the same pass.
- Tests proving unchanged resources receive zero patch/put/update/status/finalizer/delete calls anywhere in the full loop path.
- Tests for create plus status, update plus status, status plus finalizer, delete, and cloud status in the same pass.
- Large-count verification proving the single-call apply path remains bounded and does not fall back to per-resource calls or per-resource re-gets.
- `go test ./internal/stateoperator -run 'Manager|Sweep' -count=1`.
- `go test ./...`.
</mandatory_manual_verification>

<acceptance_criteria>
- [x] I reproduced or inspected the broken behavior enough to understand the failure.
- [x] I fixed the bug.
- [x] The full loop has exactly one initial gather phase for all required Kubernetes state and NSX Manager list-groups state.
- [x] There are zero Kubernetes or NSX Manager re-get/re-list calls after the initial gather anywhere in the loop path.
- [x] The full loop has exactly one processing sweep that compares gathered maps using the special combined network-cloud/group identity key.
- [x] The processing sweep performs hash-map comparison only against gathered state and performs no client calls.
- [x] Manager sweep writes are submitted through one maximum apply boundary per pass.
- [x] There is one patch call maximum, carrying all resources that need patching.
- [x] There is one put call maximum, used only for new resources or where patch is impossible.
- [x] No single resource is both patched and put in the same pass.
- [x] Already-correct resources receive zero patch/put/update/status/finalizer/delete calls anywhere in the entire loop path.
- [x] The new apply boundary still supports resource-version dependencies for status/finalizer writes without per-resource re-querying.
- [x] Tests prove the call count and behavior for mixed write buckets.
- [x] I manually verified with concrete calls, commands, logs, screenshots, external service status, or other evidence that the bug no longer occurs.
- [x] The verification evidence is recorded in the task or linked artifact.
</acceptance_criteria>

<verification_evidence>
Implementation evidence:
- `internal/stateoperator/operator.go` now uses the `NSXNetworkCloud` object from the global list directly in `runCloudSweep`; the post-list `Get` refresh was removed.
- `internal/stateoperator/manager_pipeline.go` now builds `ManagerNSXWritePlan` with patch/put/delete buckets keyed by `BindingKey{NetworkCloudFQDN, GroupID}` and rejects the same key in both patch and put buckets.
- `internal/stateoperator/manager_pipeline.go` now calls `ApplyManagerNSXWrites` once for classified NSX writes and `ApplyManagerKubeWrites` once for the complete Kubernetes write plan.
- `internal/stateoperator/manager_kube_writes.go` now returns `ManagerKubeApplyResult` and performs resource-version dependent create/update/status/finalizer/delete/cloud-status staging inside one Kubernetes apply boundary without re-querying.
- `internal/stateoperator/reconciler.go` now makes controller-runtime `NetworkCloudReconciler` and `GroupReconciler` observer-only event loggers; they do not `Get`, `List`, `Update`, `Status().Update`, mutate finalizers, or call NSX manager.
- Obsolete split-phase helpers `managerKubeObjectWrites`, `managerKubePostObjectWrites`, and their finalizer split helpers were deleted.

Focused test evidence:
- `TestStartDoesNotQuerySameCloudTwiceInOneSweep` proves the state operator does not re-`Get` a cloud after the global list.
- `TestApplyManagerPlanSubmitsMixedKubeWritesThroughOneApplyBoundary` proves mixed Kubernetes creates, updates, direct statuses, status-after-write, finalizers, deletes, and cloud status are submitted through one manager kube apply call.
- `TestKubeAPIAdapterUsesPriorBatchResourceVersionsWithoutGet` records Kubernetes API requests and proves generated status/finalizer writes use prior batch result resource versions with zero `GET` for the group.
- `TestProcessManagerSnapshotManageGroupsWriteMissingAndDriftedAndOnlyStatusMatching` proves missing managed groups are classified for put, existing drifted groups for patch, and matching groups avoid writes.
- `TestApplyManagerPlanAppliesClassifiedNSXWritesThroughPutOrPatch`, `TestApplyManagerPlanAppliesClassifiedNSXDeletes`, and `TestApplyManagerPlanRejectsSameNSXResourceInPatchAndPutBuckets` prove the NSX write bucket behavior.
- `TestNetworkCloudReconcileObservesEventWithoutClient` and `TestGroupReconcileObservesEventWithoutClientOrNSXMutation` prove controller-runtime reconcilers are not write/read escape hatches.
- `TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI` now verifies observe and manage deletion through the manager sweep path rather than direct group reconciler writes.

Gate evidence from final run:
- `make check` passed. This included lint, projectlint, `go test ./...`, `go test -race ./...`, targeted mockapi/envtest suites, largechaos tests, and coverage.
- `make test` passed.
- `make test-coverage` passed with aggregate coverage `86.9%`, above the required `80.0%` threshold.
- New boundary function coverage from `go tool cover -func=coverage.out`:
  - `ApplyManagerKubeWrites`: `82.5%`
  - `ApplyManagerNSXWrites`: `93.3%`
  - `NetworkCloudReconciler.Reconcile`: `87.5%`
  - `GroupReconciler.Reconcile`: `87.5%`
</verification_evidence>

<plan>
Path: `.ralph/tasks/bugs/manager-sweep-uses-multiple-kube-batch-calls_plans/01-one-gather-one-process-one-apply-plan.md`

NOW EXECUTE
</plan>

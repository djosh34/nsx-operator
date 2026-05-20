## Bug: Manager Sweep Must Be One Gather One Process One Apply With No Regets Or Write Churn <status>not_started</status> <passes>false</passes> <priority>ultra high</priority>

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
- [ ] I reproduced or inspected the broken behavior enough to understand the failure.
- [ ] I fixed the bug.
- [ ] The full loop has exactly one initial gather phase for all required Kubernetes state and NSX Manager list-groups state.
- [ ] There are zero Kubernetes or NSX Manager re-get/re-list calls after the initial gather anywhere in the loop path.
- [ ] The full loop has exactly one processing sweep that compares gathered maps using the special combined network-cloud/group identity key.
- [ ] The processing sweep performs hash-map comparison only against gathered state and performs no client calls.
- [ ] Manager sweep writes are submitted through one maximum apply boundary per pass.
- [ ] There is one patch call maximum, carrying all resources that need patching.
- [ ] There is one put call maximum, used only for new resources or where patch is impossible.
- [ ] No single resource is both patched and put in the same pass.
- [ ] Already-correct resources receive zero patch/put/update/status/finalizer/delete calls anywhere in the entire loop path.
- [ ] The new apply boundary still supports resource-version dependencies for status/finalizer writes without per-resource re-querying.
- [ ] Tests prove the call count and behavior for mixed write buckets.
- [ ] I manually verified with concrete calls, commands, logs, screenshots, external service status, or other evidence that the bug no longer occurs.
- [ ] The verification evidence is recorded in the task or linked artifact.
</acceptance_criteria>

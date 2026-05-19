## Task: Remove Get Then Update From Reconcile Loops <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Vastly improve reconcile-loop performance by eliminating per-item kube-api `get` then patch/update behavior from all write flows. Reconcile is currently slow because loops perform one get call and then one patch call per item. The operator must move to a gather pass, process pass, apply pass model.

Hard requirement:
- Nowhere in reconcile write flows may there be a per-resource get-then-update/get-then-patch flow.
- This applies to all kube-api write paths.
- The requirement must be manually and deeply verified, not accepted by code inspection alone.

Required reconcile model:
- Gather pass: load all relevant current CRs from kube-api and all relevant current resources from the NSX-T manager.
- Process pass: compare the gathered views in memory, divide resources into needed apply/patches, put/updates, creations, deletions, finalizer patches, status changes, and skipped already-correct resources.
- Process pass: directly prepare the typed request maps that will be passed to the kube-api batch functions.
- Apply pass: submit Kubernetes writes through the generic kube-api batch APIs from the previous task.
- Avoid kube-api writes entirely for already-correct resources, including already-correct status where `lastTransitionTime` would be the only changed value.
- Apply the no get-then-update rule to all write flows. A write path may use objects and resource versions gathered earlier, but must not perform a fresh per-item get immediately before patch or put/update.

Verification requirements:
- Add tests or instrumentation at the typed kube-api interface proving reconcile no longer performs per-item kube-api get calls before patch or put/update.
- Execute reconcile behavior against real running components where practical, using `../nsx-t-mockapi` for NSX-T behavior and a kube-api test environment.
- Test with large resource counts sufficient to show the previous O(n) get+patch pattern is gone; include at least one 10,000+ resource scenario.
- Record typed kube-api interface call counts that show gather list calls plus batched writes, not one get plus one write per resource.
- Run `go test ./...` and relevant race tests. Do not skip failing tests.

Logging and error requirements:
- Use zap structured debug logs for reconcile load, compare, batch construction, skipped unchanged resources, and batch results.
- Use info logs for larger reconcile actions, including resource counts loaded, writes planned, writes skipped, batch size, worker count, and completion summary.
- Do not ignore errors from list calls, compare logic, batch construction, batch execution, result aggregation, or verification setup.

In scope:
- All reconcile loops that read CRs, compare with NSX-T manager state, patch status/spec, put/update CRs, or otherwise write Kubernetes resources.
- Tests and manual verification evidence proving the no get-then-update rule.

Out of scope:
- Changing user-facing CRD semantics unless required to preserve existing behavior under the new reconcile model.
- Changing NSX-T write semantics beyond what is needed for the compare and batch-write flow.


</description>


<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] No reconcile write flow contains or executes a per-resource kube-api get followed by patch or put/update.
- [x] Reconcile uses a gather pass that loads all relevant CR state and all relevant NSX-T manager state before compare/write planning.
- [x] Reconcile uses a process pass that divides resources into needed apply/patches, put/updates, creations, deletions, finalizer patches, status changes, and skipped already-correct resources.
- [x] Reconcile prepares the typed batch request maps during the process pass.
- [x] Reconcile uses an apply pass that sends the prepared request maps to generic kube-api batch functions.
- [x] Already-correct resources produce no kube-api write, including no status patch when `lastTransitionTime` would be the only changed value.
- [x] Tests or instrumentation at the typed kube-api interface prove request counts are full-list plus required batched writes, not one get plus one write per item.
- [x] Large-scale verification with 10,000+ resources is run and recorded.
- [x] NSX-T-facing behavior is verified against `../nsx-t-mockapi` or equivalent testcontainers evidence.
- [x] Full relevant tests are run and recorded, including `go test ./...` and race tests for batch/reconcile paths.
</acceptance_criteria>

<verification_evidence>
- Plan executed from `.ralph/tasks/33-story-reconcile-write-efficiency/03-task-remove-get-then-update-from-reconcile-loops_plans/01-remove-get-then-update-plan.md`.
- Focused typed kube-api manager sweep evidence:
  - Command: `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run 'TestDefaultManagerSweepAppliesObserveUpsertStatusAndDeleteThroughTypedKubeAPI' -count=1 -v`
  - Result: PASS.
  - Recorded call counts from typed kube-api metrics: `groups.list=5 groups.create=3 groups.update=1 groups.update_status=2 groups.patch=2 groups.delete=1 networkclouds.update_status=1 groups.get=0 networkclouds.get=0`.
  - The test runs through envtest Kubernetes and verifies imported observe CRs, drift repair, finalizer removal, observe deletion, and zero typed apply-phase GET calls.
- Large-scale 10,000+ resource evidence:
  - Command: `GOMEMLIMIT=768MiB KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test -tags largechaos ./internal/stateoperator -run 'TestLarge(MixedGroupsThroughRealKubernetesClient|RemoteGroupsPlanObserveUpserts)' -count=1 -timeout 20m -v`
  - Result: PASS.
  - Recorded evidence: `created and paged 10000 real CRs, gathered 10000 local groups, planned 5000 managed writes, 5000 observe deletes, 5000 group status batch requests, 5000 group delete batch requests, and zero typed per-item get requests in process/apply plan in 11.931084342s`.
  - Remote-only large plan evidence: `planned 2000 observe create batch requests and 2000 status-after-create requests`.
- NSX-T mockapi / contract evidence:
  - `make check` ran `go test ./internal/nsxclient -run 'Test(MockAPIRouteInventoryIsSupportedAndContracted|TypedClientContractsAgainstMockAPI|SharedRateLimitedClientConcurrencyAgainstMockAPI)' -count=1` and passed.
  - `make check` ran `go test ./internal/stateoperator -run 'Test(NetworkCloudAddAndRemoveLifecycleAgainstPublicMockAPI|LifecycleObserveAndManageDeletionDifferAgainstMockAPI)' -count=1` under envtest and passed.
- Race and coverage evidence:
  - `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test -race ./internal/stateoperator -run 'Manager|Sweep|Reconcile' -count=1`: PASS, `internal/stateoperator 36.677s`.
  - `make test-race`: PASS for all packages after fixing request-body metrics accounting to use atomic counters.
  - `make test-coverage`: PASS, total coverage `85.0% meets 80.0% threshold`; `internal/stateoperator` coverage `84.9%`; `internal/kubeapi` coverage `80.7%`.
- Required gates:
  - `make test`: PASS.
  - `make test-large-chaos`: PASS.
  - `make check`: PASS. It completed fmt, vet, lint (`0 issues`), normal tests, race tests, contract tests, e2e tests, large-chaos tests, and coverage.
- Boundary review:
  - Used `$improve-code-boundaries` final check and split manager Kubernetes write planning/apply helpers into `internal/stateoperator/manager_kube_writes.go`, keeping `internal/kubeapi` as the typed REST/batch client and `manager_pipeline.go` focused on gather/process/sweep flow.
</verification_evidence>

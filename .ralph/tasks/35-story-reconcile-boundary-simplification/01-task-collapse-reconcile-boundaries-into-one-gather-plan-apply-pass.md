## Task: Collapse Reconcile Boundaries Into One Gather Plan Apply Pass <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Radically simplify reconciliation by replacing the current split between controller-runtime event reconcilers, periodic manager sweep, typed kube batch methods, and direct controller client writes with one coherent gather/process/apply boundary.

Boundary review using `$improve-code-boundaries` found a large mixed-responsibility problem:
- `internal/stateoperator/reconciler.go` mixes event handling, direct Kubernetes reads/writes, NSX write submission, finalizer mutation, status construction, status comparison, cloud lookup, and error classification.
- `internal/stateoperator/operator.go` has two gather concepts in one sweep: it lists all clouds, then refreshes each cloud with a second `Get` before sweeping it.
- `internal/stateoperator/manager_pipeline.go` has a cleaner gather/process/apply pipeline, but `ApplyManagerPlan` still splits Kubernetes apply across pre-object and post-object write phases.
- `internal/stateoperator/manager_kube_writes.go` buckets writes, but exposes operation-specific maps and performs several operation-specific batch calls rather than one obvious "apply this write set" boundary.
- `internal/kubeapi` is a typed REST and batch client, while `stateoperator` owns reconcile semantics. That boundary should stay, but `stateoperator` should provide a single write-set planner so event reconcilers and sweeps cannot bypass batching.

Large improvement requested:
- Introduce a single reconcile pass model for both periodic sweeps and controller-runtime events:
  1. Gather all relevant Kubernetes resources and NSX manager resources once.
  2. Build one in-memory indexed snapshot.
  3. Process that snapshot into a strongly typed write set.
  4. Apply the write set through one Kubernetes batch apply boundary and one NSX write boundary.
- Remove or shrink direct write helpers in `reconciler.go` after their behavior is represented in the shared planner.
- Make duplicate same-pass resource queries impossible by construction. If a resource is in the gathered snapshot, later phases must use the snapshot entry, not re-query Kubernetes.
- Keep structured zap logging for gather counts, buckets, skipped unchanged resources, batch sizes, dependency ordering, and pass completion.
- Do not change CRD semantics unless required to preserve existing behavior under the simplified model.

This story is intentionally large. It should be split into focused child tasks before implementation if needed, but the end state must be simpler, not another adapter layer around the current split paths.


</description>


<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Controller-runtime event reconcilers and periodic sweeps use the same gather/process/apply reconcile boundary.
- [ ] Production reconcile code has no direct Kubernetes write bypass around the shared batched write boundary.
- [ ] A single pass never queries the same Kubernetes resource twice.
- [ ] Kubernetes writes are bucketed and submitted through one maximum batch apply boundary per pass.
- [ ] Existing behavior for observe imports, observe drift repair, observe deletion, manage apply, manage deletion, finalizers, and status conditions is preserved.
- [ ] Verification includes the duplicate-query regression test, controller reconcile tests, manager sweep tests, large-count tests, and `go test ./...`.
</acceptance_criteria>

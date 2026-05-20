## Task: Collapse Reconcile Boundaries Into One Gather Plan Apply Pass <status>done</status> <passes>true</passes>

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
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Controller-runtime event reconcilers and periodic sweeps use the same gather/process/apply reconcile boundary.
- [x] Production reconcile code has no direct Kubernetes write bypass around the shared batched write boundary.
- [x] A single pass never queries the same Kubernetes resource twice.
- [x] Kubernetes writes are bucketed and submitted through one maximum batch apply boundary per pass.
- [x] Existing behavior for observe imports, observe drift repair, observe deletion, manage apply, manage deletion, finalizers, and status conditions is preserved.
- [x] Verification includes the duplicate-query regression test, controller reconcile tests, manager sweep tests, large-count tests, and `go test ./...`.
</acceptance_criteria>

<plan>
.ralph/tasks/35-story-reconcile-boundary-simplification/01-task-collapse-reconcile-boundaries-into-one-gather-plan-apply-pass_plans/01-unified-reconcile-pass.md

NOW EXECUTE
</plan>

<verification_evidence>
Implementation summary:
- Added `internal/stateoperator/reconcile_pass.go` with `ReconcileTrigger`, `ReconcilePassRunner`, `DefaultReconcilePassRunner`, and `ReconcilePassKubeClient`.
- `DefaultReconcilePassRunner` gathers all `NSXNetworkCloud` and `NSXGroup` resources exactly once per pass, narrows sweep/networkCloud/group triggers in memory, gathers NSX manager groups per selected cloud, reuses gathered local groups to build `ManagerSnapshot`, and applies through `ApplyManagerPlan`.
- `NetworkCloudReconciler` and `GroupReconciler` now call the shared runner with event triggers; context cancellation returns before invoking the runner, and runner errors propagate.
- `NSXStateOperator.runSweep` now calls the shared runner with `ReconcileTriggerSweep`; sweep cloud processing inside the pass remains concurrent and per-cloud failures are logged without stopping the sweeper, preserving prior sweep behavior.
- `startup.NewManager` builds one production runner for normal construction and passes it to the operator plus both controller-runtime reconcilers. Explicit `SweepCloud` remains a test-only compatibility override.
- Removed the old `defaultManagerSweep` duplicate pipeline from `manager_pipeline.go`.

Concrete test evidence:
- `make check` passed after implementation. This ran gofumpt, `go vet ./...`, `golangci-lint run ./...` with `0 issues`, `projectlint ./...`, envtest-backed `go test ./...`, `go test -race ./...`, mockapi contract/lifecycle subsets, selected non-cached API/startup/stateoperator/cmd tests, largechaos tests, and coverage.
- `make test` passed: envtest-backed `go test ./...` passed for all packages.
- `make test-coverage` passed: `coverage 87.2% meets 80.0% threshold`; package coverage included `internal/stateoperator` at `89.9%` and `internal/startup` at `86.6%`.
- Focused runner/startup tests passed while developing:
  - `go test ./internal/stateoperator -run 'TestDefaultReconcilePassRunner'`
  - `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/startup -run TestNewManagerDefaultSweepUpdatesCloudStatusWithoutCustomSweep`
  - `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test -race ./internal/stateoperator -run TestDefaultReconcilePassRunnerSweepGathersKubernetesStateOnce`
- Boundary review evidence:
  - `rg` confirmed production reconcile writes go through `RunReconcilePass` -> `ApplyManagerPlan` -> `ApplyManagerKubeWrites`; direct typed Kubernetes create/update/status/delete calls found under `internal/stateoperator` are tests or the single `manager_kube_writes.go` batch boundary.
  - The old `defaultManagerSweep` duplicate gather/process/apply implementation was removed.
  - Same-pass duplicate Kubernetes reads are covered by `TestDefaultReconcilePassRunnerSweepGathersKubernetesStateOnce`, event-scope runner tests, existing duplicate-query operator coverage, and the full `make check` suite.
</verification_evidence>

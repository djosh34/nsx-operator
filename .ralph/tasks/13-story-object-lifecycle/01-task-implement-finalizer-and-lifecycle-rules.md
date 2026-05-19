## Task: Implement Finalizer And Object Lifecycle Rules <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement Kubernetes object lifecycle behavior around the hard-coded finalizer `nsx.ing.com/finalizer`.

In scope: add finalizer to every NSXGroup the operator creates or manages; Observe deletion removes finalizer immediately without NSX delete; Manage deletion keeps finalizer until confirmed NSX absence from a successful future list; NSXNetworkCloud deletion stops future manager sweeps after the cloud object disappears; deleting a cloud does not mass-delete child NSXGroup CRs. Out of scope: any finalizer on NSXNetworkCloud unless needed by existing repo conventions and explicitly justified.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] E2E evidence proves Observe and Manage deletion differ exactly as specified.
- [x] E2E evidence proves cloud deletion does not delete child group CRs.
</acceptance_criteria>

<verification_evidence>

Focused behavior and E2E command:

```bash
KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run 'TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI|TestLifecycleCloudDeletionLeavesChildGroupsAndStopsDefaultSweepThroughTypedKubeAPI|TestStartSkipsCloudDeletedAfterGlobalList|TestGroupReconcileObserveDoesNotMutateNSXOrRequeue' -count=1 -v
```

Observed output:

```text
=== RUN   TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI
--- PASS: TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI (5.83s)
=== RUN   TestLifecycleCloudDeletionLeavesChildGroupsAndStopsDefaultSweepThroughTypedKubeAPI
--- PASS: TestLifecycleCloudDeletionLeavesChildGroupsAndStopsDefaultSweepThroughTypedKubeAPI (5.88s)
=== RUN   TestStartSkipsCloudDeletedAfterGlobalList
--- PASS: TestStartSkipsCloudDeletedAfterGlobalList (0.05s)
=== RUN   TestGroupReconcileObserveDoesNotMutateNSXOrRequeue
--- PASS: TestGroupReconcileObserveDoesNotMutateNSXOrRequeue (0.00s)
PASS
ok  	github.com/djosh34/nsx-operator/internal/stateoperator	11.797s
```

Concrete lifecycle evidence from the focused tests:

- `TestGroupReconcileObserveDoesNotMutateNSXOrRequeue` verifies a non-deleting Observe `NSXGroup` receives `nsx.ing.com/finalizer` and does not construct an NSX manager client.
- `TestStartSkipsCloudDeletedAfterGlobalList` verifies a cloud returned by the global list but deleted before per-cloud sweep does not call `SweepCloud`.
- `TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI` starts envtest and the sibling `../nsx-t-mockapi`, seeds remote groups with the real `nsxclient`, then verifies:
  - deleting an Observe group removes the Kubernetes finalizer and the mockapi remote group `observe-remote` remains present;
  - deleting a Manage group submits the mockapi delete for `manage-remote`;
  - the Manage group keeps `nsx.ing.com/finalizer` immediately after delete submission;
  - a later successful default manager sweep lists mockapi, confirms `manage-remote` absent, and lets Kubernetes delete the managed CR.
- `TestLifecycleCloudDeletionLeavesChildGroupsAndStopsDefaultSweepThroughTypedKubeAPI` starts envtest, runs a default manager sweep with the cloud present, deletes the `NSXNetworkCloud`, verifies child group `child-survives` is still present, and verifies a later default manager sweep does not construct a manager client for the deleted cloud.

Mandatory checks:

```bash
make check
```

Observed result: passed. It ran `gofumpt`, `golangci-lint run ./...` with `0 issues`, `go test ./...`, and `go test -cover ./...`.

```bash
make test
```

Observed result: passed for all packages:

```text
ok  	github.com/djosh34/nsx-operator/api/v1alpha	(cached)
ok  	github.com/djosh34/nsx-operator/cmd/nsx-operator	(cached)
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)
ok  	github.com/djosh34/nsx-operator/internal/config	(cached)
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	(cached)
ok  	github.com/djosh34/nsx-operator/internal/kubeapi	(cached)
ok  	github.com/djosh34/nsx-operator/internal/logging	(cached)
ok  	github.com/djosh34/nsx-operator/internal/names	(cached)
ok  	github.com/djosh34/nsx-operator/internal/nsxclient	(cached)
ok  	github.com/djosh34/nsx-operator/internal/startup	(cached)
ok  	github.com/djosh34/nsx-operator/internal/stateoperator	(cached)
ok  	github.com/djosh34/nsx-operator/internal/statuscondition	(cached)
```

```bash
make test-coverage
```

Observed result: passed; every package reported at least 80% statement coverage:

```text
ok  	github.com/djosh34/nsx-operator/api/v1alpha	(cached)	coverage: 100.0% of statements
ok  	github.com/djosh34/nsx-operator/cmd/nsx-operator	(cached)	coverage: 80.8% of statements
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)	coverage: 100.0% of statements
ok  	github.com/djosh34/nsx-operator/internal/config	(cached)	coverage: 82.9% of statements
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	(cached)	coverage: 87.8% of statements
ok  	github.com/djosh34/nsx-operator/internal/kubeapi	(cached)	coverage: 80.9% of statements
ok  	github.com/djosh34/nsx-operator/internal/logging	(cached)	coverage: 96.2% of statements
ok  	github.com/djosh34/nsx-operator/internal/names	(cached)	coverage: 93.9% of statements
ok  	github.com/djosh34/nsx-operator/internal/nsxclient	(cached)	coverage: 80.4% of statements
ok  	github.com/djosh34/nsx-operator/internal/startup	(cached)	coverage: 80.9% of statements
ok  	github.com/djosh34/nsx-operator/internal/stateoperator	(cached)	coverage: 80.2% of statements
ok  	github.com/djosh34/nsx-operator/internal/statuscondition	(cached)	coverage: 91.1% of statements
```

Final improve-code-boundaries check:

- Lifecycle logic remains in `internal/stateoperator`; no Kubernetes lifecycle behavior was moved into startup or `kubeapi`.
- No `NSXNetworkCloud` finalizer was added.
- No `ownerReferences` were added from `NSXGroup` to `NSXNetworkCloud`; cloud deletion cannot garbage-collect child groups.
- Existing `ManagerPlan` and `kubeAPIAdapter` boundaries were reused; no duplicate lifecycle pipeline or DTO layer was introduced.

</verification_evidence>

<plan>
.ralph/tasks/13-story-object-lifecycle/01-task-implement-finalizer-and-lifecycle-rules_plans/2026-05-19-finalizer-lifecycle-plan.md
</plan>

DONE

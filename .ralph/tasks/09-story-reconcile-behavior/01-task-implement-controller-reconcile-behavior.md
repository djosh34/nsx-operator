## Task: Implement Controller Reconcile Behavior <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement controller-runtime Reconcile behavior for both CRDs while keeping retry behavior owned by future sweeps or Kubernetes events.

In scope: NSXNetworkCloud create/update no-op behavior that lets next sweep pick up changes; cloud deletion stops future sweeps once the object disappears; NSXGroup Observe reconcile performs no NSX mutation and removes finalizer immediately when deletionTimestamp is set; NSXGroup Manage reconcile submits managed apply when not deleting, sets `Applying=True` and `Synced=Unknown`, and performs no explicit requeue; Manage deletion sends NSX DeleteGroup, sets `Deleting=True`, keeps finalizer, and performs no explicit requeue. Conflict/precondition errors set `Applying=False` and `Synced=Unknown`; rate-limit, unavailable, and network errors set affected conditions Unknown where applicable.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Tests prove Observe reconcile never mutates NSX.
- [x] Tests prove Manage reconcile does not explicitly requeue and delegates confirmation to later sweeps/events.
- [x] Tests cover 409, 412, 429, 503, and network error condition behavior.
</acceptance_criteria>

<plan>
.ralph/tasks/09-story-reconcile-behavior/01-task-implement-controller-reconcile-behavior_plans/2026-05-19-controller-reconcile-behavior-plan.md
</plan>

NOW EXECUTE

<verification_evidence>
Completed on 2026-05-19.

Concrete commands run after implementation:

- `go test ./internal/stateoperator -run 'Test(NetworkCloudReconcile|GroupReconcile)' -count=1`
  - Result: passed.
  - Focused evidence for cloud no-op reconcile, observe no-mutation, observe deletion finalizer removal, manage apply/delete no explicit requeue, and classified apply/delete NSX errors.
- `make test`
  - Result: passed.
  - Packages reported passing: `api/v1alpha`, `cmd/nsx-operator`, `internal/buildinfo`, `internal/config`, `internal/httpratelimit`, `internal/kubeapi`, `internal/logging`, `internal/nsxclient`, `internal/startup`, `internal/stateoperator`, `internal/statuscondition`.
- `make check`
  - Result: passed.
  - Included `gofumpt -w .`, `golangci-lint run ./...` with `0 issues`, `go test ./...`, and `go test -cover ./...`.
- `make test-coverage`
  - Result: passed.
  - Coverage evidence: all reported packages were at least 80%; `internal/stateoperator` reported `80.2%`, `internal/startup` reported `80.9%`, and `internal/nsxclient` reported `80.3%`.

Behavior covered by tests:

- `TestGroupReconcileObserveDoesNotMutateNSXOrRequeue` fails if observe reconcile constructs an NSX manager client.
- `TestGroupReconcileObserveDeletionRemovesFinalizerWithoutNSXMutation` fails if observe deletion constructs an NSX manager client or keeps the operator finalizer.
- `TestGroupReconcileManageAppliesNSXStatusFinalizerAndDoesNotRequeue` verifies managed apply calls NSX, adds finalizer, sets `Applying=True` and `Synced=Unknown`, and returns `reconcile.Result{}`.
- `TestGroupReconcileManageDeletionDeletesNSXStatusKeepsFinalizerAndDoesNotRequeue` verifies managed deletion calls `DeleteGroup`, keeps finalizer, sets `Deleting=True` and `Synced=Unknown`, and returns `reconcile.Result{}`.
- Apply error tests cover 409 conflict, 412 precondition failed, 429 rate limited, 503 unavailable, and network errors with the required condition behavior and no explicit requeue.
- Delete classified error tests cover 409, 412, 429, 503, and network errors for affected delete conditions.

Final improve-code-boundaries review:

- Removed the shared no-op `NSXStateOperator.Reconcile` boundary so the sweep runnable is no longer reused as a controller reconciler for unrelated CRDs.
- Added `NetworkCloudReconciler` and `GroupReconciler` as type-specific controller-runtime boundaries.
- Kept sweep snapshot planning in the existing manager pipeline and kept immediate reconcile behavior focused on local lifecycle/write intent and condition updates.
- Startup now registers type-specific reconcilers while still adding `NSXStateOperator` only as the periodic sweep runnable.
</verification_evidence>

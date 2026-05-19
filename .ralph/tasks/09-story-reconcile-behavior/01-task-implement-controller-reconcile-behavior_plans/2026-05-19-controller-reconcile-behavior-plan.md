# Plan: Controller Reconcile Behavior

Task: `.ralph/tasks/09-story-reconcile-behavior/01-task-implement-controller-reconcile-behavior.md`

## Context

The current `NSXStateOperator` owns both the periodic sweep runnable and `Reconcile`, and startup registers that same reconciler for both `NSXNetworkCloud` and `NSXGroup`. That is a boundary problem: `reconcile.Request` carries only a name/namespace key, so one shared reconcile method cannot reliably know whether the request came from the cloud controller or group controller when both CRDs can have the same name.

Existing sweep behavior already handles broad observe/manage drift detection through `GatherManagerSnapshot`, `ProcessManagerSnapshot`, and `ApplyManagerPlan`. This task is narrower: immediate controller-runtime reconcile should handle local lifecycle/write intent and condition updates, then return an empty result so retries are owned by future sweeps or Kubernetes events.

## Boundary Plan Using `$improve-code-boundaries`

- Split the controller-runtime reconcile boundary by resource type.
- Keep `NSXStateOperator` as the periodic sweep runnable and dependency holder.
- Add type-specific reconcilers, for example:
  - `NetworkCloudReconciler` for `NSXNetworkCloud`.
  - `GroupReconciler` for `NSXGroup`.
- Update `startup.NewManager` to register those type-specific reconcilers instead of registering the sweep runnable itself for both CRDs.
- Keep the manager sweep pipeline as the deeper module for full remote/local comparison. Reconcile should not duplicate sweep snapshot planning.
- Add small focused helpers only where they hide real policy:
  - finalizer add/remove for groups.
  - status updates for apply/delete/error outcomes.
  - NSX error classification for 409, 412, 429, 503, and network errors.

## Public Interface And Types

Use existing public repository interfaces where possible:

- `stateoperator.New(options Options)` remains the construction entry point.
- `NSXStateOperator.Start(ctx)` remains the runnable periodic sweeper.
- `NetworkCloudReconciler.Reconcile(ctx, req)` returns `reconcile.Result{}` for found, deleting, and not-found clouds.
- `GroupReconciler.Reconcile(ctx, req)` returns `reconcile.Result{}` for observe/manage success and classified NSX errors; unexpected errors may return an error if the local Kubernetes operation itself failed.
- Extend `Options` only if needed with a typed group reconcile manager client factory. Prefer reusing existing `ManagerClientFactory`.
- Reuse `ManagerClient` and `kubeapi` concepts where they fit, but controller-runtime fake clients should drive most unit tests through the public `Reconcile` methods.
- Add one package constant for the group finalizer, for example `nsx.ing.com/finalizer`.

## Required Behavior

- `NSXNetworkCloud` create/update reconcile:
  - Get the cloud by request key.
  - If not found, log debug and return empty result.
  - If found, log debug fields and return empty result.
  - Do not call NSX and do not explicitly requeue.
  - Cloud deletion needs no finalizer work; once the API object disappears, sweeps stop because `runSweep` lists existing clouds.

- `NSXGroup` observe mode, not deleting:
  - Ensure no NSX mutation is made.
  - Return empty result.
  - Do not explicitly requeue.

- `NSXGroup` observe mode, deleting:
  - Remove the group finalizer immediately when present.
  - Do not call NSX.
  - Return empty result.

- `NSXGroup` manage mode, not deleting:
  - Ensure the group finalizer is present.
  - Build a managed apply from the desired spec and submit it through `ManagerClient` using existing managed write semantics where possible.
  - Set `Applying=True` and `Synced=Unknown`.
  - Do not explicitly requeue.

- `NSXGroup` manage mode, deleting:
  - Call `DeleteGroup` for `spec.groupID`.
  - Set `Deleting=True`.
  - Keep the finalizer so a later sweep/event can confirm and remove it.
  - Return empty result.

- 409 conflict and 412 precondition errors:
  - Set `Applying=False` and `Synced=Unknown` for manage apply.
  - Set affected delete condition to a non-success status for delete path if needed; acceptance requires conflict/precondition condition behavior for apply at minimum.
  - Return empty result unless a Kubernetes status/finalizer update fails.

- 429 rate-limit, 503 unavailable, and network errors:
  - Mark affected conditions `Unknown` where applicable.
  - For manage apply, `Applying=Unknown` and `Synced=Unknown`.
  - For manage delete, `Deleting=Unknown` and `Synced=Unknown` if `Synced` is present/meaningful on that path.
  - Return empty result unless a Kubernetes status/finalizer update fails.

## TDD Execution Plan Using `$tdd`

Use vertical red/green slices. Do not write all tests first.

1. Red: add a test that cloud reconcile for an existing `NSXNetworkCloud` returns an empty result, does not call NSX, and logs the key/resource type.
   Green: add `NetworkCloudReconciler` with no-op found/not-found behavior and update startup registration minimally.

2. Red: add a test that observe `NSXGroup` reconcile does not call any NSX mutation and returns an empty result.
   Green: add `GroupReconciler` dispatch for observe mode.

3. Red: add a test that deleting observe `NSXGroup` removes the finalizer and makes no NSX mutation.
   Green: implement finalizer removal for observe deletion.

4. Red: add a test that manage `NSXGroup` reconcile submits managed apply, sets `Applying=True` and `Synced=Unknown`, adds/keeps finalizer, and returns an empty result.
   Green: implement manage apply using existing `ManagerClient` write path or a shared exported wrapper around it if needed.

5. Red: add a test that manage deletion calls `DeleteGroup`, sets `Deleting=True`, keeps finalizer, and returns an empty result.
   Green: implement manage deletion path.

6. Red/green one case at a time for error behavior:
   - 409 conflict sets `Applying=False`, `Synced=Unknown`.
   - 412 precondition failed sets `Applying=False`, `Synced=Unknown`.
   - 429 rate limit sets `Applying=Unknown`, `Synced=Unknown`.
   - 503 unavailable sets `Applying=Unknown`, `Synced=Unknown`.
   - network error sets `Applying=Unknown`, `Synced=Unknown`.

7. Add or update startup/envtest integration coverage proving the manager registers cloud and group controllers through the new type-specific reconcilers and still starts the periodic sweeper.

8. Refactor only after green:
   - centralize condition construction for reconcile outcomes.
   - centralize NSX error classification.
   - remove any duplicate type-dispatch code left behind.
   - keep tests at public reconcile behavior level, not helper internals.

## Verification Commands

Run all required checks, without skipping failures:

```bash
make check
make test
make test-coverage
```

Coverage must remain at least 80% overall, and new reconcile code must have focused coverage for observe no-mutation, manage no-explicit-requeue, and 409/412/429/503/network condition behavior.

Manual verification evidence should be recorded in the task file after execution. Acceptable evidence for this task: exact `go test`/`make` commands and the focused test names proving the behavior, plus relevant log assertions from tests. If a live NSX-T verification is needed, use `../nsx-t-mockapi` with testcontainers rather than a real service.

NOW EXECUTE

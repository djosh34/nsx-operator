# Plan: Implement Manage Mode Reconciliation

Task: `.ralph/tasks/11-story-manage-reconciliation/01-task-implement-manage-reconciliation.md`

## Current Design Reading

`internal/stateoperator/manager_pipeline.go` is already the best public seam for Manage behavior:

- `GatherManagerSnapshot` reads typed Kubernetes groups and remote NSX policy groups.
- `ProcessManagerSnapshot` decides observe imports, observe deletes, managed writes, and status plans.
- `ApplyManagerPlan` applies Kubernetes and NSX mutations in deterministic order.
- `RemoteGroupFromNSXGroup` parses representable remote group expression state and expression IDs.

The direct `GroupReconciler` path in `internal/stateoperator/reconciler.go` already submits immediate Manage apply/delete operations and status updates, but it currently does not resolve remote expression IDs before apply. The sweep path can resolve those IDs because it already lists remote groups.

## Boundary Cleanup Required

Use `$improve-code-boundaries` during implementation to keep Manage orchestration behind the manager pipeline instead of spreading resource-specific expression logic across reconcilers.

Boundary target:

- Extract managed expression application out of `applyManagedWrite` into a small internal helper that owns represented-expression decisions: patch existing IP expression, delete existing IP expression when desired CIDRs are empty, add missing IP expression as `cidrs`, patch existing path expression, delete existing path expression when `segment_path` is removed, and add missing path expression as `segment`.
- Extend `ManagerClient` only with the missing path-expression operations needed by the behavior: `AddGroupPathExpression` and `DeleteGroupPathExpression`.
- Add a plan/applier shape for confirmed delete completion, for example `ManagedFinalizerRemovals []string`, so finalizer removal is only driven by a future successful NSX list proving remote absence. Do not remove the finalizer in the immediate delete reconciler after HTTP 200.
- Keep observe mode remote-to-spec mirroring isolated in observe helpers. Manage mode must never call observe spec replacement helpers for existing CRs.

## Interface And Type Design

Expected public/internal interfaces after implementation:

- `ManagerClient` includes:
  - `ListGroups`
  - `PatchGroup`
  - `PatchGroupIPAddressExpression`
  - `AddGroupIPAddressExpression`
  - `DeleteGroupIPAddressExpression`
  - `PatchGroupPathExpression`
  - `AddGroupPathExpression`
  - `DeleteGroupPathExpression`
  - `DeleteGroup`
- `ManagedGroupWrite` continues to carry desired values plus resolved expression IDs:
  - `Name`
  - `Key`
  - `DisplayName`
  - `CIDRs`
  - `SegmentPath`
  - `IPAddressExpressionID`
  - `PathExpressionID`
- `ManagerPlan` gains delete-confirmation output:
  - `ManagedFinalizerRemovals []string`
- `ManagerKubeApplier` gains:
  - `RemoveGroupFinalizer(ctx, name, finalizer string) error`

Do not add a separate DTO for local desired spec unless implementation shows repeated validation/conversion noise. The current `ManagedGroupWrite` is sufficient if expression operation construction is factored into one helper.

## TDD Execution Plan

Use `$tdd` with vertical slices. Write one failing behavioral test, implement only that behavior, then continue.

### Slice 1: Missing Path Expression Is Created

Red:

- Add one `ApplyManagerPlan` behavior test in `internal/stateoperator/manager_pipeline_test.go`.
- Input: managed write with `SegmentPath` set and no `PathExpressionID`.
- Expect operation order: `patch-group:<group>`, then `add-path:<group>:segment`.
- Recorder should capture the path expression ID and path payload, not just a string, so the test proves the public NSX client operation is meaningful.

Green:

- Add `AddGroupPathExpression` to `ManagerClient`.
- Implement `(*nsxclient.Client).AddGroupPathExpression` in `routes.go` using the existing group path-expression route and `?action=add`.
- Extend `operationRecorder`.
- Update `applyManagedWrite` or extracted helper to create ID `segment` when desired segment path exists but no remote path expression ID exists.

### Slice 2: Removed Segment Path Deletes Existing Path Expression

Red:

- Add one `ApplyManagerPlan` behavior test.
- Input: managed write with nil `SegmentPath` and `PathExpressionID: "existing-segment"`.
- Expect: `patch-group:<group>`, then `delete-path:<group>:existing-segment`.

Green:

- Add `DeleteGroupPathExpression` to `ManagerClient`.
- Implement `(*nsxclient.Client).DeleteGroupPathExpression`.
- Extend recorder and helper logic.

### Slice 3: Sweep Plans Confirmed Finalizer Removal Only After Remote Absence

Red:

- Add one `ProcessManagerSnapshot` test for a Manage group with `DeletionTimestamp` and finalizer where no matching remote group exists.
- Expect no `ManagedDeletes`, a `ManagedFinalizerRemovals` entry for the CR name, and a deleting/synced terminal status that reflects confirmed remote absence.
- Add one paired test where the deleting Manage group still has a matching remote group.
- Expect `ManagedDeletes` for the group ID, no finalizer removal, `Deleting=True`, and `Synced=Unknown`.

Green:

- In `ProcessManagerSnapshot`, branch deleting Manage groups before normal Manage drift handling.
- If remote exists, plan `ManagedDeletes` and delete-submitted status.
- If remote is absent, plan finalizer removal and confirmed-deleted status.
- Keep Observe deletion behavior unchanged.

### Slice 4: Apply Removes Finalizer Through Kube Applier

Red:

- Add one `ApplyManagerPlan` test with `ManagedFinalizerRemovals`.
- Expect finalizer removal occurs after statuses and before cloud status, or document and assert the chosen order.
- Add typed kube adapter coverage if existing fake test coverage does not observe finalizer mutation through `kubeAPIAdapter`.

Green:

- Extend `ManagerKubeApplier`.
- Implement `kubeAPIAdapter.RemoveGroupFinalizer` using get/update and preserving unrelated finalizers.
- Extend `operationRecorder`.

### Slice 5: Manage Spec Is Never Rewritten From Remote

Red:

- Add an integration-style manager sweep test through `defaultManagerSweep` with a Manage CR whose remote group has different display name, CIDRs, and segment path.
- Run the sweep.
- Assert the CR spec still equals the original desired spec and NSX write operations used the desired spec.

Green:

- Fix any accidental spec mutation in Manage branches. Expected result is likely already mostly true in `ProcessManagerSnapshot`, but this test is required acceptance coverage.

### Slice 6: Immediate Reconciler Applies Desired Segment Path

Red:

- Add or extend `GroupReconcileManageAppliesNSXStatusFinalizerAndDoesNotRequeue` with a desired `SegmentPath`.
- Since immediate reconcile cannot know remote expression IDs, expect `add-path:<group>:segment`.
- Also verify existing status expectations remain.

Green:

- Let `managedWriteFromLocal(group, RemoteGroup{})` plus the extracted expression helper handle segment add consistently.

### Slice 7: Client Route Contract Coverage

Red:

- Extend `internal/nsxclient/contract_test.go` and route tests to cover `AddGroupPathExpression` and `DeleteGroupPathExpression` by behavior through HTTP method/path/query, not brittle file text.

Green:

- Ensure routes match existing NSX Policy API conventions for group path expressions.

## Validation Plan

Required automated checks:

- `make check`
- `make test`
- `make test-coverage`
- Confirm `make test-coverage` reports 80%+ total coverage and new code paths have focused coverage through stateoperator and nsxclient tests.

Required manual/e2e evidence:

- Use `../nsx-t-mockapi` with testcontainers or local execution.
- Record concrete evidence in this task file or a linked artifact:
  - remote missing create/patch
  - drift repair
  - success synced
  - in-progress unknown realized
  - failure realized false
  - delete waits for confirmed absence
- Include commands, logs, and observed resource/status output. Do not mark acceptance based on code inspection only.

## Implementation Notes

- Keep zap structured logging. Add debug logs for each planned/applied managed expression operation and info logs for larger sweep/apply/delete/finalizer milestones if the touched code lacks visibility.
- Never ignore errors with `_`.
- Do not add a full group PUT replacement.
- Do not add tags or ownership metadata.
- Do not alter unrelated remote expressions.
- Do not remove finalizers based only on `DeleteGroup` HTTP 200.

NOW EXECUTE

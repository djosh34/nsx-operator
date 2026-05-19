Plan path: `.ralph/tasks/33-story-reconcile-write-efficiency/01-task-stop-unneeded-status-update-churn_plans/01-status-write-churn-plan.md`

# Stop Unneeded Status Update Churn

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- Current task file: `.ralph/tasks/33-story-reconcile-write-efficiency/01-task-stop-unneeded-status-update-churn.md`.
- `internal/statuscondition/status.go` already owns condition construction, deterministic ordering, `UnsupportedReason`, `ObservedGeneration`, and `LastTransitionTime` preservation when the condition `Status` does not change.
- `internal/stateoperator/manager_pipeline.go` currently builds desired statuses and applies every `GroupStatusPlan` and `CloudStatusPlan`; the current `kubeAPIAdapter` also fetches current objects before `UpdateStatus`, but it does not skip equal status.
- `internal/stateoperator/reconciler.go` has direct manage apply/delete status writes through `GroupReconciler.updateGroupStatus`; this path also needs the same no-op guard.
- Existing tests already cover many condition timestamp semantics and manager pipeline status shapes, so the new tests should focus on externally observable write/no-write behavior and logging, not private helper internals.

## Public Interface And Type Design

- No CRD schema change.
- No Kubernetes API type change.
- No NSX client API change.
- Add a small status comparison boundary in `internal/statuscondition`, for example:
  - `type StatusWriteDecision struct { Needed bool; Reason string; DriftFields []string }`
  - `CompareGroupStatus(current nsxv1alpha.NSXGroupStatus, desired nsxv1alpha.NSXGroupStatus) StatusWriteDecision`
  - `CompareNetworkCloudStatus(current nsxv1alpha.NSXNetworkCloudStatus, desired nsxv1alpha.NSXNetworkCloudStatus) StatusWriteDecision`
- Keep comparison semantics user-visible:
  - Compare `UnsupportedReason`.
  - Compare condition type set and order as produced by the builders.
  - Compare each condition's `Type`, `Status`, `Reason`, `Message`, `ObservedGeneration`, and `LastTransitionTime`.
  - Treat `nil` and empty condition slices as equivalent only when both represent no conditions.
  - Return a stable `Reason` such as `status_equal`, `unsupported_reason_changed`, `condition_count_changed`, `condition_type_changed`, `condition_status_changed`, `condition_reason_changed`, `condition_message_changed`, `condition_observed_generation_changed`, or `condition_last_transition_time_changed`.
- If implementation proves that `LastTransitionTime` must be ignored in comparison to meet Kubernetes condition semantics, switch this plan back to `TO BE VERIFIED` first. The expected design is that builders receive the persisted status as `previous`, so no-op desired status preserves the persisted transition time and strict equality is safe.

## Boundary Cleanup

- Use `$improve-code-boundaries` by keeping status equality in `internal/statuscondition`, next to status construction, instead of spreading ad hoc `reflect.DeepEqual` calls across reconcilers and adapters.
- Do not add another DTO layer that duplicates `NSXGroupStatus` or `NSXNetworkCloudStatus`.
- Do not push status comparison into `internal/kubeapi`; that package should remain a typed REST client and should not know which status fields are semantically meaningful.
- Keep `ManagerPlan` focused on work that should actually be applied. Prefer filtering no-op status plans during `ProcessManagerSnapshot` and direct reconcile status helpers. Use the final write adapter as a guard only if a race or test proves a second protection point is needed.
- Avoid modifying status builder behavior unless a RED test proves it is wrong. Timestamp preservation already exists and should be defended, not rewritten.

## Behaviors To Prove

- A manager sweep for an already-correct `NSXNetworkCloud` and existing `NSXGroup` produces zero status write work for those objects.
- A manager sweep still plans exactly the required status writes when any meaningful group field is stale, including condition `Status`, `Reason`, `Message`, `ObservedGeneration`, `UnsupportedReason`, or `LastTransitionTime` when the persisted transition timestamp is genuinely wrong for the desired condition state.
- `LastTransitionTime` stays stable across a no-op reconcile and changes only when a condition `Status` transitions.
- Direct manage apply/delete reconcile skips `Client.Status().Update` when the generated status equals the persisted group status.
- Direct manage apply/delete reconcile still writes status when an NSX outcome changes the user-visible condition state or message.
- Structured zap debug logs record status comparison decisions with resource identity, resource kind, whether a write was needed, and decision reason.
- Info logs are emitted only for actual status writes or larger reconcile/sweep actions that already deserve info-level visibility.

## TDD Execution Plan

1. [x] RED: Add a manager pipeline behavior test for an existing Observe or Manage group whose persisted status already matches the desired sweep status, including old `LastTransitionTime`. Assert `ProcessManagerSnapshot` has no `GroupStatusPlan` for that resource and preserves existing behavior for unrelated work.
2. [x] GREEN: Add `statuscondition.CompareGroupStatus` and use it in the group status planning points in `ProcessManagerSnapshot` so equal current/desired status is not added to `ManagerPlan.GroupStatuses`.
3. [x] RED: Add a manager pipeline behavior test for an already-correct cloud status. Assert no `CloudStatusPlan` is produced while the rest of the plan remains correct.
4. [x] GREEN: Add `statuscondition.CompareNetworkCloudStatus` and use it before assigning `ManagerPlan.CloudStatus`.
5. [x] RED: Add table-driven behavior coverage through public planning code showing stale group condition `Status`, `Reason`, `Message`, `ObservedGeneration`, and `UnsupportedReason` each produces exactly one `GroupStatusPlan`.
6. [x] GREEN: Complete comparison reasons and field drift detection. Keep helper tests focused on public status values if a compact table in `internal/statuscondition` is clearer than duplicating huge manager snapshots.
7. [x] RED: Add a timestamp behavior test proving no-op manager planning keeps an old `LastTransitionTime` and produces no status write, while a condition `Status` transition produces a planned status whose `LastTransitionTime` is the new clock time.
8. [x] GREEN: Adjust only the minimal condition planning or comparison code required. Do not refresh timestamps for non-transition updates.
9. [x] RED: Add a direct `GroupReconciler` test with a counting status writer or controller-runtime fake wrapper proving `updateGroupStatus` skips `Status().Update` when the generated manage apply/delete status equals persisted status.
10. [x] GREEN: Teach `GroupReconciler.updateGroupStatus` to compare `group.Status` with the desired status before calling `Client.Status().Update`.
11. [x] RED: Add direct reconciler drift tests proving stale apply/delete outcome status still calls `Status().Update` exactly once.
12. [x] GREEN: Preserve direct reconcile writes for meaningful status drift and return all client errors; no ignored errors.
13. [x] RED: Add zap observer assertions for manager sweep status comparison decisions and direct group reconcile decisions. Required fields should include `component`, resource kind/name, `statusWriteNeeded`, and `statusWriteReason`; include `networkCloudFQDN`/`groupID` where available.
14. [x] GREEN: Add structured debug logs for all status comparison decisions and info logs for actual status writes. Keep logs jsonl-compatible through existing zap logging.
15. [x] REFACTOR: Run `$improve-code-boundaries` review on touched code. Remove any duplicated compare code, avoid leaking kube client details into statuscondition, and keep plan/apply boundaries clear.
16. [x] VERIFY: Run focused tests after each green step, then full required checks:
    - `go test ./internal/statuscondition ./internal/stateoperator`
    - `go test -race ./internal/stateoperator`
    - `make check`
    - `make test`
    - `make test-coverage`
17. [x] MANUAL EVIDENCE: Record concrete evidence in the task file after implementation, including commands and output showing no-op status resource versions or counted status writes remain unchanged, and stale status still writes.
18. [ ] DONE: Only after all checks pass and coverage is 80%+, set `<passes>true</passes>`, run `/bin/bash .ralph/task_switch.sh`, commit all files with `task finished 01-task-stop-unneeded-status-update-churn: ...`, push, then quit immediately.

## Design Tripwires

- If strict status comparison fights Kubernetes condition semantics, switch the final marker and task marker back to `TO BE VERIFIED`, document the proposed comparison rule, and quit immediately.
- If filtering `ManagerPlan` status entries breaks required metrics or later batching assumptions, switch back to `TO BE VERIFIED` and propose a `StatusWriteDecision`-based plan shape before code changes continue.
- If controller-runtime fake clients cannot reliably count `Status().Update`, verify direct reconcile through resource version stability or a small wrapper at the client boundary, but do not test private helpers only.
- If implementation needs any CRD status field, enum, or public interface change, switch back to `TO BE VERIFIED` and quit immediately.

NOW EXECUTE

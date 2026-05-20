Plan path: `.ralph/tasks/bugs/state-operator-sweep-requeries-listed-cloud_plans/01-verify-single-cloud-query-boundary.md`

# State Operator Single Cloud Query Boundary

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- Current task file: `.ralph/tasks/bugs/state-operator-sweep-requeries-listed-cloud.md`.
- The task requires one state-operator sweep to avoid querying the same `NSXNetworkCloud` twice after the global cloud list.
- Current code inspected:
  - `internal/stateoperator/operator.go` `runSweep` lists `NSXNetworkCloud` objects into `clouds`.
  - `runSweep` currently passes a pointer to each listed cloud item into `runCloudSweep`.
  - `runCloudSweep` currently calls `o.sweepCloud(ctx, *cloud, sweep)` directly and does not call `client.Get`.
  - `internal/stateoperator/operator_test.go` contains `TestStartDoesNotQuerySameCloudTwiceInOneSweep` and a `duplicateQueryDetectingClient`.
- The task description appears to describe older broken code, so execution must first reproduce or verify the current state before making any changes.

## Public Interface And Type Design

- Keep the existing public operator surface:
  - `type CloudSweepFunc func(ctx context.Context, cloud nsxv1alpha.NSXNetworkCloud, sweep SweepContext) error`
  - `type SweepContext struct { ID string }`
  - `New(options Options) (*NSXStateOperator, error)`
  - `Start(ctx context.Context) error`
- Do not add a new refresh callback, stale-object callback, or second client boundary for cloud validation.
- The listed `NSXNetworkCloud` object is the gathered cloud state for the sweep. Stale or deleted objects must be handled at the apply/write boundary where Kubernetes can return `NotFound`, not by a defensive post-list `Get`.
- If execution finds that stale/deleted cloud handling cannot remain safe without a per-cloud `Get`, switch this plan back to `TO BE VERIFIED` and redesign the boundary before coding.

## Boundary Cleanup From `$improve-code-boundaries`

- Boundary smell to prevent: `operator.go` should not own both a global list boundary and a per-item refresh boundary for the same cloud resource in one sweep.
- Desired boundary:
  - `runSweep` gathers cloud objects once.
  - `runCloudSweep` receives gathered cloud state and logs/sweeps that state.
  - `defaultManagerSweep` and lower apply functions handle write-time races using returned Kubernetes errors.
- Keep controller-runtime client access centralized in `runSweep` for cloud gathering. Do not hide a second cloud query in `runCloudSweep`, default sweep setup, or tests.
- Prefer deleting stale refresh helpers if any remain. Do not replace them with another duplicate query behind a different name.

## Behaviors To Prove

- A sweep that lists `NSXNetworkCloud/cloud-a` does not later `Get` `NSXNetworkCloud/cloud-a` in the same sweep.
- `TestStartDoesNotQuerySameCloudTwiceInOneSweep` passes without weakening `duplicateQueryDetectingClient` or its duplicate assertion.
- Deleted-after-list handling remains explicit and safe:
  - If `TestStartSkipsCloudDeletedAfterGlobalList` exists, keep it passing.
  - If the test was renamed or removed, locate the current behavioral equivalent or add a focused integration-style test through `Start`.
- Refresh-failure handling remains covered:
  - If `TestStartSkipsCloudWhenRefreshFails` exists, keep it passing.
  - If that old behavior no longer exists because refresh was removed, record the replacement evidence in the task and verify write-time `NotFound` handling instead.
- No errors are ignored with blank identifier assignments.
- New or touched logging uses zap structured fields and keeps large actions at info/debug levels consistent with the repo instructions.

## TDD Execution Plan

Follow vertical red-green cycles. Do not write all tests first.

1. [x] RED/REPRO: Run `go test ./internal/stateoperator -run TestStartDoesNotQuerySameCloudTwiceInOneSweep -count=1 -v` and record whether the task's failing evidence still reproduces.
2. [x] GREEN: If the duplicate-query test fails, remove the duplicate cloud `Get` from the sweep path by passing the listed cloud object through the cloud sweep boundary. If it already passes, do not churn production code.
3. [x] RED/REPRO: Run `go test ./internal/stateoperator -run 'TestStartDoesNotQuerySameCloudTwiceInOneSweep|TestStartSkipsCloudDeletedAfterGlobalList|TestStartSkipsCloudWhenRefreshFails' -count=1 -v`.
4. [x] GREEN: If stale/deleted-cloud tests fail because old refresh behavior was removed, add or update one public-interface test through `Start` that proves deletion races do not cause unsafe writes at the current apply boundary.
5. [x] REFACTOR: Run the `$improve-code-boundaries` review on touched state-operator code. Remove any leftover refresh-only helper or duplicate DTO shape if execution finds one.
6. [x] VERIFY focused state-operator package:
   - `go test ./internal/stateoperator -count=1`
7. [x] VERIFY required bug evidence:
   - `go test ./internal/stateoperator -run 'TestStartDoesNotQuerySameCloudTwiceInOneSweep|TestStartSkipsCloudDeletedAfterGlobalList|TestStartSkipsCloudWhenRefreshFails' -count=1`
8. [x] VERIFY repository gates:
   - `make check`
   - `make test`
   - `make test-coverage`
9. [x] VERIFY coverage:
   - Confirm new or changed code is at least 80% covered.
   - Confirm aggregate `make test-coverage` remains at least 80%.
10. [x] RECORD manual verification evidence in `.ralph/tasks/bugs/state-operator-sweep-requeries-listed-cloud.md`:
    - exact commands,
    - pass/fail outcomes,
    - whether the originally reported failure was stale or still reproduced,
    - boundary review notes.
11. [x] COMPLETE only after all gates pass:
    - tick the task acceptance criteria,
    - set `<passes>true</passes>`,
    - run `/bin/bash .ralph/task_switch.sh`,
    - add all files including `.ralph`,
    - commit with `task finished state-operator-sweep-requeries-listed-cloud: ...`,
    - push,
    - quit immediately.

## Switch-Back Conditions

- If execution discovers the current public `CloudSweepFunc` interface cannot express safe write-time stale handling, switch this plan back to `TO BE VERIFIED` before coding.
- If the deleted-after-list behavior requires a new state type or changed custom resource schema, switch this plan back to `TO BE VERIFIED` before coding.
- If required tests named in the bug task no longer exist and no equivalent behavior can be identified, switch this plan back to `TO BE VERIFIED` and document the missing test evidence.

NOW EXECUTE

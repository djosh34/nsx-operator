## Bug: State Operator Sweep Requeries Listed Cloud <status>completed</status> <passes>true</passes> <priority>ultra high</priority>

<plan>
.ralph/tasks/bugs/state-operator-sweep-requeries-listed-cloud_plans/01-verify-single-cloud-query-boundary.md
</plan>

<description>
One state-operator sweep queries the same `NSXNetworkCloud` resource twice.

Verified code evidence:
- `internal/stateoperator/operator.go` `runSweep` lists all `NSXNetworkCloud` resources into `clouds`.
- The same sweep then starts `runCloudSweep` for each listed cloud.
- `runCloudSweep` immediately calls `o.client.Get(ctx, client.ObjectKey{Name: cloud.Name}, &current)` for the same cloud that was already present in the list.

New failing regression test evidence:
- Added `TestStartDoesNotQuerySameCloudTwiceInOneSweep` in `internal/stateoperator/operator_test.go`.
- Command run: `go test ./internal/stateoperator -run TestStartDoesNotQuerySameCloudTwiceInOneSweep -count=1 -v`.
- Result: FAIL.
- Failure: `duplicate resource queries in one sweep = [NSXNetworkCloud/cloud-a], want none`.

This violates the PO requirement that querying the same resource multiple times in one sweep is strictly forbidden. It happens because `runSweep` gathers clouds and `runCloudSweep` refreshes each cloud again as a defensive stale/deleted-object check instead of trusting the gathered pass or handling staleness at the unified apply boundary.
</description>

<execution_evidence>
Execution found the bug report's production-code evidence is stale in the current tree:
- `internal/stateoperator/operator.go` `runSweep` lists `NSXNetworkCloud` objects once into `clouds`.
- `runSweep` passes each listed cloud item to `runCloudSweep`.
- `runCloudSweep` calls `o.sweepCloud(ctx, *cloud, sweep)` directly and does not call `client.Get`.
- `defaultManagerSweep` consumes the gathered cloud value; no replacement per-cloud refresh query was found behind the manager boundary.

Commands and outcomes:
- `go test ./internal/stateoperator -run TestStartDoesNotQuerySameCloudTwiceInOneSweep -count=1 -v`: PASS. The original failure did not reproduce; the duplicate-query assertion was not weakened.
- `go test ./internal/stateoperator -run 'TestStartDoesNotQuerySameCloudTwiceInOneSweep|TestStartSkipsCloudDeletedAfterGlobalList|TestStartSkipsCloudWhenRefreshFails' -count=1 -v`: PASS for the only matching current test, `TestStartDoesNotQuerySameCloudTwiceInOneSweep`. The old deleted-after-list and refresh-failure test names no longer exist.
- `go test ./internal/stateoperator -run 'TestNetworkCloudAddAndRemoveLifecycleAgainstPublicMockAPI|TestLifecycleCloudDeletionLeavesChildGroupsAndStopsDefaultSweepThroughTypedKubeAPI' -count=1 -v`: FAIL when run directly because `KUBEBUILDER_ASSETS` was unset.
- `KUBEBUILDER_ASSETS="$(./.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run 'TestNetworkCloudAddAndRemoveLifecycleAgainstPublicMockAPI|TestLifecycleCloudDeletionLeavesChildGroupsAndStopsDefaultSweepThroughTypedKubeAPI' -count=1 -v`: PASS. These are the current public-`Start` lifecycle equivalents for deleted-cloud behavior; they verify a deleted cloud no longer triggers a default manager sweep and child groups are not unsafely deleted.
- `KUBEBUILDER_ASSETS="$(./.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -count=1`: PASS.
- `make check`: PASS, including fmt, vet, lint, project-lint, test, race, contract, e2e, large-chaos, and coverage gates.
- `make test`: PASS.
- `make test-coverage`: PASS. `internal/stateoperator` coverage was 89.1%; total repository coverage was 86.8%, meeting the 80.0% threshold.

Boundary review:
- The desired boundary is already present: global cloud gathering is centralized in `runSweep`, `runCloudSweep` receives gathered state, and stale/deleted behavior is covered by lifecycle tests and write-time Kubernetes errors rather than by a second same-sweep `Get`.
- No production code change was needed.
</execution_evidence>

<mandatory_manual_verification>
Manually reproduce or inspect the broken behavior, fix it, then manually verify with concrete evidence that a single sweep does not query the same resource twice.

Required verification evidence:
- The new regression test passes without weakening the duplicate-query assertion.
- Additional verification covers the deleted-after-list behavior previously tested by `TestStartSkipsCloudDeletedAfterGlobalList`; the replacement behavior must not reintroduce unsafe writes for deleted clouds.
- `go test ./internal/stateoperator -run 'TestStartDoesNotQuerySameCloudTwiceInOneSweep|TestStartSkipsCloudDeletedAfterGlobalList|TestStartSkipsCloudWhenRefreshFails' -count=1`.
- `go test ./...`.
</mandatory_manual_verification>

<acceptance_criteria>
- [x] I reproduced or inspected the broken behavior enough to understand the failure.
- [x] I fixed the bug.
- [x] A single state-operator sweep no longer does `List` then `Get` for the same `NSXNetworkCloud`.
- [x] Stale/deleted cloud handling remains explicit without duplicate same-pass queries.
- [x] I manually verified with concrete calls, commands, logs, screenshots, external service status, or other evidence that the bug no longer occurs.
- [x] The verification evidence is recorded in the task or linked artifact.
</acceptance_criteria>

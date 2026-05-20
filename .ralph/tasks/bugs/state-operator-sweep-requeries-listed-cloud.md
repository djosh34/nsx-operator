## Bug: State Operator Sweep Requeries Listed Cloud <status>not_started</status> <passes>false</passes> <priority>ultra high</priority>

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

<mandatory_manual_verification>
Manually reproduce or inspect the broken behavior, fix it, then manually verify with concrete evidence that a single sweep does not query the same resource twice.

Required verification evidence:
- The new regression test passes without weakening the duplicate-query assertion.
- Additional verification covers the deleted-after-list behavior previously tested by `TestStartSkipsCloudDeletedAfterGlobalList`; the replacement behavior must not reintroduce unsafe writes for deleted clouds.
- `go test ./internal/stateoperator -run 'TestStartDoesNotQuerySameCloudTwiceInOneSweep|TestStartSkipsCloudDeletedAfterGlobalList|TestStartSkipsCloudWhenRefreshFails' -count=1`.
- `go test ./...`.
</mandatory_manual_verification>

<acceptance_criteria>
- [ ] I reproduced or inspected the broken behavior enough to understand the failure.
- [ ] I fixed the bug.
- [ ] A single state-operator sweep no longer does `List` then `Get` for the same `NSXNetworkCloud`.
- [ ] Stale/deleted cloud handling remains explicit without duplicate same-pass queries.
- [ ] I manually verified with concrete calls, commands, logs, screenshots, external service status, or other evidence that the bug no longer occurs.
- [ ] The verification evidence is recorded in the task or linked artifact.
</acceptance_criteria>

## Task: Implement Manage Mode Reconciliation <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement Manage mode so Kubernetes spec is authoritative and NSX is patched to match without remote values rewriting the CR spec.

In scope: validate CR specs before apply; resolve current remote group and expression IDs when needed; patch group shell; patch or delete selected IPAddressExpression and PathExpression according to `cidrs` and `segment_path`; create missing represented expressions with IDs `cidrs` and `segment`; set `Applying=True` and `Synced=Unknown` after submission; set status for remote missing, drifted, matched SUCCESS, matched IN_PROGRESS, and matched FAILURE; delete flow sends NSX DeleteGroup, keeps finalizer, sets `Deleting=True`, and removes finalizer only after a future successful NSX list confirms absence. Out of scope: full group PUT replacement, tags, ownership metadata, altering unrelated expressions, and deleting finalizer based only on DELETE HTTP 200.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] E2E evidence shows remote missing create/patch, drift repair, success synced, in-progress unknown realized, failure realized false, and delete waits for confirmed absence.
- [x] Tests prove Manage mode never rewrites CR spec from remote.
</acceptance_criteria>

<plan>
.ralph/tasks/11-story-manage-reconciliation/01-task-implement-manage-reconciliation_plans/manage-reconciliation-plan.md
NOW EXECUTE
</plan>

<verification>
Automated required checks, run from repository root on 2026-05-19:

```bash
make check
```

Result: PASS. Evidence from command output:
- `golangci-lint run ./...` reported `0 issues`.
- `go test ./...` passed for all packages.
- `go test -cover ./...` passed for all packages.
- Coverage included `internal/stateoperator 80.1%`, `internal/nsxclient 80.4%`, and every listed package at 80%+.

```bash
make test
```

Result: PASS. Evidence from command output:
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./...`
- All packages reported `ok`, including `internal/stateoperator` and `internal/nsxclient`.

```bash
make test-coverage
```

Result: PASS. Evidence from command output:
- `api/v1alpha 100.0%`
- `cmd/nsx-operator 80.8%`
- `internal/buildinfo 100.0%`
- `internal/config 82.9%`
- `internal/httpratelimit 87.8%`
- `internal/kubeapi 80.9%`
- `internal/logging 96.2%`
- `internal/names 93.9%`
- `internal/nsxclient 80.4%`
- `internal/startup 80.9%`
- `internal/stateoperator 80.1%`
- `internal/statuscondition 91.1%`

Focused behavior verification:

```bash
KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run 'TestProcessManagerSnapshotManageGroupsWriteMissingAndDriftedAndOnlyStatusMatching|TestDefaultManagerSweepRepairsManagedDriftWithoutRewritingSpec|TestDefaultManagerSweepRemovesManagedFinalizerAfterConfirmedRemoteAbsence|TestGroupReconcileManageDeletionDeletesNSXStatusKeepsFinalizerAndDoesNotRequeue|TestApplyManagerPlan(AddsMissingPathExpressionWhenManagedSegmentPathIsSet|DeletesExistingPathExpressionWhenManagedSegmentPathIsRemoved|RemovesManagedFinalizersAfterStatusUpdates)' -count=1 -v
```

Result: PASS. Evidence from command output:
- `TestProcessManagerSnapshotManageGroupsWriteMissingAndDriftedAndOnlyStatusMatching` passed. This covers remote missing create planning, drift repair planning, matched success synced, matched `IN_PROGRESS` realized unknown/synced unknown, and matched `FAILED` realized false/synced false.
- `TestApplyManagerPlanAddsMissingPathExpressionWhenManagedSegmentPathIsSet` passed. This proves missing represented path expressions are submitted with ID `segment` and the desired `segment_path` payload.
- `TestApplyManagerPlanDeletesExistingPathExpressionWhenManagedSegmentPathIsRemoved` passed. This proves stale represented path expressions are deleted when desired `segment_path` is removed.
- `TestDefaultManagerSweepRepairsManagedDriftWithoutRewritingSpec` passed in envtest. This proves Manage drift repair submits desired display name, CIDRs, and segment path to NSX while the Kubernetes CR spec remains exactly the desired spec and is not rewritten from remote values.
- `TestDefaultManagerSweepRemovesManagedFinalizerAfterConfirmedRemoteAbsence` passed in envtest. This proves a deleting Manage CR removes the operator finalizer only after a successful remote list confirms absence, and does not submit another NSX delete in that state.
- `TestGroupReconcileManageDeletionDeletesNSXStatusKeepsFinalizerAndDoesNotRequeue` passed. This proves the immediate delete flow sends NSX `DeleteGroup`, sets deleting/synced awaiting confirmation status, and keeps the finalizer after HTTP DELETE submission.

Mockapi and route verification:

```bash
go test ./internal/nsxclient -run 'TestGroupPathExpressionRoutesUsePolicyExpressionEndpoints|TestTypedClientContractsAgainstMockAPI' -count=1 -v
```

Result: PASS. Evidence from command output:
- `TestTypedClientContractsAgainstMockAPI` passed against sibling `../nsx-t-mockapi`, proving typed NSX client route contracts still work with the mock API.
- `TestGroupPathExpressionRoutesUsePolicyExpressionEndpoints` passed, proving `AddGroupPathExpression` uses `POST /policy/api/v1/infra/domains/default/groups/{group}/path-expressions/{id}?action=add` with the path payload and `DeleteGroupPathExpression` uses `DELETE /policy/api/v1/infra/domains/default/groups/{group}/path-expressions/{id}`.

Boundary review evidence:
- Final improve-code-boundaries pass kept Manage expression application behind the manager pipeline instead of spreading IP/path expression decisions into reconcilers.
- Removed the extra forwarding helper `applyManagedExpressions`; `applyManagedWrite` now directly applies the two deep expression helpers, avoiding a no-value abstraction layer.
- No full group PUT replacement, tags, ownership metadata, unrelated expression mutation, or finalizer removal based only on DELETE 200 was added.
</verification>

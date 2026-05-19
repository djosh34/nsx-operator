## Task: Enforce NSX Client And Write Semantics <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Ensure all NSX writes follow the explicit PATCH/delete semantics and that the NSX client remains a transport/client layer without hidden reconciliation behavior.

In scope: group patches use PATCH APIs for represented fields; no full PUT group replacement for group management; no complete expression list overwrite; no tags; no ownership metadata; no alteration of unrelated NSX fields; IPAddressExpression patch replaces only selected IP expression; PathExpression patch replaces only selected path expression; selected represented expressions are deleted only when spec says the represented field is absent; NSX client returns typed errors and never automatically read-modify-writes, refetches after stale revision, reapplies after 409/412, or retries 429/503. Out of scope: operator-level next-sweep decisions already covered in runtime/reconcile tasks.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Contract tests inspect mockapi requests and prove only the documented PATCH/delete endpoints and bodies are used.
- [x] Tests prove unrelated remote expressions survive managed writes.
</acceptance_criteria>

<verification_evidence>
- Focused stateoperator command passed:
  - `go test ./internal/stateoperator -run 'TestApplyManagerPlanPatchesOnlyRepresentedGroupWriteFields|TestManagedWriteUsesSelectedPatchEndpointsAndPreservesUnrelatedMockAPIExpressions|TestManagedWriteDeletesOnlySelectedIPAddressExpressionWhenCIDRsAreAbsent|TestApplyManagerPlanDeletesExistingPathExpressionWhenManagedSegmentPathIsRemoved' -count=1 -v`
  - Evidence from output: all four named tests passed; package `github.com/djosh34/nsx-operator/internal/stateoperator` passed.
- Focused nsxclient command passed:
  - `go test ./internal/nsxclient -run 'TestTypedClientContractsAgainstMockAPI|TestGroupPathExpressionRoutesUsePolicyExpressionEndpoints|TestWriteStatusErrorsAreTypedAndNotRetried|TestStatusErrorsMapTypedCodes' -count=1 -v`
  - Evidence from output: typed mockapi contract, path-expression route, typed status mapping, and write no-retry tests passed; package `github.com/djosh34/nsx-operator/internal/nsxclient` passed.
- Mockapi request evidence from `TestManagedWriteUsesSelectedPatchEndpointsAndPreservesUnrelatedMockAPIExpressions`:
  - `PATCH /policy/api/v1/infra/domains/default/groups/managed-write-mock` with body `{"id":"managed-write-mock","display_name":"Managed Write Mock","resource_type":"Group"}`.
  - `PATCH /policy/api/v1/infra/domains/default/groups/managed-write-mock/ip-address-expressions/selected-ip` with body `{"id":"selected-ip","resource_type":"IPAddressExpression","ip_addresses":["10.42.0.0/24"]}`.
  - `PATCH /policy/api/v1/infra/domains/default/groups/managed-write-mock/path-expressions/selected-path` with body `{"id":"selected-path","resource_type":"PathExpression","paths":["/infra/segments/new"]}`.
  - Read-back evidence asserted IP members include unrelated `10.99.0.1` plus managed `10.42.0.0/24`; group members include unrelated `/infra/domains/default/groups/unrelated-group`; segment members include unrelated `/infra/segments/unrelated` plus managed `/infra/segments/new`.
- Mockapi delete evidence from `TestManagedWriteDeletesOnlySelectedIPAddressExpressionWhenCIDRsAreAbsent`:
  - Recorded only `PATCH /policy/api/v1/infra/domains/default/groups/managed-delete-selected-ip` and `DELETE /policy/api/v1/infra/domains/default/groups/managed-delete-selected-ip/ip-address-expressions/selected-ip`.
  - Read-back evidence asserted unrelated IP `10.99.0.2` remained, selected IP `10.2.0.0/24` was absent, and unrelated segment `/infra/segments/unrelated-delete` remained.
- Local path-expression delete evidence:
  - `TestApplyManagerPlanDeletesExistingPathExpressionWhenManagedSegmentPathIsRemoved` asserted `ApplyManagerPlan` emits `patch-group:removed-segment` then `delete-path:removed-segment:existing-segment` when managed segment path is absent.
  - `TestGroupPathExpressionRoutesUsePolicyExpressionEndpoints` asserted `DeleteGroupPathExpression` sends `DELETE /policy/api/v1/infra/domains/default/groups/web/path-expressions/old-segment` with no query string.
- Typed error/no-retry evidence:
  - `TestWriteStatusErrorsAreTypedAndNotRetried` covered 409, 412, 429, and 503 from `PatchGroup`, asserted `errors.As` to `ConflictError`, `PreconditionFailedError`, `RateLimitedError`, and `ServiceUnavailableError`, and asserted exactly one request per case.
- Mandatory final checks passed after the final file split:
  - `make check`: lint reported `0 issues`; `go test ./...` passed; `go test -cover ./...` passed.
  - `make test`: `go test ./...` passed for all packages.
  - `make test-coverage`: passed with package coverage including `internal/nsxclient` 80.4% and `internal/stateoperator` 80.2%; every listed package was 80%+.
- Final boundary check:
  - `internal/nsxclient` remains a transport layer with typed request payloads and no retry/refetch/reconcile loop.
  - `internal/stateoperator` still owns write intent and selected-expression decisions.
  - Write payload DTOs are intentionally narrow (`GroupPatch`, `IPAddressExpressionPatch`, `PathExpressionPatch`) so read-only `Resource` metadata does not leak into managed PATCH bodies.
  - Mockapi request-recorder tests and helpers live in `internal/stateoperator/manager_pipeline_write_semantics_test.go` instead of adding more bulk to the already-large manager pipeline test file.
</verification_evidence>

<plan>
.ralph/tasks/14-story-nsx-write-semantics/01-task-enforce-nsx-write-semantics_plans/2026-05-19-nsx-write-semantics-plan.md
</plan>

DONE

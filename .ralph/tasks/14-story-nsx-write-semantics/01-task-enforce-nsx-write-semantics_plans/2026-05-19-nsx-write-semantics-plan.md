# Enforce NSX Client And Write Semantics Plan

Plan path: `.ralph/tasks/14-story-nsx-write-semantics/01-task-enforce-nsx-write-semantics_plans/2026-05-19-nsx-write-semantics-plan.md`

## Current Reading

- Task file: `.ralph/tasks/14-story-nsx-write-semantics/01-task-enforce-nsx-write-semantics.md`.
- Required skills read: `$tdd` and `$improve-code-boundaries`.
- Relevant code:
  - `internal/nsxclient/routes.go` exposes typed transport operations for policy groups, group expressions, deletes, and status-returning calls.
  - `internal/nsxclient/client.go` owns request construction and response handling. It does one `httpClient.Do` per call, returns typed status errors for 409, 412, 429, and 503, and currently has no retry/refetch/reapply loop.
  - `internal/nsxclient/contract_test.go` already starts the sibling `../nsx-t-mockapi` and verifies every supported route is contracted.
  - `internal/stateoperator/manager_pipeline.go` owns manager write semantics. It patches the group resource, patches/adds/deletes selected IP and path expression resources, and does not pass full group expression lists from local desired state.
  - `internal/stateoperator/manager_pipeline_test.go` already has manager-plan and apply-order tests plus mockapi/envtest E2E patterns.
- Observed acceptance gaps:
  - Existing tests cover route presence and operation order, but not exact outbound methods, URLs, query actions, and JSON bodies for managed group writes.
  - Existing tests do not yet prove unrelated mockapi-stored remote expressions survive a managed write.
  - Existing tests do not explicitly prove managed writes never call group `PUT`, never send a complete `expression` list, and never send tags or ownership metadata.
  - Existing tests do not explicitly prove `nsxclient` returns typed errors without retrying 409, 412, 429, or 503.

## Boundary Design

- Keep `internal/nsxclient` as a thin typed transport/client layer:
  - no reconciliation decisions,
  - no automatic read-modify-write,
  - no refetch-after-stale-revision logic,
  - no retry policy for 409, 412, 429, or 503,
  - no hidden special cases based on group expression contents.
- Keep write-intent decisions inside `internal/stateoperator`:
  - `ManagedGroupWrite` continues to carry represented desired fields plus the selected remote expression IDs discovered during snapshot processing.
  - `applyManagedWrite` remains the place that translates write intent into typed client calls.
- Avoid creating duplicate DTOs for request assertions. Tests should decode the actual outbound JSON into `map[string]any` or existing `nsxclient` types at the test boundary only.
- Boundary smell to check with `$improve-code-boundaries`: do not add a second "group patch builder" layer unless it removes real duplication. The existing deep module is `ApplyManagerPlan`; strengthen its behavioral contract before introducing new public API.
- If execution reveals existing code sends unwanted resource fields because `nsxclient.Resource` cannot distinguish read-only metadata from write payloads, introduce a small write payload type at the stateoperator boundary instead of teaching `nsxclient` reconciliation semantics.

## Public Interface And Behavior

- No CRD schema change is planned.
- No exported operator API change is planned unless tests prove a write payload type is required.
- The observable behavior should be:
  - Managed group writes call policy group `PATCH`, not group `PUT`.
  - Group patches include only represented group-level fields, currently group ID/display name/resource type when needed, and never include full `expression`, `extended_expression`, `tags`, ownership metadata, or unrelated remote fields.
  - Managed CIDR changes patch or add only the selected `IPAddressExpression` resource. If the desired CIDR list is empty and a selected IP expression exists, that selected expression is deleted.
  - Managed segment path changes patch or add only the selected `PathExpression` resource. If desired segment path is absent and a selected path expression exists, that selected expression is deleted.
  - Unrelated remote expressions remain stored in mockapi after managed writes; mockapi-backed delete persistence evidence is limited to expression routes the sibling mockapi currently implements.
  - `nsxclient` maps 409, 412, 429, and 503 responses to typed errors and performs exactly one outbound request for each call.

## TDD Execution Plan

Execute as vertical red-green-refactor cycles. Do not write a batch of tests first.

1. [x] RED: add one `internal/stateoperator` behavior test for a managed drift write with existing IP and path expression IDs. Assert via the public `ApplyManagerPlan` API and `operationRecorder` that only `patch-group`, selected `patch-ip`, and selected `patch-path` operations occur, and assert the recorded group patch has no full expression fields, no extended expression, no tags, and no ownership/read-only metadata.
2. [x] GREEN: adjust `applyManagedWrite` or introduce a minimal write payload so the test passes. Preserve structured error handling and existing operation order.
3. [x] RED: add one mockapi-backed test that seeds a group with unrelated expressions plus selected represented IP/path expressions, runs a managed write through the real `nsxclient`, then reads back from mockapi and proves unrelated expressions survived.
4. [x] GREEN: change only the write path needed to preserve unrelated remote expressions. Prefer selected expression PATCH/delete endpoints over any group expression-list replacement.
5. [x] RED: extend the mockapi-backed request evidence using a recording `http.RoundTripper` around the mockapi transport. Assert the real client sent:
   - `PATCH /policy/api/v1/infra/domains/default/groups/{group-id}`,
   - `PATCH /policy/api/v1/infra/domains/default/groups/{group-id}/ip-address-expressions/{selected-id}`,
   - `PATCH /policy/api/v1/infra/domains/default/groups/{group-id}/path-expressions/{selected-id}`,
   - `DELETE` only for selected absent represented IP expressions supported by mockapi,
   - no policy group `PUT`,
   - no `POST ?action=remove` or full expression overwrite for managed reconciliation.
6. [x] GREEN: fix endpoint selection only if the red test shows drift. Do not broaden `ManagerClient` beyond the task behavior.
7. [x] RED: add one `internal/nsxclient` transport test with a counting `RoundTripper` for 409, 412, 429, and 503. For each status, call a representative write method, assert the concrete typed error with `errors.As`, and assert the count is exactly one.
8. [x] GREEN: adjust `statusError` or response handling only if a status is not typed. Do not add retry logic.
9. [x] RED: revise the focused delete/absence behavior test so it remains executable against current dependencies:
   - keep mockapi-backed persistence/read-back evidence for selected `IPAddressExpression` delete, because sibling `../nsx-t-mockapi` implements `policy.groups.ip_address_expressions.delete`,
   - prove selected `PathExpression` delete through `ApplyManagerPlan` plus the existing/local `nsxclient` HTTP transport route contract, because sibling `../nsx-t-mockapi` does not register `DELETE /path-expressions/{expression-id}`,
   - prove that when a represented field is present, no delete is issued for that represented expression.
10. [x] GREEN: fix `applyManagedIPAddressExpression`, `applyManagedPathExpression`, or the test shape only if behavior differs. Do not patch sibling `../nsx-t-mockapi` as part of this operator task unless a later task explicitly targets that repository.
11. [x] Refactor after green:
    - collapse duplicated request-recording assertions into small test helpers,
    - keep helpers private to the package tests,
    - avoid new production abstractions unless they are required to prevent muddy write payload boundaries,
    - add or preserve zap debug/info logs for larger write actions if code paths change.

## Concrete Verification

Run these before completion and record exact output/evidence in the task file:

1. [x] Focused tests for write semantics, for example:
   - `go test ./internal/stateoperator -run 'WriteSemantics|ManagedWrite|UnrelatedExpressions' -count=1 -v`
   - `go test ./internal/nsxclient -run 'TypedStatusErrors|NoRetry|Contract' -count=1 -v`
2. [x] `make check`
3. [x] `make test`
4. [x] `make test-coverage`
5. [x] Confirm `make test-coverage` reports 80%+ total/package coverage and that new code paths have behavior coverage.
6. [x] Capture verification evidence in this task:
   - exact focused command outputs,
   - recorded mockapi request method/path/query/body evidence,
   - read-back evidence that unrelated expressions survive managed writes,
   - local transport evidence for path-expression DELETE, including method/path and exactly one request,
   - typed error/no-retry request counts for 409, 412, 429, and 503,
   - final mandatory command outputs and coverage percentages.

## Completion Steps

- [x] Design remained valid; if selected write payload types or interfaces need redesign, switch task marker back to `TO BE VERIFIED` and stop.
- [x] Final `$improve-code-boundaries` pass:
  - verify `nsxclient` remains transport-only,
  - verify stateoperator owns write intent,
  - verify there are no duplicate request DTO layers or stringly rendering paths,
  - verify no unrelated refactor churn was introduced.
- [x] Update `.ralph/tasks/14-story-nsx-write-semantics/01-task-enforce-nsx-write-semantics.md` with concrete evidence and set `<passes>true</passes>`.
- [x] Run `/bin/bash .ralph/task_switch.sh`.
- [ ] `git add` all changed files, including `.ralph`.
- [ ] Commit with `task finished 01-task-enforce-nsx-write-semantics: enforce NSX write semantics`.
- [ ] Push.

## Execution Finding Resolved In Plan

- During execution, `TestManagedWriteDeletesOnlySelectedExpressionsWhenRepresentedFieldsAreAbsent` proved the planned mockapi-backed delete evidence cannot currently pass for path expressions.
- Concrete failed command:
  - `go test ./internal/stateoperator -run TestManagedWriteDeletesOnlySelectedExpressionsWhenRepresentedFieldsAreAbsent -count=1 -v`
- Concrete failure:
  - `ApplyManagerPlan()` called `DELETE /policy/api/v1/infra/domains/default/groups/managed-delete-selected/path-expressions/selected-path`.
  - The sibling `../nsx-t-mockapi` returned `405 Method Not Allowed`.
  - Mockapi logs show `policy.groups.path_expressions.patch` is registered for `PATCH`, but no `DELETE` route is registered for `/path-expressions/{expression-id}`.
- Decision:
  - Do not expand this operator task into the sibling `nsx-t-mockapi` repository; the final commit for this task must be self-contained in `nsx-operator`.
  - Use mockapi for the supported persistence behavior it can prove: selected IP-expression delete and unrelated-expression survival across managed writes.
  - Use local transport/client contract evidence for selected path-expression DELETE until a separate mockapi task adds the missing route.

DONE

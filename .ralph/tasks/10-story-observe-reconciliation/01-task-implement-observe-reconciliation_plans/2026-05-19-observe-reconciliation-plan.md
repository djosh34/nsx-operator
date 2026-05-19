# Plan: Observe Mode Reconciliation

Task: `.ralph/tasks/10-story-observe-reconciliation/01-task-implement-observe-reconciliation.md`

## Current State

- The manager sweep pipeline already gathers local `NSXGroup` CRs and remote NSX groups, converts remote groups into `RemoteGroup`, processes a pure `ManagerSnapshot` into a `ManagerPlan`, and applies that plan through Kubernetes and NSX boundaries.
- Existing tests already cover remote-only Observe import, existing Observe spec replacement, Observe delete when remote is missing, unsupported remote status, default sweep application through typed kube API, and Observe controller deletion without NSX mutation.
- The selected task is not marked done and still has unmet acceptance criteria. Code inspection shows likely gaps:
  - Observe upsert objects currently set only `ObjectMeta{Name: ...}` and do not visibly add `GroupFinalizer`, while the task explicitly requires imported Observe CRs to have the finalizer.
  - Expression tests currently cover a combined IP+Path parse and a generic unsupported case, but not the exact required behaviors: empty expression, IP expression, IP OR segment expression, and unsupported expression.
  - Manual/e2e evidence for remote-only import, remote spec replacement, remote missing CR delete, and user delete without NSX delete is not recorded in this task.

## Interface And Behavior Target

Keep the public surface already established by previous stories:

- `GatherManagerSnapshot(ctx, cloud, listGroups, managerClientFactory)` remains the external-read boundary.
- `RemoteGroupFromNSXGroup(networkCloudFQDN, group)` remains the remote projection boundary for NSX group expression parsing.
- `ProcessManagerSnapshot(snapshot, now)` remains the pure policy boundary. It must not call Kubernetes, NSX, HTTP, loggers, clocks, or context.
- `ApplyManagerPlan(ctx, kubeApplier, managerClient, plan)` remains the apply boundary and must not call NSX for Observe-only changes.
- `GroupReconciler.Reconcile(ctx, req)` remains responsible for user deletion of Observe groups: remove the Kubernetes finalizer and do not construct or call an NSX manager client.

Required Observe behavior:

- Remote-only NSX group after successful gather plans an `NSXGroup` CR:
  - deterministic name from normalized cloud FQDN and `groupID`,
  - `GroupFinalizer`,
  - `spec.mode=Observe`,
  - `spec` projected from the remote group using the same remote projection rules,
  - status conditions for current remote state.
- Existing Observe CR with remote present replaces its spec from the remote while preserving Observe mode and ensuring the finalizer is present.
- Existing Observe CR with remote missing after successful gather plans Kubernetes CR delete and no NSX delete.
- Observe CR user deletion removes the finalizer and does not call NSX delete.
- Unsupported remote expressions preserve the best representable spec, set `UnsupportedExpression=True`, and keep `Synced=False` or `Unknown` according to current condition derivation.

## Expression Projection Rules

The expression parser should be explicit enough that future maintainers can tell which NSX expression shapes are supported:

- Empty `expression` and empty `extended_expression` are supported and produce empty `CIDRs`, nil `SegmentPath`, and `UnsupportedExpression=false`.
- A single `IPAddressExpression` is supported and maps `ip_addresses` unchanged into `spec.cidrs`.
- A supported IP OR segment expression maps one `IPAddressExpression` plus one `PathExpression` with exactly one path into `spec.cidrs` plus `spec.segment_path`.
- NSX `ConjunctionOperator` with `conjunction_operator=OR` may appear between the IP and path expressions and should not by itself make the expression unsupported.
- Unknown resource types, malformed JSON, duplicate IP expressions, duplicate path expressions, non-OR conjunction operators, extended expressions, and multi-path `PathExpression` values mark `UnsupportedExpression=true` while preserving any best representable IP/path facts already parsed.

If execution proves NSX represents IP OR segment differently than this plan assumes, switch the task marker back to `TO BE VERIFIED` and stop instead of forcing the wrong type design.

## Boundary Plan Using `$improve-code-boundaries`

Use the smell catalog as the final review checklist:

- Watch `internal/stateoperator/manager_pipeline.go` for smell 8, "too much in one file." Do not split mechanically, but if expression parsing grows materially, move the remote expression projection into a focused file in the same package, for example `remote_group_projection.go`.
- Avoid smell 1, useless overabstraction: do not introduce a duplicate DTO for `NSXGroupSpec`. The canonical local desired shape is already `nsxv1alpha.NSXGroupSpec`.
- Avoid smell 5, shared shape drift: keep one remote-to-observe-spec function and reuse it for both remote-only import and existing Observe replacement.
- Avoid smell 10, tiny one-off helpers: only extract helpers that carry real policy, such as "new observe group object from remote" or "parse supported expression token."
- Avoid smell 14, too public: keep new helpers private unless tests can cover behavior through `RemoteGroupFromNSXGroup`, `ProcessManagerSnapshot`, `ApplyManagerPlan`, or `GroupReconciler`.
- Keep gather/process/apply boundaries intact. Kubernetes and NSX client calls must not enter pure process or expression projection logic.
- Final review after all tests are green: scan for unchecked errors, duplicate object construction, stringly expression logic scattered across branches, and any Observe path that can call `DeleteGroup`.

## TDD Execution Plan Using `$tdd`

Follow vertical red-green cycles. One failing behavior test, minimum implementation, then move on.

1. [x] RED: add a focused `ProcessManagerSnapshot` test proving a remote-only Observe import includes `GroupFinalizer` on the planned upsert.
   GREEN: centralize Observe upsert construction and add the finalizer there.

2. [x] RED: extend the existing Observe drift test to assert existing Observe replacement also carries `GroupFinalizer`.
   GREEN: reuse the same centralized Observe object construction for existing Observe replacement.

3. [x] RED: add `RemoteGroupFromNSXGroup` behavior test for empty remote expression.
   GREEN: confirm or minimally adjust parsing so empty expression is supported with empty represented spec and `UnsupportedExpression=false`.

4. [x] RED: add `RemoteGroupFromNSXGroup` behavior test for a single `IPAddressExpression`.
   GREEN: keep IP parse behavior direct and ensure expression ID is retained.

5. [x] RED: add `RemoteGroupFromNSXGroup` behavior test for IP OR segment expression, including a `ConjunctionOperator` with `conjunction_operator=OR` between one IP expression and one one-path `PathExpression`.
   GREEN: teach the parser to accept that OR conjunction while retaining both expression IDs and the best representable spec.

6. [x] RED: add or split an unsupported-expression test that proves unsupported tokens preserve best representable fields and set `UnsupportedExpression=true`.
   GREEN: adjust parser only as needed; do not drop representable CIDRs or segment path because of a later unsupported token.

7. [x] RED: add/update apply-level test proving an Observe-only plan with upsert, status, and delete can run with `managerClient=nil` and never calls NSX `DeleteGroup`.
   GREEN: this should already be true in `ApplyManagerPlan`; if not, fix the apply boundary.

8. [x] RED: add/update controller reconcile test proving user deletion of an Observe group with finalizer removes that finalizer and never constructs the NSX manager client.
   GREEN: this likely already exists; if it exists and is sufficient, record it as evidence rather than duplicating it.

9. [ ] Refactor after green:
   - Collapse duplicate Observe upsert object construction.
   - If expression parsing becomes crowded, move it into a private same-package file without changing the public behavior surface.
   - Run focused tests after each refactor step.

## Verification Plan

Run these commands and record concrete output evidence in the task file:

```bash
go test ./internal/stateoperator -run 'Test(ProcessManagerSnapshot|RemoteGroupFromNSXGroup|ApplyManagerPlan|GroupReconcileObserve)' -count=1
make check
make test
make test-coverage
```

Coverage must remain at least 80% overall and new code must be covered at 80%+ by the focused stateoperator tests.

Manual/e2e evidence to record:

- Remote-only import: use the default sweep path with typed kube API and a recording or mock NSX manager response; prove the Observe CR is created with deterministic name, finalizer, Observe spec, and synced status.
- Remote change replacement: start with an Observe CR whose spec differs from remote; run sweep; prove the CR spec is replaced from remote and remains Observe.
- Remote missing delete: start with an Observe CR and a successful remote list that omits it; run sweep; prove Kubernetes CR delete occurs and no NSX delete occurs.
- User delete without NSX delete: reconcile an Observe CR with deletion timestamp/finalizer or use the existing controller test; prove finalizer removal and no NSX manager construction/delete.

NOW EXECUTE

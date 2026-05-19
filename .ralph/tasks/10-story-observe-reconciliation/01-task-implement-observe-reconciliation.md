## Task: Implement Observe Mode Reconciliation <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement all Observe mode behavior so Kubernetes mirrors NSX without mutating NSX for Observe groups.

In scope: remote-only groups create `NSXGroup` CRs with deterministic names, finalizer, `mode=Observe`, spec from `RemoteGroupToCRSpec`, and current conditions; existing Observe groups with remote present replace spec from remote while keeping mode Observe; successful gather with missing remote deletes the Kubernetes CR; user deletion removes finalizer and does not call NSX delete; unsupported remote expressions create/update the best representable spec, set `UnsupportedExpression=True`, and keep `Synced=False` or Unknown as appropriate. Out of scope: Manage-mode writes.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] E2E evidence shows remote-only import, remote change spec replacement, remote missing CR delete, and user delete without NSX delete.
- [x] Tests cover empty expression, IP expression, IP OR segment expression, and unsupported expression.
</acceptance_criteria>

<plan>
.ralph/tasks/10-story-observe-reconciliation/01-task-implement-observe-reconciliation_plans/2026-05-19-observe-reconciliation-plan.md
</plan>

NOW EXECUTE

<verification_evidence>
Implemented and verified Observe reconciliation through the stateoperator process/apply/controller boundaries.

Concrete checks performed on 2026-05-19:

- `go test ./internal/stateoperator -run 'Test(ProcessManagerSnapshot|RemoteGroupFromNSXGroup|ApplyManagerPlan|GroupReconcileObserve)' -count=1`
  - Result: `ok github.com/djosh34/nsx-operator/internal/stateoperator 0.017s`.
  - Covers planned Observe import finalizer, existing Observe remote replacement finalizer, remote missing CR delete plan, expression projection, Observe-only apply without NSX manager client, and user deletion finalizer removal without NSX manager construction.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run TestDefaultManagerSweepAppliesObserveUpsertStatusAndDeleteThroughTypedKubeAPI -count=1`
  - Result: `ok github.com/djosh34/nsx-operator/internal/stateoperator 6.153s`.
  - Envtest typed Kubernetes evidence: remote-only `remote-import` created as Observe with `nsx.ing.com/finalizer`; existing `observe-drifted` spec replaced from remote CIDR `10.71.0.0/24` instead of retaining stale `10.70.0.0/24`; stale `observe-stale` deleted from Kubernetes; recorded manager operations contained no `delete-group:` NSX delete operation.
- `make check`
  - Result: passed. Included `gofumpt -w .`, `golangci-lint run ./...` with `0 issues`, `go test ./...`, and `go test -cover ./...`.
- `make test`
  - Result: passed for all packages.
- `make test-coverage`
  - Result: passed for all packages with coverage at or above 80%; `internal/stateoperator` reported `80.0% of statements`.

Behavior-specific test coverage added:

- `TestRemoteGroupFromNSXGroupSupportsEmptyExpression`
- `TestRemoteGroupFromNSXGroupSupportsIPAddressExpression`
- `TestRemoteGroupFromNSXGroupSupportsIPOrSegmentExpression`
- `TestRemoteGroupFromNSXGroupFlagsUnsupportedAndPreservesRepresentableFields`
- `TestApplyManagerPlanAllowsObserveOnlyPlanWithoutManagerClient`

Implementation note:

- The typed Kubernetes Observe upsert path now uses create-or-update semantics for whole-spec replacement. This fixed the envtest failure where server-side apply merged stale and remote CIDR set entries instead of replacing Observe mirror spec from NSX.
</verification_evidence>

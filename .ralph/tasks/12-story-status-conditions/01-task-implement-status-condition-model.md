## Task: Implement Status Condition Model <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement status updates using conditions only for both CRDs. There must be no top-level synced field, remote object field, revision field, fingerprint field, or last sweep timestamp on `NSXGroup`.

In scope: condition helpers for `Reachable`, `Swept`, `RemotePresent`, `SpecMatchesRemote`, `UnsupportedExpression`, `Realized`, `Synced`, `Applying`, and `Deleting`; correct True/False/Unknown behavior from the design; observedGeneration handling; lastTransitionTime update only on transitions; descriptive reason/message; synced derivation based on RemotePresent=True, SpecMatchesRemote=True, UnsupportedExpression=False, and Realized=True. Out of scope: business logic encoded in reason/message strings.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Tests prove each condition status rule, including Unknown cases.
- [x] CRD/object verification proves statuses contain conditions only.
</acceptance_criteria>

<plan>
.ralph/tasks/12-story-status-conditions/01-task-implement-status-condition-model_plans/2026-05-19-status-condition-model-plan.md

NOW EXECUTE
</plan>

<verification>
Executed on 2026-05-19:

- `go test ./internal/statuscondition`
  - Passed.
  - Covers transition-time preservation, transition-time update on status change, deterministic condition ordering, observedGeneration assignment, helper coverage for `Reachable`, `Swept`, `RemotePresent`, `SpecMatchesRemote`, `UnsupportedExpression`, `Realized`, `Synced`, `Applying`, and `Deleting`, plus `Synced` True/False/Unknown derivation.
- `go test ./internal/stateoperator -run 'TestProcessManagerSnapshot|TestBuildBindings|TestRemoteGroupFromNSXGroup|TestGatherManagerSnapshot|TestApplyManagerPlan' -count=1`
  - Passed.
  - Covers cloud gather failure/success conditions, previous transition-time preservation, observedGeneration propagation, Observe full group condition sets, Manage missing/drifted/matched status rules, and `Realized=Unknown` deriving `Synced=Unknown`.
- `make check`
  - Passed.
  - Lint reported `0 issues.`
  - `go test ./...` passed under envtest.
  - `go test -cover ./...` passed under envtest.
- `make test`
  - Passed: all packages including `api/v1alpha`, `internal/stateoperator`, and `internal/statuscondition`.
- `make test-coverage`
  - Passed.
  - Coverage evidence: `api/v1alpha` 100.0%, `cmd/nsx-operator` 80.8%, `internal/buildinfo` 100.0%, `internal/config` 82.9%, `internal/httpratelimit` 87.8%, `internal/kubeapi` 80.9%, `internal/logging` 96.2%, `internal/nsxclient` 80.3%, `internal/startup` 80.9%, `internal/stateoperator` 81.6%, `internal/statuscondition` 91.1%.

CRD/object conditions-only verification:

- `api/v1alpha/crd_integration_test.go` now installs both CRDs into envtest, reads their structural OpenAPI schemas through the apiextensions client, and asserts each `status` schema exposes only the `conditions` property.
- The same envtest test updates status through the Kubernetes status subresource with valid `status.conditions` plus synthetic non-condition fields (`status.synced` and `status.remoteObject`), then reads the stored object and asserts the stored status properties are exactly `conditions`.

Final `$improve-code-boundaries` review:

- Production condition merge/order/generation/transition logic lives in `internal/statuscondition`.
- `internal/stateoperator/manager_pipeline.go` no longer has the old ad hoc `condition(...)` constructor and checks all condition-builder errors.
- No `NSXGroupStatus` top-level synced, remote object, revision, fingerprint, or last sweep timestamp fields exist in `api/v1alpha/types.go` or the CRD schemas.
</verification>

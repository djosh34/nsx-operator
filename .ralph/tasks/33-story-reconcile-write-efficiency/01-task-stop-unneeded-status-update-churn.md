## Task: Stop Unneeded Status Update Churn <status>not_started</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Stop the operator from patching Kubernetes status when the persisted status is already correct. The operator currently updates each resource `status` during reconcile regardless of whether the status fields are already correct. That causes unnecessary kube-api traffic, resource version churn, and slow reconciliation.

Status updates must be based on the current persisted status and the desired status produced by reconcile. If the persisted status already has the correct condition set, condition statuses, condition reasons, condition messages, observed generations, unsupported reason, sync state, and other status fields, reconcile must not call the kube-api status write path. In the current CRD status shape, `metav1.Condition.lastTransitionTime` is the time field that must not cause churn: it must stay equal when the condition `status` did not transition. If status content is equal, there is no benefit in writing only to refresh a timestamp; the next reconcile will rebuild the same correct status and must still skip the write.

Required behavior:
- Compare desired status against the current status before creating or applying a status write.
- Do not issue status write calls when the persisted status is already correct.
- Keep issuing status writes when meaningful status fields change, including condition status, reason, message, observed generation, unsupported reason, sync state, or any other user-visible correctness field.
- Preserve correct Kubernetes condition semantics: `lastTransitionTime` must only move when a condition actually transitions, not on every reconcile.
- Add zap structured debug logs for status comparison decisions, including resource identity, whether a status write was needed, and the reason.
- Add info logs only for larger actions or status writes that actually happen.
- Do not ignore errors from comparisons, status construction, client calls, patch/update generation, or test setup.

In scope:
- Status comparison helpers, condition builders, reconcile planning, reconcile apply paths that write status, kube-api client status write behavior if needed, unit tests, integration tests using fake or real kube-api behavior, and manual verification evidence.

Out of scope:
- Changing the meaning of any status condition beyond suppressing unneeded writes.
- Rewriting all reconcile data loading or batching behavior; that is covered by later tasks in this story.


</description>


<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Reconcile does not call the kube-api status write path when the persisted status is already correct.
- [x] Reconcile does call the kube-api status write path when a meaningful status field is stale or incorrect.
- [x] `lastTransitionTime` is stable across no-op reconciles and only changes on actual condition status transitions.
- [x] Tests prove no-op status reconcile produces zero status write calls for already-correct resources.
- [x] Tests prove meaningful status drift still produces exactly the required status write calls.
- [x] Tests prove a stale reason, message, observed generation, unsupported reason, or condition status produces a status write.
- [x] Debug logs record status comparison and skip/write decisions with structured zap fields.
- [x] Full relevant tests are run and recorded, including `go test ./...`; any race-sensitive status path tests are also run with `go test -race`.
</acceptance_criteria>

<verification_evidence>
Date: 2026-05-20

- Focused RED/GREEN evidence:
  - `go test ./internal/stateoperator -run TestProcessManagerSnapshotSkipsAlreadyCorrectGroupStatus -count=1` initially failed because `ProcessManagerSnapshot` emitted a `GroupStatusPlan` for `manage-ready`; after adding `statuscondition.CompareGroupStatus` and the planning guard it passed.
  - `go test ./internal/stateoperator -run TestProcessManagerSnapshotSkipsAlreadyCorrectCloudStatus -count=1` initially failed because `CloudStatus` was always planned; after adding `CompareNetworkCloudStatus` and the cloud planning guard it passed.
  - `go test ./internal/stateoperator -run TestGroupReconcileManageSkipsAlreadyCorrectApplyStatusUpdate -count=1` initially failed with `Status().Update calls = 1, want 0`; after adding the direct `GroupReconciler.updateGroupStatus` compare guard and structured debug log it passed.
- Concrete behavior tests now passing:
  - `go test ./internal/statuscondition -count=1` passed; `TestCompareGroupStatusDetectsMeaningfulDrift` verifies status/reason/message/observedGeneration/lastTransitionTime/unsupportedReason decision reasons and drift fields.
  - `go test ./internal/stateoperator -run 'TestProcessManagerSnapshot(SkipsAlreadyCorrect|PlansStatusForMeaningfulGroupDrift)|TestGroupReconcileManage(DeleteSkipsAlreadyCorrectDeleteStatusUpdate|DeleteWritesStaleDeleteStatusOnce|SkipsAlreadyCorrectApplyStatusUpdate|WritesStaleApplyStatusOnce)' -count=1` passed; this proves no-op manager planning skips writes, stale fields plan writes, direct apply/delete no-op calls zero `Status().Update`, and stale apply/delete calls exactly one `Status().Update`.
  - The direct no-op apply test asserts zap fields `resourceKind=NSXGroup`, `groupName=group-a`, `networkCloudFQDN=nsx-a.example.test`, `groupID=group-a`, `statusWriteReason=status_equal`, and `statusWriteNeeded=false`.
- Required full gates passed:
  - `make check` passed. Output included `0 issues.` from golangci-lint, `go test ./...` passing, `go test -race ./...` passing, contract/e2e/large-chaos targets passing, and coverage `84.4% meets 80.0% threshold`.
  - `make test` passed with `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./...`; all packages reported `ok`.
  - `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test -race ./internal/stateoperator` passed.
  - `make test-coverage` passed with total coverage `84.4%`, `internal/stateoperator` coverage `83.0%`, and `internal/statuscondition` coverage `88.0%`.
- Discovery errors were not ignored:
  - `rg ... test ...` reported `test: No such file or directory`; subsequent discovery used real repo paths.
  - `sed internal/stateoperator/reconciler_test.go` failed because that file does not exist; reconciler tests are in `internal/stateoperator/operator_test.go`.
  - `sed internal/logging/fields.go` failed because logging fields live in `internal/logging/logging.go`.
  - Plain `go test ./internal/statuscondition ./internal/stateoperator -count=1` failed outside `make test` because envtest tests require `KUBEBUILDER_ASSETS`; the required make/envtest-backed commands above passed.
</verification_evidence>

<plan>
.ralph/tasks/33-story-reconcile-write-efficiency/01-task-stop-unneeded-status-update-churn_plans/01-status-write-churn-plan.md
NOW EXECUTE
</plan>

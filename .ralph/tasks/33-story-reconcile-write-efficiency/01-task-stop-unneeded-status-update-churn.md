## Task: Stop Unneeded Status Update Churn <status>not_started</status> <passes>false</passes>

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
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Reconcile does not call the kube-api status write path when the persisted status is already correct.
- [ ] Reconcile does call the kube-api status write path when a meaningful status field is stale or incorrect.
- [ ] `lastTransitionTime` is stable across no-op reconciles and only changes on actual condition status transitions.
- [ ] Tests prove no-op status reconcile produces zero status write calls for already-correct resources.
- [ ] Tests prove meaningful status drift still produces exactly the required status write calls.
- [ ] Tests prove a stale reason, message, observed generation, unsupported reason, or condition status produces a status write.
- [ ] Debug logs record status comparison and skip/write decisions with structured zap fields.
- [ ] Full relevant tests are run and recorded, including `go test ./...`; any race-sensitive status path tests are also run with `go test -race`.
</acceptance_criteria>

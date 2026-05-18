## Task: Implement Manage Mode Reconciliation <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement Manage mode so Kubernetes spec is authoritative and NSX is patched to match without remote values rewriting the CR spec.

In scope: validate CR specs before apply; resolve current remote group and expression IDs when needed; patch group shell; patch or delete selected IPAddressExpression and PathExpression according to `cidrs` and `segment_path`; create missing represented expressions with IDs `cidrs` and `segment`; set `Applying=True` and `Synced=Unknown` after submission; set status for remote missing, drifted, matched SUCCESS, matched IN_PROGRESS, and matched FAILURE; delete flow sends NSX DeleteGroup, keeps finalizer, sets `Deleting=True`, and removes finalizer only after a future successful NSX list confirms absence. Out of scope: full group PUT replacement, tags, ownership metadata, altering unrelated expressions, and deleting finalizer based only on DELETE HTTP 200.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] E2E evidence shows remote missing create/patch, drift repair, success synced, in-progress unknown realized, failure realized false, and delete waits for confirmed absence.
- [ ] Tests prove Manage mode never rewrites CR spec from remote.
</acceptance_criteria>

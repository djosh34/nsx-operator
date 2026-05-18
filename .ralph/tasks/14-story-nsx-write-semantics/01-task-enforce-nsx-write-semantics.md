## Task: Enforce NSX Client And Write Semantics <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Ensure all NSX writes follow the explicit PATCH/delete semantics and that the NSX client remains a transport/client layer without hidden reconciliation behavior.

In scope: group patches use PATCH APIs for represented fields; no full PUT group replacement for group management; no complete expression list overwrite; no tags; no ownership metadata; no alteration of unrelated NSX fields; IPAddressExpression patch replaces only selected IP expression; PathExpression patch replaces only selected path expression; selected represented expressions are deleted only when spec says the represented field is absent; NSX client returns typed errors and never automatically read-modify-writes, refetches after stale revision, reapplies after 409/412, or retries 429/503. Out of scope: operator-level next-sweep decisions already covered in runtime/reconcile tasks.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Contract tests inspect mockapi requests and prove only the documented PATCH/delete endpoints and bodies are used.
- [ ] Tests prove unrelated remote expressions survive managed writes.
</acceptance_criteria>

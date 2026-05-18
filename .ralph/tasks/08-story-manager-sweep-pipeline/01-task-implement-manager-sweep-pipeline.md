## Task: Implement Per-Manager Gather Process Apply Pipeline <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement the manager sweep pipeline as exactly `gather all info -> process all info -> apply all planned changes`. The process stage must be pure and must not call Kubernetes, NSX, or network APIs.

In scope: `ManagerSnapshot`, `ManagerPlan`, binding key/types, `BuildBindings`, and `ProcessManagerSnapshot`; gather lists NSXGroup CRs filtered by normalized `spec.networkCloudFQDN`, lists all NSX groups with pagination, and stores remote groups/expression IDs in memory; processing plans remote-only Observe upserts, Observe spec replacement, Observe delete after successful gather, Manage create/patch on missing or drifted remote, status-only plans for matching Manage groups, and cloud condition updates for gather failure without child mass-marking; apply order is Observe upserts, managed NSX writes/deletes, group statuses, Observe deletes, cloud status.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Unit tests prove `ProcessManagerSnapshot` is deterministic and has no client dependencies.
- [ ] Integration evidence proves apply ordering and pagination.
- [ ] Failure tests prove failed gather only updates cloud conditions and does not mark all child groups missing.
</acceptance_criteria>

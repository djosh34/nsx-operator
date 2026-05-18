## Task: Implement Typed Kubernetes CRD Client <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/kubeapi` as a typed client-go wrapper for `NSXNetworkCloud` and `NSXGroup` CRs without using the official Kubernetes code generator. Callers should use typed objects and status structs, not raw JSON status patches.

In scope: clients for List, Get, Create, Update, Apply, UpdateStatus, Delete, and Watch for both CRDs; typed `ListOptions`, `FieldFilter`, and `filterBy(field, value)` abstraction mapping to field selectors such as `spec.networkCloudFQDN=<fqdn>`; required resourceVersion on Update, status-only update behavior, required apply field manager, and typed object handling. Out of scope: official code generation and raw `[]byte` status patch APIs.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Tests use a real or fake Kubernetes API path sufficient to prove field selectors, apply, watch, and status updates.
- [ ] Tests prove UpdateStatus cannot mutate spec.
</acceptance_criteria>

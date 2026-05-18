## Task: Implement Observe Mode Reconciliation <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement all Observe mode behavior so Kubernetes mirrors NSX without mutating NSX for Observe groups.

In scope: remote-only groups create `NSXGroup` CRs with deterministic names, finalizer, `mode=Observe`, spec from `RemoteGroupToCRSpec`, and current conditions; existing Observe groups with remote present replace spec from remote while keeping mode Observe; successful gather with missing remote deletes the Kubernetes CR; user deletion removes finalizer and does not call NSX delete; unsupported remote expressions create/update the best representable spec, set `UnsupportedExpression=True`, and keep `Synced=False` or Unknown as appropriate. Out of scope: Manage-mode writes.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] E2E evidence shows remote-only import, remote change spec replacement, remote missing CR delete, and user delete without NSX delete.
- [ ] Tests cover empty expression, IP expression, IP OR segment expression, and unsupported expression.
</acceptance_criteria>

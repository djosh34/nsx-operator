## Task: Implement Controller Reconcile Behavior <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement controller-runtime Reconcile behavior for both CRDs while keeping retry behavior owned by future sweeps or Kubernetes events.

In scope: NSXNetworkCloud create/update no-op behavior that lets next sweep pick up changes; cloud deletion stops future sweeps once the object disappears; NSXGroup Observe reconcile performs no NSX mutation and removes finalizer immediately when deletionTimestamp is set; NSXGroup Manage reconcile submits managed apply when not deleting, sets `Applying=True` and `Synced=Unknown`, and performs no explicit requeue; Manage deletion sends NSX DeleteGroup, sets `Deleting=True`, keeps finalizer, and performs no explicit requeue. Conflict/precondition errors set `Applying=False` and `Synced=Unknown`; rate-limit, unavailable, and network errors set affected conditions Unknown where applicable.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Tests prove Observe reconcile never mutates NSX.
- [ ] Tests prove Manage reconcile does not explicitly requeue and delegates confirmation to later sweeps/events.
- [ ] Tests cover 409, 412, 429, 503, and network error condition behavior.
</acceptance_criteria>

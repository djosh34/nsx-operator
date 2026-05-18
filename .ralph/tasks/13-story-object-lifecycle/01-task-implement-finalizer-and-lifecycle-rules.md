## Task: Implement Finalizer And Object Lifecycle Rules <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement Kubernetes object lifecycle behavior around the hard-coded finalizer `nsx.ing.com/finalizer`.

In scope: add finalizer to every NSXGroup the operator creates or manages; Observe deletion removes finalizer immediately without NSX delete; Manage deletion keeps finalizer until confirmed NSX absence from a successful future list; NSXNetworkCloud deletion stops future manager sweeps after the cloud object disappears; deleting a cloud does not mass-delete child NSXGroup CRs. Out of scope: any finalizer on NSXNetworkCloud unless needed by existing repo conventions and explicitly justified.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] E2E evidence proves Observe and Manage deletion differ exactly as specified.
- [ ] E2E evidence proves cloud deletion does not delete child group CRs.
</acceptance_criteria>

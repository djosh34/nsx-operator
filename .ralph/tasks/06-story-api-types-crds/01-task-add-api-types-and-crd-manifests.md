## Task: Add API Go Types And CRD Manifests <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add `api/v1alpha` Go types and Kubernetes CRD manifests for `NSXNetworkCloud` and `NSXGroup` using API group `nsx.ing.com` and version `v1alpha`. The CRDs are cluster-scoped and must install on Kubernetes v1.32 or newer.

In scope: structs exactly matching the design for specs, status structs with `[]metav1.Condition`, top-level resource/list types, mode constants `Observe` and `Manage`, condition constants, scheme registration, and any required deepcopy support; CRDs with `openAPIV3Schema`, status subresource, selectableFields, printer columns, and status conditions only. `NSXGroup` must expose `networkCloudFQDN`, `groupID`, `display_name`, `mode`, `cidrs`, and nullable `segment_path`; no `domainId` field may exist anywhere in CRD specs.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] CRDs are installed into a real Kubernetes API server and accepted.
- [ ] Verification proves selectableFields work for `spec.networkCloudFQDN` and other documented fields.
</acceptance_criteria>

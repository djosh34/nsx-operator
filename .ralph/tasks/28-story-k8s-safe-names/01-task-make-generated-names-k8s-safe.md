## Task: Make Generated Names Kubernetes Safe <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Fix naming issues caused by `metadata.name` not accepting all source names or IDs. The current issue is that NSX `groupID` values are not always valid Kubernetes object names, so `names.go` must convert the entire generated name into a Kubernetes-safe value.

The conversion must apply to the complete name that will be used as `metadata.name`, not just one component. It must handle invalid characters, uppercase characters, separators, leading/trailing invalid characters, empty or all-invalid inputs, and Kubernetes length limits. It must preserve stable deterministic naming so the same source object always maps to the same Kubernetes object name. If truncation is required, include a deterministic suffix to avoid collisions.

In scope: update `names.go` and callers/tests as needed; make the full output valid for Kubernetes `metadata.name`; add unit tests covering group IDs and names with invalid characters; add collision-resistance tests for truncation if relevant; verify generated CRs can be applied to a Kubernetes API.

Out of scope: changing NSX IDs; changing user-visible spec fields that should retain original NSX values; introducing a database or lookup service for name mapping.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] `names.go` converts the entire generated `metadata.name` to a Kubernetes-safe name.
- [ ] The generated name satisfies Kubernetes DNS label/subdomain requirements used by the relevant CRD type.
- [ ] Invalid characters, uppercase letters, repeated separators, leading invalid characters, trailing invalid characters, empty values, and all-invalid values are covered by tests.
- [ ] Long generated names are handled within Kubernetes name length limits with deterministic collision-resistant behavior.
- [ ] Original NSX identifiers remain available in spec/status fields where the operator needs exact source identity.
- [ ] Tests prove problematic `groupID` values no longer create invalid Kubernetes object names.
- [ ] Live or envtest verification proves generated resources with formerly invalid group IDs can be created and reconciled.
</acceptance_criteria>

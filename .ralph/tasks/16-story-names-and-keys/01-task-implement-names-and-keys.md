## Task: Implement NSXGroup Names And Keys <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/names` so NSXGroup logical identities map to stable, readable Kubernetes metadata names with no hashes or random suffixes.

In scope: `NormalizeNetworkCloudFQDN`, `NSXGroupLogicalID`, `NSXGroupName`, and `ParseNSXGroupName`; logical identity uses `<networkCloudFQDN>/<groupID>`; metadata name encodes FQDN and group ID with `--`; ports encode as `-8443`; examples must match `nsx-a.example.net/app-foo -> nsx-a.example.net--app-foo` and `nsx-a.example.net:8443/app-foo -> nsx-a.example.net-8443--app-foo`; names must be deterministic and round-trip tested. `spec.networkCloudFQDN` and `spec.groupID` remain the source of truth.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Tests cover normalization, examples, and round-trip parse behavior.
- [ ] Tests prove no hash or random suffix is used.
</acceptance_criteria>

## Task: Make Generated Names Kubernetes Safe <status>completed</status> <passes>true</passes>

<plan>
.ralph/tasks/28-story-k8s-safe-names/01-task-make-generated-names-k8s-safe_plans/01-k8s-safe-generated-names.md
DONE
</plan>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Fix naming issues caused by `metadata.name` not accepting all source names or IDs. The current issue is that NSX `groupID` values are not always valid Kubernetes object names, so `names.go` must convert the entire generated name into a Kubernetes-safe value.

The conversion must apply to the complete name that will be used as `metadata.name`, not just one component. It must handle invalid characters, uppercase characters, separators, leading/trailing invalid characters, empty or all-invalid inputs, and Kubernetes length limits. It must preserve stable deterministic naming so the same source object always maps to the same Kubernetes object name. If truncation is required, include a deterministic suffix to avoid collisions.

In scope: update `names.go` and callers/tests as needed; make the full output valid for Kubernetes `metadata.name`; add unit tests covering group IDs and names with invalid characters; add collision-resistance tests for truncation if relevant; verify generated CRs can be applied to a Kubernetes API.

Out of scope: changing NSX IDs; changing user-visible spec fields that should retain original NSX values; introducing a database or lookup service for name mapping.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] `names.go` converts the entire generated `metadata.name` to a Kubernetes-safe name.
- [x] The generated name satisfies Kubernetes DNS label/subdomain requirements used by the relevant CRD type.
- [x] Invalid characters, uppercase letters, repeated separators, leading invalid characters, trailing invalid characters, empty values, and all-invalid values are covered by tests.
- [x] Long generated names are handled within Kubernetes name length limits with deterministic collision-resistant behavior.
- [x] Original NSX identifiers remain available in spec/status fields where the operator needs exact source identity.
- [x] Tests prove problematic `groupID` values no longer create invalid Kubernetes object names.
- [x] Live or envtest verification proves generated resources with formerly invalid group IDs can be created and reconciled.
</acceptance_criteria>

<verification_evidence>
- `go test ./internal/names -count=1` passed; `internal/names` coverage is `100.0%` in `make test-coverage`.
- `go test ./internal/stateoperator -run TestProcessManagerSnapshotImportsRemoteOnlyGroupsAsObserveUpserts -count=1` passed with problematic remote `GroupID` `App/Web_GROUP`, generated observe name `nsx-a.example.test-8443-app-web-group`, Kubernetes DNS-subdomain validation, and exact `Spec.GroupID` preservation.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./api/v1alpha -run TestCRDsInstallStatusSubresourceSelectableFieldsAndSchema -count=1 -v` passed against envtest Kubernetes API server `1.32.0`; evidence log included field selector `spec.groupID=App/Web_GROUP` returning `[nsx-a.example.net-8443-app-web-group]`.
- `make check` passed after final boundary cleanup. It ran gofumpt, `go vet ./...`, `golangci-lint run ./...` with `0 issues`, normal tests, race tests, mockapi tests, largechaos tests, and coverage threshold.
- `make test` passed.
- `make test-coverage` passed with total coverage `83.7%`, meeting the `80.0%` threshold; package coverage included `internal/names 100.0%` and `internal/stateoperator 80.4%`.
- Final improve-code-boundaries pass found no production `ParseNSXGroupName` callers, no duplicate sanitization outside `internal/names`, and no runtime dependency on decoding source identity from `metadata.name`.
</verification_evidence>

## Task: Support Multiple Segment Paths in NSXGroup Spec <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Allow one `NSXGroup` to model several NSX segment policy paths while preserving the existing one-to-one mapping between the Kubernetes field and the single NSX `PathExpression` used by the operator. Today `NSXGroup.spec.segment_path` is either one path or absent, and the operator writes/reads one `PathExpression` containing either one `paths` entry or none. Production needs several cases where one group has multiple segment paths in that same `PathExpression`.

CRD/API changes required:
- Replace `NSXGroup.spec.segment_path` with `NSXGroup.spec.segment_paths`.
- Go type change in `api/v1alpha.NSXGroupSpec`: remove `SegmentPath *string `json:"segment_path,omitempty"` and add `SegmentPaths []string `json:"segment_paths,omitempty"``.
- CRD schema change in `config/crd/bases/nsx.ing.com_nsxgroups.yaml`: remove `spec.properties.segment_path`; add `spec.properties.segment_paths` as an array of strings with `x-kubernetes-list-type: set`.
- `segment_paths` must be optional and omitted when empty. It must support zero, one, or many paths.
- Keep `spec.cidrs` unchanged and required. Do not add `domainId` or any alternate NSX domain field.
- Update descriptions to say every item in `segment_paths` maps unchanged into one NSX `PathExpression.paths` array, not to multiple path expressions.
- Update all samples/manifests/tests/docs that currently use `segment_path` to use `segment_paths`.

Behavior changes required:
- Remote projection must treat a single NSX `PathExpression` with multiple `paths` entries as supported and populate all paths into `NSXGroup.spec.segment_paths`.
- The existing one-to-one mapping is from local `segment_paths` to one NSX `PathExpression.paths` array. Do not create one NSX `PathExpression` per segment path.
- Manage mode must add or patch exactly one NSX `PathExpression` whose `paths` payload equals `spec.segment_paths` when non-empty.
- Manage mode must delete the existing managed path expression when `spec.segment_paths` is empty and a remote path expression exists.
- Spec comparison must compare `segment_paths` as a stable set/list according to the CRD semantics. If `x-kubernetes-list-type: set` is used, tests must prove equivalent ordering does not produce false drift.
- Backward compatibility is not required unless the implementer decides to add explicit conversion/migration. If compatibility is added, the task still must end with the served CRD field named `segment_paths`.

In scope:
- API structs, deepcopy, CRD schema, CRD integration tests, JSON shape tests, sample manifests, manager pipeline projection/comparison/write logic, status behavior affected by remote projection, and mockapi/testcontainer verification where NSX behavior is involved.
- Replace existing tests that assert `segment_path` is a string with tests that assert `segment_paths` is an array of strings and rejects non-array/non-string values.

Out of scope:
- Multiple NSX path-expression objects for the same group.
- Changing `cidrs`, `mode`, `groupID`, `display_name`, or `networkCloudFQDN` semantics.


</description>


<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] The served `NSXGroup` CRD exposes `spec.segment_paths` as an optional array of strings with set semantics and no longer exposes `spec.segment_path`.
- [x] `api/v1alpha.NSXGroupSpec` contains `SegmentPaths []string `json:"segment_paths,omitempty"``, with deepcopy and JSON tests proving independent copies and correct field shape.
- [x] A remote NSX group with one `PathExpression.paths` value becomes an `NSXGroup` with one `spec.segment_paths` item, and a remote NSX group with several `PathExpression.paths` values becomes an `NSXGroup` with several `spec.segment_paths` items without marking `UnsupportedExpression=True`.
- [x] Manage mode sends exactly one NSX `PathExpression` containing the complete `spec.segment_paths` array when paths are configured.
- [x] Manage mode deletes the existing path expression when `spec.segment_paths` is empty.
- [x] Tests prove ordering behavior for `segment_paths` matches the CRD list semantics and does not create false drift for equivalent sets.
- [x] Full relevant tests are run and recorded, including unit tests and CRD integration tests; NSX-facing behavior is verified against `../nsx-t-mockapi` or equivalent testcontainers evidence.
</acceptance_criteria>

<plan>
.ralph/tasks/31-story-multiple-segment-paths/01-task-support-multiple-segment-paths-in-nsxgroup-spec_plans/2026-05-19-segment-paths-plan.md
</plan>

NOW EXECUTE

<verification_evidence>
- API type and JSON/deepcopy verification:
  - `go test ./api/v1alpha -run 'Test(DeepCopyObjectKeepsNetworkCloudAndGroup|JSONShapeUsesPublicAPIFieldNames)' -count=1`: `ok`.
  - `NSXGroupSpec` now exposes `SegmentPaths []string `json:"segment_paths,omitempty"``. Tests prove independent slice copies for `NSXGroup` and `NSXGroupList`, JSON uses `segment_paths`, nil/empty slices are omitted, and the legacy field is not emitted.
- CRD/envtest verification:
  - `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./api/v1alpha -run TestCRDsInstallStatusSubresourceSelectableFieldsAndSchema -count=1`: `ok`.
  - The envtest CRD test creates groups with omitted, one, and multiple `spec.segment_paths`; rejects non-array and non-string-item values; and verifies the old field is rejected or pruned.
  - Served CRD schema in `crds/nsx.ing.com_nsxgroups.yaml` has `segment_paths` as `type: array`, string items, and `x-kubernetes-list-type: set`; it no longer serves the old field.
- Stateoperator projection/comparison/write verification:
  - `go test ./internal/stateoperator -run 'Test(RemoteGroupFromNSXGroup|ProcessManagerSnapshotImportsRemoteOnlyGroupsAsObserveUpserts|ApplyManagerPlan(AddsMissingPathExpression|DeletesExistingPathExpression|RunsOperationsInExactOrder|PatchesOnlyRepresentedGroupWriteFields|WriteDisabled))' -count=1`: `ok`.
  - `go test ./internal/stateoperator -run 'Test(ProcessManagerSnapshotManageGroupsWriteMissingAndDriftedAndOnlyStatusMatching|ProcessManagerSnapshotObserveGroupWithLegacyFinalizerPlansFinalizerRemovalOnly)' -count=1`: `ok`.
  - Tests cover remote single-`PathExpression` projection with several `paths`, Observe import to `spec.segment_paths`, Manage drift comparison with different path order, one path-expression add/patch containing the full array, and deletion of the selected path expression when local paths are absent.
- Mockapi/NSX-facing verification:
  - The stateoperator package test suite passed with mockapi-backed write semantics.
  - `TestManagedWriteUsesSelectedPatchEndpointsAndPreservesUnrelatedMockAPIExpressions` records the actual mockapi HTTP writes and verifies exactly one selected path-expression PATCH:
    - path: `/policy/api/v1/infra/domains/default/groups/managed-write-mock/path-expressions/selected-path`
    - body contains one `PathExpression` with `paths: ["/infra/segments/new", "/infra/segments/extra"]`
    - unrelated remote path expressions remain represented in mockapi member queries.
- Full required verification:
  - `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./... -count=1`: all packages `ok`.
  - `make check`: passed. Evidence included `gofumpt`, `go vet`, `golangci-lint` with `0 issues`, envtest-backed `go test ./...`, race tests, mockapi contract tests, e2e tests, large-chaos tests, and coverage.
  - `make test`: passed; all packages returned `ok`.
  - `make test-coverage`: passed with total coverage `83.7%`, meeting the `80.0%` threshold. Relevant changed packages include `api/v1alpha` at `100.0%` and `internal/stateoperator` at `80.3%`.
- Final `$improve-code-boundaries` review:
  - The API boundary uses one served field: `NSXGroupSpec.SegmentPaths []string`.
  - The stateoperator boundary uses one slice shape for remote projection and managed writes: `RemoteGroup.SegmentPaths []string` and `ManagedGroupWrite.SegmentPaths []string`.
  - No new DTO, wrapper type, or per-segment-path expression model was added; the operator still writes exactly one NSX `PathExpression` for all configured paths.
  - The obsolete segment pointer-copy helper was removed. One local comparison helper handles set-style `segment_paths` equality at the stateoperator comparison boundary.
  - Boundary scan `rg -n 'segment_path([^s]|$)|SegmentPath([^s]|$)' api internal crds config docs README.md -g '!coverage.out'` found no legacy API field usage; matches are only unrelated NSX client route helpers named for NSX segment resources.
</verification_evidence>

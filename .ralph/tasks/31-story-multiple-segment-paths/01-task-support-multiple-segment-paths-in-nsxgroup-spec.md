## Task: Support Multiple Segment Paths in NSXGroup Spec <status>not_started</status> <passes>false</passes>

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
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] The served `NSXGroup` CRD exposes `spec.segment_paths` as an optional array of strings with set semantics and no longer exposes `spec.segment_path`.
- [ ] `api/v1alpha.NSXGroupSpec` contains `SegmentPaths []string `json:"segment_paths,omitempty"``, with deepcopy and JSON tests proving independent copies and correct field shape.
- [ ] A remote NSX group with one `PathExpression.paths` value becomes an `NSXGroup` with one `spec.segment_paths` item, and a remote NSX group with several `PathExpression.paths` values becomes an `NSXGroup` with several `spec.segment_paths` items without marking `UnsupportedExpression=True`.
- [ ] Manage mode sends exactly one NSX `PathExpression` containing the complete `spec.segment_paths` array when paths are configured.
- [ ] Manage mode deletes the existing path expression when `spec.segment_paths` is empty.
- [ ] Tests prove ordering behavior for `segment_paths` matches the CRD list semantics and does not create false drift for equivalent sets.
- [ ] Full relevant tests are run and recorded, including unit tests and CRD integration tests; NSX-facing behavior is verified against `../nsx-t-mockapi` or equivalent testcontainers evidence.
</acceptance_criteria>

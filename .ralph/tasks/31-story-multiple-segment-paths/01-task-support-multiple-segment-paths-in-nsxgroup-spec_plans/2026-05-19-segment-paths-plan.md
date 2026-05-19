Plan path: `.ralph/tasks/31-story-multiple-segment-paths/01-task-support-multiple-segment-paths-in-nsxgroup-spec_plans/2026-05-19-segment-paths-plan.md`

# Support Multiple Segment Paths in NSXGroup Spec

## Required Skills Read

- `$tdd`: use vertical red-green slices through public behavior, not bulk tests followed by bulk implementation.
- `$improve-code-boundaries`: avoid duplicate local shapes, wrong-place mapping, helper proliferation, and stringly/manual boundary drift.

## Current Shape

- Public Kubernetes API type is `api/v1alpha.NSXGroupSpec`.
- Served CRD manifest in this repo is `crds/nsx.ing.com_nsxgroups.yaml`; there is no `config/crd/bases` directory in the current tree.
- `internal/stateoperator/manager_pipeline.go` is the deep module for remote projection, local/remote compare, managed write planning, and NSX apply.
- Current path membership is modeled three times as a single optional string:
  - `api/v1alpha.NSXGroupSpec.SegmentPath *string`
  - `stateoperator.RemoteGroup.SegmentPath *string`
  - `stateoperator.ManagedGroupWrite.SegmentPath *string`
- Current remote projection treats a `PathExpression` with anything other than exactly one `paths` entry as unsupported, while preserving only the first path.
- Current managed apply creates, patches, or deletes exactly one `PathExpression`, but payload construction wraps the one string as `[]string{*SegmentPath}`.

## Public Interface Design

- Replace `NSXGroup.spec.segment_path` with `NSXGroup.spec.segment_paths`.
- Replace `api/v1alpha.NSXGroupSpec.SegmentPath *string` with:

```go
SegmentPaths []string `json:"segment_paths,omitempty"`
```

- Keep `CIDRs []string` required and unchanged.
- Keep exactly one NSX `PathExpression` for all configured segment paths.
- Treat `nil` and empty `SegmentPaths` as absent for JSON, comparison, and managed writes.
- Because the CRD list type is `set`, compare segment paths with set semantics for drift. Do not sort or mutate user-facing slices just to compare; use a small local comparison function at the stateoperator boundary.
- Do not add backward-compatibility conversion unless execution proves the API server requires it. The served field must be `segment_paths`; `segment_path` should be absent from CRD/API JSON.

## Boundary Design

- Use one canonical shape at each boundary:
  - API desired/observed shape: `NSXGroupSpec.SegmentPaths []string`.
  - Remote manager shape: `RemoteGroup.SegmentPaths []string`.
  - Managed write shape: `ManagedGroupWrite.SegmentPaths []string`.
- Do not introduce a new DTO, wrapper type, or per-path expression model. The existing deep module can carry slices directly.
- Keep NSX client types unchanged: `nsxclient.PathExpression.Paths []string` and `PathExpressionPatch.Paths []string` already represent the needed boundary.
- Flatten helper pressure:
  - Replace pointer-copy helpers for segment paths with existing slice copy behavior.
  - Add only one comparison helper if needed, e.g. `stringSetEqual(left, right []string) bool`, and use it for set-style fields at comparison points.
  - Keep path-expression patch construction in `applyManagedPathExpression`; do not create a second render/build layer.

## Behaviors to Test

- API deepcopy keeps `SegmentPaths` independent for `NSXGroup` and `NSXGroupList`.
- API JSON uses `segment_paths` as an array, omits it when nil/empty, decodes it into `SegmentPaths`, and never emits `segment_path`.
- CRD accepts omitted `segment_paths`, one path, and multiple string paths.
- CRD rejects non-array `segment_paths`, non-string items, and old `segment_path` when possible through schema validation.
- Remote projection maps a single `PathExpression.paths` entry to one `SegmentPaths` item without `UnsupportedExpression=True`.
- Remote projection maps several `PathExpression.paths` entries into all `SegmentPaths` items without `UnsupportedExpression=True`.
- Remote projection keeps zero-path `PathExpression` supported as an empty segment path set unless execution shows NSX never emits this; then document and switch back to `TO BE VERIFIED`.
- Existing unsupported cases remain unsupported: extended expressions, duplicate path expression objects, bad JSON, unknown expression types, unsupported conjunctions.
- Observe mode imports remote `SegmentPaths` into `NSXGroup.spec.segment_paths`.
- Manage mode treats local and remote path lists as set-equivalent, so ordering differences do not plan drift.
- Manage mode sends exactly one `PathExpressionPatch.Paths` containing all configured `SegmentPaths` when non-empty.
- Manage mode deletes the existing selected path expression when `SegmentPaths` is nil/empty and a remote path expression exists.
- Mockapi/write-semantics test proves the actual NSX-facing request body has one path-expression endpoint call with the full `paths` array.

## TDD Execution Plan

1. [x] RED: update `api/v1alpha/types_test.go` public API JSON/deepcopy tests to expect `SegmentPaths []string`, `segment_paths`, omitted empty slices, and no `segment_path`.
2. [x] GREEN: change `api/v1alpha/types.go` and `api/v1alpha/deepcopy.go` to make the API tests pass.
3. [x] RED: update `api/v1alpha/crd_integration_test.go` to create groups with omitted, one, and many `segment_paths`, reject bad `segment_paths`, and reject/remove old `segment_path`.
4. [x] GREEN: update `crds/nsx.ing.com_nsxgroups.yaml` schema description and properties:
   - remove `segment_path`
   - add optional `segment_paths`
   - type array, item type string, `x-kubernetes-list-type: set`
5. [x] RED: update `internal/stateoperator/manager_pipeline_test.go` remote projection tests so one and many `PathExpression.paths` values are supported and fully projected.
6. [x] GREEN: change `RemoteGroup.SegmentPath` to `RemoteGroup.SegmentPaths`, copy full `PathExpression.Paths`, and stop marking multi-path expressions unsupported.
7. [x] RED: update observe/spec comparison tests to prove `observeSpecFromRemote` uses `SegmentPaths` and set-equivalent ordering does not create drift.
8. [x] GREEN: update `observeSpecFromRemote`, `groupSpecsEqual`, `managedSpecMatchesRemote`, and `managedWriteFromLocal` to carry copied slices and compare segment paths as sets.
9. [x] RED: update apply tests so managed writes add/patch one path expression whose `Paths` equals all configured segment paths.
10. [x] GREEN: update `ManagedGroupWrite.SegmentPaths` and `applyManagedPathExpression` to add/patch/delete exactly one selected path expression based on slice emptiness.
11. [x] RED: update mockapi write-semantics tests to assert the real HTTP request body contains the full `paths` array and only one path-expression write.
12. [x] GREEN: update the write path until mockapi/testcontainer behavior passes.
13. [x] RED/GREEN incrementally replace remaining `SegmentPath`, `segment_path`, and old single-path fixtures in:
    - `internal/stateoperator/operator_test.go`
    - `internal/kubeapi/client_test.go`
    - `internal/startup/manager_test.go`
    - `config/compose/manifests/sample.yaml`
    - `crds/nsx.ing.com_nsxgroups.yaml`
    - any docs/samples found by `rg "segment_path|SegmentPath"`
14. [x] Run focused tests after each slice, at minimum:
    - `go test ./api/v1alpha -run 'Test(DeepCopy|JSONShape|CRDs)' -count=1`
    - `go test ./internal/stateoperator -run 'Test(RemoteGroupFromNSXGroup|ProcessManagerSnapshot|ApplyManagerPlan|ManagedWrite)' -count=1`
15. [x] Refactor while green:
    - remove obsolete pointer-copy code paths for segment path
    - keep only one segment-path comparison helper if needed
    - avoid a new path expression abstraction unless duplication becomes real
16. [x] Run final checks:
    - `make check`
    - `make test`
    - `make test-coverage`
17. [x] Record concrete evidence in the task:
    - focused test commands and results
    - `make check`, `make test`, `make test-coverage` results
    - mockapi/write-semantics evidence showing one `PathExpression` request with multiple `paths`
18. [x] Final improve-code-boundaries review:
    - `rg "SegmentPath|segment_path"` should show only task/plan/evidence history, not active code or served manifests.
    - Confirm there is no extra DTO or per-path expression model.
    - Confirm compare/write/projection code uses one slice shape at each boundary.

## Design Check

- This plan keeps the existing API boundary small and literal: `segment_paths` is the only new served field.
- This plan keeps NSX membership semantics unchanged except for allowing several paths in the same `PathExpression.paths` array.
- This plan avoids backward-compatible dual fields because the task explicitly does not require compatibility.
- This plan avoids testing private helpers directly; tests use public API structs, CRD envtest behavior, stateoperator public planning/apply functions, and mockapi-facing HTTP requests.

If implementation proves the `segment_paths` slice type, CRD set semantics, one-PathExpression write model, or manager comparison boundary is wrong, replace this final marker with `TO BE VERIFIED`, document the proposed design change above, update the task marker, and quit immediately.

NOW EXECUTE

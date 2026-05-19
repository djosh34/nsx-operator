## Task: Add Global And Per-NetworkCloud NSX Write Disable Controls <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add explicit controls that can disable every non-GET write call to NSX before it reaches any NSX manager, while preserving read/list behavior. The operator needs a global configuration field that disables all NSX write calls, and each NetworkCloud custom resource also needs its own field that enables or disables NSX write calls for that specific NetworkCloud.

The global configuration field is a hard override for all non-GET calls to NSX. When global writes are disabled, no per-NetworkCloud setting may re-enable NSX writes. When global writes are enabled, the per-NetworkCloud field decides whether that NetworkCloud may perform non-GET NSX calls. "Write calls" means every NSX HTTP method other than GET, including but not limited to POST, PUT, PATCH, and DELETE. GET/list calls to NSX must continue so observe/status behavior can still run.

In scope: add the global config field; add the NetworkCloud CRD/API field; wire the setting through reconciliation and NSX client call sites so writes are blocked before the HTTP request is sent; preserve existing observe/list/get behavior; make status/logging explain when writes are skipped because of global or per-resource configuration; keep zap structured logging; update tests and generated manifests as required by the repo conventions.

Out of scope: changing the semantics of existing PATCH/delete write behavior beyond adding the write gate; adding unrelated fields; changing unrelated reconciliation behavior; adding metrics, except where an existing metric has to remain correct when writes are skipped.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] A global config field exists and, when disabled, prevents all non-GET NSX HTTP calls for every manager and every NetworkCloud.
- [ ] A per-NetworkCloud CRD/API field exists and, when disabled, prevents all non-GET NSX HTTP calls for only that NetworkCloud while still allowing GET/list calls.
- [ ] The global field overrides the per-NetworkCloud field; per-resource writes cannot be re-enabled when global writes are disabled.
- [ ] Tests prove POST, PUT, PATCH, and DELETE NSX calls are blocked when writes are disabled, and GET/list calls still occur.
- [ ] Tests prove blocked writes do not reach the sibling `../nsx-t-mockapi` service or equivalent request recorder.
- [ ] Structured zap logs include manager/resource/function context and whether writes were skipped because of global config or per-NetworkCloud config.
- [ ] Existing normal, contract, and relevant e2e test gates pass, including `make test`, `make test-contract`, and any CRD/API generation verification used by this repo.
</acceptance_criteria>

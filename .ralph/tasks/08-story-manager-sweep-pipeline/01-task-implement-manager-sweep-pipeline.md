## Task: Implement Per-Manager Gather Process Apply Pipeline <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement the manager sweep pipeline as exactly `gather all info -> process all info -> apply all planned changes`. The process stage must be pure and must not call Kubernetes, NSX, or network APIs.

In scope: `ManagerSnapshot`, `ManagerPlan`, binding key/types, `BuildBindings`, and `ProcessManagerSnapshot`; gather lists NSXGroup CRs filtered by normalized `spec.networkCloudFQDN`, lists all NSX groups with pagination, and stores remote groups/expression IDs in memory; processing plans remote-only Observe upserts, Observe spec replacement, Observe delete after successful gather, Manage create/patch on missing or drifted remote, status-only plans for matching Manage groups, and cloud condition updates for gather failure without child mass-marking; apply order is Observe upserts, managed NSX writes/deletes, group statuses, Observe deletes, cloud status.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Unit tests prove `ProcessManagerSnapshot` is deterministic and has no client dependencies.
- [x] Integration evidence proves apply ordering and pagination.
- [x] Failure tests prove failed gather only updates cloud conditions and does not mark all child groups missing.
</acceptance_criteria>

<plan>
.ralph/tasks/08-story-manager-sweep-pipeline/01-task-implement-manager-sweep-pipeline_plans/2026-05-19-manager-sweep-pipeline-plan.md
</plan>

<verification_evidence>
- `make check` passed after implementation:
  - `golangci-lint run ./...`: `0 issues.`
  - envtest-backed `go test ./...`: all packages passed, including `internal/kubeapi`, `internal/startup`, and `internal/stateoperator`.
  - envtest-backed `go test -cover ./...`: all reported package coverage was 80%+:
    - `api/v1alpha`: 100.0%
    - `cmd/nsx-operator`: 80.8%
    - `internal/buildinfo`: 100.0%
    - `internal/config`: 82.9%
    - `internal/httpratelimit`: 87.8%
    - `internal/kubeapi`: 80.9%
    - `internal/logging`: 96.2%
    - `internal/nsxclient`: 80.3%
    - `internal/startup`: 80.9%
    - `internal/stateoperator`: 82.8%
- Explicit `make test` passed:
  - `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./...`
  - all packages returned `ok`, including envtest packages.
- Explicit `make test-coverage` passed:
  - `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test -cover ./...`
  - all package coverage remained 80%+ with the percentages listed above.
- Focused processing tests prove pure deterministic behavior through public value interfaces:
  - gather failure plans only cloud status and leaves Observe upserts, managed writes/deletes, group statuses, and Observe deletes empty.
  - `BuildBindings` sorts local groups by CR name, sorts remote groups by binding key, and rejects duplicate local/remote logical identities.
  - remote-only groups plan deterministic Observe upserts and status.
  - existing Observe groups mirror remote spec or delete when the successful gather shows the remote is gone.
  - Manage groups plan writes for missing/drifted remote groups and status-only for matching remote groups.
- Integration-style boundary tests prove:
  - `GatherManagerSnapshot` passes a normalized `spec.networkCloudFQDN` field selector and uses real `nsxclient.ListGroups` pagination against an `httptest` NSX server with cursor pages.
  - `ApplyManagerPlan` runs operations in exact order: Observe upserts, managed NSX writes/deletes, group statuses, Observe deletes, cloud status.
  - default stateoperator sweep applies Observe upsert/status/delete through the real typed kube CRD client under envtest.
  - default stateoperator sweep updates cloud status under envtest when gather fails.
- Final `$improve-code-boundaries` review:
  - `ProcessManagerSnapshot(snapshot, now)` accepts only values and explicit time; it has no context, Kubernetes client, NSX client, logger, HTTP, or global clock dependency.
  - `BuildBindings` is pure value sorting/deduplication.
  - Kubernetes/NSX calls are isolated to `GatherManagerSnapshot`, `ApplyManagerPlan`, the kube adapter, and startup dependency construction.
  - No unchecked error was introduced; error returns are wrapped at gather/apply/startup boundaries.
</verification_evidence>

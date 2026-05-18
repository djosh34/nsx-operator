## Task: Add API Go Types And CRD Manifests <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add `api/v1alpha` Go types and Kubernetes CRD manifests for `NSXNetworkCloud` and `NSXGroup` using API group `nsx.ing.com` and version `v1alpha`. The CRDs are cluster-scoped and must install on Kubernetes v1.32 or newer.

In scope: structs exactly matching the design for specs, status structs with `[]metav1.Condition`, top-level resource/list types, mode constants `Observe` and `Manage`, condition constants, scheme registration, and any required deepcopy support; CRDs with `openAPIV3Schema`, status subresource, selectableFields, printer columns, and status conditions only. `NSXGroup` must expose `networkCloudFQDN`, `groupID`, `display_name`, `mode`, `cidrs`, and nullable `segment_path`; no `domainId` field may exist anywhere in CRD specs.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] CRDs are installed into a real Kubernetes API server and accepted.
- [x] Verification proves selectableFields work for `spec.networkCloudFQDN` and other documented fields.
</acceptance_criteria>

<plan>
.ralph/tasks/06-story-api-types-crds/01-task-add-api-types-and-crd-manifests_plans/2026-05-19-api-types-crd-manifests-plan.md
</plan>

<verification>
Focused API package command:

```bash
KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./api/v1alpha -count=1 -v
```

Evidence from the focused run:

- `TestCRDsInstallStatusSubresourceSelectableFieldsAndSchema` passed against envtest Kubernetes API server version `1.32.0`.
- The test logged `CRD nsxnetworkclouds.nsx.ing.com is Established` and `CRD nsxgroups.nsx.ing.com is Established`, proving both CRDs installed into a real kube-apiserver.
- The test created real cluster-scoped `NSXNetworkCloud` and `NSXGroup` custom resources through the dynamic client.
- The test updated `/status` for `cloud-a` and `group-a`; logs showed each status subresource update kept spec unchanged and stored the expected condition.
- The test proved selectable fields with API-server field selector calls:
  - `spec.networkCloudFQDN=nsx-a.example.net` returned `[cloud-a]` for `NSXNetworkCloud`.
  - `spec.networkCloudId=cloud-b` returned `[cloud-b]`.
  - `spec.networkCloudFQDN=nsx-a.example.net` returned `[group-a]` for `NSXGroup`.
  - `spec.groupID=app-b` returned `[group-b]`.
  - `spec.mode=Manage` returned `[group-a]`.
- The test proved schema rejection for invalid `NSXGroup.spec.mode`, non-string `NSXGroup.spec.segment_path`, missing `NSXGroup.spec.cidrs`, and missing `NSXNetworkCloud.spec.networkCloudFQDN`.
- Unit tests also passed for scheme registration, deepcopy independence, list deepcopy independence, nil deepcopy behavior, and JSON field shape including `display_name`, `segment_path`, `conditions`, and absent `domainId`.

Coverage command:

```bash
KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test -coverprofile=/tmp/api-v1alpha.cover ./api/v1alpha -count=1 && go tool cover -func=/tmp/api-v1alpha.cover
```

Coverage evidence:

- `github.com/djosh34/nsx-operator/api/v1alpha` reported `coverage: 100.0% of statements`.
- `go tool cover -func=/tmp/api-v1alpha.cover` reported total `100.0%`.

Required full checks:

```bash
make test
```

Passed. Output included all packages passing, including `github.com/djosh34/nsx-operator/api/v1alpha`.

```bash
make test-coverage
```

Passed. Output included:

- `api/v1alpha` coverage `100.0%`.
- `cmd/nsx-operator` coverage `81.6%`.
- `internal/buildinfo` coverage `100.0%`.
- `internal/config` coverage `82.9%`.
- `internal/httpratelimit` coverage `87.8%`.
- `internal/logging` coverage `96.2%`.
- `internal/nsxclient` coverage `80.3%`.
- `internal/startup` coverage `82.8%`.

```bash
make check
```

Passed. Output included:

- `gofumpt -w .`
- `golangci-lint run ./...`
- `0 issues.`
- `go test ./...` passed.
- `go test -cover ./...` passed with every listed package at 80%+ coverage.

Final boundary review using `$improve-code-boundaries`:

- Production API boundary is a single `api/v1alpha` package for types, constants, scheme registration, and deepcopy support.
- No duplicate production DTO/spec shapes were added.
- CRD manifests are static Kubernetes artifacts under `config/crd/bases`; no stringly YAML rendering layer was added.
- Status remains conditions-only in Go types and CRD schemas.
- `rg` found no `domainId` in `api/` or `config/`; the only hits are task/plan text and tests asserting `domainId` is absent from JSON.
</verification>

DONE

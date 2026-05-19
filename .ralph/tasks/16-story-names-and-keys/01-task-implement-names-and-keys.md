## Task: Implement NSXGroup Names And Keys <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/names` so NSXGroup logical identities map to stable, readable Kubernetes metadata names with no hashes or random suffixes.

In scope: `NormalizeNetworkCloudFQDN`, `NSXGroupLogicalID`, `NSXGroupName`, and `ParseNSXGroupName`; logical identity uses `<networkCloudFQDN>/<groupID>`; metadata name encodes FQDN and group ID with `--`; ports encode as `-8443`; examples must match `nsx-a.example.net/app-foo -> nsx-a.example.net--app-foo` and `nsx-a.example.net:8443/app-foo -> nsx-a.example.net-8443--app-foo`; names must be deterministic and round-trip tested. `spec.networkCloudFQDN` and `spec.groupID` remain the source of truth.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Tests cover normalization, examples, and round-trip parse behavior.
- [x] Tests prove no hash or random suffix is used.
</acceptance_criteria>

Plan: `.ralph/tasks/16-story-names-and-keys/01-task-implement-names-and-keys_plans/2026-05-19-names-and-keys-plan.md`

<verification_evidence>
- Focused `internal/names` verification passed:
  - Command: `go test ./internal/names -run 'TestNSXGroupNameUsesReadableStableProjection|TestNSXGroupNameIsDeterministicWithoutGeneratedSuffix|TestParseNSXGroupNameRoundTripsGeneratedNames|TestParseNSXGroupNameRejectsMalformedNames' -v`
  - Output included passing subtests for `plain_fqdn`, `fqdn_with_port`, deterministic no-suffix generation, round-trip parsing, normalized-input round trip, and malformed inputs `cloud-only`, `--group`, `cloud--`, and `cloud--group--extra`.
- Required examples are locked by `TestNSXGroupNameUsesReadableStableProjection`:
  - `nsx-a.example.net/app-foo -> nsx-a.example.net--app-foo`
  - `nsx-a.example.net:8443/app-foo -> nsx-a.example.net-8443--app-foo`
- Round-trip and no random/hash suffix behavior are locked by tests:
  - `TestParseNSXGroupNameRoundTripsGeneratedNames` parses generated names back to normalized `NSXGroupLogicalID` values.
  - `TestNSXGroupNameIsDeterministicWithoutGeneratedSuffix` calls `NSXGroupName` 20 times for the same logical ID and asserts the exact readable projection every time.
- Focused package verification passed:
  - `go test ./internal/names`: `ok`
  - `go test ./internal/names -cover`: `coverage: 93.9% of statements`
  - `go test ./internal/stateoperator -run TestProcessManagerSnapshotImportsRemoteOnlyGroupsAsObserveUpserts`: `ok`
  - envtest-backed `go test -coverprofile=/tmp/stateoperator.cover ./internal/stateoperator`: `coverage: 80.2% of statements`
- Required repo checks passed:
  - `make check`: lint reported `0 issues`; envtest-backed `go test ./...` passed; envtest-backed `go test -cover ./...` passed.
  - `make test`: all packages returned `ok`.
  - `make test-coverage`: all packages returned `ok` and all package coverage was 80%+:
    - `api/v1alpha`: 100.0%
    - `cmd/nsx-operator`: 80.8%
    - `internal/buildinfo`: 100.0%
    - `internal/config`: 82.9%
    - `internal/httpratelimit`: 87.8%
    - `internal/kubeapi`: 80.9%
    - `internal/logging`: 96.2%
    - `internal/names`: 93.9%
    - `internal/nsxclient`: 80.3%
    - `internal/startup`: 80.9%
    - `internal/stateoperator`: 80.2%
    - `internal/statuscondition`: 91.1%
- Final `$improve-code-boundaries` review:
  - `internal/names` is the only implementation of `NormalizeNetworkCloudFQDN`, `NSXGroupName`, and `ParseNSXGroupName`.
  - `internal/stateoperator` no longer contains the private `observeGroupName` renderer or exported `NormalizeNetworkCloudFQDN` helper.
  - `internal/stateoperator` and `internal/startup` call `names.NormalizeNetworkCloudFQDN`; Observe import names use `names.NSXGroupName`.
  - Boundary scan command `rg -n "observeGroupName|func NormalizeNetworkCloudFQDN|strings\\.ReplaceAll\\([^\\n]*\\\"\\:\\\"|uuid|rand|hash|sha" internal api -g '*.go'` found the normalization and colon encoding only in `internal/names`; no random/hash suffix helper was found for NSXGroup metadata names.
  - No new mirror DTO layer was added; `stateoperator.BindingKey` remains the planner-local key shape and converts only at the metadata-name boundary.
</verification_evidence>

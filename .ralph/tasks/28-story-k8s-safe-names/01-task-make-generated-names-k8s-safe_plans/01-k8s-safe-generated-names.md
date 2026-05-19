# Plan: Kubernetes-Safe Generated NSXGroup Names

## Current State

- `internal/names.NSXGroupName` previously normalized only the NSX manager FQDN enough to be readable, replaced `:` with `-`, then appended the raw trimmed NSX `groupID`.
- That means imported observe-mode `NSXGroup` objects created by `stateoperator.ProcessManagerSnapshot` can get invalid Kubernetes `metadata.name` values when the remote NSX group ID contains uppercase letters, underscores, slashes, repeated separators, leading/trailing invalid characters, the `--` delimiter, or long values.
- `ParseNSXGroupName` is only referenced by tests, not production code. Runtime identity already uses `spec.networkCloudFQDN` and `spec.groupID`, so generated object names do not need to be reversible.
- The relevant Kubernetes validation target for CRD object `metadata.name` is DNS subdomain validation: lower-case alphanumeric, `-`, `.`, start/end alphanumeric, and at most 253 characters.

## Boundary Design

- Keep the existing public interface:
  - `names.NSXGroupName(names.NSXGroupLogicalID) string`
  - `names.NSXGroupLogicalID`
  - `names.NormalizeNetworkCloudFQDN(string) string`
- Move Kubernetes object-name safety into `NSXGroupName` itself. Callers should not know which pieces need escaping or truncation.
- Build the readable candidate from normalized source identity first, then sanitize the complete generated name. This satisfies the task requirement that conversion applies to the complete `metadata.name`, not only to one component.
- Preserve exact NSX identity only in API spec fields:
  - `NSXGroup.Spec.NetworkCloudFQDN`
  - `NSXGroup.Spec.GroupID`
- Treat `ParseNSXGroupName` as a stale boundary. Since production code does not use it and safe names will be lossy by design, remove it and its tests instead of pretending CR names are an identity decoder.

## Implementation Design

- In `internal/names/names.go`, implement one private sanitizer used by `NSXGroupName`:
  - lower-case the full generated candidate
  - replace every run of invalid DNS-subdomain characters with `-`
  - collapse repeated `-` and `.`
  - trim leading and trailing non-alphanumeric separators
  - use a stable fallback `nsx-group-<hash>` when the candidate has no alphanumeric characters after sanitization
  - enforce the 253-character DNS subdomain limit
  - when truncation is needed, preserve a readable prefix and append a deterministic hash suffix derived from the unsanitized complete candidate
- Use a deterministic suffix format that keeps the final name DNS-safe, for example `<trimmed-prefix>-<hexhash>`.
- Prefer Go standard library hashing, such as SHA-256 truncated to enough hex characters, over adding dependencies.
- Use `k8s.io/apimachinery/pkg/util/validation.IsDNS1123Subdomain` in tests to avoid duplicating Kubernetes validation rules in local assertions.

## TDD Execution Plan

- [x] Red/green cycle 1: Add a focused `internal/names` test proving `NSXGroupName` lowercases and sanitizes a complete name with invalid FQDN/groupID characters into a Kubernetes DNS-subdomain-safe name while preserving readable identity. Run `go test ./internal/names -run TestNSXGroupNameMakesCompleteMetadataNameKubernetesSafe -count=1`; it should fail before implementation, then pass after the minimal sanitizer.
- [x] Red/green cycle 2: Add cases for leading/trailing invalid characters, repeated separators, empty and all-invalid inputs through the public `NSXGroupName` interface. Run the targeted `internal/names` tests; implement only the missing fallback/cleanup behavior.
- [x] Red/green cycle 3: Add a long-name test that proves output length is at most 253, passes Kubernetes DNS-subdomain validation, is deterministic, and uses different suffixes for different long inputs with the same readable prefix. Run the targeted `internal/names` test; implement truncation and hash suffix.
- [x] Red/green cycle 4: Update or remove existing `ParseNSXGroupName` tests according to the boundary decision. If removing `ParseNSXGroupName`, run `go test ./internal/names -count=1` and fix compile/lint fallout.
- [x] Red/green cycle 5: Add/adjust a `stateoperator.ProcessManagerSnapshot` behavior test proving a remote group with a formerly invalid `groupID` imports as an observe upsert with a valid object name and keeps the original exact `groupID` in `Spec.GroupID`. Run `go test ./internal/stateoperator -run TestProcessManagerSnapshotImportsRemoteOnlyGroupsAsObserveUpserts -count=1` or a new narrowly named test.
- [x] Red/green cycle 6: Add envtest/API verification that an `NSXGroup` using a generated safe name from a problematic source group ID can be created by the Kubernetes API and field-selected by the original `spec.groupID`. Prefer extending `api/v1alpha/crd_integration_test.go` without brittle string assertions against YAML.

## Improve-Code-Boundaries Pass

- Boundary smell to fix: `ParseNSXGroupName` models `metadata.name` as reversible source identity, but production identity already lives in spec fields and the new safe-name algorithm must be lossy for invalid and truncated inputs.
- Remove `ParseNSXGroupName` and `restorePort` unless execution finds a real production caller.
- Keep sanitization private to `internal/names`; avoid adding a general-purpose public helper unless another production caller actually needs it.
- Final boundary check after tests: verify no caller manually sanitizes generated names outside `internal/names`, no duplicate name-shape structs are introduced, and no test asserts implementation-only helper details.

## Verification Commands

- Targeted red/green commands during implementation:
  - `go test ./internal/names -count=1`
  - `go test ./internal/stateoperator -run 'TestProcessManagerSnapshot.*Import' -count=1`
  - `KUBEBUILDER_ASSETS="$$(.bin/setup-envtest use 1.32.x -p path)" go test ./api/v1alpha -run TestCRDsInstallStatusSubresourceSelectableFieldsAndSchema -count=1`
- Required final commands:
  - `make check`
  - `make test`
  - `make test-coverage`
- Record concrete verification evidence in the task file before setting `<passes>true</passes>`.

## Completion Checklist

- [x] `NSXGroupName` sanitizes the full generated object name, not only `groupID`.
- [x] Tests cover invalid characters, uppercase letters, repeated separators, leading/trailing invalid characters, empty values, all-invalid values, and long-name truncation.
- [x] Collision-resistance is tested for truncated names.
- [x] Original NSX identifiers remain unchanged in spec/status fields.
- [x] Envtest/manual API verification evidence is recorded in the task file.
- [x] Final improve-code-boundaries pass completed after tests.

DONE

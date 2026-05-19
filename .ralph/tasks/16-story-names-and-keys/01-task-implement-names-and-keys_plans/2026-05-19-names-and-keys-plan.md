## Plan: NSXGroup Names And Keys

Task: `.ralph/tasks/16-story-names-and-keys/01-task-implement-names-and-keys.md`

### Current State

- `internal/stateoperator/manager_pipeline.go` already contains a private `NormalizeNetworkCloudFQDN` helper used by gather and startup via `stateoperator.NormalizeNetworkCloudFQDN`.
- The same file also contains a private `observeGroupName` helper that generates deterministic Observe import names by lowercasing/trimming the FQDN, replacing `:` with `-`, and joining cloud and group with `--`.
- Tests already assert one deterministic Observe import name in `internal/stateoperator`, but there is no canonical `internal/names` package and no public parse/round-trip behavior.
- `spec.networkCloudFQDN` and `spec.groupID` are already the logical source of truth for bindings. This task should not move identity into `metadata.name`; it should only make metadata names a stable projection of that identity.

### Public Interface

Create `internal/names` with a small exported surface:

```go
package names

type NSXGroupLogicalID struct {
	NetworkCloudFQDN string
	GroupID          string
}

func NormalizeNetworkCloudFQDN(value string) string
func NSXGroupName(id NSXGroupLogicalID) string
func ParseNSXGroupName(value string) (NSXGroupLogicalID, error)
```

Behavior:

- `NormalizeNetworkCloudFQDN` trims whitespace, removes trailing slashes, parses optional URL schemes when present, lowercases the host, and preserves an explicit port as `host:port`.
- `NSXGroupLogicalID` stores the normalized FQDN and trimmed group ID. The logical identity string concept is exactly `<networkCloudFQDN>/<groupID>`, but no `String()` method is required unless execution needs it for errors or logs.
- `NSXGroupName` returns a Kubernetes metadata name projection by replacing `:` in the normalized FQDN with `-`, then joining cloud and group with `--`.
- Required examples:
  - `nsx-a.example.net/app-foo` maps to `nsx-a.example.net--app-foo`.
  - `nsx-a.example.net:8443/app-foo` maps to `nsx-a.example.net-8443--app-foo`.
- `ParseNSXGroupName` parses only the metadata-name projection back to `NSXGroupLogicalID`. It splits on the single identity separator `--`, converts a trailing numeric `-<port>` in the cloud segment back to `:<port>`, normalizes the reconstructed FQDN, and trims the group ID.
- Reject malformed names with checked errors: empty input, missing separator, empty cloud segment, empty group segment, or ambiguous multiple `--` separators. Do not ignore parse errors.

### Boundary Plan Using `$improve-code-boundaries`

- Move all NSXGroup identity/name formatting knowledge into `internal/names`; remove the private duplicate formatter from `internal/stateoperator`.
- Replace `stateoperator.NormalizeNetworkCloudFQDN` with `names.NormalizeNetworkCloudFQDN` at call sites in `internal/stateoperator` and `internal/startup`.
- Keep `BindingKey` in `internal/stateoperator` for planner internals. Do not force the whole planner to use `names.NSXGroupLogicalID` unless doing so clearly removes duplicate conversion. The stable boundary is the names package API, not a broad DTO migration.
- Keep metadata-name parsing out of the planner unless there is a real call site. `ParseNSXGroupName` exists for the canonical names package and is tested directly.
- Avoid overengineering escaping. The task examples define `--` as the separator and port encoding as `-8443`; execution should implement exactly that surface. If tests reveal group IDs may contain `--` and must round-trip, switch this plan back to `TO BE VERIFIED` before changing the encoding design.
- Final boundary review: confirm there is one normalization implementation, one NSXGroup metadata-name renderer, no hash/random suffix helper, no ad hoc `strings.ReplaceAll(fqdn, ":", "-")` name rendering outside `internal/names`, and no new mirror DTOs.

### TDD Execution Plan Using `$tdd`

Use vertical red-green cycles. Write one behavior test, run it to RED, implement only enough for GREEN, then continue.

1. [x] Tracer bullet: normalization keeps host identity stable.
   - RED: add `internal/names` test cases for whitespace, uppercase host, trailing slash, URL input with scheme/path, and explicit port preservation.
   - GREEN: implement `NormalizeNetworkCloudFQDN` using `net/url` and string trimming, with every error checked.

2. [x] Required examples produce stable metadata names.
   - RED: test `NSXGroupName(NSXGroupLogicalID{NetworkCloudFQDN: "nsx-a.example.net", GroupID: "app-foo"}) == "nsx-a.example.net--app-foo"` and the `:8443` example.
   - GREEN: implement `NSXGroupName` by normalizing FQDN, trimming group ID, replacing only the port colon in the normalized FQDN with `-`, and joining with `--`.

3. [x] Name generation is deterministic and contains no hash/random suffix.
   - RED: call `NSXGroupName` repeatedly for the same logical ID and assert the exact same value each time; assert the result is exactly the readable projection and has no extra segment beyond the encoded FQDN, separator, and group ID.
   - GREEN: keep the function pure and remove any temptation to add hash, UUID, timestamp, or random suffix code.

4. [x] Metadata names round-trip through parse.
   - RED: table-test names from logical IDs including plain FQDN, FQDN with port, uppercase/space input normalization, and a simple dashed group ID like `app-foo`; assert `ParseNSXGroupName(NSXGroupName(id))` returns the normalized FQDN and trimmed group ID.
   - GREEN: implement `ParseNSXGroupName` with explicit validation and numeric trailing-port restoration.

5. [x] Malformed metadata names fail clearly.
   - RED: table-test empty input, `cloud-only`, `--group`, `cloud--`, and `cloud--group--extra`; assert each returns a non-nil error.
   - GREEN: add parse validation with descriptive errors.

6. [x] Stateoperator imports use the canonical package.
   - RED: update or add a `ProcessManagerSnapshot` behavior test for a remote-only group with `NetworkCloudFQDN: "NSX-A.Example.Test:8443"` and assert Observe upsert name `nsx-a.example.test-8443--app-web` while spec remains `nsx-a.example.test:8443`.
   - GREEN: replace private `observeGroupName` with `names.NSXGroupName` and replace internal normalization calls with `names.NormalizeNetworkCloudFQDN`.

7. [x] Startup uses the canonical normalizer.
   - RED: add or adjust a startup test only if an existing public behavior can verify manager base URL normalization without brittle string inspection. If no suitable public behavior exists, skip a new test and rely on names package plus stateoperator integration tests.
   - GREEN: update `internal/startup` imports and call sites to use `internal/names`.

8. [x] Refactor and boundary review.
   - Remove the old `NormalizeNetworkCloudFQDN` and `observeGroupName` helpers from `internal/stateoperator`.
   - Run focused tests after each cleanup: `go test ./internal/names`, `go test ./internal/stateoperator`, and any touched startup package tests.
   - Scan with `rg` for duplicate normalization/name formatting and for ignored errors.

### Required Verification

- `go test ./internal/names`
- `go test ./internal/stateoperator`
- `go test ./internal/startup`
- `make check`
- `make test`
- `make test-coverage`
- Coverage of the new `internal/names` package must be 80% or higher, and global `make test-coverage` must also report 80% or higher.
- Manual verification evidence must be appended to the task file before setting `<passes>true</passes>`, including exact commands and relevant output showing required examples, round-trip behavior, no random/hash suffix behavior, and full check/test/coverage results.
- Final `$improve-code-boundaries` review must record that all NSXGroup naming and normalization logic now lives in `internal/names`, planner/startup call sites reuse it, and no duplicate renderer remains.

NOW EXECUTE

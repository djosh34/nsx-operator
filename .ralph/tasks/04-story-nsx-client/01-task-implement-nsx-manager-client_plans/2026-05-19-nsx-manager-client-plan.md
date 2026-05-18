## Plan: Typed NSX Manager API Client

Task: `.ralph/tasks/04-story-nsx-client/01-task-implement-nsx-manager-client.md`

### Current State

- The repository has validated config, zap JSONL logging, and `internal/httpratelimit`, but no NSX client package yet.
- The sibling `../nsx-t-mockapi` repository exposes the route families this operator must use. It is a black-box sibling service from this module's perspective because its Go `internal` packages are not importable here.
- Mock API default Basic Auth credentials are `nsx_admin` / `nsx_password`.
- The mock API logs every registered route at startup, but it does not expose a route-inventory HTTP endpoint. Client coverage must therefore be compared against the sibling route source or a generated manifest in tests.
- `make check` formats with gofumpt, runs golangci-lint, runs all tests, and runs coverage.

### Public Interface

Create package `internal/nsxclient` with this public surface:

```go
type Options struct {
	BaseURL    string
	HTTPClient *http.Client
	Username   string
	Password   string
	Logger     *zap.Logger
}

func NewClient(options Options) (*Client, error)

func DecodeListResults[T any](reader io.Reader) (results []*T, cursor string, resultCount int, err error)
```

Interface rules:

- `BaseURL` is required and must parse as an absolute HTTP or HTTPS URL.
- `HTTPClient == nil` uses `http.DefaultClient`.
- `Logger == nil` uses `zap.NewNop()`.
- Basic Auth username and password are required. The client must never log the password.
- `Client` is safe for concurrent use because it is immutable after construction and relies on `http.Client` concurrency safety.
- Typed methods accept `context.Context` first and return typed resources or slices plus `error`.
- List methods follow all `cursor` pages and return only the accumulated typed result slice. The generic `DecodeListResults` returns one page's `cursor` and `result_count` for focused tests and reusable pagination internals.
- Group methods do not accept a domain ID. They always route through NSX domain `default`.

### Core Types

Use one shared metadata shape and typed resource structs:

```go
type Resource struct {
	ID               string `json:"id,omitempty"`
	DisplayName      string `json:"display_name,omitempty"`
	Description      string `json:"description,omitempty"`
	ResourceType     string `json:"resource_type,omitempty"`
	Path             string `json:"path,omitempty"`
	ParentPath       string `json:"parent_path,omitempty"`
	RelativePath     string `json:"relative_path,omitempty"`
	Revision         int64  `json:"_revision,omitempty"`
	CreateUser       string `json:"_create_user,omitempty"`
	LastModifiedUser string `json:"_last_modified_user,omitempty"`
	CreateTime       int64  `json:"_create_time,omitempty"`
	LastModifiedTime int64  `json:"_last_modified_time,omitempty"`
}
```

Concrete typed resources embed `Resource` and add only fields used by mockapi/operator routes:

- `Group`, `IPAddressExpression`, `PathExpression`, `GroupMember`, `ConsolidatedEffectiveIPAddresses`
- `FirewallSection`, `FirewallRule`, `FirewallRuleStats`
- `IPSet`, `IPElement`
- `SecurityPolicy`, `SecurityRule`, `SecurityPolicyStats`, `SecurityRuleStats`
- `Segment`, `SegmentState`, `SegmentStatistics`
- `Tier0`, `Tier1`, `Tier1State`
- `EULAAcceptance`
- Search support: `SearchResult`, `SearchQueryOptions`

Use `json.RawMessage` only for explicitly open NSX payload fields where mockapi accepts route-family-specific flexible JSON. Do not expose map-based request builders as the primary interface.

### Error Types

Expose typed status errors using `errors.As`:

```go
type StatusError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
}

type ConflictError struct{ StatusError }            // 409
type PreconditionFailedError struct{ StatusError }  // 412
type RateLimitedError struct{ StatusError }         // 429
type ServiceUnavailableError struct{ StatusError }  // 503
```

Response handling:

- 2xx with response target: decode JSON from the response stream with `json.Decoder`.
- 2xx without response target: drain and close body.
- Non-2xx: read a bounded response body preview, close the body, and return a typed error for 409, 412, 429, and 503; return `StatusError` for other non-2xx status codes.
- Always close every response body, including decode-error and status-error paths.

### Route Coverage Design

Implement typed client methods covering the accepted sibling `nsx-t-mockapi` route inventory:

Manager routes:

- Search: `SearchManagerQuery`, `SearchManagerDSL`
- Firewall sections: list, create, create-with-rules, get, update, delete, revise, list-with-rules, update-with-rules, revise-with-rules
- Firewall rules: list, create, create-multiple, stats, get, update, delete, revise
- IP sets: list, create, get, update, delete, add IP, remove IP, members

Policy routes:

- Search: `SearchPolicyQuery`, `SearchPolicyDSL`
- EULA acceptance: get acceptance
- Global derived state: consolidated effective group IP addresses, global Tier-1 segment state, global Tier-1 segment statistics
- Groups: list, patch, put, get, delete
- Group IP address expressions: patch, add, remove, delete
- Group path expressions: patch
- Group members: IP addresses, IP groups, segments
- Security policies: list, put, delete, get, patch, revise, statistics
- Security rules: list, patch, delete, put, get, statistics, revise
- Infra segments: list, state list, put, delete, patch, get, state, statistics
- Tier-0: list
- Tier-1: list, get, state
- Tier-1 segments: list, state list, delete, patch, put, get, state, statistics

Coverage guard:

- Add a private `supportedRoutes []RouteCoverage` manifest in `internal/nsxclient` or `_test.go` with route name, method, template, action, and the method name that covers it.
- Add a route inventory test that reads these sibling files:
  - `../nsx-t-mockapi/internal/httpapi/app.go`
  - `../nsx-t-mockapi/internal/httpapi/manager_routes.go`
  - `../nsx-t-mockapi/internal/httpapi/policy_routes.go`
- The test extracts route names from `Name: "..."` and `auth*Route("...")` registrations and compares them with `supportedRoutes`.
- The same test fails if a supported route lacks a contract-test case. This makes `make test` fail when mockapi route inventory and typed client coverage diverge.
- Do not import sibling `internal` Go packages.

### Boundary Plan Using `$improve-code-boundaries`

- Keep HTTP request construction, Basic Auth stamping, response decoding, status-error mapping, body closing, and pagination in a small private transport layer inside `internal/nsxclient`.
- Keep path construction private and centralize it with `pathEscape` helpers. Do not hand-concatenate the same route template in each method.
- Keep the domain boundary deep: group/security-policy/security-rule methods hard-code `default` in one private `defaultDomainPath` helper and never expose domain as a public parameter.
- Keep route coverage metadata separate from runtime client code if it is only for tests. Do not make test inventory concerns part of the production API.
- Remove duplication aggressively after green tests: route action handling, list pagination loops, and CRUD path methods should share private helpers instead of repeated request code.
- Avoid DTO churn. If the mockapi payload shape is already the NSX JSON shape, use it as the client type directly instead of adding request/response mirror types with conversion glue.
- Final review after checks: scan for public internals, duplicate type shapes, route strings spread across files, map-based request APIs where typed structs are expected, and logging that leaks credentials.

### TDD Execution Plan Using `$tdd`

Follow vertical red-green cycles. Write one behavior test, see it fail, implement the minimum code to pass, then continue. Do not write all tests first.
 Tracer bullet: construct a client and patch/read/list/delete a policy group through the sibling mockapi process.
   - RED: start `../nsx-t-mockapi` as a subprocess with a temp config/database and a free localhost port; call `PatchGroup`, `GetGroup`, `ListGroups`, and `DeleteGroup`; assert the HTTP paths work without passing a domain ID.
   - GREEN: add `internal/nsxclient`, `Options`, `NewClient`, request helper, group type, Basic Auth, JSON encoding/decoding, and group methods.
 Basic Auth on every request.
   - RED: use a recording `RoundTripper` and prove both read and write methods include Basic Auth.
   - GREEN: stamp Basic Auth in the single private request builder.
 Response bodies always close.
   - RED: use custom bodies that record `Close` for success, status error, and JSON decode error.
   - GREEN: defer close inside the private response handler and avoid ignored close errors.
 Generic stream list decoding.
   - RED: call `DecodeListResults[Group]` with an `io.Reader` stream containing `results`, `cursor`, and `result_count`; assert typed pointers, cursor, and count.
   - GREEN: implement with `json.Decoder` directly on the stream.
 Pagination for list methods.
   - RED: use an `httptest.Server` returning two cursor pages for one list route; assert the client follows `cursor` until empty and returns both results.
   - GREEN: implement a private `listAll[T]` helper used by all typed list methods.
 Typed status errors.
   - RED: table-test 409, 412, 429, and 503 responses with `errors.As` for `ConflictError`, `PreconditionFailedError`, `RateLimitedError`, and `ServiceUnavailableError`.
   - GREEN: implement bounded status-body reading and typed error mapping.
 Manager firewall section/rule and IP set contract coverage.
   - RED/GREEN in small route-family slices against mockapi: create prerequisite resources, call one typed method at a time, and mark the route covered only after the method proves the HTTP contract.
   - Families: firewall sections, firewall rules, firewall rule stats, IP sets, IP set member actions, IP set members.
 Policy group expression and member contract coverage.
   - RED/GREEN one behavior at a time against mockapi.
   - Cover IP address expression patch/add/remove/delete, path expression patch, and group member list methods.
 Policy security contract coverage.
   - RED/GREEN one behavior at a time against mockapi.
   - Cover security policy list/put/get/patch/revise/delete/statistics and security rule list/put/get/patch/revise/delete/statistics.
 Segment, gateway, global derived state, search, and EULA contract coverage.
    - RED/GREEN one behavior at a time against mockapi.
    - Cover infra segments, Tier-0 list, Tier-1 list/get/state, Tier-1 segments, global Tier-1 segment state/statistics, consolidated group IPs, policy/manager search, and EULA acceptance.
 Route inventory divergence guard.
    - RED: temporarily remove one route from `supportedRoutes` and confirm the route inventory test fails; restore it before continuing.
    - GREEN: keep the inventory extractor and contract-case comparison in normal test flow.
 Refactor after green.
    - Run focused tests after each refactor.
    - Collapse duplicated route helpers and remove unnecessary request/response mirror types.
    - Keep all errors checked; no ignored `_ :=` error assignments.

### Mockapi Test Harness

- Create test helpers under `internal/nsxclient` tests to start the sibling service as a subprocess:
  - choose a free localhost port,
  - write a temp mockapi config with that port and a temp database path,
  - run `go run ../nsx-t-mockapi/cmd/nsx-t-mockapi serve -config <config>`,
  - wait until an authenticated `GET /policy/api/v1/eula/acceptance` succeeds,
  - stop the process with context cancellation and wait for it to exit.
- Use the real default credentials `nsx_admin` / `nsx_password`.
- Keep subprocess stderr captured so failures include mockapi logs as concrete evidence.
- Use `httptest.Server` instead of mockapi only for synthetic protocol behavior that mockapi does not naturally expose, such as forced 429/503 status mapping and custom paginated cursor pages.

### Verification

Run focused commands during implementation:

```bash
go test ./internal/nsxclient
go test -race ./internal/nsxclient
go test -cover ./internal/nsxclient
```

Then run all required checks:

```bash
make check
make test
make test-coverage
```

Coverage requirement:

- `internal/nsxclient` coverage must be 80%+.
- Overall `make test-coverage` must also report 80%+ for all packages.

Manual evidence to record in the task file before setting `<passes>true</passes>`:

- Contract test output proving typed methods work against `../nsx-t-mockapi`.
- Route inventory test output showing no divergence.
- Focused `go test -race ./internal/nsxclient` output.
- Focused `go test -cover ./internal/nsxclient` output with 80%+.
- Full `make check`, `make test`, and `make test-coverage` output.
- Brief note that tests prove pagination, stream decoding, Basic Auth, body closure, and typed errors for 409/412/429/503.

### Completion Steps

- Mark TDD checklist items complete in this plan as they are implemented.
- If implementation proves the public interface, route coverage design, or type design is wrong, replace the final marker with `TO BE VERIFIED` and quit immediately.
- If design remains valid and all checks pass:
  - update the task file with concrete verification evidence,
  - set `<passes>true</passes>`,
  - run `/bin/bash .ralph/task_switch.sh`,
  - add all files,
  - commit with `task finished 01-task-implement-nsx-manager-client: implement typed NSX manager client`,
  - include summary and test evidence in the commit message,
  - push,
  - quit immediately.

Plan path: `.ralph/tasks/04-story-nsx-client/01-task-implement-nsx-manager-client_plans/2026-05-19-nsx-manager-client-plan.md`

NOW EXECUTE

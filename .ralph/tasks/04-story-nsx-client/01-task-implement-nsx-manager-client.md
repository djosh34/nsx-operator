## Task: Implement Typed NSX Manager API Client <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/nsxclient` as a concurrent-safe typed client for the NSX-T 4.2.3 APIs used by this operator and the parent-dir `nsx-t-mockapi`. The group APIs must hide the domain ID and always use hard-coded NSX domain `default`.

In scope: `NewClient` options for base URL, shared HTTP client, Basic Auth, and zap logger; JSON request/response behavior; Basic Auth on every request; closed response bodies; typed group methods for list/read/patch/delete group, IP address expression patch/delete, and path expression patch/delete; generic `DecodeListResults[T any]` using `json.Decoder` directly on streams to return `[]*T`, `cursor`, and `resultCount`; pagination of all list calls until cursor is empty; typed status errors for 409, 412, 429, and 503; route inventory from parent `nsx-t-mockapi`; typed methods and contract tests for every mockapi route family listed in the design. Out of scope: automatic retry loops, automatic refetch/reapply after 409/412, automatic retry after 429/503, and caller-owned reconciliation logic.

Method families to cover from mockapi/OpenAPI: firewall sections/rules/stats, ip-sets, policy groups, group IP-address expressions, group path expressions, group members, security policies/rules/stats, segments, segment state/statistics, tier-0 list, tier-1 list/read/state/segments, and EULA acceptance.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Contract tests exercise every typed client method against `nsx-t-mockapi`.
- [x] CI or a test fails when route inventory and typed client coverage diverge.
- [x] Tests prove list pagination, stream decoding, Basic Auth, response body closure, and typed error mapping.
</acceptance_criteria>

<verification_evidence>
Implemented package:
- `internal/nsxclient.Options` and `NewClient` for base URL, shared HTTP client, Basic Auth, and zap logger.
- Typed resource methods for mockapi Manager and Policy route families: manager search, firewall sections/rules/stats, IP sets/members, policy search, EULA acceptance, groups, group IP address/path expressions, group members, security policies/rules/stats, infra segments/state/statistics, Tier-0 list, Tier-1 list/get/state, Tier-1 segments, global Tier-1 segment state/statistics, and consolidated effective group IP addresses.
- `DecodeListResults[T any]` uses `json.Decoder` on the input stream and returns typed `[]*T`, `cursor`, and `result_count`.
- Pagination follows `cursor` until empty for all list methods.
- Basic Auth is stamped in one request builder on every request.
- Response bodies are closed on success, status-error, and decode-error paths.
- Status errors map 409, 412, 429, and 503 to typed errors usable with `errors.As`.
- Group, security policy, and security rule APIs keep NSX domain `default` private; no public domain parameter was added.

Behavior and contract evidence:
- `TestTypedClientContractsAgainstMockAPI` builds a temporary `../nsx-t-mockapi` binary, starts it with a temp config/database on localhost, waits for authenticated EULA readiness, then exercises every supported typed client method against the running mockapi.
- `TestMockAPIRouteInventoryIsSupportedAndContracted` reads sibling route source files and fails if mockapi route names and typed-client contract coverage diverge.
- `TestClientAddsBasicAuthToReadAndWriteRequests` proves read and write methods include Basic Auth.
- `TestResponseBodiesCloseForSuccessStatusErrorAndDecodeError` proves response body closure on success, status error, and JSON decode error.
- `TestDecodeListResultsStreamsTypedPointers` proves stream list decoding returns typed pointers, cursor, and result count.
- `TestListMethodsFollowPaginationUntilCursorIsEmpty` proves list pagination follows cursor pages through the default-domain group list route.
- `TestStatusErrorsMapTypedCodes` proves typed mappings for 409, 412, 429, and 503.

Final command evidence from 2026-05-19:

```text
$ go test ./internal/nsxclient -run TestTypedClientContractsAgainstMockAPI -count=1 -timeout=90s -v
=== RUN   TestTypedClientContractsAgainstMockAPI
=== PAUSE TestTypedClientContractsAgainstMockAPI
=== CONT  TestTypedClientContractsAgainstMockAPI
--- PASS: TestTypedClientContractsAgainstMockAPI (0.58s)
PASS
ok  	github.com/djosh34/nsx-operator/internal/nsxclient	0.584s

$ go test ./internal/nsxclient -run TestMockAPIRouteInventoryIsSupportedAndContracted
ok  	github.com/djosh34/nsx-operator/internal/nsxclient	0.005s

$ go test -race ./internal/nsxclient -count=1
ok  	github.com/djosh34/nsx-operator/internal/nsxclient	1.604s

$ go test ./internal/nsxclient -count=1 -cover
ok  	github.com/djosh34/nsx-operator/internal/nsxclient	0.586s	coverage: 80.3% of statements

$ make check
/home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/.bin/gofumpt -w .
/home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/.bin/golangci-lint run ./...
0 issues.
go test ./...
ok  	github.com/djosh34/nsx-operator/cmd/nsx-operator	(cached)
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)
ok  	github.com/djosh34/nsx-operator/internal/config	(cached)
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	(cached)
ok  	github.com/djosh34/nsx-operator/internal/logging	(cached)
ok  	github.com/djosh34/nsx-operator/internal/nsxclient	0.572s
ok  	github.com/djosh34/nsx-operator/internal/startup	(cached)
go test -cover ./...
ok  	github.com/djosh34/nsx-operator/cmd/nsx-operator	(cached)	coverage: 81.6% of statements
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)	coverage: 100.0% of statements
ok  	github.com/djosh34/nsx-operator/internal/config	(cached)	coverage: 82.9% of statements
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	(cached)	coverage: 87.8% of statements
ok  	github.com/djosh34/nsx-operator/internal/logging	(cached)	coverage: 96.2% of statements
ok  	github.com/djosh34/nsx-operator/internal/nsxclient	0.601s	coverage: 80.3% of statements
ok  	github.com/djosh34/nsx-operator/internal/startup	(cached)	coverage: 82.8% of statements

$ make test
go test ./...
ok  	github.com/djosh34/nsx-operator/cmd/nsx-operator	(cached)
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)
ok  	github.com/djosh34/nsx-operator/internal/config	(cached)
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	(cached)
ok  	github.com/djosh34/nsx-operator/internal/logging	(cached)
ok  	github.com/djosh34/nsx-operator/internal/nsxclient	(cached)
ok  	github.com/djosh34/nsx-operator/internal/startup	(cached)

$ make test-coverage
go test -cover ./...
ok  	github.com/djosh34/nsx-operator/cmd/nsx-operator	(cached)	coverage: 81.6% of statements
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)	coverage: 100.0% of statements
ok  	github.com/djosh34/nsx-operator/internal/config	(cached)	coverage: 82.9% of statements
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	(cached)	coverage: 87.8% of statements
ok  	github.com/djosh34/nsx-operator/internal/logging	(cached)	coverage: 96.2% of statements
ok  	github.com/djosh34/nsx-operator/internal/nsxclient	(cached)	coverage: 80.3% of statements
ok  	github.com/djosh34/nsx-operator/internal/startup	(cached)	coverage: 82.8% of statements
```

Final improve-code-boundaries review:
- Transport concerns are private to `internal/nsxclient`: request construction, Basic Auth stamping, response decoding, typed status errors, body closure, and cursor pagination.
- Route inventory and contract coverage metadata live only in tests, not production runtime code.
- Domain handling is centralized in private path helpers and hard-codes `default` without exposing domain IDs in public group/security methods.
- A post-check refactor removed the private list-result type switch and uses the generic pagination helper directly from typed route methods.
- No password logging was added; client construction logs only the base URL.
</verification_evidence>

<plan>
.ralph/tasks/04-story-nsx-client/01-task-implement-nsx-manager-client_plans/2026-05-19-nsx-manager-client-plan.md
</plan>

NOW EXECUTE

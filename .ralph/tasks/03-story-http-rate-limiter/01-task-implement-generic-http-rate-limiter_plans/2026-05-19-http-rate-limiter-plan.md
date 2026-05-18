## Plan: Generic HTTP Rate-Limited RoundTripper

Task: `.ralph/tasks/03-story-http-rate-limiter/01-task-implement-generic-http-rate-limiter.md`

### Current State

- The repository has config fields for HTTP rate limiting, but no `internal/httpratelimit` package yet.
- The task is NSX-independent and should not import NSX, startup, or config packages.
- Existing logging uses zap JSON logs to stderr; the limiter should accept a zap logger and default to `zap.NewNop()` when nil.
- `make check` runs gofumpt, golangci-lint, `go test ./...`, and coverage.

### Public Interface

Create package `internal/httpratelimit` with only this public surface:

```go
type Config struct {
	MaxRequestsInFlightPerHost  int
	MaxRequestsPerSecondPerHost int
}

func NewRoundTripper(base http.RoundTripper, cfg Config, log *zap.Logger) http.RoundTripper
```

Interface decisions:

- `base == nil` uses `http.DefaultTransport`.
- `log == nil` uses `zap.NewNop()`.
- Config values are expected to be positive because config validation already enforces that at the application boundary.
- Because `NewRoundTripper` cannot return an error, non-positive limits will be clamped to `1` and logged at info level with the original values. This prevents dead buckets and keeps failures observable without adding a second public constructor.
- Tests will exercise the limiter through `http.Client` or the returned `http.RoundTripper`, not private bucket internals.

### Boundary Plan Using `$improve-code-boundaries`

- Keep all limiter mechanics in `internal/httpratelimit`; do not add config-to-limiter translation here.
- Keep bucket keys, bucket registry, body wrapper, and helper types private to avoid the "too public" smell.
- Use one canonical host-port key builder inside the package. Do not duplicate host normalization in tests or other packages.
- Avoid mixed responsibilities by separating only real boundaries:
  - public wrapper construction,
  - host-port key normalization,
  - bucket lookup/creation,
  - request acquisition/release behavior.
- Avoid overengineering: no queues, route buckets, request priority, URL-specific buckets, background cleanup, or exported diagnostics.
- Final review after tests: scan for duplicate request-state shapes, unnecessary helper fragmentation, public internals, and config validation leaking into this package.

### Implementation Design

- Add direct dependency `golang.org/x/time/rate` for proven context-aware token bucket behavior.
- Package-level registry:
  - `sync.Mutex`
  - `map[string]*bucket`
  - key string is canonical effective `host:port`
- Key normalization:
  - use `req.URL.Hostname()` and `req.URL.Port()`
  - lower-case host
  - default missing port by scheme: `https` -> `443`, `http` -> `80`
  - explicit port wins
  - IPv6 hosts are rejoined with `net.JoinHostPort`
- Bucket:
  - `*rate.Limiter` for max requests per second
  - buffered channel semaphore for max in-flight requests
- RoundTrip flow:
  - normalize key and get shared bucket
  - log debug before waiting, after rate permit, after in-flight acquire, before base RoundTrip, on error, and on release
  - wait for rate permit with request context
  - acquire in-flight semaphore with request context
  - call base RoundTrip
  - if RoundTrip returns error, release immediately and return the original error
  - if response or body is nil, release immediately and return the response
  - wrap non-nil response body so `Close` releases exactly once and returns the underlying close error
- Use `sync.Once` in the body wrapper to make repeated `Close` calls safe.

### TDD Execution Plan Using `$tdd`

Follow vertical red-green cycles. Do not write all tests first.

1. [x] Tracer bullet: same effective host+port shares global in-flight bucket across wrapper instances.
   - RED: two clients with separate wrappers and `MaxRequestsInFlightPerHost: 1` target `https://EXAMPLE.test` and `https://example.test:443`; second request blocks until first response body closes.
   - GREEN: add package, constructor, global bucket registry, default host-port normalization for https, in-flight semaphore, and body close release.

2. [x] Different ports are isolated.
   - RED: concurrent requests to `https://example.test:443` and `https://example.test:8443` both enter their base transports without waiting.
   - GREEN: ensure bucket key includes effective port and explicit port normalization.

3. [x] HTTP default port normalization.
   - RED: `http://EXAMPLE.test` and `http://example.test:80` share the same bucket.
   - GREEN: add or fix `http` default port handling.

4. [x] Context cancellation while waiting.
   - RED: with one request holding the in-flight slot, a second request with a short context deadline returns a context deadline/cancellation error and does not call the base transport.
   - GREEN: implement context-aware semaphore acquisition and ensure no synthetic throttle errors.

5. [x] RoundTrip error releases immediately.
   - RED: first request reaches a base transport that returns an error; a second request to the same bucket can enter immediately afterward.
   - GREEN: release on base error path.

6. [x] Idempotent Body.Close release.
   - RED: closing the first response body twice must not release two slots; with limit 1, only one waiting request may proceed after the close.
   - GREEN: add `sync.Once` body wrapper release.

7. [x] Requests-per-second behavior.
   - RED: with rate `1` per second, the second request waits until context deadline when no token becomes available quickly enough, and base transport is not called for the second request.
   - GREEN: wire the rate limiter before in-flight acquisition while preserving request context errors.

8. [x] Nil defaults and nil response body.
   - RED: nil base uses `http.DefaultTransport` enough to not panic in construction; a successful response with nil body releases the slot immediately.
   - GREEN: fill constructor defaults and nil body release.

### Verification

Run each focused test package command during the TDD cycles:

```bash
go test ./internal/httpratelimit
go test -race ./internal/httpratelimit
```

Then run required full checks:

```bash
make check
make test
make test-coverage
```

Coverage requirement:

- New package coverage must be 80%+.
- `make test-coverage` must also report 80%+ overall.

Manual evidence to record in the task file before setting `<passes>true</passes>`:

- `go test -race ./internal/httpratelimit` output showing concurrent shared-bucket coverage passed.
- `make check` output.
- `make test` output.
- `make test-coverage` output including coverage percentages.
- Brief note that tests cover default port normalization, different-port isolation, context cancellation while waiting, RoundTrip error release, and idempotent Body.Close release.

### Completion Steps

- Mark TDD checklist items complete in this plan as they are implemented.
- If implementation proves the interface or type design is wrong, replace the final marker with `TO BE VERIFIED` and quit immediately.
- If design remains valid and all checks pass:
  - update the task file with concrete verification evidence,
  - set `<passes>true</passes>`,
  - run `/bin/bash .ralph/task_switch.sh`,
  - add all files,
  - commit with `task finished 01-task-implement-generic-http-rate-limiter: implement generic HTTP rate limiter`,
  - include summary and test evidence in the commit message,
  - push,
  - quit immediately.

Plan path: `.ralph/tasks/03-story-http-rate-limiter/01-task-implement-generic-http-rate-limiter_plans/2026-05-19-http-rate-limiter-plan.md`

NOW EXECUTE

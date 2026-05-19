## Plan: Validate HTTP Concurrency Model Across Manager Goroutines

Task: `.ralph/tasks/15-story-http-concurrency-model/01-task-validate-http-concurrency-model.md`

### Current State

- `internal/httpratelimit` already provides a global effective host+port `http.RoundTripper` registry with race-safe buckets.
- Existing limiter tests cover same host+port sharing, default port normalization, different-port isolation, context cancellation, base transport errors, idempotent body close release, nil defaults, and nil response body release.
- `internal/startup.NewManager` currently constructs NSX manager clients without passing an `HTTPClient`, so those clients fall back to `http.DefaultClient` and do not use the configured limiter.
- `internal/config.Config` already carries `HTTPRateLimiter` settings and startup logs those values.
- `stateoperator.NSXStateOperator` already runs one goroutine per `NSXNetworkCloud` during each global sweep; the missing piece is shared HTTP transport wiring into the NSX clients those goroutines create.
- Existing mockapi-backed tests already build and run `../nsx-t-mockapi`, so the repository has precedent for concrete NSX-T mock verification.

### Public Interface

No user-facing API or CRD shape changes are needed.

Internal wiring behavior:

- `startup.NewManager` should create one shared `*http.Client` for NSX manager traffic.
- That shared client should use `httpratelimit.NewRoundTripper(http.DefaultTransport, cfg, logger)`.
- `cfg` should be converted directly from `options.Config.HTTPRateLimiter`.
- Every NSX client created by the `managerClientFactory` inside `startup.NewManager` should receive the same shared `*http.Client`.
- The existing NSX manager base URL construction remains `https://` plus `names.NormalizeNetworkCloudFQDN`.
- No request queues, worker pools, manager queues, per-manager semaphores, or route-specific throttling should be added.

### Boundary Plan Using `$improve-code-boundaries`

- Keep limiter mechanics in `internal/httpratelimit`; do not move bucket registry, host+port normalization, or semaphore behavior into startup, stateoperator, or nsxclient.
- Keep startup responsible only for application composition:
  - translate config rate limiter fields to `httpratelimit.Config`,
  - construct one shared rate-limited `http.Client`,
  - pass that client to NSX clients.
- Do not introduce a second NSX HTTP abstraction unless implementation proves the constructor body is becoming unclear.
- Do not add diagnostic-only exported internals to the limiter.
- Prefer one small private startup helper if the conversion/client construction would otherwise clutter `NewManager`.
- Final review after checks:
  - verify only startup imports `internal/httpratelimit` for production wiring,
  - verify no duplicate host normalization was added outside the limiter and existing `names.NormalizeNetworkCloudFQDN`,
  - verify no queues or worker pools were introduced,
  - verify the NSX client remains unaware of rate-limit internals.

### TDD Execution Plan Using `$tdd`

Follow vertical red-green cycles. Do not write all tests first.

1. [x] Startup wiring tracer bullet: manager sweeps sharing one effective host+port serialize at the HTTP transport boundary.
   - RED: add a behavior test through public `startup.NewManager` with envtest and three `NSXNetworkCloud` objects.
   - Test mechanics:
     - use a TLS `httptest.Server` that handles `GET /policy/api/v1/infra/domains/default/groups` and returns an empty NSX list response,
     - temporarily replace `http.DefaultTransport` with a transport that dials every requested host+port to the TLS server while preserving the original request URL host,
     - configure `HTTPRateLimiter.MaxRequestsInFlightPerHost: 1` and a high per-second limit,
     - create two clouds targeting the same effective `nsx-a.example.net:443` bucket and one cloud targeting `nsx-a.example.net:8443`,
     - start the manager and let the first `nsx-a.example.net:443` request block inside the test server,
     - assert the second same-bucket request does not reach the server until the first response is released,
     - assert the `nsx-a.example.net:8443` request reaches the server while the first `:443` request is still blocked.
   - GREEN: wire one shared rate-limited `*http.Client` into `startup.NewManager` and pass it to every `nsxclient.NewClient` call from the manager client factory.

2. [x] Startup config propagation: configured rate limiter values are used instead of hard-coded defaults.
   - RED: extend the focused startup test or add a second focused startup test where `MaxRequestsInFlightPerHost: 2` allows two same-host manager sweeps to reach the server before either response is released.
   - GREEN: ensure `startup.NewManager` converts `config.HTTPRateLimiterConfig` directly into `httpratelimit.Config`.

3. [x] Existing limiter race evidence still holds.
   - RED is already represented by the existing concurrent limiter tests.
   - GREEN: run `go test -race ./internal/httpratelimit` after startup wiring and keep the output for task evidence.

4. [x] Mockapi-backed integration evidence.
   - Use existing `../nsx-t-mockapi` test style: build/start the sibling mockapi process from Go tests or run a focused manual command.
   - Verify real `nsxclient.Client` calls with a shared rate-limited transport can talk to mockapi and produce concrete logs/evidence for same-host sharing and different-host/different-port isolation.
   - If the TLS-only `startup.NewManager` test already proves manager goroutine behavior but does not produce enough mockapi evidence for the acceptance criteria, add a focused integration test that starts mockapi and routes multiple logical manager host headers through a shared limited transport.

5. [x] Final boundary refactor.
   - With tests green, simplify the startup client construction if needed.
   - Remove any test helper duplication that is not carrying behavior.
   - Keep logs structured with zap and do not ignore any errors.

### Verification Commands

Run focused commands during implementation:

```bash
go test ./internal/startup -run TestNewManagerSharesRateLimitedTransportAcrossCloudSweeps -count=1 -v
go test ./internal/startup -run TestNewManagerUsesConfiguredHTTPRateLimiterLimits -count=1 -v
go test -race ./internal/httpratelimit
```

Run required full checks before completing the task:

```bash
make check
make test
make test-coverage
```

Coverage requirements:

- Any new production code must have 80%+ coverage.
- `make test-coverage` must report 80%+ overall.

### Evidence To Record In Task

- Focused startup test output proving:
  - same effective `nsx-a.example.net:443` manager sweeps serialize,
  - `nsx-a.example.net:8443` is admitted while `:443` is blocked,
  - configured in-flight limit changes behavior.
- `go test -race ./internal/httpratelimit` output.
- Mockapi-backed integration output or logs if added for NSX-T evidence.
- `make check` output.
- `make test` output.
- `make test-coverage` output including package and overall coverage.
- Final improve-code-boundaries review notes showing no extra queues, no duplicate host normalization, and no limiter internals leaked into nsxclient/stateoperator.

### Completion Steps

- Mark TDD checklist items complete in this plan as they are implemented.
- If implementation proves this interface or type design is wrong, replace the final marker with `TO BE VERIFIED` and quit immediately.
- If design remains valid and all checks pass:
  - update the task file with concrete verification evidence,
  - set `<passes>true</passes>`,
  - run `/bin/bash .ralph/task_switch.sh`,
  - add all files, including `.ralph` files,
  - commit with `task finished 01-task-validate-http-concurrency-model: validate manager HTTP concurrency model`,
  - include summary, verification evidence, and implementation challenges in the commit message,
  - push,
  - quit immediately.

Plan path: `.ralph/tasks/15-story-http-concurrency-model/01-task-validate-http-concurrency-model_plans/2026-05-19-http-concurrency-validation-plan.md`

NOW EXECUTE

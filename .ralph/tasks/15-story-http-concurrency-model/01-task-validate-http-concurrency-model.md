## Task: Validate HTTP Concurrency Model Across Manager Goroutines <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Wire and verify HTTP concurrency so manager goroutines are independent while sharing the same global per host+port limiter registry through the shared rate-limited transport.

In scope: all NSX clients share the same rate-limited transport; two clients targeting `https://nsx-a.example.net` share the `nsx-a.example.net:443` bucket; `https://nsx-a.example.net` and `https://nsx-a.example.net:8443` use different buckets; different NSX managers do not block each other unless they target the same effective host+port; no request queues, worker pools, or manager queues are introduced. Out of scope: route-specific throttling.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Race tests prove shared limiter behavior across concurrent manager goroutines.
- [x] E2E or integration logs prove same-host sharing and different-host/different-port isolation.
</acceptance_criteria>

<plan>
.ralph/tasks/15-story-http-concurrency-model/01-task-validate-http-concurrency-model_plans/2026-05-19-http-concurrency-validation-plan.md
</plan>

<verification_evidence>
- TDD RED evidence before production wiring:
  - `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/startup -run TestNewManagerSharesRateLimitedTransportAcrossCloudSweeps -count=1 -v`
  - Failed with `second same host request reached NSX server before first same host response was released`, proving manager clients were not yet sharing the host+port in-flight limiter.
- Startup manager goroutine behavior:
  - `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/startup -run 'TestNewManager(SharesRateLimitedTransportAcrossCloudSweeps|UsesConfiguredHTTPRateLimiterLimits)' -count=1 -v`
  - Passed.
  - `TestNewManagerSharesRateLimitedTransportAcrossCloudSweeps` creates three `NSXNetworkCloud` objects through envtest and public `startup.NewManager`; two clouds target `https://nsx-a.example.net` and one targets `https://nsx-a.example.net:8443`. The routed TLS server blocks the first `nsx-a.example.net:443` response, verifies the second `:443` request does not reach the server before release, and verifies `nsx-a.example.net:8443` reaches the server while `:443` is blocked.
  - `TestNewManagerUsesConfiguredHTTPRateLimiterLimits` configures `MaxRequestsInFlightPerHost: 2` and verifies two same-host manager sweeps reach the server before the first response is released.
- Mockapi-backed NSX-T integration evidence:
  - `go test ./internal/nsxclient -run TestSharedRateLimitedClientConcurrencyAgainstMockAPI -count=1 -v`
  - Passed.
  - Verbose output included `mockapi concurrency evidence: same logical host nsx-mock-a.example.net:80 shared one in-flight slot; nsx-mock-a.example.net:8080 reached mockapi while :80 was blocked`.
  - The test starts the sibling `../nsx-t-mockapi`, uses real `nsxclient.Client.ListGroups` calls, and shares one `httpratelimit` transport across logical NSX manager clients.
- Limiter race evidence:
  - `go test -race ./internal/httpratelimit`
  - Passed: `ok github.com/djosh34/nsx-operator/internal/httpratelimit 1.124s`.
- Required full verification:
  - `make check`
  - Passed: `gofumpt -w .`, `golangci-lint run ./...` with `0 issues`, `go test ./...`, and `go test -cover ./...`.
  - `make test`
  - Passed: all packages `ok`.
  - `make test-coverage`
  - Passed with package coverage at or above 80%: `api/v1alpha 100.0%`, `cmd/nsx-operator 80.8%`, `internal/buildinfo 100.0%`, `internal/config 82.9%`, `internal/httpratelimit 87.8%`, `internal/kubeapi 80.9%`, `internal/logging 96.2%`, `internal/names 93.9%`, `internal/nsxclient 80.4%`, `internal/startup 82.8%`, `internal/stateoperator 80.2%`, `internal/statuscondition 91.1%`.
- Final improve-code-boundaries review:
  - Production limiter mechanics remain in `internal/httpratelimit`.
  - Startup is the only production package newly importing `internal/httpratelimit`; it translates config and composes one shared NSX `*http.Client`.
  - `nsxclient` remains unaware of limiter internals and only receives an ordinary `*http.Client`.
  - `stateoperator` has no limiter dependency.
  - No request queues, worker pools, manager queues, or route-specific throttling were introduced.
</verification_evidence>

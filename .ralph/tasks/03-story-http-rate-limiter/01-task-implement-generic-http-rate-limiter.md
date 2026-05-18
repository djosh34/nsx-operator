## Task: Implement Generic HTTP Rate-Limited RoundTripper <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/httpratelimit` as an NSX-independent `http.RoundTripper` wrapper that globally rate-limits by effective host+port. It must block until a request is allowed, honor request context cancellation, and never return throttle errors except context cancellation/deadline errors.

In scope: public API `Config` and `NewRoundTripper(base http.RoundTripper, cfg Config, log *zap.Logger)`; nil base defaults to `http.DefaultTransport`; host+port keys normalize default ports (`https` to 443, `http` to 80) and lower-case host; same host+port shares a global bucket across all wrapper instances; different ports use different buckets; max requests in flight enforced per bucket; response bodies wrapped so the in-flight semaphore is released exactly once on `Body.Close`; RoundTrip errors release immediately; map and per-host state are race-safe. Out of scope: queues, route prefix buckets, URL-specific buckets, and NSX imports.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] `go test -race` evidence covers concurrent callers sharing the same host+port bucket.
- [x] Tests cover default port normalization, different-port isolation, context cancellation while waiting, RoundTrip error release, and idempotent Body.Close release.
</acceptance_criteria>

<verification_evidence>
Implemented package:
- `internal/httpratelimit.Config`
- `internal/httpratelimit.NewRoundTripper(base http.RoundTripper, cfg Config, log *zap.Logger) http.RoundTripper`
- NSX-independent limiter using only `net/http`, standard sync/time helpers, zap logging, and `golang.org/x/time/rate`.

Behavior evidence:
- `TestRoundTripperSharesInFlightBucketAcrossWrappersForSameEffectiveHostPort` verifies concurrent callers using separate wrappers share the same global effective `https` host+port bucket.
- `TestRoundTripperNormalizesHTTPDefaultPort` verifies `http://host` and `http://host:80` share a bucket.
- `TestRoundTripperIsolatesDifferentPorts` verifies the same host on `443` and `8443` uses different buckets.
- `TestRoundTripperReturnsContextErrorWhileWaitingForInFlightSlot` verifies waiting on the in-flight slot returns `context.DeadlineExceeded` and does not call the base transport.
- `TestRoundTripperReturnsContextErrorWhileWaitingForRatePermit` verifies waiting on the per-second rate permit returns `context.DeadlineExceeded` and does not call the base transport.
- `TestRoundTripperReleasesInFlightSlotAfterBaseError` verifies a base `RoundTrip` error releases the in-flight slot immediately.
- `TestRoundTripperReleasesInFlightSlotOnceWhenBodyClosedMultipleTimes` verifies repeated `Body.Close` calls release exactly one slot.
- `TestRoundTripperNilBaseUsesDefaultTransport` verifies nil base uses `http.DefaultTransport` against an `httptest` server.
- `TestRoundTripperReleasesInFlightSlotForNilResponseBody` verifies a nil response body releases immediately.

Final command evidence from 2026-05-19:

```text
$ go test -race ./internal/httpratelimit
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	1.121s

$ go test -cover ./internal/httpratelimit
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	0.112s	coverage: 87.8% of statements

$ make check
/home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/.bin/gofumpt -w .
/home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/.bin/golangci-lint run ./...
0 issues.
go test ./...
ok  	github.com/djosh34/nsx-operator/cmd/nsx-operator	(cached)
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)
ok  	github.com/djosh34/nsx-operator/internal/config	(cached)
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	0.108s
ok  	github.com/djosh34/nsx-operator/internal/logging	(cached)
ok  	github.com/djosh34/nsx-operator/internal/startup	(cached)
go test -cover ./...
ok  	github.com/djosh34/nsx-operator/cmd/nsx-operator	(cached)	coverage: 81.6% of statements
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)	coverage: 100.0% of statements
ok  	github.com/djosh34/nsx-operator/internal/config	(cached)	coverage: 82.9% of statements
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	(cached)	coverage: 87.8% of statements
ok  	github.com/djosh34/nsx-operator/internal/logging	(cached)	coverage: 96.2% of statements
ok  	github.com/djosh34/nsx-operator/internal/startup	(cached)	coverage: 82.8% of statements

$ make test
go test ./...
ok  	github.com/djosh34/nsx-operator/cmd/nsx-operator	(cached)
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)
ok  	github.com/djosh34/nsx-operator/internal/config	(cached)
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	(cached)
ok  	github.com/djosh34/nsx-operator/internal/logging	(cached)
ok  	github.com/djosh34/nsx-operator/internal/startup	(cached)

$ make test-coverage
go test -cover ./...
ok  	github.com/djosh34/nsx-operator/cmd/nsx-operator	(cached)	coverage: 81.6% of statements
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)	coverage: 100.0% of statements
ok  	github.com/djosh34/nsx-operator/internal/config	(cached)	coverage: 82.9% of statements
ok  	github.com/djosh34/nsx-operator/internal/httpratelimit	(cached)	coverage: 87.8% of statements
ok  	github.com/djosh34/nsx-operator/internal/logging	(cached)	coverage: 96.2% of statements
ok  	github.com/djosh34/nsx-operator/internal/startup	(cached)	coverage: 82.8% of statements
```

Final improve-code-boundaries review:
- Public surface remains only `Config` and `NewRoundTripper`.
- Bucket registry, host-port normalization, rate wait, semaphore, and body wrapper are private to `internal/httpratelimit`.
- `go list -deps ./internal/httpratelimit` confirmed no config, startup, controller, or NSX package dependency is imported.
- No duplicated host normalization or config translation was added outside the package.
</verification_evidence>

## Task: Implement Generic HTTP Rate-Limited RoundTripper <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/httpratelimit` as an NSX-independent `http.RoundTripper` wrapper that globally rate-limits by effective host+port. It must block until a request is allowed, honor request context cancellation, and never return throttle errors except context cancellation/deadline errors.

In scope: public API `Config` and `NewRoundTripper(base http.RoundTripper, cfg Config, log *zap.Logger)`; nil base defaults to `http.DefaultTransport`; host+port keys normalize default ports (`https` to 443, `http` to 80) and lower-case host; same host+port shares a global bucket across all wrapper instances; different ports use different buckets; max requests in flight enforced per bucket; response bodies wrapped so the in-flight semaphore is released exactly once on `Body.Close`; RoundTrip errors release immediately; map and per-host state are race-safe. Out of scope: queues, route prefix buckets, URL-specific buckets, and NSX imports.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] `go test -race` evidence covers concurrent callers sharing the same host+port bucket.
- [ ] Tests cover default port normalization, different-port isolation, context cancellation while waiting, RoundTrip error release, and idempotent Body.Close release.
</acceptance_criteria>

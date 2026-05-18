## Task: Implement Immutable Startup Config Module <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/config` so the operator parses and validates all runtime configuration exactly once before controller-runtime, Kubernetes clients, or NSX clients start. The validated config must be immutable from startup onward; components must consume the validated struct and must not revalidate or hot reload config.

In scope: parse the documented YAML shape for `operator.tickInterval`, `httpRateLimiter.maxRequestsInFlightPerHost`, `httpRateLimiter.maxRequestsPerSecondPerHost`, `nsx.auth`, `nsx.tls`, and `logging.level`; resolve exactly one complete Basic Auth credential source using precedence `NSX_USERNAME`/`NSX_PASSWORD`, then `NSX_USERNAME_FILE`/`NSX_PASSWORD_FILE`, then config username/password, then config usernameFile/passwordFile; read credential files once; trim one trailing newline; reject missing or empty credential files; validate positive tick and rate-limit values; validate `tls.caBundleFile` exists when set; ensure credential values and credential file contents are never logged. Out of scope: per-cloud credentials and hot reload.

The task must update startup wiring so invalid config exits before any Kubernetes or NSX clients are created.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Unit tests cover credential precedence, file trimming, empty/missing files, invalid numeric values, missing CA bundle file, and no resolved credential source.
- [ ] Startup integration evidence shows invalid config exits before client construction.
</acceptance_criteria>

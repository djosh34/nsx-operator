## Task: Implement Immutable Startup Config Module <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/config` so the operator parses and validates all runtime configuration exactly once before controller-runtime, Kubernetes clients, or NSX clients start. The validated config must be immutable from startup onward; components must consume the validated struct and must not revalidate or hot reload config.

In scope: parse the documented YAML shape for `operator.tickInterval`, `httpRateLimiter.maxRequestsInFlightPerHost`, `httpRateLimiter.maxRequestsPerSecondPerHost`, `nsx.auth`, `nsx.tls`, and `logging.level`; resolve exactly one complete Basic Auth credential source using precedence `NSX_USERNAME`/`NSX_PASSWORD`, then `NSX_USERNAME_FILE`/`NSX_PASSWORD_FILE`, then config username/password, then config usernameFile/passwordFile; read credential files once; trim one trailing newline; reject missing or empty credential files; validate positive tick and rate-limit values; validate `tls.caBundleFile` exists when set; ensure credential values and credential file contents are never logged. Out of scope: per-cloud credentials and hot reload.

The task must update startup wiring so invalid config exits before any Kubernetes or NSX clients are created.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Unit tests cover credential precedence, file trimming, empty/missing files, invalid numeric values, missing CA bundle file, and no resolved credential source.
- [x] Startup integration evidence shows invalid config exits before client construction.
</acceptance_criteria>

<verification_evidence>
Implementation summary:
- Added `internal/config` with value-only validated runtime `Config`, private raw YAML DTOs, Basic Auth source precedence, credential file reads, one trailing newline/CRLF trim, positive tick/rate-limit validation, supported logging level validation, and CA bundle existence validation.
- Added `internal/startup.Run`, which calls `config.Load` before constructor hooks and returns before Kubernetes/NSX constructor hooks on invalid config.
- Added `cmd/nsx-operator` with `--config`, process environment mapping, zap JSONL logging to stderr, and non-zero exit on startup failure.

Focused tests:
- `go test ./internal/config` passed after adding behavior coverage for valid YAML, env credential precedence, env credential files and trimming, config credential files fallback, partial source rejection, missing/empty credential files without secret leakage, no credential source, invalid numeric values, and missing CA bundle.
- `go test ./internal/startup` passed with tests proving invalid config returns before Kubernetes/NSX constructor hooks and valid config invokes hooks in order with the loaded config.
- `go test ./cmd/nsx-operator` passed with command behavior tests for missing/invalid flags, invalid config exit, valid config exit, and environment mapping.

Manual startup verification:
Command run:
```bash
go run ./cmd/nsx-operator --config /tmp/tmp.Gff1dKbpx3/invalid.yaml 2>/tmp/tmp.Gff1dKbpx3/stderr.jsonl
```

Observed evidence:
```text
exit_status=1
{"level":"info","ts":1779141981.9947948,"msg":"loading startup config","config_path":"/tmp/tmp.Gff1dKbpx3/invalid.yaml"}
{"level":"info","ts":1779141981.9948685,"msg":"startup config validation failed","error":"operator.tickInterval must be positive"}
{"level":"info","ts":1779141981.994874,"msg":"startup failed","error":"load startup config: operator.tickInterval must be positive"}
exit status 1
secret_scan=clean
```

The invalid manual config contained `username: manual-user` and `password: super-secret-manual-password`; `rg 'manual-user|super-secret-manual-password' stderr.jsonl` found no matches.

Required checks:
```text
make check
0 issues.
go test ./... passed
go test -cover ./... passed
coverage: cmd/nsx-operator 84.6%, internal/buildinfo 100.0%, internal/config 82.9%, internal/startup 83.3%

make test
ok github.com/djosh34/nsx-operator/cmd/nsx-operator
ok github.com/djosh34/nsx-operator/internal/buildinfo
ok github.com/djosh34/nsx-operator/internal/config
ok github.com/djosh34/nsx-operator/internal/startup

make test-coverage
ok github.com/djosh34/nsx-operator/cmd/nsx-operator coverage: 84.6% of statements
ok github.com/djosh34/nsx-operator/internal/buildinfo coverage: 100.0% of statements
ok github.com/djosh34/nsx-operator/internal/config coverage: 82.9% of statements
ok github.com/djosh34/nsx-operator/internal/startup coverage: 83.3% of statements
```

Final `improve-code-boundaries` review:
- `internal/config` is the only owner of raw YAML parsing, validation, environment lookup, credential source precedence, and credential file reading.
- `internal/startup` owns ordering only and does not duplicate config validation.
- `cmd/nsx-operator` owns process concerns only: flags, environment capture, JSONL stderr logger, exit code.
- No Kubernetes, controller-runtime, NSX, HTTP transport, or rate-limiter dependencies were introduced in this foundational task.
</verification_evidence>

<plan>
.ralph/tasks/01-story-config-module/01-task-implement-config-module_plans/plan-20260518-config-module.md
</plan>

NOW EXECUTE

## Task: Implement Zap JSONL Logging <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/logging` and wire the operator to use zap structured logging in JSON lines to stderr. Debug logging should be available for detailed actions, while larger lifecycle actions should log at info. Logs must use structured fields instead of formatted plaintext context.

In scope: create a logger constructor honoring validated `logging.level`; emit JSONL to stderr; provide helpers or conventions for required fields `component`, `networkCloudFQDN`, `groupID`, `sweepID`, and `reconcileKey`; scrub or avoid plaintext credentials and credential file contents; migrate operator/client logs to zap where touched. Out of scope: external log aggregation.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Tests or captured process output prove logs are valid JSONL on stderr.
- [x] Verification proves credentials and credential file contents are not present in logs.
</acceptance_criteria>

<verification_evidence>
Required checks:

- `make check` passed:
  - `gofumpt -w .`
  - `golangci-lint run ./...`
  - `0 issues.`
  - `go test ./...` passed for `cmd/nsx-operator`, `internal/buildinfo`, `internal/config`, `internal/logging`, and `internal/startup`.
  - `go test -cover ./...` passed with package coverage: `cmd/nsx-operator` 81.6%, `internal/buildinfo` 100.0%, `internal/config` 82.9%, `internal/logging` 96.2%, `internal/startup` 82.8%.
- `make test` passed: `go test ./...` passed for all packages.
- `make test-coverage` passed: `go test -cover ./...` passed with all packages at or above 80%.

Focused behavior tests added:

- `internal/logging` verifies info/debug/warn/error level behavior, unsupported level errors, JSONL output to injected sinks, default stderr sink behavior, `NewStderr`, and required field helpers for `component`, `networkCloudFQDN`, `groupID`, `sweepID`, and `reconcileKey`.
- `internal/startup` verifies runtime logger factory usage after validated config, bootstrap logging for config validation failures, constructor short-circuiting, structured startup debug fields, and credential-source logging without username/password material.
- `cmd/nsx-operator` verifies valid config stderr is JSONL, invalid flag stderr is JSONL, sentinel credentials are absent from stderr, logger construction failures are handled, sync errors are reported, and env parsing behavior is preserved.

Manual/process verification:

Command run:

```bash
go run ./cmd/nsx-operator --config "$config" >"$stdout_file" 2>"$stderr_file"
```

The config used sentinel credentials:

- username: `manual-sentinel-user`
- password: `manual-sentinel-password`
- `logging.level: debug`

Captured evidence:

```text
status=0
stdout_bytes=0
stderr_lines=4
json_lines=4
secret_hits=
stderr_file=/tmp/tmp.E9XB9kXriI/stderr.jsonl
```

Parsed stderr JSONL summary:

```json
[
  {
    "level": "info",
    "msg": "loading startup config",
    "component": "startup",
    "credential_source": null,
    "logging_level": null
  },
  {
    "level": "debug",
    "msg": "loaded startup config",
    "component": "startup",
    "credential_source": "config_values",
    "logging_level": "debug"
  },
  {
    "level": "info",
    "msg": "startup completed",
    "component": "startup",
    "credential_source": null,
    "logging_level": null
  },
  {
    "level": "info",
    "msg": "operator process exiting",
    "component": "cmd",
    "credential_source": null,
    "logging_level": null
  }
]
```

Final `$improve-code-boundaries` review:

- Production `zapcore` usage is centralized in `internal/logging`.
- `cmd/nsx-operator` owns process concerns only: flags, environment capture, exit codes, and logger sync/reporting.
- `internal/startup` owns startup ordering and non-secret structured lifecycle logging only.
- The old command-local `newStderrJSONLogger` and command-local `stderrWriteSyncer` were removed.
- Startup logs credential source only; raw usernames, passwords, auth structs, and credential file contents are not logged.
</verification_evidence>

<plan>
.ralph/tasks/02-story-logging/01-task-implement-zap-jsonl-logging_plans/plan-20260519-zap-jsonl-logging.md
</plan>

NOW EXECUTE

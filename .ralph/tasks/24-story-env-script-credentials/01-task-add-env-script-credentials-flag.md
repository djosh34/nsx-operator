## Task: Add Env Script Credentials Flag <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add a simple `env-script` flag that lets the operator load NSX credentials from an executable script. The flag must be passed through normal CLI/config loading, inspect the target script's shebang (`#!`) to determine which script executor/interpreter to invoke, execute it with that interpreter, then extract only the needed environment variables for `NSX_USERNAME` and `NSX_PASSWORD`.

Keep the implementation stupid simple. Do not introduce a broad secret provider framework, templating system, shell-specific parser, or unrelated credential refactor. The implementation must read the `#!` line at the top of the file and use that shebang to determine what script executor to invoke. The operator must capture the script output in the simplest reliable format already used by the project or a minimal `KEY=VALUE` environment-style output, and must fail clearly if either `NSX_USERNAME` or `NSX_PASSWORD` is missing.

In scope: add the `env-script` flag; pass it through to config/credential loading; parse the script's shebang and execute the script with the indicated interpreter; extract `NSX_USERNAME` and `NSX_PASSWORD`; log larger actions at info and detailed behavior at debug using zap structured JSONL to stderr; add tests for success, missing variables, non-zero script exit, missing/invalid shebang, and shebang interpreter selection.

Out of scope: managing Kubernetes Secrets; adding encryption; changing credential precedence except as needed to make the new flag deterministic and documented in tests; supporting arbitrary shell snippets instead of an executable script path.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] A new `env-script` flag can be passed to the operator command.
- [x] The flag value is passed into config or credential loading without bypassing existing validation.
- [x] The configured script's `#!` shebang is read and used to choose the script executor/interpreter.
- [x] The implementation does not hardcode `bash script` or assume every env script is Bash.
- [x] `NSX_USERNAME` and `NSX_PASSWORD` are extracted from the script output or exported environment data.
- [x] Missing `NSX_USERNAME` or `NSX_PASSWORD` produces a clear error and does not continue with empty credentials.
- [x] Non-zero script exit and execution failures return errors that are logged and surfaced.
- [x] Tests cover the happy path, missing username, missing password, script execution failure, missing/invalid shebang, and shebang interpreter selection.
- [x] Relevant existing test gates pass and no errors are intentionally ignored.
</acceptance_criteria>

<verification_evidence>
Automated gates run on 2026-05-19:

- `make check` passed. Output included fmt/vet/lint success, `go test ./...`, race tests, mockapi contract tests, e2e tests, largechaos tests, and `coverage 83.6% meets 80.0% threshold`.
- `make test` passed with envtest-backed `go test ./...`.
- `make test-coverage` passed with `coverage 83.6% meets 80.0% threshold`.
- Changed package coverage from `make test-coverage`: `cmd/nsx-operator` 81.1%, `internal/config` 87.7%, `internal/startup` 85.2%.

Manual verification commands and observed evidence:

- Created a temporary fake interpreter script and credential script whose first line was `#!<tmp>/fake-interpreter.sh --format env`, then ran:

```bash
go run ./cmd/nsx-operator --config "$tmp/config.yaml" --env-script "$tmp/credentials.custom"
```

- The fake interpreter recorded argv proving shebang interpreter selection and optional args were used:

```text
--format
env
<tmp>/credentials.custom
```

- Relevant JSONL stderr showed config loading through the env script and the validated credential source:

```json
{"level":"info","msg":"loading nsx credentials from env script","component":"config","env_script_path":"<tmp>/credentials.custom","interpreter":"<tmp>/fake-interpreter.sh"}
{"level":"debug","msg":"loaded startup config","component":"startup","logging_level":"debug","credential_source":"env_script","nsx_writes_enabled":true,"operator_tick_interval":30,"operator_metrics_bind_address":":8080","http_max_requests_in_flight_per_host":8,"http_max_requests_per_second_per_host":20}
```

- The command later exited `1` while starting the real runtime manager because the temporary manual config did not point at a running NSX endpoint. That occurred after config loaded successfully and after `credential_source` was logged as `env_script`.
- A missing-password env script exited `1` before runtime construction and logged clear validation errors:

```json
{"level":"info","msg":"startup config validation failed","component":"startup","error":"env script \"<tmp>/missing-password.sh\" did not provide NSX_PASSWORD"}
{"level":"info","msg":"startup failed","component":"cmd","error":"load startup config: env script \"<tmp>/missing-password.sh\" did not provide NSX_PASSWORD"}
```

- A non-zero env script exited `1` before runtime construction and logged sanitized execution failure context:

```json
{"level":"info","msg":"env script credential command failed","component":"config","env_script_path":"<tmp>/nonzero.sh","interpreter":"/bin/sh","exit_code":7,"error":"exit status 7"}
{"level":"info","msg":"startup config validation failed","component":"startup","error":"execute env script \"<tmp>/nonzero.sh\" with interpreter \"/bin/sh\": exit status 7"}
```

- Manual sentinel credential strings such as `manual-script-user`, `manual-script-password`, `manual-missing-user`, `manual-stdout-user`, `manual-stdout-password`, `manual-stderr-secret`, `lower-priority-user`, and `lower-priority-password` were searched in the relevant stderr output and did not appear.
</verification_evidence>

<plan>
.ralph/tasks/24-story-env-script-credentials/01-task-add-env-script-credentials-flag_plans/01-env-script-credentials-plan.md
</plan>

NOW EXECUTE

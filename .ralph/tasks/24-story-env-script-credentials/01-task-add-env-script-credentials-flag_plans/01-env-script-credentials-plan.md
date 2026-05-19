## Plan: Env Script Credentials Flag

Task: `.ralph/tasks/24-story-env-script-credentials/01-task-add-env-script-credentials-flag.md`

### Current Shape

- `cmd/nsx-operator/main.go` owns process-level concerns: flags, environment capture, stderr JSONL logger setup, signal context, and exit codes.
- `internal/startup.Run` owns startup ordering and logging. It calls `config.Load` before runtime construction and logs the resolved credential source at debug level without logging credential material.
- `internal/config.Load` owns raw YAML parsing, validation, environment lookup, credential source precedence, credential file reads, and the validated immutable `config.Config`.
- `internal/config.resolveBasicAuth` is the single credential-resolution boundary today. Existing precedence is env values, env files, config values, then config files.
- Tests already cover config behavior through public `config.Load`, command behavior through `run`, and startup logging through `startup.Run`.

### Interface And Boundary Design

Keep the implementation deliberately small and keep validation inside `internal/config`.

- Add `EnvScriptPath string` to `config.Options`.
- Add `CredentialSourceEnvScript CredentialSource = "env_script"`.
- Add `--env-script` to `cmd/nsx-operator`, and pass its value to `startup.Options.Config.EnvScriptPath`.
- Do not add a secret-provider framework, template engine, shell parser, or broad credential abstraction.
- Treat `--env-script` as an explicit selected credential source. If it is set, run it before considering `NSX_USERNAME`, `NSX_PASSWORD`, `*_FILE`, or YAML credentials. If the script fails or omits either required variable, return a clear error and do not fall back to lower-priority credentials.
- Preserve the existing public `config.Load(config.Options)` interface as the only config entry point for tests and startup.

This applies the `improve-code-boundaries` skill by keeping:

- flag parsing in `cmd/nsx-operator`;
- credential validation and resolution in `internal/config`;
- startup sequencing in `internal/startup`;
- script execution as a private credential-resolution detail, not a new cross-package framework.

### Env Script Behavior

- Read the target script file and inspect only the first line.
- Require the first line to start with `#!`.
- Trim the shebang and parse the interpreter plus optional interpreter arguments with simple whitespace splitting.
- Fail clearly when the shebang is missing, empty, or has no interpreter path.
- Execute the interpreter selected by the shebang as:

```text
<interpreter> <shebang args...> <script path>
```

- Capture stdout and stderr. Do not log stdout/stderr directly because either may contain secrets. Include script path, interpreter path, exit status, and sanitized error context only.
- Parse stdout as minimal environment-style lines:

```text
NSX_USERNAME=value
NSX_PASSWORD=value
```

- Ignore unrelated keys. Ignore blank lines.
- Reject malformed nonblank lines if needed for clear failure, but keep the parser simple.
- Extract only `NSX_USERNAME` and `NSX_PASSWORD`.
- Missing or empty `NSX_USERNAME` returns a clear `env script did not provide NSX_USERNAME` style error.
- Missing or empty `NSX_PASSWORD` returns a clear `env script did not provide NSX_PASSWORD` style error.
- Do not log or include credential values in errors.

### Logging Plan

- Use the existing zap JSONL stderr path. Do not introduce another logger.
- Add an optional `Logger *zap.Logger` to `config.Options` only if implementation needs direct config-layer logs for this task. If used, default nil to `zap.NewNop()`.
- Log larger script-resolution actions at info, for example `loading nsx credentials from env script`, with structured fields like `component=config`, `env_script_path`, and `interpreter`.
- Log detailed behavior at debug, for example shebang parsing and extracted key presence booleans. Never log `NSX_USERNAME`, `NSX_PASSWORD`, stdout, or stderr.
- If adding a logger to `config.Options` makes startup wiring muddy or spreads logging concerns too far into config tests, keep logging in startup/command only and document that decision during execution.

### TDD Execution Plan

Use vertical slices. Do not write all tests first.

- [x] RED: add a `cmd/nsx-operator` behavior test proving `run([]string{"--config", configPath, "--env-script", scriptPath})` succeeds and the runtime manager receives `CredentialSourceEnvScript` credentials from script output.
- [x] GREEN: add the `--env-script` flag, add `EnvScriptPath` to `config.Options`, add the credential source constant, and implement the smallest happy-path script resolver.
- [x] RED: add a `config.Load` behavior test proving missing `NSX_USERNAME` from script output fails clearly and does not fall back to config credentials.
- [x] GREEN: validate required username and keep selected-source failure terminal.
- [x] RED: add a `config.Load` behavior test proving missing `NSX_PASSWORD` from script output fails clearly and does not leak username/password values.
- [x] GREEN: validate required password and sanitize errors.
- [x] RED: add a `config.Load` behavior test proving a non-zero script exit returns a surfaced error without leaking stdout/stderr credential material.
- [x] GREEN: handle `exec.ExitError` and command execution failures with sanitized wrapping.
- [x] RED: add `config.Load` behavior tests for missing shebang and empty/invalid shebang.
- [x] GREEN: read and validate the first line before execution.
- [x] RED: add a `config.Load` behavior test for shebang interpreter selection using a temporary fake interpreter script that records its argv and emits credentials, then a credential script whose shebang points to that fake interpreter with an optional argument.
- [x] GREEN: execute exactly the shebang-selected interpreter plus optional args plus script path.
- [x] RED: add or extend command stderr/logging behavior tests to prove valid and failing env-script runs still emit JSONL and do not leak credential values.
- [x] GREEN: wire any config-layer structured logs through startup if needed, keeping values redacted.
- [x] REFACTOR: apply `improve-code-boundaries` review. Keep one parse function for env output, one shebang execution function, no broad provider interfaces, and no validation outside `internal/config`.

### Concrete Implementation Notes

- Prefer a private helper such as `resolveEnvScriptBasicAuth(path string) (BasicAuth, error)` or a tiny private `envScriptCredentials(path string) (username string, password string, err error)` inside `internal/config`.
- Use `os.ReadFile` or the existing `readFile` helper for shebang inspection. Because script execution needs a real executable path, do not pretend `fs.FS` can execute scripts.
- Continue using `readFile` for credential files; do not disturb existing file-credential behavior.
- Use `exec.Command` with explicit args. Do not invoke `bash`, `sh`, or shell snippets unless that interpreter is present in the script shebang.
- Check and propagate every error. Do not use `_ := ...` for errors.
- Use `strings.Cut(line, "=")` for `KEY=VALUE` parsing and preserve spaces in values after the `=`.
- Decide during implementation whether duplicate keys use last value to match `environMap` behavior; document and test if added. Keep scope minimal unless required.
- Be careful with `flag` package argument syntax: tests should cover both normal `--env-script path` and invalid flag behavior indirectly through existing tests.

### Verification Plan

Required automated gates before completion:

- [x] `go test ./internal/config ./cmd/nsx-operator ./internal/startup`
- [x] focused new-code coverage check for `internal/config` and `cmd/nsx-operator` if package coverage is near threshold
- [x] `make check`
- [x] `make test`
- [x] `make test-coverage`

Required manual verification evidence to record in the task:

- [x] create a temporary env script with a non-Bash shebang or fake interpreter;
- [x] run `go run ./cmd/nsx-operator --config <config> --env-script <script>` using a config that otherwise contains lower-priority credentials;
- [x] show exit status and sanitized JSONL stderr;
- [x] show that the selected credential source is `env_script` through debug logs or test/runtime-manager evidence;
- [x] show missing-variable and non-zero-script commands fail without credential leakage.

If implementation proves the selected precedence, `config.Options` shape, logging placement, shebang parsing, or credential output format is wrong, replace the final marker with `TO BE VERIFIED`, document the proposed design change here, and quit immediately.

NOW EXECUTE

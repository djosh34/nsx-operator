# Plan: Implement Immutable Startup Config Module

Task: `.ralph/tasks/01-story-config-module/01-task-implement-config-module.md`

## Current State

- Repository has a Go module, Makefile, and one tiny `internal/buildinfo` package.
- There is no `internal/config`, `cmd/`, controller-runtime manager, Kubernetes client, NSX client, or logging package yet.
- This task is the first runtime feature. The public interface must be small and stable enough for later stories to consume without re-parsing config or duplicating credential logic.
- Because no real client construction exists yet, startup ordering evidence should be provided by a narrow startup orchestration boundary with injected constructor functions. That lets tests and manual commands prove invalid config stops before client hooks are called, without adding fake Kubernetes/NSX implementations.

## Interface Design

Add `internal/config` as the only owner of runtime config parsing, validation, and credential resolution.

Public types:

- `type Config struct`
  - `Operator OperatorConfig`
  - `HTTPRateLimiter HTTPRateLimiterConfig`
  - `NSX NSXConfig`
  - `Logging LoggingConfig`
- `type OperatorConfig struct`
  - `TickInterval time.Duration`
- `type HTTPRateLimiterConfig struct`
  - `MaxRequestsInFlightPerHost int`
  - `MaxRequestsPerSecondPerHost int`
- `type NSXConfig struct`
  - `Auth BasicAuth`
  - `TLS TLSConfig`
- `type BasicAuth struct`
  - `Username string`
  - `Password string`
  - `Source CredentialSource`
- `type CredentialSource string`
  - `CredentialSourceEnv`
  - `CredentialSourceEnvFiles`
  - `CredentialSourceConfigValues`
  - `CredentialSourceConfigFiles`
- `type TLSConfig struct`
  - `CABundleFile string`
- `type LoggingConfig struct`
  - `Level string`
- `type Options struct`
  - `Path string`
  - `Environ map[string]string`
  - `FS fs.FS`
- `func Load(options Options) (Config, error)`

Parsing rules:

- YAML shape:

```yaml
operator:
  tickInterval: 30s
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: 20
nsx:
  auth:
    username: user
    password: pass
    usernameFile: /path/to/username
    passwordFile: /path/to/password
  tls:
    caBundleFile: /path/to/ca.pem
logging:
  level: info
```

- Use `time.ParseDuration` for `operator.tickInterval` and reject missing, zero, or negative values.
- Reject zero or negative `httpRateLimiter.maxRequestsInFlightPerHost` and `httpRateLimiter.maxRequestsPerSecondPerHost`.
- Preserve `logging.level` as a validated non-empty string for the later logging task. Accept at least `debug`, `info`, `warn`, and `error`.
- Resolve exactly one complete Basic Auth source by precedence:
  - `NSX_USERNAME` and `NSX_PASSWORD`
  - `NSX_USERNAME_FILE` and `NSX_PASSWORD_FILE`
  - YAML `nsx.auth.username` and `nsx.auth.password`
  - YAML `nsx.auth.usernameFile` and `nsx.auth.passwordFile`
- A source only qualifies when both fields for that source are set. Reject partial higher-priority or selected sources with a clear redacted error.
- Read credential files once during `Load`, trim exactly one trailing newline sequence (`\n` or `\r\n`) from each credential value, and reject missing or empty files after that trim.
- Validate `nsx.tls.caBundleFile` exists when set. Do not parse certificate contents in this task unless a later task needs it.
- Never include credential values or credential file contents in error strings. File paths may appear in errors because they are configuration locations, not secret contents.
- Return a value-only `Config` with unexported raw YAML types hidden inside the package. The returned struct is effectively immutable by convention: no pointers, no setters, no reload API, and startup passes the same value to later components.

Add a narrow startup boundary without implementing future clients:

- `internal/startup` owns ordering.
- Public types:
  - `type RuntimeConstructors struct { Kubernetes func(config.Config) error; NSX func(config.Config) error }`
  - `type Options struct { Config config.Options; Constructors RuntimeConstructors; Logger *zap.Logger }`
  - `func Run(options Options) error`
- `Run` calls `config.Load` first. If it fails, it logs a redacted info/error-level startup failure and returns before any constructor function is invoked.
- On valid config, `Run` invokes the constructor hooks in the intended future order. The hooks can be no-op placeholders until later client stories replace them.
- Add `cmd/nsx-operator/main.go` as the executable startup entry point:
  - parse `--config` using the standard `flag` package;
  - create a zap JSON encoder to stderr directly in main/startup for now, because the dedicated logging package is a later story;
  - call `startup.Run`;
  - exit non-zero on error.

## TDD Plan

Use the `$tdd` skill with vertical red-green slices. Tests must exercise public interfaces and behavior, not private helper shape.

- [x] RED: `internal/config.Load` accepts a minimal valid YAML file with config credentials and returns typed durations, rate limits, TLS path, logging level, and resolved Basic Auth source. GREEN: add raw YAML parsing, public config structs, and basic validation.
- [x] RED: environment variables `NSX_USERNAME` and `NSX_PASSWORD` override all lower credential sources. GREEN: add credential source precedence for env values.
- [x] RED: env credential files override config values, read files once, and trim one trailing newline without trimming other intentional characters. GREEN: add file credential resolution using the injected `fs.FS`.
- [x] RED: empty and missing credential files are rejected with errors that do not contain secret contents. GREEN: harden file reading and redacted errors.
- [x] RED: config username/password are used only when env value and env-file sources are absent. GREEN: complete config value source selection.
- [x] RED: config usernameFile/passwordFile are used as the final fallback. GREEN: complete config file source selection.
- [x] RED: no resolved credential source returns a redacted validation error. GREEN: add missing-source validation.
- [x] RED: invalid numeric values and invalid or missing tick interval are rejected. GREEN: complete numeric validation.
- [x] RED: missing `tls.caBundleFile` is rejected when set. GREEN: add CA bundle existence check.
- [x] RED: selected partial credential sources, such as only `NSX_USERNAME`, are rejected rather than silently falling through to lower-priority credentials. GREEN: add partial-source validation.
- [x] RED: startup with invalid config returns before Kubernetes and NSX constructor hooks are called. GREEN: implement `internal/startup.Run`.
- [x] RED: startup with valid config calls constructor hooks after config load. GREEN: complete startup happy path.
- [x] RED/verification: run the compiled `cmd/nsx-operator` with an invalid config and capture JSONL stderr plus exit code as manual evidence. GREEN: wire main to `startup.Run` and zap JSONL stderr.

Per-cycle rules:

- Run the focused package test after each red-green slice.
- Never add a batch of tests ahead of implementation.
- Keep tests behavior-oriented: they should call `config.Load` or `startup.Run`, not private parsing functions.
- Do not add brittle tests that inspect source text, Makefile text, or log formatting internals.

## improve-code-boundaries Review

Use the `$improve-code-boundaries` skill during planning and before completion.

Boundary decisions:

- `internal/config` owns raw YAML DTOs, environment lookup, file reads, validation, and credential precedence. No future controller, client, or runtime package should re-parse YAML, re-read credential files, or revalidate runtime knobs.
- Public `Config` is the deep module boundary. It should expose already-validated Go types, not raw stringly YAML fields except where the value is inherently a string such as `logging.level` and `tls.caBundleFile`.
- Credential source enums prevent stringly source checks in later code and provide useful non-secret diagnostics.
- `internal/startup` owns sequencing only. It must not duplicate validation rules and must not know raw YAML field names.
- `cmd/nsx-operator` owns process concerns only: flags, stderr logger construction, and exit code.
- Avoid adding Kubernetes, controller-runtime, NSX, HTTP transport, or rate limiter dependencies in this task. Later stories should replace constructor hooks with real modules without changing `internal/config`.

Potential boundary cleanup after implementation:

- If startup constructor hooks feel like fake production API, keep them internal to `startup` tests or make them explicitly named `ClientConstructors` so they document the future boundary.
- If config validation starts accumulating unrelated concerns, split raw parsing/resolution into unexported files inside `internal/config`, not separate packages.
- If logging bootstrap grows beyond minimal zap-to-stderr setup, stop and leave deeper logger construction to story 02.

## Verification Plan

Record concrete evidence in the task file:

- `make check` success output.
- `make test` success output.
- `make test-coverage` success output, showing total package coverage is 80%+ and new packages are 80%+.
- Focused config/startup test commands run during TDD where useful.
- Manual startup evidence:
  - create a temporary invalid YAML config with a bad value such as `operator.tickInterval: 0s`;
  - run `go run ./cmd/nsx-operator --config <file>`;
  - record non-zero exit status;
  - record JSONL stderr showing redacted config validation failure;
  - record that startup constructor evidence from tests proves no Kubernetes or NSX constructor is called on invalid config.
- Secret-safety evidence:
  - tests include a credential-file error case with a sentinel secret value and assert the error string does not contain it;
  - manual logs do not include username/password values or credential file contents.

## Completion Steps

After execution:

- [x] Run `make check`.
- [x] Run `make test`.
- [x] Run `make test-coverage`.
- [x] Ensure coverage for new Go code and overall `make test-coverage` are 80%+.
- [x] Perform final `$improve-code-boundaries` review and refactor if the implementation is muddy.
- [x] Update task acceptance criteria and verification evidence.
- [x] Set `<passes>true</passes>` in the task file only after all checks pass.
- [ ] Run `/bin/bash .ralph/task_switch.sh`.
- [ ] Add all changed files, including `.ralph` files.
- [ ] Commit with message starting `task finished 01-task-implement-config-module: ...`.
- [ ] Push.

NOW EXECUTE

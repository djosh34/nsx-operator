# Plan: Implement Zap JSONL Logging

Task: `.ralph/tasks/02-story-logging/01-task-implement-zap-jsonl-logging.md`

## Current State

- `cmd/nsx-operator/main.go` currently constructs a zap JSON encoder and stderr write syncer directly in the command package.
- `internal/startup.Run` accepts a `*zap.Logger`, defaults to `zap.NewNop()`, and logs a few lifecycle messages.
- `internal/config.Load` already validates `logging.level` as one of `debug`, `info`, `warn`, or `error`.
- There is no `internal/logging` package yet, so logging construction, output destination, level parsing, and field-key conventions do not have a stable home.
- Credential resolution already avoids putting credential values in config errors. The logging task must preserve that property when logging startup failures and loaded config details.

## Interface Design

Add `internal/logging` as the only owner of zap JSONL logger construction and required structured field keys.

Public package surface:

- `type Options struct`
  - `Level string`
  - `Sink zapcore.WriteSyncer`
- `func New(options Options) (*zap.Logger, error)`
- `func NewStderr(level string) (*zap.Logger, error)`
- Field helper functions returning `zap.Field`:
  - `func Component(value string) zap.Field`
  - `func NetworkCloudFQDN(value string) zap.Field`
  - `func GroupID(value string) zap.Field`
  - `func SweepID(value string) zap.Field`
  - `func ReconcileKey(value string) zap.Field`

Logger construction rules:

- Use `zap.NewProductionEncoderConfig()` with `zapcore.NewJSONEncoder`.
- Write one JSON object per log line to the provided sink, and to `os.Stderr` for `NewStderr`.
- Honor the validated `logging.level` exactly:
  - `debug` enables debug, info, warn, and error;
  - `info` enables info, warn, and error;
  - `warn` enables warn and error;
  - `error` enables error only.
- Return an explicit error for unsupported levels even though config validation should normally prevent that path.
- Do not log plaintext credentials, credential file contents, or raw auth structs from this package.
- Keep the package deep: callers get a `*zap.Logger` and field helpers, not encoder/core/write-syncer internals.

Wire startup around validated config:

- Keep `internal/startup` responsible for startup ordering, not zap internals.
- Replace direct logger injection with a logger factory boundary:
  - `type LoggerFactory func(config.LoggingConfig) (*zap.Logger, error)`
  - `Options.LoggerFactory LoggerFactory`
  - `Options.BootstrapLogger *zap.Logger`
- `Run` uses `BootstrapLogger` only before validated config exists and for config-load failures. If it is nil, use `zap.NewNop()`.
- After `config.Load` succeeds, call `LoggerFactory(loadedConfig.Logging)`. If no factory is provided, use `logging.NewStderr`.
- Use the runtime logger for subsequent lifecycle logs and constructor logs.
- Log larger lifecycle actions at info:
  - loading startup config;
  - config validation failure;
  - logger construction failure;
  - constructing Kubernetes clients;
  - constructing NSX clients;
  - startup completed.
- Log detailed non-secret action context at debug:
  - loaded logging level;
  - credential source, never username/password;
  - tick interval and rate-limiter numbers;
  - constructor start/success details.
- Always use structured fields. Do not use formatted plaintext context in messages.
- Do not ignore `logger.Sync()` errors in command shutdown. The existing `stderrWriteSyncer.Sync` no-op should move into `internal/logging` or become unnecessary there.

Wire command process concerns:

- `cmd/nsx-operator` should own flags, environment capture, exit codes, and final logger sync behavior.
- It should not own zap encoder/core construction after this task.
- For missing or invalid flags, create an info-level stderr JSONL bootstrap logger through `internal/logging` where possible and log structured errors to stderr.
- For valid config paths, call `startup.Run` with:
  - `config.Options{Path: *configPath, Environ: environMap(os.Environ())}`;
  - `BootstrapLogger` from `logging.NewStderr("info")`;
  - `LoggerFactory` that calls `logging.NewStderr(loadedLogging.Level)`.
- Ensure all command paths flush the logger and report sync errors to stderr if sync fails.

## TDD Plan

Use the `$tdd` skill with vertical red-green slices. Each test should prove behavior through a public package boundary and each implementation step should be just enough to make that new behavior pass.

- [x] RED: `internal/logging.New` with `Level: "info"` writes info logs as valid JSONL to the provided sink and suppresses debug logs. GREEN: add `internal/logging` with `Options`, level parsing, JSON encoder, and sink use.
- [x] RED: `internal/logging.New` with `Level: "debug"` emits debug logs as JSONL. GREEN: complete zap level handling.
- [x] RED: unsupported logging levels return an error from `internal/logging.New` without writing logs. GREEN: add defensive validation in the logging package.
- [x] RED: logging field helpers produce the required JSON keys `component`, `networkCloudFQDN`, `groupID`, `sweepID`, and `reconcileKey` when used with a public logger. GREEN: add field helpers as the convention boundary.
- [x] RED: `startup.Run` with valid config and `logging.level: debug` uses the configured runtime logger factory so debug startup details are emitted. GREEN: add `LoggerFactory`, create runtime logger after `config.Load`, and add non-secret debug fields.
- [x] RED: `startup.Run` with invalid config logs a structured startup validation failure through the bootstrap logger and does not call Kubernetes or NSX constructors. GREEN: keep bootstrap logging before validated config and preserve constructor short-circuiting.
- [x] RED: `startup.Run` logs credential source but never logs sentinel username/password values or credential file contents. GREEN: audit startup fields and avoid auth value logging.
- [x] RED: `cmd/nsx-operator` process output on stderr is valid JSONL for a valid config. GREEN: move command logger construction to `internal/logging` and wire startup options.
- [x] RED: command/process evidence for a config containing sentinel credentials proves logs do not contain those sentinel values. GREEN: fix any command or startup logs that leak raw config/auth values.

Per-cycle rules:

- Run focused tests for the package under change after each red-green slice.
- Do not batch all tests ahead of implementation.
- Keep tests behavior-oriented: parse emitted log lines as JSON and assert public effects, not zap internals.
- Do not assert brittle source text, Makefile text, or private helper names.
- Never ignore errors. Do not write `_ := ...` for returned errors.

## improve-code-boundaries Review

Use the `$improve-code-boundaries` skill during planning and before completion.

Boundary decisions:

- Move zap encoder/core/stderr write sync knowledge out of `cmd/nsx-operator` into `internal/logging`. This fixes the current wrong-place smell.
- Keep raw YAML validation in `internal/config`; `internal/logging` accepts only the reduced, validated level string or fails defensively when called incorrectly.
- Keep startup as sequencing logic. It should not know zapcore details and should not duplicate config validation.
- Keep `cmd/nsx-operator` as process bootstrap only: flags, environment map, exit code, and sync/reporting.
- Avoid introducing a custom logger interface unless tests force it. A concrete `*zap.Logger` plus an injectable factory is a smaller boundary and matches existing code.
- Avoid wrapping every field in a context object for this foundational task. Field helper functions are enough to establish stable JSON keys without overabstracting future controller/client logs.

Potential cleanup after implementation:

- Delete the command-local `newStderrJSONLogger` and `stderrWriteSyncer` once `internal/logging.NewStderr` exists.
- If `startup.Options` becomes too broad, split process-only logger sync behavior back into `cmd` and keep startup with only `BootstrapLogger` plus `LoggerFactory`.
- If tests need to capture command stderr, prefer executing the built command or temporarily redirecting `os.Stderr` in a helper rather than exposing production-only hooks.

## Verification Plan

Record concrete evidence in the task file:

- `make check` success output.
- `make test` success output.
- `make test-coverage` success output showing all new packages and total package coverage are at least 80%.
- Focused TDD command output where useful, especially for `internal/logging`, `internal/startup`, and `cmd/nsx-operator`.
- Manual/process evidence:
  - run `go run ./cmd/nsx-operator --config <valid config>` and capture stderr;
  - show each stderr line parses as JSON;
  - show level behavior by comparing `logging.level: debug` versus `logging.level: info` if needed;
  - show sentinel credentials and credential file contents are absent from captured logs.
- Final `$improve-code-boundaries` review:
  - command package no longer owns zap internals;
  - logging package owns JSONL stderr construction and field-key conventions;
  - startup owns ordering and non-secret structured lifecycle logging only.

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
- [ ] Commit with message starting `task finished 01-task-implement-zap-jsonl-logging: ...`.
- [ ] Push.

NOW EXECUTE

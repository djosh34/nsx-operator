## Plan: CLI Flag Help Errors

Task: `.ralph/tasks/25-story-cli-flag-help-errors/01-task-print-help-for-bad-flags.md`

Plan path: `.ralph/tasks/25-story-cli-flag-help-errors/01-task-print-help-for-bad-flags_plans/01-cli-flag-help-errors-plan.md`

### Current Shape

- `cmd/nsx-operator/main.go` owns process-level concerns: bootstrap logger construction, flag parsing, required `--config` validation, signal context, startup execution, logger sync, and process exit code selection.
- The command uses Go's standard `flag.FlagSet`, not Cobra or another CLI framework.
- The flag set is constructed inside `run(args []string)` with `flag.ContinueOnError`.
- `flagSet.SetOutput(io.Discard)` currently suppresses the standard flag package parse error and usage output.
- `--config` is manually required after parsing. Missing `--config` currently returns exit code `2` and logs `config path is required`, but it does not print usage.
- Existing command tests call `run` directly, capture real `os.Stderr`, verify exit codes, and parse JSONL logs. This is the right public boundary for this feature.

### Interface And Boundary Design

Keep the behavior at the command boundary in `cmd/nsx-operator`; do not add a new CLI abstraction or move flag parsing into config/startup.

- Add a small private command usage/error path in `cmd/nsx-operator/main.go`.
- Keep `run(args []string) int` as the public test seam for command behavior.
- Keep the existing flags and their names:
  - `--config`
  - `--env-script`
- Print usage/help to stderr for:
  - unknown flags and parse errors returned by `flagSet.Parse`;
  - wrong flag value formatting if a future typed flag parse returns an error;
  - missing required `--config`.
- Preserve non-zero usage exit code `2` for flag parse and required flag validation failures.
- Preserve the original error message alongside help text.
- Preserve successful command behavior: no usage/help should be printed for valid arguments unless a later startup/runtime error happens.
- Keep zap JSONL startup logging for usage failures through the existing bootstrap logger. Help text is user-facing stderr, so tests that parse JSONL must isolate JSON lines or only parse stderr for cases where help is not expected.

### Help Text Shape

Use the standard flag package's help output rather than building a custom documentation system.

- Set the `FlagSet` output to `os.Stderr` or a captured writer at the point usage is printed.
- Define `flagSet.Usage` only if needed to make the command name and available flags clear. Prefer the standard form:

```text
Usage of nsx-operator:
  -config string
        path to operator config YAML
  -env-script string
        path to executable script that prints NSX credentials
```

- For parse errors, ensure the original parse error remains visible. The standard flag package already prints errors such as `flag provided but not defined: -not-a-real-flag` when output is not discarded; if manual printing is needed, print `parse flags: <err>` before usage.
- For missing `--config`, print `config path is required` before usage and keep the structured log error as `config path is required`.

### Improve-Code-Boundaries Review

Required skill use: `$improve-code-boundaries`.

Boundary choice:

- Keep flag parsing and usage rendering in `cmd/nsx-operator/main.go`, because only the command layer knows CLI flags and process stderr behavior.
- Do not push usage/help behavior into `internal/config` or `internal/startup`; those packages should not know command-line flag syntax.
- Avoid a new `CLI` type or helper package. A private helper is acceptable only if it removes duplication between parse-error and missing-required-flag handling.
- If a helper is added, keep it narrow, for example `printUsageError(flagSet *flag.FlagSet, err error)` or `failUsage(bootstrapLogger *zap.Logger, flagSet *flag.FlagSet, err error) int`. Do not create an options DTO that just mirrors local variables.
- After implementation, re-check that command error handling is simpler rather than split across several small one-off helpers.

### TDD Execution Plan

Required skill use: `$tdd`.

Use vertical red-green slices. Do not write all command usage tests first.

1. [x] RED: add one `cmd/nsx-operator` behavior test proving an unknown flag exits `2`, stderr includes the original unknown flag error, and stderr includes help text listing `-config` and `-env-script`.
   - Run: `go test ./cmd/nsx-operator -run TestRunPrintsHelpForUnknownFlag -count=1`.
   - Expected RED reason: current code discards flag package output and only emits JSONL.
2. [x] GREEN: change the parse-error path to print the original parse error and standard usage/help to stderr while keeping the existing structured `startup failed` log and logger sync behavior.
   - Run the focused test until it passes.
3. [x] RED: add one behavior test proving missing `--config` exits `2`, stderr includes `config path is required`, and stderr includes the same help text.
   - Run: `go test ./cmd/nsx-operator -run TestRunPrintsHelpWhenConfigFlagIsMissing -count=1`.
   - Expected RED reason: missing required flag currently logs JSONL only.
4. [x] GREEN: change the missing-config branch to print the required-flag error plus usage/help before syncing the logger.
   - Run the focused test until it passes.
5. [x] RED: add one behavior test for an invalid flag value format. Because current flags are string flags, introduce the test by passing the existing standard-library wrong shape that is observable today, for example `--config` without a value. Assert exit `2`, original error text such as `flag needs an argument`, and help text.
   - Run: `go test ./cmd/nsx-operator -run TestRunPrintsHelpForMissingFlagValue -count=1`.
6. [x] GREEN: reuse the parse-error path; no additional production behavior should be needed if parse errors share one boundary.
7. [x] RED: add or update a success-path behavior test proving a valid invocation with `--config` and a fake successful runtime exits `0` and does not include `Usage of nsx-operator` in stderr.
   - Run: `go test ./cmd/nsx-operator -run TestRunDoesNotPrintHelpForValidConfig -count=1`.
8. [x] GREEN: keep usage printing only in parse/required flag failure branches.
9. [x] REFACTOR: apply `improve-code-boundaries` review.
   - Collapse duplicate usage-error branches if they now repeat logger sync or help printing.
   - Keep tests behavior-focused through `run` and stderr/exit code, not private helpers.
   - Avoid making fields/functions exported only for tests.

### Test Helper Notes

- Existing `parseCommandLogs` expects every non-empty stderr line to be JSON. Usage-error stderr will intentionally include non-JSON help lines.
- Prefer adding a small test helper that extracts JSON-looking lines before calling `parseCommandLogs`, or keep usage-help assertions separate from JSONL parsing for these tests.
- Do not assert the entire help output byte-for-byte. Assert stable user-facing behavior:
  - command usage header;
  - presence of `-config`;
  - presence of `-env-script`;
  - presence of the original error message;
  - exit code `2`.
- Do not add brittle tests that only inspect source text, flag names in files, or private helper output.

### Manual Verification Plan

Record concrete evidence in the task file after implementation:

- Build or run the command with an unknown flag:

```bash
go run ./cmd/nsx-operator --not-a-real-flag
```

- Run the command with missing required `--config`:

```bash
go run ./cmd/nsx-operator
```

- Run the command with a malformed flag value shape:

```bash
go run ./cmd/nsx-operator --config
```

- Run a successful command path with a temporary valid config and fake runtime through tests, and record that help text is absent.
- Capture exit statuses and representative stderr snippets showing both the original error and help text.
- Keep JSONL logging evidence for startup failure where applicable.

### Required Final Verification

Only after execution is complete:

- [x] `go test ./cmd/nsx-operator -count=1`
- [x] `make check`
- [x] `make test`
- [x] `make test-coverage`
- [x] Confirm total coverage remains at least 80%.
- [x] Confirm the changed `cmd/nsx-operator` code has sufficient new behavior coverage, targeting at least 80% package coverage.
- [x] Run a final `improve-code-boundaries` pass on the command code and remove any unnecessary helper/type split introduced during implementation.

### Files Expected To Change During Execution

- `cmd/nsx-operator/main.go`
- `cmd/nsx-operator/main_test.go`
- `.ralph/tasks/25-story-cli-flag-help-errors/01-task-print-help-for-bad-flags.md`
- `.ralph/tasks/25-story-cli-flag-help-errors/01-task-print-help-for-bad-flags_plans/01-cli-flag-help-errors-plan.md`

If implementation proves the standard flag usage shape, command boundary, test interface, or stderr/log ordering design is wrong, replace the final marker with `TO BE VERIFIED`, document the proposed design change here, and quit immediately.

NOW EXECUTE

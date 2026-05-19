## Task: Print Help For Bad Or Missing Flags <status>not_started</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Make CLI startup errors more helpful when users pass wrong flags or omit required flags. On bad or missing flag errors, the command must say how to use it by always printing the command help output.

The desired behavior is intentionally simple: when flag parsing or required-flag validation fails, surface the actual error and print the same help text a user would get from the `help` command. Do not replace the existing CLI library or create a custom documentation system. Keep stderr/stdout behavior consistent with the CLI framework already in use.

In scope: detect wrong flag values, unknown flags, and missing required flags; print command help on those failures; preserve non-zero exit behavior; keep useful structured zap logging for startup/debug details where the app logger is available; add tests or command-level verification for each failure mode.

Out of scope: redesigning command names; adding interactive prompts; changing successful command output; suppressing the underlying parse/validation error.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Unknown flags return a non-zero error and print command help.
- [x] Wrongly formatted or invalid flag values return a non-zero error and print command help.
- [x] Missing required flags return a non-zero error and print command help.
- [x] The original error message remains visible alongside the help text.
- [x] Successful invocations do not print help unexpectedly.
- [x] Tests or scripted verification capture stderr/stdout and exit status for the relevant failure cases.
- [x] Relevant existing test gates pass and no errors are intentionally ignored.
</acceptance_criteria>

<plan>
.ralph/tasks/25-story-cli-flag-help-errors/01-task-print-help-for-bad-flags_plans/01-cli-flag-help-errors-plan.md
</plan>

<verification_evidence>
Automated gates:

- `go test ./cmd/nsx-operator -count=1`: passed.
- `make check`: passed, including fmt, vet, lint, normal tests, race tests, mockapi-backed contract/e2e/large-chaos targets, and coverage.
- `make test`: passed.
- `make test-coverage`: passed with total coverage `83.7%` and `cmd/nsx-operator` package coverage `82.0%`.

Manual CLI verification used a built binary so the shell status reflected the program exit code directly:

```bash
tmpdir=$(mktemp -d)
binary="$tmpdir/nsx-operator"
go build -o "$binary" ./cmd/nsx-operator
"$binary" --not-a-real-flag
"$binary"
"$binary" --config
```

Observed unknown flag result:

```text
status=2
stdout_bytes=0
flag provided but not defined: -not-a-real-flag
Usage of nsx-operator:
  -config string
        path to operator config YAML
  -env-script string
        path to executable script that prints NSX credentials
{"level":"info","ts":1779221124.5393822,"msg":"startup failed","component":"cmd","error":"parse flags: flag provided but not defined: -not-a-real-flag"}
```

Observed missing config result:

```text
status=2
stdout_bytes=0
config path is required
Usage of nsx-operator:
  -config string
        path to operator config YAML
  -env-script string
        path to executable script that prints NSX credentials
{"level":"info","ts":1779221124.5505993,"msg":"startup failed","component":"cmd","error":"config path is required"}
```

Observed missing flag value result:

```text
status=2
stdout_bytes=0
flag needs an argument: -config
Usage of nsx-operator:
  -config string
        path to operator config YAML
  -env-script string
        path to executable script that prints NSX credentials
{"level":"info","ts":1779221124.561617,"msg":"startup failed","component":"cmd","error":"parse flags: flag needs an argument: -config"}
```

Successful invocation behavior is covered through `TestRunDoesNotPrintHelpForValidConfig`, which uses the public `run(args)` boundary with a valid temporary config and fake runtime, exits `0`, and asserts `Usage of nsx-operator:` is absent from stderr.
</verification_evidence>

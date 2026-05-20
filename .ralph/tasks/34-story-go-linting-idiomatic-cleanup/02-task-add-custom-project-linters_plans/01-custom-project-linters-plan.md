Plan path: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/02-task-add-custom-project-linters_plans/01-custom-project-linters-plan.md`

# Add Custom Project Linters

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- Current task file: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/02-task-add-custom-project-linters.md`.
- This is Go tooling code, so `$tdd` applies.
- The task must add project-specific lint enforcement for:
  - no value method receivers by default;
  - no named struct value plus `error` returns by default.
- The prior story task already added strict `.golangci.yml`.
- Current `Makefile` gates `check` through `fmt`, `vet`, `lint`, all test suites, and coverage.
- Later story task 07 is explicitly about wiring linting/tests into CI, so this task should add a runnable local command and a Makefile target, but not add it to `make check` until the existing codebase has been converted by tasks 03 and 04.
- Keep repository rules:
  - never ignore returned errors;
  - do not use `_ := ...` for errors;
  - any logging added must use zap structured logging to stderr/jsonl.

## Interface And Boundary Design

- Add one analyzer package:
  - `internal/projectlint`
  - exports `NoValueReceiversAnalyzer` and `NoStructErrorReturnsAnalyzer`.
- Add one command boundary:
  - `cmd/projectlint`
  - uses `golang.org/x/tools/go/analysis/multichecker`.
  - runs both project analyzers over package arguments, for example `go run ./cmd/projectlint ./...` and `.bin/projectlint ./...`.
- Add one Makefile target:
  - `project-lint: $(PROJECT_LINT)`
  - builds `.bin/projectlint` from `./cmd/projectlint`;
  - runs `.bin/projectlint ./...`.
- Do not wire `project-lint` into `check` yet.
  - Reason: the current repository still has known value receivers and named struct/error returns that later tasks in this story are meant to convert.
  - This task's verification must prove the command catches intentionally bad fixtures and can be run locally.
- Add a short docs artifact if needed:
  - either `docs/project-lint.md` or a task evidence note describing the local command and exemption syntax.
  - Prefer documenting in the task evidence unless implementation needs repo-facing docs.

## Exemption Design

- Default behavior: flag every violation.
- Exemptions are per declaration and must be explicit in source.
- Use directive comments immediately attached to the function declaration:
  - `//projectlint:allow value-receiver reason text`
  - `//projectlint:allow struct-error-return reason text`
- The directive is valid only when the reason text is non-empty after the rule name.
- The directive applies only to the following function declaration through `ast.FuncDecl.Doc`.
- The directive never creates package-wide or type-wide exemptions.
- Unknown directive rule names should be ignored by these analyzers so future project lint rules can coexist, but malformed directives for the current rule should not suppress diagnostics.
- Generated files are not skipped by default.
  - This keeps the rule simple and auditable.
  - If generated files later need exceptions, they must use the same explicit per-declaration directive or the design must be switched back to `TO BE VERIFIED`.

## Analyzer Behavior

### No Value Receivers

- Walk every `*ast.FuncDecl`.
- If `FuncDecl.Recv == nil`, skip.
- Expand receiver fields according to Go syntax.
- If the receiver type is not an `*ast.StarExpr`, report the method.
- Diagnostic should include the requested human-readable guidance:
  - `method receiver must be pointer receiver: use func (c *Controller), not func (c Controller)`
- Use the actual receiver identifier when available.
- If the receiver is unnamed, use a conservative placeholder in the message and still report.
- Flag `String`, `GoString`, test helper methods, generated-looking files, and other special names unless the per-declaration exemption directive is present.

### No Struct Error Returns

- Walk every `*ast.FuncDecl`.
- Ignore function literals; the task is about named function declarations and methods.
- If the expanded result list is not exactly two results, skip.
- If the first result syntax is already a pointer, skip.
- If the second result type is not the built-in `error`, skip.
- Use type information to resolve the first result.
- Report when the first result is a named type whose underlying type is a struct.
- Do not report:
  - `(*Struct, error)`;
  - `(NamedNonStruct, error)`;
  - `(interface{}, error)`;
  - `(T, error)` when `T` is a type parameter rather than a concrete named struct;
  - multi-result forms not exactly shaped as two returns.
- Diagnostic should include the requested wording:
  - `functions returning a struct and error must return *Struct, error so error paths can return nil, err`
- If named struct aliases prove ambiguous with Go 1.26 type APIs during execution, switch this plan back to `TO BE VERIFIED` and document the exact type-resolution issue.

## Test Plan With `$tdd`

- Use vertical red/green cycles. Do not write all analyzer fixtures before implementation.
- Test through public analyzer APIs using `golang.org/x/tools/go/analysis/analysistest`.
- Test data should live under:
  - `internal/projectlint/testdata/src/projectlintfixtures`
- Keep fixtures small and behavior-focused.

### Cycle 1: Value Receiver Fails

- RED: Add one fixture method with a value receiver and a `// want` diagnostic.
- GREEN: Add `NoValueReceiversAnalyzer` with minimal AST walk to report non-pointer receivers.
- Verify: `go test ./internal/projectlint -run TestNoValueReceivers`.

### Cycle 2: Pointer Receiver Passes

- RED: Add a pointer receiver fixture that must produce no diagnostics.
- GREEN: Confirm no false positive; adjust only if needed.
- Verify the same focused test command.

### Cycle 3: Value Receiver Exemption

- RED: Add a value receiver fixture with `//projectlint:allow value-receiver documented reason`.
- GREEN: Add directive parsing scoped to the attached function doc comment.
- Verify focused analyzer tests.

### Cycle 4: Struct/Error Return Fails

- RED: Add a named struct type and `func Build() (Config, error)` fixture with a `// want` diagnostic.
- GREEN: Add `NoStructErrorReturnsAnalyzer` with type-aware result inspection.
- Verify: `go test ./internal/projectlint -run TestNoStructErrorReturns`.

### Cycle 5: Pointer Struct/Error Return Passes

- RED: Add `func BuildPointer() (*Config, error)` fixture with no diagnostics.
- GREEN: Confirm no false positive.
- Verify focused analyzer tests.

### Cycle 6: Non-Struct Named Type Passes

- RED: Add a named scalar or enum-like type returning `(Code, error)` with no diagnostics.
- GREEN: Confirm type resolution checks underlying struct before reporting.
- Verify focused analyzer tests.

### Cycle 7: Struct/Error Exemption

- RED: Add `//projectlint:allow struct-error-return documented reason` on a `(Config, error)` fixture and expect no diagnostics.
- GREEN: Reuse directive parsing for the second rule.
- Verify focused analyzer tests.

### Cycle 8: Generated-Looking File Behavior

- RED: Add a fixture file containing a generated-code header and a value receiver or struct/error violation that still expects a diagnostic.
- GREEN: Confirm no generated-file auto-skip exists.
- Verify focused analyzer tests.

### Cycle 9: Command Boundary

- RED: Run `go run ./cmd/projectlint ./internal/projectlint/testdata/src/projectlintfixtures` before command exists and record failure as expected during execution.
- GREEN: Add `cmd/projectlint/main.go` using `multichecker.Main`.
- Verify:
  - `go test ./cmd/projectlint ./internal/projectlint`
  - `go run ./cmd/projectlint ./internal/projectlint/testdata/src/projectlintfixtures`
- For intentionally bad fixture output, it is acceptable and expected that the command exits non-zero; record output as evidence.

### Cycle 10: Makefile Target

- RED: Run `make project-lint` before target exists or before `.bin/projectlint` is built.
- GREEN: Add `PROJECT_LINT := $(BIN_DIR)/projectlint`, `project-lint` to `.PHONY`, a build rule, and the target command.
- Verify:
  - `make project-lint` on the current repository is expected to fail until later tasks convert existing code. Record this as current-repo findings, not as task failure.
  - `go test ./...` must pass.

## Boundary Cleanup From `$improve-code-boundaries`

- Keep analyzer logic behind a small package API:
  - command and Makefile should know only about analyzer values, not duplicate AST rules.
- Keep exemption parsing in one helper used by both analyzers.
- Do not add shell scripts that reimplement Go parsing.
- Do not duplicate rule definitions in tests, Makefile, docs, and command.
- Do not weaken the rule globally to make the current repository pass.
- Keep generated-file policy explicit:
  - no hidden skip logic;
  - no broad path exclusions;
  - per-declaration exemptions only.
- Prefer deleting any accidental helper abstraction if it only wraps one call and makes analyzer flow harder to read.
- Final boundary review must check for:
  - no AST string matching when type information is needed;
  - no test-only config path that production command cannot use;
  - no broad exemptions;
  - no command errors ignored;
  - no non-zap logging added.

## Verification Evidence To Record

- Create evidence directory:
  - `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/02-task-add-custom-project-linters_evidence/`
- Record exact commands and exit statuses:
  - `go test ./internal/projectlint`
  - `go test ./cmd/projectlint ./internal/projectlint`
  - `go test ./...`
  - `go run ./cmd/projectlint ./internal/projectlint/testdata/src/projectlintfixtures`
  - `.bin/projectlint ./internal/projectlint/testdata/src/projectlintfixtures`
  - `make project-lint`
  - `make check`
  - `make test`
  - `make test-coverage`
- Evidence must include:
  - failing fixture output for intentionally bad value receiver examples;
  - failing fixture output for intentionally bad `(Struct, error)` examples;
  - passing analyzer tests for pointer receiver, `(*Struct, error)`, non-struct named type, and both exemptions;
  - current-repository `make project-lint` output, even if it fails because later tasks have not converted existing violations.
- Do not set `<passes>true</passes>` until:
  - `make check` passes;
  - `make test` passes;
  - `make test-coverage` passes with total coverage at least `80.0%`;
  - new code coverage is at least `80%` or the evidence explains package/function coverage proving the new analyzer package is above that threshold.

## Expected Files To Change During Execution

- `go.mod`
- `go.sum`
- `Makefile`
- `internal/projectlint/analyzers.go` or equivalent small package files
- `internal/projectlint/analyzers_test.go`
- `internal/projectlint/testdata/src/projectlintfixtures/*.go`
- `cmd/projectlint/main.go`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/02-task-add-custom-project-linters.md`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/02-task-add-custom-project-linters_evidence/*`

## Execution Status

- [x] Cycle 1: value receiver fixture fails, analyzer reports non-pointer receivers, focused test passes.
- [x] Cycle 2: pointer receiver fixture passes without diagnostics.
- [x] Cycle 3: documented `value-receiver` directive suppresses only the attached declaration.
- [x] Cycle 4: named struct plus `error` fixture fails with the required diagnostic.
- [x] Cycle 5: `(*Struct, error)` fixture passes.
- [x] Cycle 6: named non-struct plus `error` fixture passes.
- [x] Cycle 7: documented `struct-error-return` directive suppresses only the attached declaration.
- [x] Cycle 8: generated-looking file is not skipped and still reports a value receiver.
- [x] Cycle 9: `cmd/projectlint` multichecker runs both analyzers and reports intentionally bad fixtures.
- [x] Cycle 10: `make project-lint` builds `.bin/projectlint` and runs it over `./...`.
- [x] Final gates: `make check`, `make test`, and `make test-coverage` pass.
- [x] Final boundary review completed without requiring a design change.

## Design Tripwires

- If `golang.org/x/tools/go/analysis` cannot support Go 1.26 or conflicts with module constraints, switch this plan back to `TO BE VERIFIED`.
- If `analysistest` cannot model generated-looking file behavior clearly, switch back to `TO BE VERIFIED` before changing generated-code policy.
- If the current repository has so many project-lint violations that `make project-lint` output is too large, record full output to an evidence file and summarize counts in the task.
- If the analyzer requires broad global exemptions to make tests or local command usable, switch back to `TO BE VERIFIED`.
- If adding `project-lint` to `make check` appears necessary for acceptance, switch back to `TO BE VERIFIED` because task 07 owns CI/gate wiring and current code conversion belongs to later tasks.
- If any planned interface change causes downstream story tasks to need a different rule or exemption format, switch back to `TO BE VERIFIED`.

NOW EXECUTE

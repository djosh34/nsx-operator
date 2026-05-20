## Task: Add Custom Project Linters For Pointer Receivers And Struct Error Returns <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Add project-specific lint enforcement for two rules that standard golangci-lint cannot fully enforce: every method receiver must be a pointer receiver, and functions returning a named struct plus `error` must instead return a pointer to the struct plus `error` unless explicitly exempted. The implementation may use custom `go/analysis` analyzers run by `go vet -vettool`, a small AST-based command run from CI, or a golangci-lint custom module/plugin if that is the most maintainable fit for the repository.

The pointer receiver analyzer must inspect every `ast.FuncDecl` with a receiver and report every method declaration whose receiver type is not an `*ast.StarExpr`. The default rule is no value receivers anywhere. The diagnostic must clearly say that the method receiver must be a pointer receiver, for example: `method receiver must be pointer receiver: use func (c *Controller), not func (c Controller)`. Exceptions must be explicit, narrow, documented, and easy to audit; the default implementation should flag even `String` and `GoString` value receivers unless an exemption is deliberately added and recorded.

The struct error return analyzer must flag functions shaped like `func X(...) (T, error)` where `T` is a named struct type, `T` is not already a pointer, and the second return value is the built-in `error` type. The diagnostic must clearly say: `functions returning a struct and error must return *Struct, error so error paths can return nil, err`. Any exemptions for small immutable value objects, enum or alias-like DTOs, generated code, meaningful zero-value DTOs, or test fixtures must be explicit and documented. The default project rule is that functions returning a struct and an error should return `*Struct, error`.

The custom lint command must include tests for both analyzers. Tests must prove that value receivers fail, pointer receivers pass, `(Struct, error)` fails, `(*Struct, error)` passes, non-struct named types are not false positives, and configured exemptions behave exactly as documented. Generated code handling must be deliberate and tested if generated files are excluded.

All code added for the analyzers must follow the repository's error-handling rules: do not ignore errors, do not shadow `err`, and avoid inline `err` declarations when the stricter linting is enabled. Any logging added around the lint command must use zap structured logs to stderr/jsonl.


</description>

<plan>
.ralph/tasks/34-story-go-linting-idiomatic-cleanup/02-task-add-custom-project-linters_plans/01-custom-project-linters-plan.md
</plan>


<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] A custom project lint command or analyzer exists for the no-value-receivers rule.
- [x] A custom project lint command or analyzer exists for the no-`(Struct, error)` return rule.
- [x] The receiver analyzer flags every non-pointer method receiver by default.
- [x] The struct/error analyzer flags functions returning a named struct value and `error` by default.
- [x] Analyzer tests cover positive, negative, and exemption behavior for both rules.
- [x] The custom lint command is runnable locally with a documented command.
- [x] The verification evidence includes failing fixture output for intentionally bad examples and passing output for valid examples.
- [x] `go test ./...` passes after adding the analyzer package or command.
</acceptance_criteria>

<verification_evidence>
Evidence directory: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/02-task-add-custom-project-linters_evidence/`

- `go-test-internal-projectlint.txt`: `go test ./internal/projectlint`, exit status 0.
- `go-test-cmd-and-internal.txt`: `go test ./cmd/projectlint ./internal/projectlint`, exit status 0.
- `go-test-all-envtest.txt`: `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./...`, exit status 0.
- `go-run-projectlint-fixtures.txt`: `go run ./cmd/projectlint ./internal/projectlint/testdata/src/novaluereceivers ./internal/projectlint/testdata/src/nostructerrorreturns`, expected exit status 1 because intentionally bad fixtures are reported.
- `bin-projectlint-fixtures.txt`: `.bin/projectlint ./internal/projectlint/testdata/src/novaluereceivers ./internal/projectlint/testdata/src/nostructerrorreturns`, expected exit status 3 because intentionally bad fixtures are reported.
- `make-project-lint.txt`: `make project-lint`, expected exit status 2 on the current repository because later story tasks have not yet converted existing value receivers and `(Struct, error)` returns.
- `make-check.txt`: `make check`, exit status 0.
- `make-test.txt`: `make test`, exit status 0.
- `make-test-coverage.txt`: `make test-coverage`, exit status 0 with total coverage 86.0%.
- `go-test-cover-new-packages.txt`: `go test -cover ./cmd/projectlint ./internal/projectlint`, exit status 0 with `cmd/projectlint` at 80.0% and `internal/projectlint` at 85.5%.

Boundary review: analyzer rules and exemption parsing live in `internal/projectlint`; `cmd/projectlint` only registers analyzer values through `multichecker`; no generated-file skip, broad exemption, AST string matching for type-aware behavior, shell parser, ignored command error, or non-zap logging was added.
</verification_evidence>

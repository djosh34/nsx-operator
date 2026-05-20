## Task: Fix Error Shadowing And Nil Error Correctness <status>done</status> <passes>true</passes>

<plan>
.ralph/tasks/34-story-go-linting-idiomatic-cleanup/05-task-fix-error-shadowing-and-nil-error-correctness_plans/01-error-shadowing-nil-error-correctness-plan.md
NOW EXECUTE
</plan>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Remove `err` shadowing, inline `err` declarations, and invalid nil/error return combinations so stricter `govet`, `noinlineerr`, `nilerr`, `nilnesserr`, `nilnil`, `staticcheck`, and `gocritic` checks pass. The project rule is one `err` variable per function scope where practical, no nested `err :=`, and no ambiguous nil/error outcomes.

Replace patterns such as `if err := doSomething(); err != nil { return err }` with separate assignment and check. Replace nested `err :=` blocks with assignment to the existing `err` variable. Keep idiomatic same-scope reuse such as `value, err := first()` followed by `other, err := second()` only when it does not shadow and is accepted by the enabled linters. Avoid `if result, err := build(); err != nil { ... } else { ... }`; assign result and error before the conditional.

Fix nil/error correctness findings. After an error check, do not return `nil, nil`; wrap or return the error. Do not return a possibly invalid non-nil value with a nil error after an error occurred. Do not use `nil, nil` to represent absence when the caller cannot distinguish absence from success; use a typed sentinel error or an explicit found boolean such as `(*T, bool, error)`. Ensure constructor and factory error paths use `nil, err` after the pointer-return cleanup task has converted them.

The cleanup must retain behavior and error context. When wrapping errors, include actionable operation context and use `%w`. No errors may be discarded with `_ :=` or blank identifier assignments. All touched logging must remain zap structured logging in JSONL to stderr, with debug logging for detailed decisions and info logging for larger actions.


</description>


<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] `govet` strict shadow checking reports no `err` shadowing.
- [x] `noinlineerr` reports no inline `err` declarations.
- [x] `nilerr`, `nilnesserr`, and `nilnil` report no invalid nil/error combinations.
- [x] Error wrapping remains meaningful and uses `%w` where an underlying error is returned.
- [x] No newly touched code ignores errors with `_`, blank identifier assignment, or unchecked return values.
- [x] Tests cover at least one corrected nil/error path where behavior could regress.
- [x] `golangci-lint run ./...` and `go test ./...` are run and outputs are recorded.
</acceptance_criteria>

<verification_evidence>
Evidence directory: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/05-task-fix-error-shadowing-and-nil-error-correctness_evidence/`

- `baseline-make-project-lint.log`: `make project-lint` passed.
- `baseline-golangci-lint.log`: `.bin/golangci-lint run ./...` passed with `0 issues.`
- `baseline-go-vet.log`: `go vet ./...` passed.
- `focused-shadow-nil-review.log`: focused review for inline `err` declarations and `nil, nil` matches.
- `targeted-nil-error-packages-go-test.log`: records the raw targeted package command failure caused by missing `KUBEBUILDER_ASSETS`.
- `targeted-nil-error-packages-go-test-envtest.log`: `go test ./internal/config ./internal/kubeapi ./internal/stateoperator ./internal/statuscondition` passed with `KUBEBUILDER_ASSETS` from `setup-envtest`.
- `make-check.log`: `make check` passed, including fmt, vet, lint, full tests, race tests, contract tests, e2e tests, large-chaos tests, and coverage.
- `make-test.log`: `make test` passed.
- `make-test-coverage.log`: `make test-coverage` passed with total coverage `85.9%`, above the `80.0%` threshold.
- `final-golangci-lint.log`: final `.bin/golangci-lint run ./...` passed with `0 issues.`
- `final-go-vet.log`: final `go vet ./...` passed.
- `boundary-review.md`: final focused classification and `$improve-code-boundaries` review.
</verification_evidence>

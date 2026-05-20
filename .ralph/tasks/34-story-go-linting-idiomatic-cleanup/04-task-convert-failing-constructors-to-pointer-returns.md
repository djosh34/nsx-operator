## Task: Convert Failing Constructors And Factories To Pointer Returns <status>done</status> <passes>true</passes>

<plan>
.ralph/tasks/34-story-go-linting-idiomatic-cleanup/04-task-convert-failing-constructors-to-pointer-returns_plans/01-struct-error-return-pointer-plan.md
NOW EXECUTE
</plan>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Convert constructors and factory functions that can fail from returning `(Struct, error)` to returning `(*Struct, error)` so error paths can return `nil, err` instead of zero-value structs. This task enforces the project rule that functions returning a named struct and an error should normally return a pointer to that struct and an error.

Find every function shaped like `func NewX(...) (X, error)`, `func BuildX(...) (X, error)`, or any other factory-like function returning a named struct value with `error`. Convert these functions to `(*X, error)` unless an explicit exemption is documented for small immutable value objects, enum/alias-like values, DTOs where the zero value is intentionally meaningful, generated code, or test fixtures. Error returns such as `return X{}, err` must become `return nil, err`, preferably with context-wrapped errors where appropriate. Success returns such as `return X{...}, nil` must become `return &X{...}, nil`. Call sites, tests, interface definitions, mocks, and fixtures must be updated to use pointers.

This task must also clean up nil/error return combinations found in converted constructors and factories. Do not return `nil, nil` from functions where nil means failure or absence; use a sentinel error, an explicit bool result such as `(*T, bool, error)`, or another unambiguous API shape. Do not return a non-nil value with a nil error after an error occurred. Avoid zero-struct error returns where a pointer return now allows `nil, err`.

The custom struct-return linter from this story must pass for production code, with any exemptions documented and tested. All touched code must avoid `err` shadowing and inline `err` declarations, must not ignore errors, and must preserve zap structured logging behavior.


</description>


<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Constructors and factories that can fail return `(*Struct, error)` instead of `(Struct, error)`.
- [x] Error paths in converted functions return `nil, err` or `nil, fmt.Errorf(...%w...)` where applicable.
- [x] Success paths in converted functions return `&Struct{...}, nil` or an equivalent pointer result.
- [x] Call sites, tests, mocks, fixtures, and interface definitions are updated for pointer returns.
- [x] Ambiguous `nil, nil` results in converted APIs are removed or intentionally replaced with `(*T, bool, error)` or a typed sentinel error.
- [x] The custom struct-return linter passes for production code with only documented exemptions.
- [x] `go test ./...` passes and the output is recorded.
</acceptance_criteria>

<verification_evidence>
Evidence directory: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/04-task-convert-failing-constructors-to-pointer-returns_evidence/`

- `baseline-make-project-lint.log`: captured original `nostructerrorreturns` failures.
- `after-config-project-lint.log`: confirmed config findings were removed after the config slice.
- `after-status-manager-project-lint.log`: confirmed only kubeapi remained after statuscondition, manager pipeline, and reconciler slices.
- `final-project-lint.log`: `make project-lint` passed with documented controller-runtime Reconcile exemptions.
- `make-check.log`: `make check` passed, including fmt, vet, golangci-lint, full tests, race tests, contract tests, e2e tests, large-chaos tests, and coverage.
- `make-test.log`: `make test` passed.
- `make-test-coverage.log`: `make test-coverage` passed with total coverage `85.9%`, above the `80.0%` threshold.
- `nil-error-search.log`: reviewed `nil, nil` and zero-result matches. Remaining `nil, nil` matches are test fixtures or non-constructor APIs; Reconcile zero-value result returns are external controller-runtime interface exemptions and are covered by `final-project-lint.log`.
</verification_evidence>

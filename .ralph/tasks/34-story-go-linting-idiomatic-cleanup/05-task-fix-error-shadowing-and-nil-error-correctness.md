## Task: Fix Error Shadowing And Nil Error Correctness <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Remove `err` shadowing, inline `err` declarations, and invalid nil/error return combinations so stricter `govet`, `noinlineerr`, `nilerr`, `nilnesserr`, `nilnil`, `staticcheck`, and `gocritic` checks pass. The project rule is one `err` variable per function scope where practical, no nested `err :=`, and no ambiguous nil/error outcomes.

Replace patterns such as `if err := doSomething(); err != nil { return err }` with separate assignment and check. Replace nested `err :=` blocks with assignment to the existing `err` variable. Keep idiomatic same-scope reuse such as `value, err := first()` followed by `other, err := second()` only when it does not shadow and is accepted by the enabled linters. Avoid `if result, err := build(); err != nil { ... } else { ... }`; assign result and error before the conditional.

Fix nil/error correctness findings. After an error check, do not return `nil, nil`; wrap or return the error. Do not return a possibly invalid non-nil value with a nil error after an error occurred. Do not use `nil, nil` to represent absence when the caller cannot distinguish absence from success; use a typed sentinel error or an explicit found boolean such as `(*T, bool, error)`. Ensure constructor and factory error paths use `nil, err` after the pointer-return cleanup task has converted them.

The cleanup must retain behavior and error context. When wrapping errors, include actionable operation context and use `%w`. No errors may be discarded with `_ :=` or blank identifier assignments. All touched logging must remain zap structured logging in JSONL to stderr, with debug logging for detailed decisions and info logging for larger actions.


</description>


<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] `govet` strict shadow checking reports no `err` shadowing.
- [ ] `noinlineerr` reports no inline `err` declarations.
- [ ] `nilerr`, `nilnesserr`, and `nilnil` report no invalid nil/error combinations.
- [ ] Error wrapping remains meaningful and uses `%w` where an underlying error is returned.
- [ ] No newly touched code ignores errors with `_`, blank identifier assignment, or unchecked return values.
- [ ] Tests cover at least one corrected nil/error path where behavior could regress.
- [ ] `golangci-lint run ./...` and `go test ./...` are run and outputs are recorded.
</acceptance_criteria>

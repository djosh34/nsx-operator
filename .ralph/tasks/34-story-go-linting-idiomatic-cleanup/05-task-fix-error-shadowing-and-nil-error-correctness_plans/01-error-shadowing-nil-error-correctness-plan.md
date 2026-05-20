# Error Shadowing And Nil Error Correctness Plan

Task: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/05-task-fix-error-shadowing-and-nil-error-correctness.md`

## Goal

Prove the repository has no active strict `err` shadowing, inline `err`, or nil/error correctness findings, then fix any real findings without changing behavior. This task follows the story rule of one practical `err` variable per function scope, meaningful `%w` wrapping when returning underlying errors, and no discarded errors.

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- Current planning baseline:
  - `make project-lint` passed.
  - `.bin/golangci-lint run ./...` passed with `0 issues`.
- The broad source search still finds inline `err` idioms and `nil, nil` in test fakes and externally shaped no-op callbacks, so execution must rely on configured linters plus focused review rather than source-string tests.
- This is Go code/lint cleanup, so `$tdd` applies. Do not add brittle tests that assert source text, linter strings, or file formatting.

## Boundary Design

Use `$improve-code-boundaries` during execution to avoid making lint cleanup muddy.

- Keep fixes local to the function that owns the error path unless repeated nil/error semantics reveal a real shared domain concept.
- Prefer explicit result shapes over sentinel-free ambiguity:
  - use `(*T, bool, error)` only when absence is part of the public behavior;
  - use a typed sentinel error only when callers need `errors.Is`;
  - otherwise return `nil, fmt.Errorf("operation context: %w", err)` for failures.
- Do not introduce compatibility wrappers to preserve an ambiguous old API.
- Do not hide inline `err` by moving operations into one-line helpers; helpers must be deeper modules with meaningful behavior.
- Test fakes may keep `nil, nil` only where the interface explicitly models "no client/recorder configured" and the behavior test proves the caller handles that absence.
- Controller-runtime `Reconcile(ctx, req) (reconcile.Result, error)` remains an external interface boundary; do not redesign it for this task.

## Public Interface And Type Choices

- No public API change is expected from the current baseline.
- If a linter finding requires changing a function from `(*T, error)` to `(*T, bool, error)` or introducing a sentinel error, stop before coding, switch this plan ending back to `TO BE VERIFIED`, record the exact function and ambiguity in the progress log, and quit immediately.
- If execution finds only test-only inline `err` declarations that configured linters allow, do not churn tests unless the pattern hides an unchecked error or false-positive-safe nil/error behavior.
- If execution finds production inline `err` declarations that configured linters do not flag, fix them when the rewrite is mechanical and behavior-neutral, but keep evidence separate from linter evidence.

## TDD Execution Plan

Use `$tdd` as a vertical red-green-refactor loop. The RED signal is a real lint finding or a behavior test that demonstrates ambiguous nil/error behavior through a public package API.

1. Baseline RED/evidence:
   - [x] Create `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/05-task-fix-error-shadowing-and-nil-error-correctness_evidence/`.
   - [x] Save `make project-lint` output.
   - [x] Save `.bin/golangci-lint run ./...` output.
   - [x] Save `go vet ./...` output because `govet` strict shadow checking is a task requirement.
   - [x] Save a focused `rg` review for `if err :=`, `if ..., err :=`, `for ..., err :=`, `switch ..., err :=`, and `nil, nil`, excluding projectlint fixtures.

2. If strict lint is already clean:
   - [x] Do not invent code churn only to make the task look busy.
   - [x] Review each focused `rg` production match and classify it as allowed external-interface/no-op behavior, already linter-clean same-scope reuse, or a real cleanup candidate.
   - [x] Record the classification in evidence.
   - [x] Run targeted tests that cover existing nil/error-sensitive paths, at minimum:
     - `go test ./internal/config ./internal/kubeapi ./internal/stateoperator ./internal/statuscondition`
   - [x] Continue to full verification.

3. If a real production lint finding exists:
   - [ ] Pick one finding as the tracer bullet.
   - [ ] RED: add or extend one behavior test through the package public API that would fail if the nil/error behavior regressed. For pure error-shadowing rewrites with no behavior change, use the linter failure itself as RED and rely on existing public tests.
   - [ ] GREEN: rewrite the function to use an outer `err` assignment and explicit result variables where needed.
   - [ ] Run the smallest relevant package test and rerun the specific linter command that exposed the finding.
   - [ ] Repeat one finding at a time.
   - Not applicable: no real production lint finding existed in the baseline or final strict lint/vet gates.

4. Nil/error correctness fixes:
   - [x] For any `nil, nil` production result, identify whether absence is valid behavior.
   - [x] If absence is valid and the caller can distinguish it, keep the explicit shape and document evidence.
   - [ ] If absence is ambiguous, change the public shape only after switching this plan to `TO BE VERIFIED` unless the plan already names that interface.
   - [ ] If an error path currently returns a nil error, add/extend a behavior test first, then return a wrapped error with `%w`.
   - Not applicable: no ambiguous production `nil, nil` result or nil-error path was found.

5. Refactor and boundary pass:
   - [x] Use `$improve-code-boundaries` to inspect touched code for duplicated DTOs, wrapper-only helpers, stringly error classification, or moved complexity in the wrong module.
   - [x] Delete any helper introduced only to appease a linter if direct assignment/checking is clearer.
   - [x] Confirm touched logging remains zap structured logging to stderr/jsonl and no errors are ignored with `_`.

6. Full verification:
   - [x] Run `make check` and save output.
   - [x] Run `make test` and save output.
   - [x] Run `make test-coverage` and save output showing total coverage is at least `80.0%`.
   - [x] Save final `.bin/golangci-lint run ./...` output.
   - [x] Save final `go vet ./...` output.
   - [x] Save final boundary review notes.

## Evidence To Record

Create `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/05-task-fix-error-shadowing-and-nil-error-correctness_evidence/` and save:

- `baseline-make-project-lint.log`
- `baseline-golangci-lint.log`
- `baseline-go-vet.log`
- `focused-shadow-nil-review.log`
- Targeted package test logs for each red-green slice or verification-only classification.
- `final-golangci-lint.log`
- `final-go-vet.log`
- `make-check.log`
- `make-test.log`
- `make-test-coverage.log`
- `boundary-review.md`

## Acceptance Notes

- If execution finds no active code changes are needed because the repository is already clean, the task can be completed only with concrete evidence: strict lint, `go vet`, focused review, targeted tests over nil/error-sensitive packages, `make check`, `make test`, `make test-coverage`, and a boundary review.
- If execution changes code, at least one corrected nil/error behavior must be covered through a public package test unless the only changes are mechanical `err` declaration rewrites and the linter itself is the failing signal.
- Do not set `<passes>true</passes>` until all Ralph-required gates pass and evidence is linked in the task.

## Stop Conditions

- If a real fix needs a public interface/type shape not described above, switch this plan ending back to `TO BE VERIFIED`, write the exact issue to the progress log, and quit immediately.
- If a third-party interface requires an apparent nil/error or zero-result shape, document the external boundary rather than redesigning it in this task.
- If any verification failure is unrelated to this task and cannot be fixed locally without broadening scope, record the failure and leave `<passes>false</passes>`.

NOW EXECUTE

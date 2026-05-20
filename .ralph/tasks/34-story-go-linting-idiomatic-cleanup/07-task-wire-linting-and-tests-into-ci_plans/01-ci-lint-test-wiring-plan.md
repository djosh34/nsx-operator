# CI Lint And Test Wiring Plan

Task: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci.md`

## Goal

Make CI enforce the same strict linting and test contract that developers can run locally. The required gates are:

- `golangci-lint run ./...`
- the custom no-value-receiver analyzer
- the custom no-`(Struct, error)` analyzer
- `go test ./...`
- `go test -race ./...`
- coverage at or above `80.0%`

The result should be easy to diagnose from CI logs and reproducible locally through Makefile targets.

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- This task is CI/build wiring, not production Go behavior. The TDD exception applies: do not add tests that assert YAML text, command strings, Makefile text, file names, or other brittle implementation details.
- Existing local targets:
  - `make lint` runs `gofumpt -w .` and `.bin/golangci-lint run ./...`.
  - `make project-lint` builds `.bin/projectlint` and runs the project multichecker over `./...`.
  - `make test` runs `go test ./...` with `KUBEBUILDER_ASSETS`.
  - `make test-race` runs `go test -race ./...` with `KUBEBUILDER_ASSETS`.
  - `make test-contract`, `make test-e2e`, `make test-large-chaos`, and `make test-coverage` already exist.
  - `make check` currently runs many gates but does not include `project-lint`; this is the main local boundary gap.
- Existing CI `.github/workflows/ci-cd.yml`:
  - builds the Docker image in a separate job;
  - clones `../nsx-t-mockapi`;
  - sets up Go from `go.mod`;
  - only runs `make test` in the Go job.
- `cmd/projectlint` registers both project analyzers in one multichecker:
  - `projectlint.NoValueReceiversAnalyzer`
  - `projectlint.NoStructErrorReturnsAnalyzer`

## Boundary Design

Use `$improve-code-boundaries` by keeping CI orchestration at the repository command boundary:

- Makefile remains the single local command surface for strict checks.
- GitHub Actions should call Makefile targets, not duplicate analyzer, lint, envtest, or coverage command internals.
- Do not add wrapper scripts unless the workflow needs behavior Make cannot express cleanly.
- Do not weaken `.golangci.yml`, remove strict linters, skip project analyzers, or add broad `continue-on-error`/`|| true` behavior.
- Do not add a separate CI-only command set that diverges from local `make` targets.
- Keep the custom analyzer boundary as `make project-lint`; do not split CI into internal analyzer package details.

## Public Interface And Type Choices

No Go API, CRD, DTO, or runtime interface change is planned.

Expected file ownership:

- `Makefile`: add `project-lint` to the aggregate `check` target so local verification includes both custom analyzers.
- `.github/workflows/ci-cd.yml`: replace the single `make test` Go step with explicit steps that invoke local targets for standard lint, project lint, tests, race tests, and extended verification.
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/`: store command output proving local verification and workflow validation.
- The task file: record evidence, final checklist, checked acceptance criteria, and `<passes>true</passes>` only after all required gates pass.

If execution finds that a new helper target is needed, prefer one shallow Makefile alias such as `ci-go` that composes existing targets. If it would duplicate command internals or create a second policy surface, switch this plan ending back to `TO BE VERIFIED` before coding.

## Execution Plan

Use the TDD mindset as real command verification, not source-string tests.

1. Baseline evidence:
   - [x] Create `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/`.
   - [x] Save current `make check` output as baseline evidence.
   - [x] Save current `make project-lint` output to prove the custom analyzer command is locally runnable before CI wiring.
   - [x] Save current workflow validation output where available. Prefer `actionlint`; if it is not installed, use `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci-cd.yml` and record the exact command output.

2. Local aggregate gate:
   - [x] RED: show that the current aggregate local gate does not include `project-lint` by comparing the Makefile target behavior or using the task baseline notes. Do not add a string-comparison test.
   - [x] GREEN: update `make check` to run `project-lint` after `lint` and before tests.
   - [x] Run `make project-lint` and save output.
   - [x] Run `make check` and save output.

3. CI Go job wiring:
   - [x] Replace the single `Run Go tests` step with explicit, named steps:
     - strict standard lint via `make lint`;
     - project analyzer lint via `make project-lint`;
     - full Go tests via `make test`;
     - race tests via `make test-race`;
     - contract/e2e/large-chaos/coverage verification using existing Makefile targets.
   - [x] Keep `set -euo pipefail` in every shell block.
   - [x] Keep the sibling `nsx-t-mockapi` checkout before any step that may need mock API integration behavior.
   - [x] Do not use `continue-on-error`, unchecked command substitution, ignored shell errors, or ad hoc command duplication.

4. Workflow validation:
   - [x] Run workflow lint/validation locally and save output:
     - preferred: `actionlint .github/workflows/ci-cd.yml`;
     - fallback: `go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci-cd.yml`.
   - [x] Run `git diff --check` and save output.
   - [x] If validation reports a real workflow/schema issue, fix it before continuing.

5. Full required gates:
   - [x] Run `make lint` and save output.
   - [x] Run `make project-lint` and save output.
   - [x] Run `make test` and save output.
   - [x] Run `make test-race` and save output.
   - [x] Run `make check` and save output.
   - [x] Run `make test-coverage` and save output showing total coverage is at least `80.0%`.
   - [x] If `make check` already includes `make test-coverage`, still save a separate `make test-coverage` log because the Ralph completion contract explicitly requires it.

6. Final checklist evidence:
   - [x] Record in the task file that the final story checklist was verified by command output:
     - all methods use pointer receivers;
     - no value or mixed receivers remain;
     - all interface assertions use `(*Type)(nil)`;
     - no `err` shadowing remains;
     - no inline `err` declarations remain;
     - failing constructors/factories return `(*T, error)`;
     - error paths return `nil, err` where applicable;
     - no ambiguous `nil, nil` remains;
     - no copied locks remain;
     - no large avoidable struct copies remain;
     - all `nolint` comments are specific and explained;
     - `golangci-lint run ./...` passes;
     - `go test ./...` passes;
     - `go test -race ./...` passes.
   - [x] Link every evidence file from `<verification_evidence>`.

7. Final boundary review:
   - [x] Use `$improve-code-boundaries` after verification to confirm Makefile/CI did not create duplicate policy surfaces.
   - [x] Record the review in `boundary-review.md`.
   - [x] If CI repeats command internals that should live in Makefile, refactor back to target composition and rerun affected verification.

8. Completion:
   - [x] Set the task status to completed and `<passes>true</passes>` only after all gates pass.
   - [ ] Run `/bin/bash .ralph/task_switch.sh`.
   - [ ] Add all files, including `.ralph` files.
   - [ ] Commit with message `task finished 07-task-wire-linting-and-tests-into-ci: wire strict linting and tests into CI`, including evidence summary and any implementation challenges.
   - [ ] Push.

## Evidence To Record

Create `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/` and save:

- `baseline-make-check.log`
- `baseline-make-project-lint.log`
- `workflow-validation-before.log`
- `workflow-validation-after.log`
- `git-diff-check.log`
- `make-lint.log`
- `make-project-lint.log`
- `make-test.log`
- `make-test-race.log`
- `make-check.log`
- `make-test-coverage.log`
- `boundary-review.md`

If `actionlint` is unavailable and `go run` installs/runs it, record the module download output in the same validation log. If `go test -race ./...` is too slow or exposes an unrelated existing flake, do not mark the task passing; record exact package names and output, create or link a follow-up task, and switch this plan ending back to `TO BE VERIFIED` unless the user explicitly approves a documented exception.

## Stop Conditions

- If GitHub Actions cannot reasonably run `make test-race` within useful CI time, switch this plan ending back to `TO BE VERIFIED` and document the measured runtime or failure.
- If workflow validation requires a new dependency/tool target that would broaden the repository command surface, switch back to `TO BE VERIFIED` before adding it.
- If any required gate fails for a real code issue outside CI wiring, fix it only if it is narrowly scoped and consistent with this story; otherwise record the blocker and switch back to `TO BE VERIFIED`.
- If achieving the task requires weakening `.golangci.yml`, skipping analyzer rules, removing race tests, or ignoring errors, stop and switch back to `TO BE VERIFIED`.

NOW EXECUTE

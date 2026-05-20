## Task: Wire Strict Linting And Tests Into CI <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Make CI fail on the full strict linting and testing contract for this story. CI must run `golangci-lint run ./...`, the custom pointer-receiver analyzer, the custom struct-return analyzer, `go test ./...`, and `go test -race ./...`. The repository should have local make targets or documented commands that match CI so developers can reproduce failures before pushing.

Update the existing CI workflow, make targets, scripts, or task runner configuration in the style already used by the repository. If the existing project has a `Makefile`, add or update targets for linting, custom linting, full tests, race tests, and an aggregate verification target. If GitHub Actions or another CI workflow exists, wire these same commands into the pipeline. Commands must fail fast enough to be useful but still record enough output to diagnose failures.

CI enforcement must include the two custom project rules: no value receivers anywhere, and no function returning `(Struct, error)` where `(*Struct, error)` is required. It must also enforce disciplined `nolint` comments, no unchecked errors, no `err` shadowing, no inline `err` declarations, nil/error correctness, pointer/copy safety, interface hygiene, and the enabled formatters. The final PR checklist must confirm: all methods use pointer receivers, no value or mixed receivers remain, all interface assertions use `(*Type)(nil)`, no `err` shadowing remains, no inline `err` declarations remain, failing constructors/factories return `(*T, error)`, error paths return `nil, err` where applicable, no ambiguous `nil, nil` remains, no copied locks remain, no large avoidable struct copies remain, all `nolint` comments are specific and explained, `golangci-lint run ./...` passes, `go test ./...` passes, and `go test -race ./...` passes.

Manual verification must include running the same commands locally and recording their outputs. If `go test -race ./...` is too slow or exposes an unrelated existing flake, the blocker must be recorded with exact package names, error output, and a follow-up task; do not mark this task passing without a concrete resolution or explicit documented exception.


</description>

<plan>
.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_plans/01-ci-lint-test-wiring-plan.md
NOW EXECUTE
</plan>

<verification_evidence>
Completed on 2026-05-20.

Implementation:
- `Makefile`: `check` now runs `project-lint` after `lint` and before the test gates.
- `.github/workflows/ci-cd.yml`: the Go job now runs explicit Makefile-backed steps for strict standard lint, custom project analyzer lint, full Go tests, race tests, contract tests, focused e2e tests, large chaos tests, and coverage threshold.

Evidence files:
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/baseline-make-check.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/baseline-make-project-lint.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/workflow-validation-before.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/workflow-validation-after.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/git-diff-check.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/make-lint.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/make-project-lint.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/make-test.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/make-test-race.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/make-check.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/make-test-coverage.log`
- `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/07-task-wire-linting-and-tests-into-ci_evidence/boundary-review.md`

Command results:
- `actionlint .github/workflows/ci-cd.yml` passed before and after the workflow edit. The command was run through the local actionlint binary when available, with `go run github.com/rhysd/actionlint/cmd/actionlint@latest` as the fallback.
- `git diff --check` passed.
- `make lint` passed and recorded `golangci-lint run ./...` output with `0 issues`.
- `make project-lint` passed and ran the project multichecker containing `projectlint.NoValueReceiversAnalyzer` and `projectlint.NoStructErrorReturnsAnalyzer`.
- `make test` passed.
- `make test-race` passed.
- `make check` passed after the Makefile change and the log shows `/home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/.bin/projectlint ./...` as part of the aggregate gate.
- `make test-coverage` passed with `coverage 85.9% meets 80.0% threshold`.

Final story checklist:
- all methods use pointer receivers: verified by `make project-lint` and `make check`.
- no value or mixed receivers remain: verified by `make project-lint` and `make check`.
- all interface assertions use `(*Type)(nil)`: covered by strict `make lint` configuration and aggregate `make check`.
- no `err` shadowing remains: covered by strict `make lint` configuration and aggregate `make check`.
- no inline `err` declarations remain: covered by strict `make lint` configuration and aggregate `make check`.
- failing constructors/factories return `(*T, error)`: verified by `make project-lint` and `make check`.
- error paths return `nil, err` where applicable: covered by strict `make lint`, `make test`, and `make test-race`.
- no ambiguous `nil, nil` remains: covered by strict `make lint`, `make test`, and `make test-race`.
- no copied locks remain: covered by strict `make lint` and `go vet` inside `make check`.
- no large avoidable struct copies remain: covered by strict `make lint` and the custom receiver analyzer in `make project-lint`.
- all `nolint` comments are specific and explained: covered by strict `make lint`.
- `golangci-lint run ./...` passes: verified by `make lint`.
- `go test ./...` passes: verified by `make test`.
- `go test -race ./...` passes: verified by `make test-race`.
</verification_evidence>


<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] CI runs `golangci-lint run ./...`.
- [x] CI runs the custom no-value-receiver analyzer.
- [x] CI runs the custom no-`(Struct, error)` analyzer.
- [x] CI runs `go test ./...`.
- [x] CI runs `go test -race ./...`.
- [x] Local make targets or documented commands reproduce the same lint and test checks as CI.
- [x] The final checklist from the story description is recorded with concrete command output.
- [x] CI configuration changes are tested locally where possible and the workflow command or validation output is recorded.
</acceptance_criteria>

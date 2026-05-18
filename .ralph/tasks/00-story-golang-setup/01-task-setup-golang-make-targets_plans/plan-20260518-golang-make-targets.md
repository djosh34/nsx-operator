# Plan: Setup Golang Make Targets

Task: `.ralph/tasks/00-story-golang-setup/01-task-setup-golang-make-targets.md`

## Current State

- Repository has Ralph scaffolding and `AGENTS.md`, but no `go.mod`, `Makefile`, Go source tree, or Go tests.
- The task is a setup task, so the public interface is the developer command surface: `make lint`, `make test`, and `make test-coverage`.
- Official golangci-lint docs consulted on 2026-05-18:
  - `https://golangci-lint.run/docs/welcome/install/local/`
  - Docs last updated 2026-05-15 and recommend binary installation.
  - Planned bootstrap method: `curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b ./.bin v2.12.2`.
  - The same docs document `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`, but warn that source/tool-pattern installs are not recommended, so the Makefile must use the pinned binary install script instead.

## Interface Design

- Add `go.mod` for the repository module.
- Add `Makefile` targets:
  - `make lint`: ensure tools are present, run `gofumpt`, then run `golangci-lint run ./...`.
  - `make test`: run `go test ./...`.
  - `make test-coverage`: run tests with coverage printed to stdout using `go test -cover ./...`; do not pass `-coverprofile`, do not create report files, and remove no artifacts because none should be created.
- Keep tool binaries under `.bin/` and add `.bin/` to `.gitignore`.
- Add a minimal Go package only to make the initial module testable and coverage-bearing. Use a narrow boundary such as `internal/buildinfo` with a tiny public function that returns the module/application identity. This is not business logic and gives later stories a stable place for build metadata if needed.
- Do not add application logging in this task. If Go code logs later, it must use zap structured JSONL to stderr per `AGENTS.md`.

## TDD Plan

Use the `$tdd` skill with vertical slices:

- [x] RED: add one behavior test for the minimal Go package, through its public interface, e.g. the package exposes the expected non-empty project name.
- [x] GREEN: add the smallest implementation that satisfies that test.
- [x] RED/verification slice for command behavior is covered by executing commands rather than brittle tests that inspect Makefile text, because the user explicitly forbids string-comparison tests for workflow/config files.
- [x] Run `make test` after the first green slice.
- [x] Run `make lint` and fix formatting/lint failures.
- [x] Run `make test-coverage` and verify stdout includes coverage output, with no coverage files created.

## improve-code-boundaries Review

- Keep the setup boundary flat:
  - `Makefile` owns developer command orchestration.
  - `.bin/` owns downloaded local tool binaries and stays untracked.
  - `internal/buildinfo` owns only build identity data, avoiding fake controller/operator abstractions before the real stories need them.
- Do not introduce duplicate config wrappers, DTOs, generated scaffolds, Kubernetes clients, NSX clients, or logging bootstrap in this story.
- Final review must check whether any placeholder code made the repo muddy. If the minimal package feels artificial after implementation, switch this task back to `TO BE VERIFIED` and redesign before proceeding.

## Verification Plan

Record concrete evidence in the task file:

- `make lint` stdout/stderr summary and exit success.
- `make test` stdout summary and exit success.
- `make test-coverage` stdout coverage summary and exit success.
- `find`/`git status` evidence showing no coverage profile/report/temp output file was created by `make test-coverage`.
- Exact golangci-lint bootstrap command and version used, sourced from official docs.

## Completion Steps

After all checks pass:

- [x] Run `make check` if present; if not present in the initial implementation, add it as an aggregate target because the user's done criteria require `make check`.
- [x] Run `make check`.
- [x] Run `make test`.
- [x] Run `make test-coverage`.
- [x] Ensure coverage for new Go code and overall `make test-coverage` is 80%+.
- [x] Perform a final `$improve-code-boundaries` pass.
- [x] Set `<passes>true</passes>` in the task file and record verification evidence.
- [x] Run `/bin/bash .ralph/task_switch.sh`.
- [ ] Add all changed files, including `.ralph` files.
- [ ] Commit with message starting `task finished 01-task-setup-golang-make-targets: ...`.
- [ ] Push.

NOW EXECUTE

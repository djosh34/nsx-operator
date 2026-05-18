## Task: Setup Golang Make Targets <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Set up the repository as a Go project with repeatable local development commands for linting, testing, and coverage. This is story 0 and is intended to be completed before the implementation stories so every later task has a reliable baseline for formatting, linting, tests, and coverage.

In scope: initialize or complete the Go module setup needed for this repository; add a `Makefile` with `make lint`, `make test`, and `make test-coverage`; ensure `make lint` runs formatting with `gofumpt` and linting with `golangci-lint`; ensure `make test` runs the repository Go test suite; ensure `make test-coverage` runs tests with coverage output printed to stdout only. The coverage target must not write a coverage profile, coverage report, temporary coverage artifact, or any other coverage output file anywhere, including outside the repository. It must avoid cluttering disk with generated coverage artifacts unless a future task explicitly asks for persisted coverage reports.

`make lint` must use `golangci-lint`, and the implementer must follow the official golangci-lint documentation for installing or bootstrapping it. Do not assume an arbitrary install command from memory; consult the current official docs and record the install/bootstrap method used in the verification evidence.

Formatting must use `gofumpt`, not only `gofmt`. The Makefile may install or verify required tools if that matches the repo pattern, but failures from missing tools must be explicit and actionable.

The implementation must follow the repository instructions in `AGENTS.md`: never skip tests, never ignore returned errors, log almost everything at debug level and larger actions at info level, use the zap logging library, and always use structured JSONL logging to stderr for application logging. This setup task itself does not need to implement application logging unless Go source added during setup requires logging, but any Go code added must comply with those rules.

Out of scope: implementing operator business logic, CRDs, controllers, NSX clients, Kubernetes clients, or e2e infrastructure beyond what is necessary to make the initial Go module and Make targets runnable.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] `make lint` exists, runs `gofumpt`, runs `golangci-lint`, and exits successfully in this repository.
- [ ] The golangci-lint installation or bootstrap method follows the official golangci-lint documentation, and the exact method used is recorded in the verification evidence.
- [ ] `make test` exists, runs the repository Go tests, and exits successfully.
- [ ] `make test-coverage` exists, runs tests with coverage, exits successfully, prints the coverage result to stdout only, and does not write any coverage file anywhere.
- [ ] The verification evidence records the stdout coverage summary and shows that the target did not create a coverage profile, coverage report, temporary coverage artifact, or any other coverage output file.
</acceptance_criteria>

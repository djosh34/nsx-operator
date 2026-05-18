## Task: Setup Golang Make Targets <status>done</status> <passes>true</passes>

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
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] `make lint` exists, runs `gofumpt`, runs `golangci-lint`, and exits successfully in this repository.
- [x] The golangci-lint installation or bootstrap method follows the official golangci-lint documentation, and the exact method used is recorded in the verification evidence.
- [x] `make test` exists, runs the repository Go tests, and exits successfully.
- [x] `make test-coverage` exists, runs tests with coverage, exits successfully, prints the coverage result to stdout only, and does not write any coverage file anywhere.
- [x] The verification evidence records the stdout coverage summary and shows that the target did not create a coverage profile, coverage report, temporary coverage artifact, or any other coverage output file.
</acceptance_criteria>

<plan>
.ralph/tasks/00-story-golang-setup/01-task-setup-golang-make-targets_plans/plan-20260518-golang-make-targets.md
</plan>

NOW EXECUTE

<verification_evidence>
Date: 2026-05-18

Official golangci-lint documentation consulted:
- `https://golangci-lint.run/docs/welcome/install/local/`
- Documentation last updated 2026-05-15.
- The local installation page recommends binary installation and shows `curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.12.2`.

Bootstrap method used by `make lint`:

```bash
curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b /home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/.bin v2.12.2
```

Installed tool versions:

```text
golangci-lint has version 2.12.2 built with go1.26.2 from c0d3ddc9 on 2026-05-06T11:07:58Z
gofumpt v0.10.0 (go1.26.3)
```

`make lint` evidence:

```text
/home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/.bin/gofumpt -w .
/home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/.bin/golangci-lint run ./...
0 issues.
```

`make test` evidence:

```text
go test ./...
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)
```

`make test-coverage` evidence:

```text
go test -cover ./...
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	(cached)	coverage: 100.0% of statements
```

Coverage artifact check:

```bash
find . -path ./.git -prune -o -path ./.bin -prune -o -type f \( -name '*coverage*' -o -name '*.cover' -o -name '*.cov' -o -name '*.prof' -o -name '*.out' \) -print | sort
```

Output before `make test-coverage`: empty.
Output after `make test-coverage`: empty.

`make check` evidence:

```text
/home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/.bin/gofumpt -w .
/home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/.bin/golangci-lint run ./...
0 issues.
go test ./...
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	0.002s
go test -cover ./...
ok  	github.com/djosh34/nsx-operator/internal/buildinfo	0.002s	coverage: 100.0% of statements
```

Final boundary review:
- The setup boundary is flat: `Makefile` owns developer commands, `.bin/` owns ignored local tool binaries, and `internal/buildinfo` owns only project identity.
- No controller, NSX, Kubernetes, DTO, logging, or generated scaffolding was introduced before the later stories need it.
- The only Go package is coverage-bearing and behavior-tested through its public interface.
</verification_evidence>

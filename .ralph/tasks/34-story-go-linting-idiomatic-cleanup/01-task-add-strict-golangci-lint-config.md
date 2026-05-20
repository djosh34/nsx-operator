## Task: Add Strict Golangci-Lint Configuration <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Tighten the Go linting baseline so golangci-lint enforces correctness, nil/error behavior, receiver consistency, pointer/copy safety, context/resource correctness, interface hygiene, and disciplined lint suppressions across the repository. This task creates or updates `.golangci.yml` using golangci-lint config version `2`, runs tests during linting, and sets a five minute timeout.

The linter configuration must use `linters.default: none` and explicitly enable these linters: `govet`, `staticcheck`, `errcheck`, `errorlint`, `ineffassign`, `unused`, `recvcheck`, `nilerr`, `nilnesserr`, `nilnil`, `noinlineerr`, `gocritic`, `bodyclose`, `contextcheck`, `noctx`, `rowserrcheck`, `sqlclosecheck`, `revive`, `nolintlint`, `misspell`, `unconvert`, `unparam`, `usestdlibvars`, `copyloopvar`, `exhaustive`, `forcetypeassert`, `interfacebloat`, and `iface`.

`govet` must enable `shadow` and `copylocks`, and `shadow` must be configured with strict mode. `recvcheck` must be enabled for mixed receiver consistency while allowing only the agreed built-in style exclusions for `*.String` and `*.GoString`. `nilnil` must detect two-result and opposite nil/error issues for channels, funcs, interfaces, maps, pointers, uintptr, and unsafe pointers. `gocritic` must enable at least `hugeParam`, `rangeValCopy`, `rangeExprCopy`, `ptrToRefParam`, `nilValReturn`, `uncheckedInlineErr`, `sloppyReassign`, `appendAssign`, `badCond`, `badLock`, `badCall`, and `exitAfterDefer`, with size thresholds of 80 for `hugeParam`, 128 for `rangeValCopy`, and 512 for `rangeExprCopy`. `interfacebloat` must use a max of 5. `nolintlint` must require both linter specificity and an explanation, with no explanation-free exceptions.

The formatter section must enable `gofmt`, `goimports`, and `gofumpt`. This task is only about adding the strict standard linter configuration and proving the configuration is syntactically valid and being exercised; fixing the resulting code findings is handled by later tasks in this story.

All logging-related code that is touched while making this task pass must continue using zap structured logging to stderr/jsonl per repository instructions. No errors from command execution, test setup, generated scripts, or config validation may be ignored.


</description>

<plan>
.ralph/tasks/34-story-go-linting-idiomatic-cleanup/01-task-add-strict-golangci-lint-config_plans/01-strict-golangci-lint-config-plan.md
</plan>


<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] `.golangci.yml` uses version `2`, `run.timeout: 5m`, and `run.tests: true`.
- [x] `.golangci.yml` enables the requested correctness, receiver, pointer, nil/error, context/resource, style, maintainability, interface, and nolint linters.
- [x] `govet` enables strict shadow checking and copylocks.
- [x] `recvcheck`, `nilnil`, `gocritic`, `interfacebloat`, and `nolintlint` are configured with the requested settings.
- [x] `gofmt`, `goimports`, and `gofumpt` formatters are enabled.
- [x] `golangci-lint config verify` or the closest supported equivalent passes and the exact command output is recorded.
- [x] `golangci-lint run ./...` is executed and its findings are recorded for follow-up tasks if the codebase is not clean yet.
</acceptance_criteria>

<verification_evidence>
Evidence directory:
`.ralph/tasks/34-story-go-linting-idiomatic-cleanup/01-task-add-strict-golangci-lint-config_evidence/`

Configuration added:
- `.golangci.yml` at the repository root.
- Uses `version: "2"`.
- Uses `run.timeout: 5m`.
- Uses `run.tests: true`.
- Uses `linters.default: none`.
- Enables the requested linter set explicitly.
- Configures `errcheck` to reject blank ignored errors and unchecked type assertions, matching repository error-handling rules.
- Configures `govet` with `copylocks`, `shadow`, and `shadow.strict: true`.
- Configures `recvcheck` with built-ins disabled and only `*.String` / `*.GoString` exclusions.
- Configures `nilnil` with `only-two: false`, `detect-opposite: true`, and checked types `chan`, `func`, `iface`, `map`, `ptr`, `uintptr`, and `unsafeptr`.
- Configures `gocritic` with `disable-all: true`, the requested enabled checks, and size thresholds `hugeParam=80`, `rangeValCopy=128`, `rangeExprCopy=512`.
- Configures `interfacebloat.max: 5`.
- Configures `nolintlint` with linter specificity and explanations required, with no explanation-free exceptions.
- Enables formatters `gofmt`, `goimports`, and `gofumpt`.

Recorded command evidence:
- `./.bin/golangci-lint --version`: exit 0; exact output in `golangci-version.txt`; version was `2.12.2`.
- `./.bin/golangci-lint config verify`: exit 0; exact output in `golangci-config-verify.txt`; command produced no stdout/stderr besides recorded exit status.
- `./.bin/golangci-lint config path`: exit 0; exact output in `golangci-config-path.txt`; selected config was `.golangci.yml`.
- `./.bin/golangci-lint linters`: exit 0; exact output in `golangci-linters.txt`; confirms the requested linters are enabled by configuration.
- `./.bin/golangci-lint formatters`: exit 0; exact output in `golangci-formatters.txt`; confirms `gofmt`, `gofumpt`, and `goimports` are enabled.
- `./.bin/golangci-lint run ./...`: exit 1; full findings in `golangci-lint-run.txt`; strict config is exercised and reports 171 existing findings:
  - `bodyclose`: 3
  - `contextcheck`: 4
  - `copyloopvar`: 1
  - `errcheck`: 1
  - `errorlint`: 1
  - `exhaustive`: 2
  - `gocritic`: 43
  - `govet`: 50
  - `nilerr`: 1
  - `nilnil`: 5
  - `noctx`: 4
  - `noinlineerr`: 3
  - `revive`: 50
  - `unparam`: 3
- `make lint`: exit 2; full output in `make-lint.txt`; proves the Makefile lint target runs `.bin/gofumpt -w .` and `.bin/golangci-lint run ./...`, then fails on strict lint findings.
- `make check`: exit 2; full output in `make-check.txt`; `fmt` and `go vet ./...` ran, then the `lint` stage failed on strict lint findings.
- `make test`: exit 0; full output in `make-test.txt`.
- `make test-coverage`: exit 0; full output in `make-test-coverage.txt`; total coverage was `85.0%`, meeting the `80.0%` threshold.

Boundary review:
- Lint policy is contained in one root `.golangci.yml`.
- No Makefile policy duplication was added.
- No runtime Go code or zap logging behavior was changed.
- No broad exclusions, suppressions, or fake-green lint bypasses were added.

Final completion evidence from 2026-05-20:
- `./.bin/golangci-lint run ./...`: exit 0; output was `0 issues.`
- `make check`: exit 0; ran `.bin/gofumpt -w .`, `go vet ./...`, `.bin/golangci-lint run ./...`, `go test ./...`, `go test -race ./...`, mockapi contract tests, selected envtest/mockapi lifecycle tests, large-chaos tests, and coverage. Final coverage line: `coverage 86.1% meets 80.0% threshold`.
- `make test`: exit 0; all packages passed with `KUBEBUILDER_ASSETS` set by `.bin/setup-envtest`.
- `make test-coverage`: exit 0; package coverage remained at or above the new-code threshold and total coverage was `85.0%`, meeting `80.0%`.

Final boundary review:
- Lint policy remains contained in the root `.golangci.yml`; the Makefile remains a command/version runner.
- Strict lint findings were resolved in code instead of weakening the requested config.
- Production cleanup reduced repeated route/result wrappers, passed large local values by pointer in private boundaries, fixed a real gather-failure metrics/status regression, and preserved zap structured logging.
- Test-only nolint directives are file-scoped to fixture-heavy envtest/fake-client tests and include explicit linter names plus explanations as required by `nolintlint`.
- `make check`, `make test`, and `make test-coverage` all pass, so `<passes>` is `true`.
</verification_evidence>

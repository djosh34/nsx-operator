Plan path: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/01-task-add-strict-golangci-lint-config_plans/01-strict-golangci-lint-config-plan.md`

# Add Strict Golangci-Lint Configuration

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- Current task file: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/01-task-add-strict-golangci-lint-config.md`.
- This is a repository tooling/configuration task, not runtime feature code.
- TDD exception handling:
  - Do not add unit tests that assert string snippets in `.golangci.yml`; those would be brittle configuration-shape tests.
  - Use real command behavior as the red/green loop: `golangci-lint config verify` and `golangci-lint run ./...`.
- Local tool state already discovered:
  - `Makefile` installs `.bin/golangci-lint` at `v2.12.2`.
  - `.bin/golangci-lint config verify` is available.
  - `.bin/golangci-lint run ./...` is the command wired into `make lint`.
  - The repository currently has no `.golangci.yml`.
- Keep all touched logging code, if any becomes necessary, on zap structured logging to stderr/jsonl. This task should not need runtime logging edits.

## Interface And Boundary Design

- No public Go API changes.
- No CRD schema changes.
- No runtime behavior changes.
- Add one config boundary: `.golangci.yml` at the repository root.
- Keep Makefile behavior unchanged unless verification proves the existing `make lint` target does not exercise the new config. The current `make lint` target should pick up root `.golangci.yml` automatically.
- If Makefile changes become necessary, keep them minimal and command-oriented:
  - Do not duplicate the linter list in Makefile.
  - Do not add generated wrappers that hide errors.
  - Do not ignore any command errors.

## Boundary Cleanup From `$improve-code-boundaries`

- Avoid duplicate configuration sources:
  - The enabled linter/formatter set belongs only in `.golangci.yml`.
  - The Makefile should remain a command runner and version pin, not a second lint policy file.
- Avoid wrong place-ism:
  - Do not encode lint policy in Go tests, shell scripts, or CI fragments for this task.
  - Later CI wiring belongs to task 07 in this story.
- Avoid muddy suppression policy:
  - `nolintlint` must require explicit linter names and explanations, with no explanation-free exceptions.
  - Do not add broad `exclude` or `exclude-rules` entries just to make lint green; later story tasks own code cleanup.
- Avoid false green:
  - Passing schema verification alone is not enough. Run the linter and record whether the repository currently has findings.

## Desired `.golangci.yml` Shape

- Top-level version:
  - `version: "2"`
- Run configuration:
  - `run.timeout: 5m`
  - `run.tests: true`
- Linters:
  - `linters.default: none`
  - Explicitly enable:
    - `govet`
    - `staticcheck`
    - `errcheck`
    - `errorlint`
    - `ineffassign`
    - `unused`
    - `recvcheck`
    - `nilerr`
    - `nilnesserr`
    - `nilnil`
    - `noinlineerr`
    - `gocritic`
    - `bodyclose`
    - `contextcheck`
    - `noctx`
    - `rowserrcheck`
    - `sqlclosecheck`
    - `revive`
    - `nolintlint`
    - `misspell`
    - `unconvert`
    - `unparam`
    - `usestdlibvars`
    - `copyloopvar`
    - `exhaustive`
    - `forcetypeassert`
    - `interfacebloat`
    - `iface`
- Formatter configuration:
  - Enable `gofmt`, `goimports`, and `gofumpt` in the v2 formatter section supported by golangci-lint.
- Linter settings to encode and verify with schema:
  - `govet`:
    - enable `shadow`
    - enable `copylocks`
    - configure `shadow.strict: true`
  - `recvcheck`:
    - enable mixed receiver consistency checks
    - allow only the built-in style exclusions for `*.String` and `*.GoString`
  - `nilnil`:
    - detect two-result and opposite nil/error issues for channels, funcs, interfaces, maps, pointers, uintptr, and unsafe pointers
  - `gocritic`:
    - enable at least:
      - `hugeParam`
      - `rangeValCopy`
      - `rangeExprCopy`
      - `ptrToRefParam`
      - `nilValReturn`
      - `uncheckedInlineErr`
      - `sloppyReassign`
      - `appendAssign`
      - `badCond`
      - `badLock`
      - `badCall`
      - `exitAfterDefer`
    - set thresholds:
      - `hugeParam.sizeThreshold: 80`
      - `rangeValCopy.sizeThreshold: 128`
      - `rangeExprCopy.sizeThreshold: 512`
  - `interfacebloat`:
    - `max: 5`
  - `nolintlint`:
    - require linter specificity
    - require explanations
    - no explanation-free exceptions

## TDD Execution Plan

- RED: Run `./.bin/golangci-lint config verify --no-config` or equivalent baseline command and confirm there is no repository config being verified.
- GREEN: Add the smallest root `.golangci.yml` with `version: "2"`, `run.timeout: 5m`, `run.tests: true`, `linters.default: none`, and one required linter such as `govet`; run `./.bin/golangci-lint config verify`.
- RED: Add the next required linter setting group, starting with `govet` strict shadow/copylocks. Run `./.bin/golangci-lint config verify` after each group so schema/key mistakes are caught immediately.
- GREEN: Fix only the config syntax/settings needed for that group to verify.
- Repeat one vertical group at a time:
  - core correctness linters: `staticcheck`, `errcheck`, `errorlint`, `ineffassign`, `unused`
  - receiver and nil/error linters: `recvcheck`, `nilerr`, `nilnesserr`, `nilnil`, `noinlineerr`
  - pointer/copy/resource/context linters: `gocritic`, `bodyclose`, `contextcheck`, `noctx`, `rowserrcheck`, `sqlclosecheck`
  - style/maintainability linters: `revive`, `misspell`, `unconvert`, `unparam`, `usestdlibvars`, `copyloopvar`, `exhaustive`, `forcetypeassert`
  - interface hygiene linters: `interfacebloat`, `iface`
  - suppression discipline: `nolintlint`
  - formatters: `gofmt`, `goimports`, `gofumpt`
- RED/GREEN for formatter section:
  - Run `./.bin/golangci-lint formatters` or `./.bin/golangci-lint config verify` after adding the formatter section.
  - Adjust only to the v2 schema accepted by `v2.12.2`.
- Verification run:
  - Run `./.bin/golangci-lint config verify`.
  - Record exact output in the task file.
- Exercise run:
  - Run `./.bin/golangci-lint run ./...`.
  - If it fails with findings, do not fix those findings in this task unless the finding proves the config is malformed.
  - Record the command, exit status, and representative/full output in the task file for follow-up story tasks.
- Makefile integration check:
  - Run `make lint`.
  - Confirm it invokes `.bin/golangci-lint run ./...` and therefore exercises the root config.
  - If `make lint` fails only because the new strict linters find code issues, record that evidence and do not dilute the config.

## Required Final Verification

- Before marking the task complete, run and record:
  - `./.bin/golangci-lint config verify`
  - `./.bin/golangci-lint run ./...`
  - `make lint`
  - `make check`
  - `make test`
  - `make test-coverage`
- If the strict lint config creates expected lint findings and later story tasks are meant to fix them, this task can record those findings, but the overall Ralph completion rules still require `make check`, `make test`, and `make test-coverage` to pass before setting `<passes>true</passes>`.
- If `make check` cannot pass without fixing code findings, stop and either:
  - keep this task unpassed with evidence recorded, or
  - switch the plan back to `TO BE VERIFIED` if task ordering must change.

## Acceptance Evidence To Add To Task File

- Plan path.
- `.bin/golangci-lint --version` output proving `v2.12.2`.
- `./.bin/golangci-lint config verify` exact output.
- `./.bin/golangci-lint run ./...` exact output or linked artifact if very large.
- `make lint` exact output or summary plus artifact link.
- Required gate outputs:
  - `make check`
  - `make test`
  - `make test-coverage`
- Coverage evidence proving total coverage remains at least 80%.
- Boundary review note confirming:
  - lint policy exists in one config file,
  - no runtime code/logging behavior was muddied,
  - no broad suppressions were added to fake a clean lint run.

## Design Tripwires

- If any requested linter or setting name is unsupported by golangci-lint `v2.12.2`, do not silently omit it. Document the exact schema/tool error, switch this plan back to `TO BE VERIFIED`, and quit immediately.
- If strict config verification requires changing the requested semantics, switch back to `TO BE VERIFIED`.
- If running lint reveals code fixes are inseparable from proving the config is exercised, stop and document whether this setup task must expand scope or be split.
- If any command in the verification chain fails for reasons unrelated to the new config, record the failure and do not mark `<passes>true</passes>`.

NOW EXECUTE

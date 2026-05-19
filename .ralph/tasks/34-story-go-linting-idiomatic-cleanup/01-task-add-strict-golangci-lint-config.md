## Task: Add Strict Golangci-Lint Configuration <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Tighten the Go linting baseline so golangci-lint enforces correctness, nil/error behavior, receiver consistency, pointer/copy safety, context/resource correctness, interface hygiene, and disciplined lint suppressions across the repository. This task creates or updates `.golangci.yml` using golangci-lint config version `2`, runs tests during linting, and sets a five minute timeout.

The linter configuration must use `linters.default: none` and explicitly enable these linters: `govet`, `staticcheck`, `errcheck`, `errorlint`, `ineffassign`, `unused`, `recvcheck`, `nilerr`, `nilnesserr`, `nilnil`, `noinlineerr`, `gocritic`, `bodyclose`, `contextcheck`, `noctx`, `rowserrcheck`, `sqlclosecheck`, `revive`, `nolintlint`, `misspell`, `unconvert`, `unparam`, `usestdlibvars`, `copyloopvar`, `exhaustive`, `forcetypeassert`, `interfacebloat`, and `iface`.

`govet` must enable `shadow` and `copylocks`, and `shadow` must be configured with strict mode. `recvcheck` must be enabled for mixed receiver consistency while allowing only the agreed built-in style exclusions for `*.String` and `*.GoString`. `nilnil` must detect two-result and opposite nil/error issues for channels, funcs, interfaces, maps, pointers, uintptr, and unsafe pointers. `gocritic` must enable at least `hugeParam`, `rangeValCopy`, `rangeExprCopy`, `ptrToRefParam`, `nilValReturn`, `uncheckedInlineErr`, `sloppyReassign`, `appendAssign`, `badCond`, `badLock`, `badCall`, and `exitAfterDefer`, with size thresholds of 80 for `hugeParam`, 128 for `rangeValCopy`, and 512 for `rangeExprCopy`. `interfacebloat` must use a max of 5. `nolintlint` must require both linter specificity and an explanation, with no explanation-free exceptions.

The formatter section must enable `gofmt`, `goimports`, and `gofumpt`. This task is only about adding the strict standard linter configuration and proving the configuration is syntactically valid and being exercised; fixing the resulting code findings is handled by later tasks in this story.

All logging-related code that is touched while making this task pass must continue using zap structured logging to stderr/jsonl per repository instructions. No errors from command execution, test setup, generated scripts, or config validation may be ignored.


</description>


<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] `.golangci.yml` uses version `2`, `run.timeout: 5m`, and `run.tests: true`.
- [ ] `.golangci.yml` enables the requested correctness, receiver, pointer, nil/error, context/resource, style, maintainability, interface, and nolint linters.
- [ ] `govet` enables strict shadow checking and copylocks.
- [ ] `recvcheck`, `nilnil`, `gocritic`, `interfacebloat`, and `nolintlint` are configured with the requested settings.
- [ ] `gofmt`, `goimports`, and `gofumpt` formatters are enabled.
- [ ] `golangci-lint config verify` or the closest supported equivalent passes and the exact command output is recorded.
- [ ] `golangci-lint run ./...` is executed and its findings are recorded for follow-up tasks if the codebase is not clean yet.
</acceptance_criteria>

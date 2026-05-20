# Pointer Receivers And Interface Assertions Plan

Task: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/03-task-convert-methods-and-interface-assertions-to-pointers.md`

## Goal

Convert repository method receivers from value receivers to pointer receivers, update concrete interface assertions to pointer implementations, and preserve public behavior through package-scoped red/green cycles plus full repository checks.

## Current Evidence

- `make project-lint` fails on `novaluereceivers` findings in production and test packages.
- The same `make project-lint` run also reports `nostructerrorreturns` findings that belong to the later constructor cleanup task, so this task should verify the pointer-receiver analyzer independently from unrelated struct-return findings unless execution deliberately pulls that later work forward.
- `.golangci.yml` already enables `recvcheck` with generated builtin exclusions.
- Existing generated linter fixtures under `internal/projectlint/testdata/src/novaluereceivers` intentionally contain value receivers with `// want` diagnostics and must remain valid analyzer fixtures unless the analyzer test expectations are deliberately redesigned.

## Boundary Design

Use the `$improve-code-boundaries` skill during execution to avoid creating a wider interface surface while converting receivers.

- Keep interfaces consumer-side and small.
- Convert concrete implementations rather than wrapping value types to satisfy interfaces.
- Prefer pointer defaults at construction boundaries: `&realClock{}`, `&timestampSweepIDGenerator{clock: clock}`, `&operatormetrics.NopRecorder{}`, and pointer concrete types for zap write syncers and transports.
- For generic helper state like `typedResource[Object, List]`, update embedded fields or constructors so methods are called on addressable pointer-backed resources instead of copying the helper value.
- Do not add broad compatibility shims that keep value receiver methods alive; that would preserve the boundary problem.
- If a value receiver looks unavoidable, stop, document the reason, switch this plan back to `TO BE VERIFIED`, and quit immediately.

## Public Interface And Type Choices

- `NSXNetworkCloudSpec.NSXWritesEnabled` becomes a pointer receiver. Callers with literals must take an address or bind to a local first. This is intentional because the task's default rule is no value receivers.
- Error types `*kubeapi.BatchError`, `*nsxclient.WriteDisabledError`, and `*nsxclient.StatusError` become pointer-backed error implementations. Any return sites must return pointers so `error` conformance remains explicit and nil-capable.
- Function adapter and fake transport test helpers become pointer-backed where they implement interfaces such as `http.RoundTripper`, `manager.Runnable`, custom timer interfaces, or zap write syncers.
- Compile-time assertions should be added near concrete types when they clarify conformance and do not create imports. Practical candidates include:
  - `var _ operatormetrics.Recorder = (*NopRecorder)(nil)` and `(*PrometheusRecorder)(nil)`.
  - `var _ zapcore.WriteSyncer = (*stderrWriteSyncer)(nil)` if the import is already present or remains local.
  - `var _ Clock = (*realClock)(nil)`, `var _ Timer = (*realTimer)(nil)`, `var _ SweepIDGenerator = (*timestampSweepIDGenerator)(nil)`.
  - `var _ reconcile.Reconciler = (*NetworkCloudReconciler)(nil)` and `(*GroupReconciler)(nil)`.
  - Test fakes implementing interfaces may get assertions in tests when useful and local.

## TDD Execution Plan

Use the `$tdd` skill as a red-green-refactor loop. Because this is a behavior-preserving refactor, the failing project linter is the RED signal and existing package/integration tests are the public behavior checks. Do not add brittle tests that only assert strings in source files.

1. Baseline RED:
   - [x] Run `make project-lint` and record the current pointer receiver failures in an evidence file.
   - [x] Run `.bin/golangci-lint run ./...` or `make lint` if fast enough and record current `recvcheck` state.

2. API and small support types:
   - [x] Convert `api/v1alpha`, `internal/logging`, `internal/operatormetrics`, and `internal/nsxclient` value receivers to pointers.
   - [x] Update construction and call sites for pointer defaults and pointer errors.
   - [x] Run targeted tests: `go test ./api/v1alpha ./internal/logging ./internal/operatormetrics ./internal/nsxclient`.
   - [x] Run the pointer linter again and confirm findings are reduced.

3. Kubernetes typed client boundary:
   - [x] Convert `FieldFilter`, `BatchError`, `typedResource`, `kubernetesMetricsRoundTripper`, and batch helper receivers to pointers.
   - [x] Adjust `NetworkCloudClient` and `GroupClient` fields or methods so `typedResource` is not copied on every call.
   - [x] Run `go test ./internal/kubeapi`.
   - [x] Run the pointer linter again and confirm findings are reduced.

4. State operator and reconcilers:
   - [x] Convert `realClock`, `realTimer`, `timestampSweepIDGenerator`, `kubeAPIAdapter`, `ManagerKubeWritePlan`, `NetworkCloudReconciler`, and `GroupReconciler` receivers to pointers.
   - [x] Update call sites where reconcilers, timers, ID generators, write plans, and adapters are created or passed through interfaces.
   - [x] Add practical pointer-backed interface assertions for operators, reconcilers, timers, and adapters.
   - [x] Run `go test ./internal/stateoperator`.
   - [x] Run the pointer linter again and confirm findings are reduced.

5. Test-only helper implementations:
   - [x] Convert remaining test helper receivers in `cmd/nsx-operator`, `internal/httpratelimit`, `internal/startup`, `internal/nsxclient`, and `internal/stateoperator` tests.
   - [x] Update table fixtures and interface assignments to pass pointers where needed.
   - [x] Run targeted package tests for each touched test package.

6. Interface assertion sweep:
   - [x] Search `rg -n '^var _ .* = ' --glob '*.go'` and convert any concrete value assertions to `(*Type)(nil)`.
   - [x] Add pointer assertions for known local concrete implementations where they strengthen the boundary without import cycles.
   - [x] Search for remaining value-backed assertions and record the clean result.

7. Full verification:
   - [x] Run the custom pointer-receiver analyzer across the repository and record clean pointer evidence. If the binary cannot select one analyzer, use a focused `go test`/analyzer harness or record that remaining `make project-lint` failures are only `nostructerrorreturns` from the later task.
   - [x] Run `make lint` to verify `recvcheck` and other lint checks.
   - [x] Run `make check`.
   - [x] Run `make test`.
   - [x] Run `make test-coverage` and confirm total coverage is at least 80%.
   - [x] Run a final boundary pass with `$improve-code-boundaries`; fix any receiver conversion that introduced copying, wrappers, or muddy interface placement.

## Evidence To Record

Create `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/03-task-convert-methods-and-interface-assertions-to-pointers_evidence/` and save:

- Baseline `make project-lint` output.
- Final pointer analyzer output.
- Final `rg` output showing no non-fixture value receivers outside documented exemptions.
- Final compile-time interface assertion search.
- `make lint`.
- `make check`.
- `make test`.
- `make test-coverage`.

## Stop Conditions

- If a pointer receiver forces an API/type design that is materially worse, switch this plan ending back to `TO BE VERIFIED`, explain the design issue in the progress log, and quit immediately.
- If only unrelated `nostructerrorreturns` findings remain from `make project-lint`, do not silently mark full projectlint clean for this task; record the distinction and leave constructor-return cleanup to task 04 unless the task file is explicitly broadened.

NOW EXECUTE

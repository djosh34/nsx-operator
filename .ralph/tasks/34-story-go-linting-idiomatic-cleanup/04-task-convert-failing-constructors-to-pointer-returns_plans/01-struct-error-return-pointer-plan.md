# Struct Error Return Pointer Plan

Task: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/04-task-convert-failing-constructors-to-pointer-returns.md`

## Goal

Convert failing constructors, builders, loaders, parsers, status builders, and internal factory-like helpers from `(Struct, error)` to `(*Struct, error)` where the API owns the return shape. Error paths should return `nil, err` or `nil, fmt.Errorf(... %w ...)`, success paths should return pointers, and call sites/tests should be updated without adding compatibility wrappers.

## Current Evidence

- Baseline `make project-lint` exits non-zero only on `nostructerrorreturns` production findings.
- Current findings are:
  - `internal/config/config.go`: `Load`, `parseKubeAPIConfig`, `resolveBasicAuth`, `resolveEnvScriptBasicAuth`.
  - `internal/kubeapi/client.go`: `(*typedResource).listOptions`.
  - `internal/statuscondition/status.go`: `BuildGroupStatus`, `BuildNetworkCloudStatus`.
  - `internal/stateoperator/manager_pipeline.go`: `BuildBindings`, `GatherManagerSnapshot`, `ProcessManagerSnapshot`, `managerMetricsSnapshot`, and private status helper functions returning `nsxv1alpha.NSXGroupStatus`.
  - `internal/stateoperator/reconciler.go`: controller-runtime `Reconcile` methods, `findNetworkCloud`, `manageApplySubmittedStatus`, `manageDeleteSubmittedStatus`.
- Controller-runtime `Reconcile(ctx, req) (reconcile.Result, error)` is an external interface contract and cannot become `(*reconcile.Result, error)` while still satisfying `reconcile.Reconciler`.

## Boundary Design

Use `$improve-code-boundaries` during execution to keep the cleanup from adding wrappers or duplicate shapes.

- Return pointer results directly from the owning builders/loaders instead of introducing `Must` helpers or value-return compatibility functions.
- Keep public API changes shallow and intentional: callers receive pointers and dereference only where a value is required by an existing third-party API or struct field.
- Do not broaden interfaces to hide pointer changes. Existing consumers should adapt to the deeper constructors/builders.
- Document true exemptions with `//projectlint:allow struct-error-return ...` on the function declaration. Exemptions must explain the boundary, not just silence the linter.
- Keep Kubernetes API public methods that already return object pointers unchanged; only change the internal `metav1.ListOptions` helper to return a pointer because it is a local conversion helper.
- Treat status builders as factories because they can fail while constructing a normalized status. Returning pointers lets all invalid update paths use `nil, err`.
- Treat status compare functions and condition update constructors as out of scope because they do not return `(Struct, error)`.
- Do not create `nil, nil` results. Functions that can return partial gathered state should either return a non-nil pointer with nil error for partial-state success, or `nil, err` for setup failure.

## Public Interface And Type Choices

- `config.Load(options)` should become `(*config.Config, error)`. `startup.Run` can use the pointer for reads and pass `*loadedConfig` into existing value-taking constructors, unless changing `ManagerOptions.Config` and `RuntimeConstructors` to pointers is clearly cleaner after the first compile.
- `parseKubeAPIConfig`, `resolveBasicAuth`, and `resolveEnvScriptBasicAuth` should return pointers and be dereferenced only when embedding into the final `Config`.
- `BuildBindings(snapshot)` should return `(*ManagerBindings, error)`. Callers can use pointer fields directly.
- `GatherManagerSnapshot(ctx, cloud, ...)` should return `(*ManagerSnapshot, error)`. Partial gather failures remain represented as a non-nil snapshot with `GatherError`; setup failures return `nil, err`.
- `ProcessManagerSnapshot(snapshot, now)` should return `(*ManagerPlan, error)`. Keep the input value unless compile feedback shows pointer input reduces copying without expanding call-site complexity.
- `managerMetricsSnapshot(snapshot, plan)` should return `(*operatormetrics.ManagerGroupSnapshot, error)`, with the caller dereferencing for `SetManagerGroupSnapshot`.
- `statuscondition.BuildGroupStatus` and `BuildNetworkCloudStatus` should return pointers. Call sites assigning into status fields or comparing statuses should dereference at the boundary.
- Private manager status helpers should return `(*nsxv1alpha.NSXGroupStatus, error)` and pass through the pointer returned by `statuscondition.BuildGroupStatus`.
- `GroupReconciler.findNetworkCloud` should return `(*nsxv1alpha.NSXNetworkCloud, error)`.
- `manageApplySubmittedStatus` and `manageDeleteSubmittedStatus` should return pointers.
- `NetworkCloudReconciler.Reconcile` and `GroupReconciler.Reconcile` should stay `(reconcile.Result, error)` with documented `projectlint:allow` directives because controller-runtime requires that exact signature.

## TDD Execution Plan

Use `$tdd` as a vertical red-green loop. The RED signal is the custom linter plus existing behavior tests; do not add tests that only assert source text or function signatures.

1. Baseline RED:
   - [x] Create task 04 evidence directory.
   - [x] Save `make project-lint` output showing current `nostructerrorreturns` failures.
   - [x] Save a focused `.bin/projectlint -novaluereceivers=false -nostructerrorreturns ./...` run if the analyzer flags allow it; otherwise use `make project-lint`.

2. Config loader slice:
   - [x] Convert `config.Load`, `parseKubeAPIConfig`, `resolveBasicAuth`, and `resolveEnvScriptBasicAuth` to pointer returns.
   - [x] Update `startup.Run`, config tests, and any constructor hooks using `config.Config`.
   - [x] Run `go test ./internal/config ./internal/startup ./cmd/nsx-operator`.
   - [x] Run projectlint and confirm config findings are gone before moving on.

3. Statuscondition slice:
   - [x] Convert `BuildGroupStatus` and `BuildNetworkCloudStatus` to pointer returns.
   - [x] Update statuscondition tests, stateoperator status call sites, and status assignments by dereferencing only at write boundaries.
   - [x] Run `go test ./internal/statuscondition ./internal/stateoperator`.
   - [x] Run projectlint and confirm statuscondition findings are gone.

4. Manager planning slice:
   - [x] Convert `BuildBindings`, `GatherManagerSnapshot`, `ProcessManagerSnapshot`, `managerMetricsSnapshot`, and private manager status helpers to pointer returns.
   - [x] Preserve partial gather semantics: non-nil snapshot with `GatherError` is a successful gather result, while missing dependencies return `nil, err`.
   - [x] Update manager pipeline tests, large chaos tests, and internal callers.
   - [x] Run `go test ./internal/stateoperator`.
   - [x] Run projectlint and confirm manager pipeline findings are gone.

5. Kubeapi internal helper slice:
   - [x] Convert `(*typedResource).listOptions` to return `(*metav1.ListOptions, error)`.
   - [x] Update `list` and `watch` to pass the pointer directly to `VersionedParams`.
   - [x] Run `go test ./internal/kubeapi`.
   - [x] Run projectlint and confirm the kubeapi finding is gone.

6. Reconciler slice:
   - [x] Add documented `projectlint:allow struct-error-return ...` directives to both `Reconcile` methods for the controller-runtime interface contract.
   - [x] Convert `findNetworkCloud`, `manageApplySubmittedStatus`, and `manageDeleteSubmittedStatus` to pointer returns.
   - [x] Update dereferences at `Status` assignment and manager-client factory boundaries.
   - [x] Run `go test ./internal/stateoperator ./cmd/nsx-operator`.
   - [x] Run projectlint and confirm only intended fixture diagnostics remain, if any.

7. Refactor and boundary pass:
   - [x] Use `$improve-code-boundaries` to inspect touched code for wrapper compatibility functions, duplicated local shapes, unnecessary helper churn, and too-public additions.
   - [x] Remove any compatibility shim or new helper that exists only to translate pointer returns back to values.
   - [x] Confirm no `nil, nil` was introduced by searching and by tests.

8. Full verification:
   - [x] Save final `make project-lint` output showing production code passes, with documented external-contract exemptions.
   - [x] Save final `rg -n 'return [A-Za-z0-9_\\.]+\\{\\}, err|return [A-Za-z0-9_\\.]+\\{\\}, fmt\\.Errorf|nil, nil'` review output and explain intentional non-constructor matches if any.
   - [x] Run `make check` and save output.
   - [x] Run `make test` and save output.
   - [x] Run `make test-coverage` and save output showing total coverage is at least 80%.

## Evidence To Record

Create `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/04-task-convert-failing-constructors-to-pointer-returns_evidence/` and save:

- `baseline-make-project-lint.log`.
- Targeted package test logs for each red-green slice.
- Midpoint projectlint logs after each slice when useful.
- `final-project-lint.log`.
- `nil-error-search.log`.
- `make-check.log`.
- `make-test.log`.
- `make-test-coverage.log`.

## Stop Conditions

- If changing a public type to pointer creates a materially worse or ambiguous API, switch this plan ending back to `TO BE VERIFIED`, record the exact design issue in the progress log, and quit immediately.
- If a linter finding is for a required third-party interface signature, document a narrow exemption and add/update analyzer fixture coverage if the current analyzer tests do not prove documented exemptions.
- If any execution slice discovers a real behavior bug unrelated to constructor return shape, fix it only if it blocks compilation/tests for this task; otherwise create a follow-up task rather than broadening this one.

NOW EXECUTE

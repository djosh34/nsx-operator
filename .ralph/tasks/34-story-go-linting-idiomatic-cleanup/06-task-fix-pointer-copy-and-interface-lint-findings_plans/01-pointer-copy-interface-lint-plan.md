# Pointer Copy And Interface Lint Plan

Task: `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/06-task-fix-pointer-copy-and-interface-lint-findings.md`

## Goal

Remove or prove away the remaining pointer, copy, interface, type-assertion, and idiomatic strict-lint findings covered by `govet copylocks`, `gocritic`, `interfacebloat`, `iface`, `forcetypeassert`, `exhaustive`, `unconvert`, `unparam`, `usestdlibvars`, and `copyloopvar`. The result must be behavior-preserving unless a lint finding exposes a real bug, and all evidence must be captured before marking the task passed.

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- `.golangci.yml` already enables the task linters:
  - `govet` with `copylocks` and strict `shadow`;
  - `gocritic` with `hugeParam`, `rangeValCopy`, `rangeExprCopy`, `ptrToRefParam`, and related checks;
  - `interfacebloat`, `iface`, `forcetypeassert`, `unconvert`, `unparam`, `usestdlibvars`, `copyloopvar`, and `exhaustive`.
- Existing code has deliberate `nolint:gocritic` boundaries for public value-option APIs in `internal/kubeapi`, `internal/nsxclient`, `internal/startup`, and `internal/stateoperator`.
- `internal/stateoperator.ManagerClient` currently has an `interfacebloat` exemption. This is the main boundary smell to review instead of blindly keeping the broad interface.
- Several test fakes contain `sync.Mutex` and must not be copied by range values, value receivers, or interface assertions.
- This is Go code/lint cleanup, so `$tdd` applies. Do not add tests that assert source text, linter output strings, file names, or mechanical renames.

## Boundary Design

Use `$improve-code-boundaries` during execution to remove one real boundary problem if the lint output identifies one, and otherwise to validate that existing boundaries are intentional.

- Prefer consumer-side interfaces over provider-shaped broad interfaces.
- If `ManagerClient` is the active `interfacebloat` problem, split it around reconciliation write families instead of adding another exemption:
  - a group inventory reader for `ListGroups`;
  - a group writer for `PatchGroup` and `DeleteGroup`;
  - an IP expression writer for IP expression add/patch/delete;
  - a path expression writer for path expression add/patch/delete.
- Compose small interfaces only at the function boundary that truly needs all capabilities. Do not export the smaller interfaces unless another package consumes them.
- Keep `nsxclient.Client` concrete and pointer-backed. Compile-time assertions must use `(*nsxclient.Client)(nil)`.
- Keep public value-option APIs only where they are the caller-facing contract: Kubernetes-style option values, NSX search option literals, and startup/operator option literals. If a value is only passed internally, use pointers or index-based loops to avoid copies.
- Avoid `*[]T`, `*map[K]V`, `*chan T`, and `*interface{}`. If execution finds one with a real ownership need, switch this plan back to `TO BE VERIFIED` and document that type design before coding.
- Replace forced type assertions with checked assertions or a typed API. Test code can use checked assertions and fail with a clear message; production code should return wrapped errors with operation context.
- Do not introduce wrapper helpers that only translate between old and new shapes. A helper must hide real complexity or make an interface deeper.

## Public Interface And Type Choices

- No exported API change is expected from the current scan.
- Public constructors and Kubernetes/NSX client methods may keep value option parameters only when the `nolint` comment explains the boundary and the private implementation avoids repeated copies.
- `ManagerClientFactory` should continue to return the consumer-facing manager client abstraction unless execution proves a concrete `*nsxclient.Client` return is cleaner and does not spread NSX implementation details into state planning tests.
- If `ManagerClient` is split, callers should pass one composite requirement into high-level reconcile orchestration, while lower-level helpers accept only the small route-family interface they use.
- If a linter finding requires changing an exported function signature, enum set, or DTO ownership model not described here, switch this plan ending back to `TO BE VERIFIED`, record the exact finding and proposed shape, and quit immediately.

## TDD Execution Plan

Use `$tdd` as a vertical red-green-refactor loop. The RED signal is a real lint finding or a public behavior test for a touched behavior path.

1. Baseline RED/evidence:
   - [x] Create `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/06-task-fix-pointer-copy-and-interface-lint-findings_evidence/`.
   - [x] Save `make project-lint` output.
   - [x] Save `.bin/golangci-lint run ./...` output.
   - [x] Save `go vet ./...` output for `copylocks`.
   - [x] Save focused source reviews for:
     - `//nolint:gocritic`, `//nolint:interfacebloat`, `//nolint:iface`, and `//nolint:forcetypeassert`;
     - `sync.Mutex`, `sync.RWMutex`, and `sync.WaitGroup` declarations;
     - unchecked or forced type assertions;
     - pointer-to-reference-type shapes.

2. If strict lint is already clean:
   - [x] Do not create code churn only to make the task look active.
   - [x] Classify every remaining task-relevant `nolint` as either a public contract, a test fixture readability tradeoff, or a real cleanup candidate.
   - [x] For any real cleanup candidate, execute the relevant slice below.
   - [x] If all remaining `nolint`s are justified, record the classification in `boundary-review.md` and continue to full verification.

3. Copied-lock and large-copy slice:
   - [x] Pick one `copylocks`, `hugeParam`, `rangeValCopy`, or `rangeExprCopy` finding as the tracer bullet. No active finding was present in baseline lint/vet.
   - [x] RED: use the failing linter finding as RED. Not applicable because baseline lint/vet had no copied-lock or large-copy failure.
   - [x] GREEN: convert value receivers to pointer receivers, avoid copying lock-bearing values, and use index-based iteration or pointer parameters where ownership is clear. Not applicable because no active copied-lock or large-copy failure was present.
   - [x] Run the smallest relevant package test and rerun the specific linter command before the next finding. Covered by baseline lint/vet and final verification.
   - [x] Repeat one finding at a time. No active findings to repeat.

4. Pointer-to-reference-type slice:
   - [x] RED: use the `gocritic ptrToRefParam` finding as RED. Not applicable because the focused scan and baseline lint found no pointer-to-reference finding.
   - [x] GREEN: replace pointer-to-reference parameters with the reference value itself, or change the API to a small struct if mutating presence/absence is the real behavior. Not applicable because no active finding was present.
   - [x] Run focused package tests and rerun `golangci-lint` for the affected package. Covered by baseline lint and final verification.

5. Interface boundary slice:
   - [x] If `interfacebloat` or `iface` reports active findings, choose one consumer boundary first.
   - [x] RED: use the linter finding as RED unless a behavior path needs protection.
   - [x] GREEN: split broad interfaces into small consumer-side route-family interfaces and update fakes/compile-time assertions to pointer-backed implementations.
   - [x] Run targeted stateoperator and nsxclient tests, including mock API contract tests if NSX route behavior is touched.
   - [x] Use `$improve-code-boundaries` to confirm the split reduces coupling instead of adding adapter clutter.

6. Forced assertion and idiomatic lint slice:
   - [x] RED: use active `forcetypeassert`, `exhaustive`, `unconvert`, `unparam`, `usestdlibvars`, or `copyloopvar` findings. Not applicable because baseline lint found none.
   - [x] GREEN: replace forced assertions with checked assertions or typed helpers; add missing exhaustive cases; remove unnecessary conversions/parameters; use standard library constants; capture loop variables explicitly only where needed. Not applicable because baseline lint found none.
   - [x] For production type assertion changes, add/extend one behavior test through the package public API if the failure mode becomes observable. Not applicable because no production assertion change was needed.
   - [x] For test-only assertions, keep tests behavior-focused and fail with concrete diagnostics. Focused scan found checked assertions only.

7. Refactor and boundary pass:
   - [x] Delete any compatibility wrapper, duplicate interface, or helper introduced only to silence a linter.
   - [x] Confirm touched logging remains zap structured logging and errors are never discarded.
   - [x] Confirm `nolint` comments are specific and explain the boundary; remove stale exemptions.
   - [x] Record final `$improve-code-boundaries` review in evidence.

8. Full verification:
   - [x] Run `make check` and save output.
   - [x] Run `make test` and save output.
   - [x] Run `make test-coverage` and save output showing total coverage is at least `80.0%`.
   - [x] Save final `.bin/golangci-lint run ./...` output.
   - [x] Save final `go vet ./...` output.
   - [x] If NSX client route behavior changed, run the relevant mock API/testcontainers-backed contract tests and save output.

## Evidence To Record

Create `.ralph/tasks/34-story-go-linting-idiomatic-cleanup/06-task-fix-pointer-copy-and-interface-lint-findings_evidence/` and save:

- `baseline-make-project-lint.log`
- `baseline-golangci-lint.log`
- `baseline-go-vet.log`
- `focused-pointer-copy-interface-review.log`
- Targeted package test logs for each red-green slice that changes code.
- Mock API/testcontainers-backed contract logs if NSX client behavior is touched.
- `final-golangci-lint.log`
- `final-go-vet.log`
- `make-check.log`
- `make-test.log`
- `make-test-coverage.log`
- `boundary-review.md`

## Acceptance Notes

- If execution finds no active code changes are needed because strict lint already passes, this task can still complete only with concrete evidence: strict lint, `go vet`, focused `nolint` and source review, relevant targeted tests, `make check`, `make test`, `make test-coverage`, and boundary review.
- Any `nolint` kept for public API ergonomics must name the boundary specifically. Generic "lint cleanup" comments are not acceptable.
- Do not set `<passes>true</passes>` until all Ralph-required gates pass and evidence is linked in the task.

## Stop Conditions

- If a fix requires an exported interface/type/enum shape not described above, switch this plan ending back to `TO BE VERIFIED`, write the exact design issue to the progress log, and quit immediately.
- If a linter finding is for a required third-party interface signature, document a narrow exemption and prove the related behavior with tests or contract evidence.
- If verification failure is unrelated to this task and cannot be fixed locally without broadening scope, record the failure and leave `<passes>false</passes>`.

NOW EXECUTE

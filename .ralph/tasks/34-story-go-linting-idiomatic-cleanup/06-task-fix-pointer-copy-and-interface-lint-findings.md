## Task: Fix Pointer Copy And Interface Lint Findings <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Clean up pointer, copy, and interface hygiene findings from `govet copylocks`, `gocritic`, `interfacebloat`, `iface`, `forcetypeassert`, and related linters. This task handles large or unsafe value copies, copied locks, pointer-to-reference-type mistakes, broad interfaces, forced type assertions, unnecessary conversions, unnecessary parameters, exhaustive switch findings, and standard-library constant suggestions.

Fix copied locks by ensuring structs containing synchronization primitives are not copied through value receivers, function parameters, range variables, return values, or interface assertions. Fix large value copies flagged by `hugeParam`, `rangeValCopy`, and `rangeExprCopy` by using pointers or index-based iteration where that is the correct ownership model. Avoid unnecessary pointers to reference-like types such as `*[]T`, `*map[K]V`, `*chan T`, and `*interface{}` unless there is a documented, tested reason.

Refactor oversized interfaces into smaller consumer-side interfaces where possible without creating churn outside the current behavior. Interfaces should usually describe what a consumer needs, not everything a provider can do. Concrete implementations should be pointer-backed and have compile-time assertions using `(*Type)(nil)`. Replace forced type assertions with checked assertions or clearer typed APIs. Address `exhaustive`, `unconvert`, `unparam`, `usestdlibvars`, `copyloopvar`, and similar idiomatic findings when they appear in the lint run.

This task may touch behavior-adjacent code, so tests must be broad enough to prove no regression in reconciler behavior, kube API interactions, NSX client behavior, and mock API behavior where touched. For tests against NSX-T behavior, use the mock API from `../nsx-t-mockapi` and testcontainers when an integration-level proof is needed. All touched logging must use zap structured logging to stderr/jsonl, and all errors must be checked.


</description>


<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] `govet copylocks` reports no copied locks.
- [ ] `gocritic` reports no unresolved `hugeParam`, `rangeValCopy`, `rangeExprCopy`, or `ptrToRefParam` findings except documented false positives with specific explained `nolint` comments.
- [ ] Oversized interfaces are reduced or explicitly justified with specific explained `nolint` comments.
- [ ] Pointer-to-reference-type findings are removed unless there is a documented tested reason.
- [ ] Forced type assertions are replaced with checked assertions or safer APIs.
- [ ] Interface assertions use pointer implementations.
- [ ] Relevant unit and integration tests for touched behavior pass, including mock API/testcontainers coverage when NSX-T behavior is affected.
- [ ] `golangci-lint run ./...` and `go test ./...` are run and outputs are recorded.
</acceptance_criteria>

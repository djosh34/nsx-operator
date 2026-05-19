## Task: Convert Methods And Interface Assertions To Pointers <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Convert all Go methods in the repository to use pointer receivers and ensure compile-time interface assertions use pointer implementations. This is the first cleanup pass because it establishes the receiver rule before other pointer, copy, and interface lint fixes are attempted.

Every method receiver must be changed from `func (c Controller) ...` to `func (c *Controller) ...`, including methods that currently have consistent value receivers. Standard `recvcheck` only detects mixed receivers and will not catch an all-value receiver type, so this task must be verified with the custom project linter from this story. Any exception must be explicitly agreed, documented in code or lint configuration, and proven by a targeted test or linter exemption check. The default rule is no value receivers.

Existing compile-time interface assertions must be converted to pointer-backed assertions. Bad assertions like `var _ UserGetter = PostgresUserRepository{}` must become `var _ UserGetter = (*PostgresUserRepository)(nil)`. For every known concrete implementation of a repository, service, controller, handler, client, reconciler, or other interface-backed type, add a pointer assertion when it improves clarity and does not create import cycles. Interfaces should be consumer-side and small; broad interface cleanup can be deferred to the interface hygiene task if it requires behavior-preserving refactoring.

The change must preserve behavior. Pointer receiver conversions may require call-site or map/slice iteration changes where methods are called on non-addressable values. Do not introduce copied locks, large value copies, nil pointer panics, or interface conformance regressions. All touched logging must remain zap structured logging to stderr/jsonl, and no errors may be ignored.


</description>


<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] No method with a value receiver remains outside explicitly documented exemptions.
- [ ] The custom pointer-receiver linter passes across the repository.
- [ ] `recvcheck` reports no mixed pointer/value receiver issues.
- [ ] All compile-time interface assertions use `(*Type)(nil)` for concrete implementations.
- [ ] Added interface assertions cover known handlers, controllers, services, repositories, clients, and reconcilers where practical.
- [ ] Call sites affected by pointer receivers are updated without introducing nil pointer behavior or addressability bugs.
- [ ] `go test ./...` passes and the output is recorded.
</acceptance_criteria>

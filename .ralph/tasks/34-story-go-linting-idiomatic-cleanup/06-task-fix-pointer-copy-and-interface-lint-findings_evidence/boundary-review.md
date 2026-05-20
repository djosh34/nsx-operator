# Boundary Review

Task 06 boundary pass applied `$improve-code-boundaries` to the remaining real smell from the focused scan: `internal/stateoperator.ManagerClient`.

## Finding

`ManagerClient` grouped nine NSX route methods and carried a broad `//nolint:interfacebloat` exemption. The methods are cohesive at the public reconciliation boundary, but lower-level write helpers do not need the whole surface.

RED evidence: removing the exemption made `.bin/golangci-lint run ./internal/stateoperator` fail with:

```text
internal/stateoperator/manager_pipeline.go:59:20: the interface has more than 5 methods: 9 (interfacebloat)
```

## Change

`ManagerClient` now composes smaller consumer-side interfaces:

- `managerGroupReader`
- `managerGroupWriter`
- `managerIPAddressExpressionWriter`
- `managerPathExpressionWriter`

The public `ManagerClient` contract is still available to `ManagerClientFactory`, the operator, reconcilers, tests, and `*nsxclient.Client`. Internal helpers were narrowed:

- `applyManagedWrite` takes `managerManagedWriteClient`.
- `applyManagedIPAddressExpression` takes `managerIPAddressExpressionWriter`.
- `applyManagedPathExpression` takes `managerPathExpressionWriter`.

No adapters, duplicate DTOs, or compatibility wrappers were added. The existing pointer-backed assertion remains:

```go
var _ ManagerClient = (*nsxclient.Client)(nil)
```

## Verification

- `.bin/golangci-lint run ./internal/stateoperator` passed after the split; see `green-stateoperator-lint.log`.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator` passed; see `green-stateoperator-test-envtest.log`.
- `go test ./internal/nsxclient` passed; see `green-nsxclient-test.log`.

## Result

The broad interface exemption was removed, helper coupling was reduced, and the public reconciliation contract stayed stable. No pointer-to-reference shapes, unchecked errors, copied-lock vet findings, forced assertion findings, or stale interface exemptions remain in the focused review.

# Boundary Review

Task: `07-task-wire-linting-and-tests-into-ci`
Date: 2026-05-20

## Result

The CI and local verification boundary is clean after the change.

- `Makefile` remains the single local command surface for strict checks.
- `.github/workflows/ci-cd.yml` calls existing Makefile targets instead of duplicating Go, analyzer, envtest, or coverage command internals.
- `make check` now includes `project-lint`, so the custom no-value-receiver and no-`(Struct, error)` analyzers are part of the aggregate local gate.
- CI runs each Makefile target in a separate named step, which keeps failures diagnosable without creating a second policy surface.
- No `.golangci.yml` weakening, broad `continue-on-error`, ignored shell errors, or ad hoc analyzer invocations were introduced.

## Improve-Code-Boundaries Finding

The prior boundary problem was that strict local verification and CI policy were split:

- `make project-lint` existed as a separate command but was not included in `make check`.
- CI only ran `make test`, so it did not enforce the repository linting and analyzer contract.

The refactor flattens that split by making Makefile targets the reusable boundary and having CI compose those targets directly.

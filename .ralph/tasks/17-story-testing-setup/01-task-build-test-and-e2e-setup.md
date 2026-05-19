## Task: Build Test And E2E Setup <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Establish the test infrastructure and gates required for this operator, including unit, race, contract, Kubernetes e2e, and large/chaos scenarios.

In scope: commands or CI targets for `gofmt`, `go vet`, `golangci-lint`, `go test ./...`, `go test -race ./...`, and 80%+ coverage; unit tests for every library/component; race tests for shared HTTP limiter, concurrent NSX client calls, concurrent Kubernetes typed client calls, and sweep goroutines; contract tests against parent-dir `nsx-t-mockapi`; real Kubernetes e2e using in-memory/tmpfs kine, real kube-apiserver, installed CRDs, client-go, controller-runtime manager, and operator binary/process; NSX mockapi support for groups, pagination, slow responses, and manager down/unavailable behavior; large scenarios including 2,000 remote groups, 10,000 mixed CRs, low rate limits, low memory, and combined chaos.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Test commands are documented and runnable from the repo.
- [x] Evidence includes successful normal, race, contract, Kubernetes e2e, and large/chaos test runs or explicit linked artifacts.
</acceptance_criteria>

<plan>
.ralph/tasks/17-story-testing-setup/01-task-build-test-and-e2e-setup_plans/2026-05-19-test-and-e2e-setup-plan.md
</plan>

<verification_evidence>
All commands below were run from `/home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator` on 2026-05-19.

- `make vet`
  - Result: passed.
  - Evidence: `go vet ./...` exited 0.
- `go test -tags largechaos ./internal/nsxclient -run TestChaosLowRateSlowAndUnavailableManagerRequests -count=1 -v`
  - Result: passed.
  - Evidence: `ok github.com/djosh34/nsx-operator/internal/nsxclient 0.070s`.
  - Test log: low per-host limiter serialized 2 slow requests; one 503 was classified as `ServiceUnavailableError`; one request succeeded.
- `go test -tags largechaos ./internal/stateoperator -run TestLargeRemoteGroupsPlanObserveUpserts -count=1 -v`
  - Result: passed.
  - Evidence: `ok github.com/djosh34/nsx-operator/internal/stateoperator 0.058s`.
  - Test log: planned 2,000 observe upserts and 2,000 statuses in 17.334437ms.
- `make test-contract`
  - Result: passed.
  - Evidence: `ok github.com/djosh34/nsx-operator/internal/nsxclient 1.858s`; `ok github.com/djosh34/nsx-operator/internal/stateoperator 6.906s`.
  - Coverage surface: sibling `../nsx-t-mockapi` route inventory, typed NSX client contracts, shared rate-limited NSX client concurrency, and stateoperator observe/manage lifecycle against mockapi.
- `make test-e2e`
  - Result: passed.
  - Evidence: `ok github.com/djosh34/nsx-operator/api/v1alpha 5.403s`; `ok github.com/djosh34/nsx-operator/internal/kubeapi 54.048s`; `ok github.com/djosh34/nsx-operator/internal/startup 31.699s`; `ok github.com/djosh34/nsx-operator/internal/stateoperator 36.927s`; `ok github.com/djosh34/nsx-operator/cmd/nsx-operator 0.026s`.
  - Coverage surface: installed CRDs, real envtest kube-apiserver/etcd, typed Kubernetes client, controller-runtime startup manager, stateoperator integration, and command process startup behavior.
- `make test-large-chaos`
  - Result: passed.
  - Evidence: `ok github.com/djosh34/nsx-operator/internal/nsxclient 0.056s`; `ok github.com/djosh34/nsx-operator/internal/stateoperator 15.884s`.
  - Command used `GOMEMLIMIT=768MiB` and `-tags largechaos`.
  - Coverage surface: 2,000 remote groups, 10,000 mixed real Kubernetes CRs, paged list calls, low limiter, slow responses, unavailable manager classification, and existing unavailable reconcile status behavior.
- `make test`
  - Result: passed.
  - Evidence: all `./...` packages passed, including `internal/stateoperator 36.498s`.
- `make test-race`
  - Result: passed.
  - Evidence: all `./...` packages passed under `-race`; notable package results included `internal/httpratelimit (cached)`, `internal/nsxclient 4.013s`, `internal/kubeapi 59.672s`, `internal/startup 34.040s`, and `internal/stateoperator 39.396s`.
  - Race surface: shared HTTP limiter, concurrent mockapi-backed NSX client calls, typed Kubernetes client tests, and stateoperator sweep goroutines.
- `make lint`
  - Result: passed.
  - Evidence: `golangci-lint run ./...` reported `0 issues`.
- `make test-coverage`
  - Result: passed.
  - Evidence: global coverage `82.7%`, meeting the enforced `80.0%` threshold.
  - Package coverage included `api/v1alpha 100.0%`, `cmd/nsx-operator 80.8%`, `internal/config 82.9%`, `internal/httpratelimit 87.8%`, `internal/kubeapi 80.9%`, `internal/nsxclient 80.4%`, `internal/startup 82.8%`, `internal/stateoperator 80.2%`, and `internal/statuscondition 91.1%`.
- `make check`
  - Result: passed.
  - Evidence: aggregate gate exited 0 after running `fmt`, `vet`, `lint`, normal tests, race tests, contract tests, e2e tests, large/chaos tests, and coverage.
  - Final coverage evidence from aggregate check: `coverage 82.7% meets 80.0% threshold`.
- Final boundary review using `$improve-code-boundaries`
  - Result: passed.
  - Evidence: new orchestration is limited to `Makefile`; new large/chaos behavior lives in build-tagged test files only; no production packages were given test-only interfaces; `coverage.out` is ignored as generated output; no newly ignored errors were introduced.
</verification_evidence>

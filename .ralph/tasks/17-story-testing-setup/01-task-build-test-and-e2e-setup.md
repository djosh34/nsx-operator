## Task: Build Test And E2E Setup <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Establish the test infrastructure and gates required for this operator, including unit, race, contract, Kubernetes e2e, and large/chaos scenarios.

In scope: commands or CI targets for `gofmt`, `go vet`, `golangci-lint`, `go test ./...`, `go test -race ./...`, and 80%+ coverage; unit tests for every library/component; race tests for shared HTTP limiter, concurrent NSX client calls, concurrent Kubernetes typed client calls, and sweep goroutines; contract tests against parent-dir `nsx-t-mockapi`; real Kubernetes e2e using in-memory/tmpfs kine, real kube-apiserver, installed CRDs, client-go, controller-runtime manager, and operator binary/process; NSX mockapi support for groups, pagination, slow responses, and manager down/unavailable behavior; large scenarios including 2,000 remote groups, 10,000 mixed CRs, low rate limits, low memory, and combined chaos.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Test commands are documented and runnable from the repo.
- [ ] Evidence includes successful normal, race, contract, Kubernetes e2e, and large/chaos test runs or explicit linked artifacts.
</acceptance_criteria>

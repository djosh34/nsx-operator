## Plan: Build Test And E2E Setup

Task: `.ralph/tasks/17-story-testing-setup/01-task-build-test-and-e2e-setup.md`

### Current State

- The repo already has normal Go tests across API types, config, logging, HTTP rate limiting, typed Kubernetes client, names, NSX client, startup, state operator, and status conditions.
- `Makefile` currently exposes `check`, `lint`, `test`, and `test-coverage`.
- `make test` and `make test-coverage` already use `setup-envtest` and `KUBEBUILDER_ASSETS`, which gives a real kube-apiserver/etcd envtest path for CRD and controller-runtime tests.
- Existing tests already include:
  - CRD envtest integration in `api/v1alpha/crd_integration_test.go`.
  - Typed Kubernetes client envtest coverage in `internal/kubeapi/client_test.go`.
  - Startup manager envtest coverage in `internal/startup/manager_test.go`.
  - Mockapi-backed NSX client contracts in `internal/nsxclient/contract_test.go`.
  - Mockapi-backed stateoperator integration in `internal/stateoperator/manager_pipeline_test.go` and write semantics tests.
  - HTTP limiter concurrency tests in `internal/httpratelimit/round_tripper_test.go`.
  - Manager goroutine concurrency tests in `internal/startup/manager_test.go`.
- Current gaps for this task are explicit gates and evidence:
  - no top-level target for `go vet`;
  - no top-level target for `go test -race ./...`;
  - no target grouping contract tests against `../nsx-t-mockapi`;
  - no target for Kubernetes e2e as a named gate;
  - no target or focused test for large/chaos scenarios such as 2,000 remote groups, 10,000 mixed CRs, low rate limits, low memory, and manager unavailable behavior;
  - no explicit 80% coverage threshold check in the Makefile target;
  - no task evidence recording the commands and outputs.

### Public Interface

Keep the user-facing API and CRD schemas unchanged.

Add or normalize these repo commands as the public verification interface:

```bash
make fmt
make vet
make lint
make test
make test-race
make test-contract
make test-e2e
make test-large-chaos
make test-coverage
make check
```

Expected semantics:

- `make fmt` runs `gofumpt -w .` and is allowed to rewrite formatting.
- `make vet` runs `go vet ./...`.
- `make lint` runs `golangci-lint run ./...` after formatting tools are installed.
- `make test` runs `go test ./...` with `KUBEBUILDER_ASSETS` populated by `setup-envtest`.
- `make test-race` runs `go test -race ./...` with `KUBEBUILDER_ASSETS`.
- `make test-contract` runs mockapi-backed contract and NSX concurrency tests that actually build/start `../nsx-t-mockapi`.
- `make test-e2e` runs focused Kubernetes e2e tests using installed CRDs, client-go/controller-runtime clients, `startup.NewManager`, and the real operator binary/process where the test is specifically about process startup.
- `make test-large-chaos` runs focused large/chaos scenarios and must not rely on shallow file inspection.
- `make test-coverage` runs coverage with an enforced global threshold of at least 80%.
- `make check` runs the full gate stack needed before completing this task: formatting, vet, lint, normal tests, race tests, contract tests, e2e tests, large/chaos tests, and coverage.

If `go test -race ./...` is too slow for routine local use but required by this task, keep it in `make check` for this task and record timing evidence. Do not silently downgrade the required gate.

### Boundary Plan Using `$improve-code-boundaries`

- Keep production behavior out of test orchestration:
  - Makefile targets should compose commands only.
  - Test harness helpers should live in test-oriented helper code, not in production startup, stateoperator, nsxclient, or kubeapi packages.
- Remove duplicated mockapi process setup if implementation touches it:
  - Existing mockapi start/build/wait logic appears in `internal/nsxclient/contract_test.go` and stateoperator tests.
  - Prefer one small test-support helper for starting `../nsx-t-mockapi`, routing logical manager hosts, and returning logs for failure evidence.
  - Keep that helper narrow: `StartMockAPI`, `Logs`, `BaseURL`, and explicit cleanup. Do not create a general framework.
- Remove duplicated envtest bootstrap if implementation touches it:
  - Existing envtest setup appears in multiple packages.
  - Prefer a helper that starts envtest with `config/crd/bases` and returns `*rest.Config`, scheme, and cleanup.
  - Do not hide behavior assertions behind helper abstractions; helpers only start infrastructure.
- Keep large/chaos test data generation behind one helper that names the scenario and returns real CR/NSX objects.
  - Avoid parallel DTOs that mirror the API types.
  - Generate `nsxv1alpha.NSXNetworkCloud`, `nsxv1alpha.NSXGroup`, and `nsxclient.Group` directly.
- Do not add fake Kubernetes clients for e2e. The e2e gate must exercise a real kube-apiserver with installed CRDs.
- Do not add string-inspection tests for Makefile/workflow targets. Per the TDD exception, validate those gates by executing the targets.
- Final boundary review must scan for:
  - test harness code leaking into production paths;
  - duplicated mockapi/envtest bootstraps that can be flattened;
  - ignored errors, especially process cleanup, body closes, command execution, and goroutine waits;
  - ad hoc command strings scattered across Go tests when a Make target or helper can own them.

### TDD Execution Plan Using `$tdd`

Use vertical red-green cycles. For Go behavior, write one behavior test, run it to RED, implement the minimum, then continue. For Makefile and workflow gates, the RED/GREEN signal is an executed command failing before the target or gate exists and passing after implementation.

1. [x] Gate tracer bullet: `make vet` exists and runs `go vet ./...`.
   - RED: run `make vet` and record failure if the target is missing or incomplete.
   - GREEN: add a `vet` target and include it in `check`.
   - Verification: `make vet`.

2. [x] Race gate: shared concurrency-sensitive packages pass under `go test -race`.
   - RED: run `make test-race` before adding the target and record the missing-target failure.
   - GREEN: add `test-race` with `KUBEBUILDER_ASSETS` populated the same way as `make test`.
   - Verification: `make test-race`.
   - Required evidence must mention race coverage for:
     - `internal/httpratelimit`;
     - concurrent NSX client calls through shared HTTP limiter/mockapi;
     - concurrent typed Kubernetes client calls;
     - stateoperator sweep goroutines.

3. [x] Contract gate: mockapi-backed NSX tests are a named runnable target.
   - RED: run `make test-contract` and record missing-target failure.
   - GREEN: add a target that runs the existing mockapi-backed contract tests, including route inventory, typed client contracts, and shared rate-limited client concurrency.
   - If implementation reveals duplicate mockapi bootstraps are slowing or obscuring tests, refactor test helpers only after the target is green.
   - Verification: `make test-contract`.

4. [x] Kubernetes e2e gate: manager/operator behavior is explicitly runnable.
   - RED: run `make test-e2e` and record missing-target failure.
   - GREEN: add a focused e2e target around existing envtest-backed CRD, typed client, startup manager, and stateoperator integration tests.
   - Add one missing behavior test only if needed: an operator process/binary e2e that builds `cmd/nsx-operator`, starts it against a real kube-apiserver with installed CRDs, creates a network cloud and group through client-go/controller-runtime, routes NSX traffic to mockapi, and observes status/CR changes through the public Kubernetes API.
   - If real kube-apiserver plus in-memory/tmpfs kine cannot be wired from available dependencies without changing the e2e interface, switch this plan back to `TO BE VERIFIED` before changing the design.
   - Verification: `make test-e2e`.

5. [x] Large remote NSX scenario: 2,000 remote groups are observed deterministically.
   - RED: add one behavior test that feeds 2,000 remote `nsxclient.Group` objects through the public stateoperator snapshot/plan/apply path and fails because no dedicated large-scenario gate exists or because the current path cannot complete within the test budget.
   - GREEN: implement the smallest test/harness support needed to pass without changing production behavior.
   - Assertions should be behavioral: count of Observe upserts, stable names/spec identities, no gather/apply error, and concrete timing/log evidence in verbose output.
   - Verification: focused `go test` plus `make test-large-chaos`.

6. [x] Large Kubernetes CR scenario: 10,000 mixed CRs are handled through real Kubernetes APIs.
   - RED: add an e2e/large test that creates a mix of Observe and Manage `NSXGroup` CRs and at least one `NSXNetworkCloud` through the real envtest kube-apiserver, then exercises typed/controller-runtime listing and sweep behavior.
   - GREEN: use pagination/list options and existing public clients; do not bypass behavior by inspecting storage directly.
   - If the full 10,000 object scenario is too slow for normal `make test` but required by this task, keep it under `make test-large-chaos` and record runtime evidence.

7. [x] Chaos scenario: low rate limits, slow NSX responses, manager unavailable/down behavior, and low-memory pressure are covered.
   - RED: add one scenario test that combines:
     - low `HTTPRateLimiter` settings;
     - mockapi or routed test server delayed responses;
     - one unavailable manager response path;
     - enough generated objects to exercise memory-sensitive paths without allocating duplicate large DTO graphs.
   - GREEN: reuse existing limiter, nsxclient, startup, and stateoperator public interfaces.
   - Assertions must observe public outcomes: statuses, errors, retry-safe state, and no race/deadlock under the race gate.

8. [x] Coverage threshold gate: global 80%+ coverage is enforced by command, not by assumption.
   - RED: run current `make test-coverage` and record that it reports coverage but does not enforce a global threshold.
   - GREEN: add coverage profile generation and a threshold check using `go tool cover -func`.
   - Verification: `make test-coverage` output must include total coverage at or above 80%.
   - If adding any new test-support package lowers package-level reporting, either cover the helper or keep it out of package-level coverage. Do not accept a hidden no-coverage package.

9. [x] Final full verification and evidence.
   - Run and record:
     - `make check`;
     - `make test`;
     - `make test-race`;
     - `make test-contract`;
     - `make test-e2e`;
     - `make test-large-chaos`;
     - `make test-coverage`.
   - Append concrete command output summaries to the task file under `<verification_evidence>`.
   - Include evidence for normal, race, contract, Kubernetes e2e, and large/chaos runs.

10. [x] Final boundary review using `$improve-code-boundaries`.
    - Run scans for ignored errors and duplicate harness/setup code.
    - Confirm test orchestration remains in Makefile/test helper code.
    - Confirm production packages were not polluted with test-only interfaces.
    - Resolve muddy boundaries before marking the task done.

### Completion Steps

- Mark plan checklist items complete as they are implemented.
- If implementation proves the interface or harness design is wrong, replace the final marker with `TO BE VERIFIED` and quit immediately.
- If design remains valid and all checks pass:
  - update the task file with concrete verification evidence;
  - set `<passes>true</passes>`;
  - run `/bin/bash .ralph/task_switch.sh`;
  - add all files, including `.ralph` files;
  - commit with `task finished 01-task-build-test-and-e2e-setup: establish test gates and evidence`;
  - include summary, verification evidence, and implementation challenges in the commit message;
  - push;
  - quit immediately.

Plan path: `.ralph/tasks/17-story-testing-setup/01-task-build-test-and-e2e-setup_plans/2026-05-19-test-and-e2e-setup-plan.md`

NOW EXECUTE

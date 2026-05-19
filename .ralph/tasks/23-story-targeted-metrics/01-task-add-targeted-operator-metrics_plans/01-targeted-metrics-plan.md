## Plan: Targeted Operator Metrics

Task: `.ralph/tasks/23-story-targeted-metrics/01-task-add-targeted-operator-metrics.md`

### Current shape

- `internal/startup/manager.go` already wires controller-runtime metrics, but `Metrics.BindAddress` is hardcoded to `"0"`, so the endpoint is disabled.
- `internal/nsxclient/client.go` owns the NSX HTTP request boundary through `Client.do` and the paginated `listAllTyped` path. Public NSX methods live in `internal/nsxclient/routes.go`.
- `internal/kubeapi/client.go` owns the typed Kubernetes REST boundary through `typedResource` methods.
- `internal/stateoperator/manager_pipeline.go` owns the manager list-groups snapshot and reconcile plan, so it is the right boundary for the last-loop inventory gauges.
- Prometheus dependencies are already present transitively through controller-runtime.

### Boundary improvement to apply

Use a small `internal/operatormetrics` package as the only place that knows Prometheus metric names, labels, and collector registration. Do not scatter `prometheus.New*` calls or label literals across `nsxclient`, `kubeapi`, `stateoperator`, and `startup`.

The boundary should expose methods shaped by events the rest of the code already owns:

- `ObserveNSXCall(manager string, function string)`
- `ObserveNSXHTTP(manager string, function string, requestBytes int64, responseBytes int64, duration time.Duration)`
- `ObserveKubernetesAPI(function string, requestBytes int64, responseBytes int64, duration time.Duration)`
- `SetManagerGroupSnapshot(manager string, snapshot ManagerGroupSnapshot)`

Keep the package deep: callers pass domain facts, the metrics package handles names, labels, counters, gauges, histograms, response byte accounting helpers, and test registries.

### Metric names and bounded labels

All labels must be bounded and must not include raw object names, URLs, paths, namespaces, CR names, or group IDs.

- `nsx_operator_nsx_groups_listed_total` gauge, labels: `manager`
- `nsx_operator_nsx_groups_observe_total` gauge, labels: `manager`
- `nsx_operator_nsx_groups_manage_total` gauge, labels: `manager`
- `nsx_operator_nsx_group_cr_updates_needed_total` gauge, labels: `manager`, `mode`
- `nsx_operator_nsx_group_cr_creates_needed_total` gauge, labels: `manager`
- `nsx_operator_nsx_client_calls_total` counter, labels: `manager`, `function`
- `nsx_operator_nsx_http_requests_total` counter, labels: `manager`
- `nsx_operator_nsx_http_bytes_total` counter, labels: `manager`, `direction`
- `nsx_operator_nsx_http_round_trip_seconds` histogram, labels: `manager`
- `nsx_operator_nsx_http_function_round_trip_seconds` histogram, labels: `manager`, `function`
- `nsx_operator_kubernetes_api_calls_total` counter, labels: `function`
- `nsx_operator_kubernetes_api_bytes_total` counter, labels: `function`, `direction`
- `nsx_operator_kubernetes_api_round_trip_seconds` histogram, labels: `function`

Allowed `mode` values: `observe`, `manage`.

Allowed `direction` values: `request`, `response`.

Allowed `function` values come from constant strings at public operation boundaries, for example `list_groups`, `get_group`, `patch_group`, `delete_group`, `groups.list`, `groups.get`, `groups.create`, `groups.update`, `groups.update_status`, `groups.delete`, `network_clouds.list`, `network_clouds.get`, and similar existing typed functions. Do not derive function labels from HTTP paths.

### Interface and type design

- Add `internal/operatormetrics` with a `Recorder` interface and a real Prometheus implementation.
- Add a no-op recorder for tests and nil-safe startup defaults.
- Add an injectable registry constructor so tests can use a private `prometheus.Registry` and scrape with `prometheus/testutil` or `promhttp.HandlerFor`.
- Add `MetricsBindAddress string` to `config.OperatorConfig` and `rawOperatorConfig`, defaulting to `":8080"` unless compatibility tests reveal this must stay disabled by default. Wire it into `metricsserver.Options.BindAddress`.
- Add a startup integration test that creates a manager with a nonzero metrics bind address and proves the metrics endpoint can be scraped. If port binding makes this flaky, test controller-runtime registry registration directly and use a local `promhttp.HandlerFor` scrape.
- Add `Recorder` to `nsxclient.Options` and store it on `Client`. The client already knows the normalized manager FQDN through `WriteControl.NetworkCloudFQDN`; use that label, falling back to the base URL host only if the write-control FQDN is empty.
- Add `Recorder` to `kubeapi.Options` and store it on each `typedResource`.
- Add `Recorder` to `stateoperator.Options` and pass it into `defaultManagerSweep`.

### TDD execution plan

Use vertical slices. Do not write all tests first.

1. RED: add an `internal/operatormetrics` test that records one NSX HTTP event and one Kubernetes API event into a private registry, scrapes metrics, and verifies only the expected low-cardinality sample series exist.
2. GREEN: implement the minimal metrics package and real recorder registration needed for that test.
3. RED: add an `nsxclient` behavior test that calls real public methods (`ListGroups`, then one write such as `PatchGroup`) through a test HTTP server and proves counters/histograms are emitted with `manager` and `function` labels.
4. GREEN: instrument `nsxclient.Client.do` and `listAllTyped` through a single request execution boundary. This may require refactoring `listAllTyped` to call a small shared `roundTrip` method so bytes, duration, and errors are counted consistently.
5. RED: add a `kubeapi` envtest behavior test using the public typed client to create/list/update/status/update/delete CRs, then scrape the private registry and prove Kubernetes function counters, byte counters, and histograms move.
6. GREEN: instrument `typedResource` methods at the REST request boundary. Prefer wrapping the rest config transport if available; if client-go does not expose bytes cleanly at this layer, account request bodies from prepared objects and response bytes by using a transport wrapper configured on the copied `rest.Config`.
7. RED: add a `stateoperator` manager sweep test that runs through `GatherManagerSnapshot`, `ProcessManagerSnapshot`, and `ApplyManagerPlan` behavior and verifies list-groups gauges for listed, observe, manage, CR updates needed by mode, and new CR creates.
8. GREEN: add a small `ManagerGroupSnapshot` summary type in `operatormetrics` and compute it once from `ManagerSnapshot` plus `ManagerPlan` in `defaultManagerSweep`.
9. RED: add a startup/config behavior test proving metrics bind address config is parsed and wired into manager options without string-comparison-only tests.
10. GREEN: wire `operator.metricsBindAddress` through config/startup and update `hack/compose/nsx-operator-config.yaml` plus `compose.yaml` to expose/scrape the endpoint for manual verification.
11. REFACTOR: apply `improve-code-boundaries` review. Remove any duplicate label strings, collapse one-off metrics helpers into `operatormetrics`, and keep public interfaces private unless cross-package callers truly need them.
12. VERIFY: run `make check`, `make test`, `make test-contract`, `make test-e2e`, and `make test-coverage`. Record coverage evidence.
13. MANUAL VERIFY: use `../nsx-t-mockapi` through the existing compose or testcontainers path, create/list/update Kubernetes CRs against envtest or compose Kubernetes, scrape `/metrics`, and paste representative series into the task file.

### Concrete implementation notes

- Add debug logs around metrics recorder setup and info logs around metrics endpoint binding, using zap structured fields.
- Never ignore errors from response body close, registry registration, scrapes, HTTP calls, or test setup.
- Use `prometheus.WrapRegistererWith` or explicit const-label discipline only inside `operatormetrics`; callers should not know collector internals.
- If duplicate global registration is a risk in tests, register project collectors to controller-runtime's registry once from startup and use private registries in package tests.
- Counter byte semantics: count request body bytes and response body bytes separately with `direction`. Include zero-byte requests/responses by incrementing the request counter even when byte counter does not increase.
- Histogram buckets can use Prometheus defaults unless tests show the project already has a bucket convention.

NOW EXECUTE

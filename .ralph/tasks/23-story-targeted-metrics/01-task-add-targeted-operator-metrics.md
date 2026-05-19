## Task: Add Targeted NSX And Kubernetes Metrics <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add only the requested metrics for NSX inventory/reconciliation and HTTP/API client behavior. Do not blindly add broad metric coverage. The metric set must remain focused, low-cardinality, and tied to manager/function labels that are useful for operating the controller.

Gauge metrics required from the NSX list-groups part of the last loop iteration:
- total listed groups per NSX manager;
- total observe groups and total manage groups found in that same list-groups processing path;
- total CR updates needed, split or labeled for observe and manage;
- total new CRs to be created.

Counter metrics required:
- per NSX manager, number of calls per function called, for example get, list, and all implemented NSX client functions;
- HTTP request count and HTTP byte count per NSX manager;
- total Kubernetes API calls per function;
- total Kubernetes API bytes per function.

Histogram metrics required:
- whole HTTP round-trip time for Kubernetes API calls;
- whole HTTP round-trip time per NSX manager;
- whole HTTP round-trip time per NSX manager and function.

In scope: choose clear metric names and bounded labels; instrument the existing NSX list-groups/reconcile path; instrument NSX HTTP client calls by manager and function; instrument Kubernetes API calls by function; expose metrics through the project's existing metrics endpoint or add the smallest conventional endpoint needed if none exists; document the metric names in the task evidence; include tests or integration verification showing values change after real calls.

Out of scope: adding unrelated metrics; high-cardinality labels such as raw object names, paths, URLs, namespaces, or CR names unless already proven safe in an existing metrics pattern; changing business behavior; adding dashboards or alert rules unless needed only as verification artifacts.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Gauge metrics report the last loop iteration's total listed groups per NSX manager from the NSX list-groups path.
- [x] Gauge metrics report observe total, manage total, observe/manage CR updates needed, and new CRs to create from that same processing path.
- [x] Counter metrics report NSX calls per manager and function.
- [x] Counter metrics report NSX HTTP request counts and byte counts per manager.
- [x] Counter metrics report Kubernetes API calls and byte counts per function.
- [x] Histogram metrics report whole HTTP round-trip time for Kubernetes API calls.
- [x] Histogram metrics report whole HTTP round-trip time per NSX manager and per NSX manager/function.
- [x] Tests or manual verification prove metrics are emitted after calls against `../nsx-t-mockapi` and real or envtest Kubernetes API calls.
- [x] Verification includes a metrics scrape or equivalent output with representative sample series and values.
- [x] The implementation avoids unrequested metrics and avoids high-cardinality labels.
- [x] Existing normal, contract, e2e, and coverage gates relevant to instrumentation pass, including `make test`, `make test-contract`, `make test-e2e`, and `make test-coverage` unless the repo's current gate documents a narrower mandatory command.
</acceptance_criteria>

<plan>
.ralph/tasks/23-story-targeted-metrics/01-task-add-targeted-operator-metrics_plans/01-targeted-metrics-plan.md
</plan>

<execution_state>DONE</execution_state>

<verification_evidence>
Implementation summary:
- Added `internal/operatormetrics` as the single Prometheus boundary for metric names, label sets, collector registration, no-op behavior, and private test registries.
- Instrumented NSX public client calls and HTTP requests with bounded `manager`, `function`, and `direction` labels. Function labels are static route-family labels such as `list_groups`, `patch_group`, `get_group`, `list_tier1s`, and `patch_security_policy`; raw paths, URLs, object names, namespaces, and IDs are not label values.
- Instrumented Kubernetes typed REST traffic at the copied `rest.Config` transport boundary with bounded labels such as `groups.create`, `groups.list`, `groups.update`, `groups.update_status`, `groups.delete`, and `network_clouds.*`.
- Instrumented the default manager sweep list-groups processing path with last-loop manager gauges for listed groups, observe/manage totals, observe/manage update-needed totals, and new CR creates.
- Exposed controller-runtime metrics through `operator.metricsBindAddress`, defaulting loaded config to `:8080`, and exposed `${NSX_OPERATOR_METRICS_PORT:-18081}:8080` in compose.

Representative scrape/equivalent metric samples verified by tests:
```text
nsx_operator_nsx_client_calls_total{function="list_groups",manager="manager-a.example.test"} 1
nsx_operator_nsx_client_calls_total{function="patch_group",manager="manager-a.example.test"} 1
nsx_operator_nsx_http_requests_total{manager="manager-a.example.test"} 2
nsx_operator_nsx_http_bytes_total{direction="request",manager="manager-a.example.test"} 12
nsx_operator_nsx_http_bytes_total{direction="response",manager="manager-a.example.test"} 44
nsx_operator_nsx_http_round_trip_seconds_count{manager="manager-a.example.test"} 2
nsx_operator_nsx_http_function_round_trip_seconds_count{function="list_groups",manager="manager-a.example.test"} 1
nsx_operator_kubernetes_api_calls_total{function="groups.create"} 1
nsx_operator_kubernetes_api_calls_total{function="groups.list"} 1
nsx_operator_kubernetes_api_calls_total{function="groups.update"} 1
nsx_operator_kubernetes_api_calls_total{function="groups.update_status"} 1
nsx_operator_kubernetes_api_calls_total{function="groups.delete"} 1
nsx_operator_kubernetes_api_bytes_total{direction="response",function="groups.create"} > 0
nsx_operator_kubernetes_api_round_trip_seconds_count{function="groups.create"} 1
nsx_operator_nsx_groups_listed_total{manager="nsx-a.example.test"} 2
nsx_operator_nsx_groups_observe_total{manager="nsx-a.example.test"} 3
nsx_operator_nsx_groups_manage_total{manager="nsx-a.example.test"} 0
nsx_operator_nsx_group_cr_updates_needed_total{manager="nsx-a.example.test",mode="observe"} 2
nsx_operator_nsx_group_cr_updates_needed_total{manager="nsx-a.example.test",mode="manage"} 0
nsx_operator_nsx_group_cr_creates_needed_total{manager="nsx-a.example.test"} 1
```

Concrete verification commands run:
- `go test ./internal/operatormetrics ./internal/config ./internal/nsxclient` passed.
- `go test ./... -run '^$'` passed compile-only checks.
- `make check` passed after the final boundary cleanup. This includes `fmt`, `vet`, `lint`, `test`, `test-race`, `test-contract`, `test-e2e`, `test-large-chaos`, and `test-coverage`.
- `make test` passed.
- `make test-coverage` passed with `coverage 83.3% meets 80.0% threshold`.
- New metrics package coverage was checked directly with `go test ./internal/operatormetrics -coverprofile=/tmp/operatormetrics.cover && go tool cover -func=/tmp/operatormetrics.cover`; package coverage was `94.9%`.

Mock API and Kubernetes evidence:
- `make check` ran `go test ./internal/nsxclient -run 'Test(MockAPIRouteInventoryIsSupportedAndContracted|TypedClientContractsAgainstMockAPI|SharedRateLimitedClientConcurrencyAgainstMockAPI)' -count=1`, which exercises NSX client calls against `../nsx-t-mockapi`; it passed.
- `make check` ran envtest-backed `go test ./internal/kubeapi` and `go test ./internal/stateoperator`; the new metric tests exercise real envtest Kubernetes API calls and scrape private Prometheus registries.
- `make check` ran the envtest + mockapi stateoperator lifecycle contract `TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI`; it passed.
</verification_evidence>

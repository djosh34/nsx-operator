## Task: Add Targeted NSX And Kubernetes Metrics <status>not_started</status> <passes>false</passes>

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
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Gauge metrics report the last loop iteration's total listed groups per NSX manager from the NSX list-groups path.
- [ ] Gauge metrics report observe total, manage total, observe/manage CR updates needed, and new CRs to create from that same processing path.
- [ ] Counter metrics report NSX calls per manager and function.
- [ ] Counter metrics report NSX HTTP request counts and byte counts per manager.
- [ ] Counter metrics report Kubernetes API calls and byte counts per function.
- [ ] Histogram metrics report whole HTTP round-trip time for Kubernetes API calls.
- [ ] Histogram metrics report whole HTTP round-trip time per NSX manager and per NSX manager/function.
- [ ] Tests or manual verification prove metrics are emitted after calls against `../nsx-t-mockapi` and real or envtest Kubernetes API calls.
- [ ] Verification includes a metrics scrape or equivalent output with representative sample series and values.
- [ ] The implementation avoids unrequested metrics and avoids high-cardinality labels.
- [ ] Existing normal, contract, e2e, and coverage gates relevant to instrumentation pass, including `make test`, `make test-contract`, `make test-e2e`, and `make test-coverage` unless the repo's current gate documents a narrower mandatory command.
</acceptance_criteria>

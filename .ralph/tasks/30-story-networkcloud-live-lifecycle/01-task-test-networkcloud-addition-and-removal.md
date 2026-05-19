## Task: Test NetworkCloud Addition And Removal <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Check and add tests proving that adding NetworkCloud resources works, is supported by the operator, and that removal also works. This must be possible to do live.

The task must inspect the current `NSXNetworkCloud` implementation, identify any missing support in API types, CRDs, clients, reconciliation, or scripts, and add the smallest needed changes so NetworkCloud create and delete flows are supported. Tests must cover addition and removal, and at least one documented verification path must be runnable live against a Kubernetes cluster and NSX-T/mockapi setup. For NSX-T-facing behavior, use the public GHCR image for mockapi, not a local `../nsx-t-mockapi` checkout. The package is published at `https://github.com/djosh34/nsx-t-mockapi/pkgs/container/nsx-t-mockapi`, and testcontainers may be used to run that image.

In scope: add or update tests for NetworkCloud creation and deletion; ensure the operator recognizes NetworkCloud resources; verify the CRD installs/applies; verify deletion/finalizer behavior; document a live command sequence using kubectl and the ingest script if available; ensure logs are structured zap JSONL to stderr for relevant actions.

Out of scope: unrelated resource kinds; broad redesign of the reconciliation architecture; implementing unsupported NSX features unless required for the NetworkCloud add/remove lifecycle.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Tests prove an `NSXNetworkCloud` resource can be added and observed or reconciled through the supported operator path.
- [ ] Tests prove an `NSXNetworkCloud` resource can be removed and the operator handles cleanup/finalizers/status correctly.
- [ ] CRD installation or apply verification includes `NSXNetworkCloud`.
- [ ] A live verification path is documented with concrete kubectl commands and expected evidence.
- [ ] Live verification uses the public GHCR `nsx-t-mockapi` image from `https://github.com/djosh34/nsx-t-mockapi/pkgs/container/nsx-t-mockapi`, not a local `../nsx-t-mockapi` checkout.
- [ ] Testcontainers-based verification, if used, pulls and runs the public GHCR mockapi image.
- [ ] The NetworkCloud ingest script, if present, is included in at least one add-flow verification path.
- [ ] Relevant existing normal, contract, e2e, and coverage gates pass, including live-capable tests where appropriate.
</acceptance_criteria>

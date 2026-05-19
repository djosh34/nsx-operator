## Task: Test NetworkCloud Addition And Removal <status>not_started</status> <passes>true</passes>

<plan>
.ralph/tasks/30-story-networkcloud-live-lifecycle/01-task-test-networkcloud-addition-and-removal_plans/01-networkcloud-add-remove-lifecycle-plan.md
NOW EXECUTE
</plan>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Check and add tests proving that adding NetworkCloud resources works, is supported by the operator, and that removal also works. This must be possible to do live.

The task must inspect the current `NSXNetworkCloud` implementation, identify any missing support in API types, CRDs, clients, reconciliation, or scripts, and add the smallest needed changes so NetworkCloud create and delete flows are supported. Tests must cover addition and removal, and at least one documented verification path must be runnable live against a Kubernetes cluster and NSX-T/mockapi setup. For NSX-T-facing behavior, use the public GHCR image for mockapi, not a local `../nsx-t-mockapi` checkout. The package is published at `https://github.com/djosh34/nsx-t-mockapi/pkgs/container/nsx-t-mockapi`, and testcontainers may be used to run that image.

In scope: add or update tests for NetworkCloud creation and deletion; ensure the operator recognizes NetworkCloud resources; verify the CRD installs/applies; verify deletion/finalizer behavior; document a live command sequence using kubectl and the ingest script if available; ensure logs are structured zap JSONL to stderr for relevant actions.

Out of scope: unrelated resource kinds; broad redesign of the reconciliation architecture; implementing unsupported NSX features unless required for the NetworkCloud add/remove lifecycle.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Tests prove an `NSXNetworkCloud` resource can be added and observed or reconciled through the supported operator path.
- [x] Tests prove an `NSXNetworkCloud` resource can be removed and the operator handles cleanup/finalizers/status correctly.
- [x] CRD installation or apply verification includes `NSXNetworkCloud`.
- [x] A live verification path is documented with concrete kubectl commands and expected evidence.
- [x] Live verification uses the public GHCR `nsx-t-mockapi` image from `https://github.com/djosh34/nsx-t-mockapi/pkgs/container/nsx-t-mockapi`, not a local `../nsx-t-mockapi` checkout.
- [x] Testcontainers-based verification, if used, pulls and runs the public GHCR mockapi image. Not used; Docker CLI helper pulls/runs the public GHCR image.
- [x] The NetworkCloud ingest script, if present, is included in at least one add-flow verification path.
- [x] Relevant existing normal, contract, e2e, and coverage gates pass, including live-capable tests where appropriate.
</acceptance_criteria>

<verification_evidence>
Execution date: 2026-05-19.

Focused automated checks passed:

```bash
go test ./internal/nsxclient -run 'Test(MockAPIRouteInventoryIsSupportedAndContracted|TypedClientContractsAgainstMockAPI|SharedRateLimitedClientConcurrencyAgainstMockAPI)' -count=1
# ok github.com/djosh34/nsx-operator/internal/nsxclient 3.523s

KUBEBUILDER_ASSETS="$(./.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run 'Test(NetworkCloudAddAndRemoveLifecycleAgainstPublicMockAPI|Lifecycle.*MockAPI|ManagedWrite.*MockAPI|DisabledNSXWritesDoNotReachMockAPI)' -count=1
# ok github.com/djosh34/nsx-operator/internal/stateoperator 23.206s

docker compose config
# resolved nsx-t-mockapi image: ghcr.io/djosh34/nsx-t-mockapi:latest
# resolved nsx-t-mockapi platform: linux/amd64
# resolved config bind: ./config/compose/mockapi-config.yaml -> /config/config.yaml
```

Required final gates passed:

```bash
make test
# passed

make test-coverage
# coverage 83.9% meets 80.0% threshold
# internal/testsupport/mockapi coverage: 83.1%

make check
# fmt/vet/lint/test/race/contract/e2e/large-chaos/test-coverage passed
# final coverage 83.7% meets 80.0% threshold
```

Live Compose verification used the public GHCR image:

```bash
docker compose images nsx-t-mockapi
# nsx-operator-scratch-nsx-t-mockapi-1 ghcr.io/djosh34/nsx-t-mockapi latest linux/amd64 f0e0c27ddf45

docker image inspect ghcr.io/djosh34/nsx-t-mockapi:latest --format '{{.RepoTags}} {{.Id}} {{.Architecture}}/{{.Os}}'
# [ghcr.io/djosh34/nsx-t-mockapi:latest] sha256:f0e0c27ddf455c21068b69c5036644f735db3d1d92125221ba909d4a37303f4a amd64/linux
```

Live stack and CRD evidence:

```bash
docker compose ps
# kine, kube-apiserver, nsx-t-mockapi, and operator were Up
# nsx-t-mockapi image was ghcr.io/djosh34/nsx-t-mockapi:latest

docker compose ps -a crd-init
# crd-init Exited (0)

export KUBECONFIG=config/compose/generated/kubeconfig-host.yaml
kubectl wait --for=condition=Established crd/nsxnetworkclouds.nsx.ing.com --timeout=60s
# customresourcedefinition.apiextensions.k8s.io/nsxnetworkclouds.nsx.ing.com condition met

kubectl get nsxnetworkclouds.nsx.ing.com
# NAME      FQDN                 CLOUDID   DISPLAYNAME   WRITESENABLED   REACHABLE   SWEPT
# mockapi   nsx-t-mockapi:8080   mockapi   Mock API      true            True        True
```

Live add/remove evidence through the ingest script:

```bash
printf '%s\n' '{"name":"cloud-live","fqdn":"nsx-t-mockapi:8080","id":"cloud-live"}' | ./scripts/ingest-networkcloud.sh
# nsxnetworkcloud.nsx.ing.com/cloud-live created

kubectl wait --for=condition=Swept nsxnetworkclouds.nsx.ing.com/cloud-live --timeout=60s
# nsxnetworkcloud.nsx.ing.com/cloud-live condition met

kubectl get nsxnetworkclouds.nsx.ing.com cloud-live -o yaml
# status.conditions included:
# - type: Reachable, status: "True", reason: GatherSucceeded, message: NSX manager gather completed
# - type: Swept, status: "True", reason: SweepPlanned, message: manager snapshot was processed

kubectl delete nsxnetworkclouds.nsx.ing.com cloud-live
# nsxnetworkcloud.nsx.ing.com "cloud-live" deleted

kubectl get nsxnetworkclouds.nsx.ing.com cloud-live
# Error from server (NotFound): nsxnetworkclouds.nsx.ing.com "cloud-live" not found
```

Operator log evidence:

```bash
docker compose logs --no-color operator | tail -120
# JSONL zap entries included constructed nsx manager client, starting/completed default manager sweep,
# reconciled network cloud for cloud-live, network cloud reconcile skipped missing object for deleted cloud-live,
# and structured controller-runtime metrics logs.

docker compose logs --no-color operator | rg 'controller-runtime|log.SetLogger'
# {"level":"info","logger":"controller-runtime.metrics","msg":"Starting metrics server",...}
# {"level":"info","logger":"controller-runtime.metrics","msg":"Serving metrics server",...}
# No plain-text log.SetLogger warning was emitted after wiring controller-runtime to zap.
```
</verification_evidence>

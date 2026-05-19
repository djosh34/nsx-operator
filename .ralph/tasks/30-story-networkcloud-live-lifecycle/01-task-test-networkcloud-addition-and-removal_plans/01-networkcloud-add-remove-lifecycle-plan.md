# NetworkCloud Add/Remove Lifecycle Plan

Plan path: `.ralph/tasks/30-story-networkcloud-live-lifecycle/01-task-test-networkcloud-addition-and-removal_plans/01-networkcloud-add-remove-lifecycle-plan.md`

## Required Skills Read

- `$tdd`: execute as vertical red-green slices, testing behavior through public Kubernetes/operator/NSX-client boundaries instead of private helpers.
- `$improve-code-boundaries`: remove the duplicated local mockapi launcher boundary and keep NetworkCloud lifecycle support in the existing API, kubeapi, startup, and stateoperator layers.

## Current Facts

- `api/v1alpha/types.go` already defines `NSXNetworkCloud`, `NSXNetworkCloudList`, `NSXNetworkCloudSpec`, and `NSXNetworkCloudStatus`.
- `crds/nsx.ing.com_nsxnetworkclouds.yaml` already installs cluster-scoped `nsxnetworkclouds.nsx.ing.com`, includes `status`, selectable fields, and required `spec.networkCloudFQDN`, `spec.networkCloudId`, and `spec.name`.
- `internal/kubeapi/client.go` already exposes `NetworkClouds().Create/Get/List/Apply/UpdateStatus/Delete/Watch`.
- `internal/startup/manager.go` already registers the `NSXNetworkCloud` controller and creates NSX clients from each NetworkCloud.
- `internal/stateoperator/operator.go` already sweeps current NetworkCloud objects and refreshes each object before sweeping, so deleted clouds should stop being swept.
- `scripts/ingest-networkcloud.sh` already exists from story 29 and is the right add-flow interface for live verification.
- `docker manifest inspect ghcr.io/djosh34/nsx-t-mockapi:latest` succeeded during planning, so use `ghcr.io/djosh34/nsx-t-mockapi:latest` as the public mockapi image unless execution proves the tag is wrong.
- Existing `internal/nsxclient` and `internal/stateoperator` mockapi helpers build and run a sibling `../nsx-t-mockapi` checkout. That conflicts with this task's public GHCR requirement and is the main boundary problem to fix.
- Existing `compose.yaml` and `docs/compose-stack.md` also describe a sibling mockapi checkout. The live verification path for this task must not depend on that checkout.

## Public Interface And Type Design

- Keep the Kubernetes API unchanged unless a RED test proves a real missing lifecycle field:
  - no new NetworkCloud enum,
  - no NetworkCloud finalizer,
  - no duplicate DTO for live verification,
  - no new CLI.
- Keep `NSXNetworkCloud` deletion as normal Kubernetes deletion with no finalizer. The correct cleanup behavior is that future operator sweeps do not construct an NSX client for the deleted cloud, while existing child `NSXGroup` objects are not silently deleted by NetworkCloud removal.
- Add one small test support boundary for public mockapi startup, for example `internal/testsupport/mockapi`, with a narrow interface:
  - `Start(t, ctx) Process`
  - `Process.BaseURL() string`
  - `Process.Logs() string`
  - optional helpers for username/password if needed by existing tests.
- The helper owns Docker/testcontainer process details, config file generation, readiness polling, logs, and cleanup. Tests should only see a reachable NSX Manager URL.
- Prefer the existing `nsxclient.Client` and operator construction paths; do not add special test-only production flags.

## Boundary Refactor Plan

- Replace both local sibling mockapi launchers with the shared public-image helper:
  - `internal/nsxclient/contract_test.go`
  - `internal/stateoperator/manager_pipeline_test.go`
  - `internal/stateoperator/manager_pipeline_write_semantics_test.go`
- Use `$improve-code-boundaries` smell 5 and smell 13 as the explicit cleanup target: one shared mockapi process shape, one config writer, one readiness path, and no repeated local checkout/build helper.
- Use the public image:
  - `ghcr.io/djosh34/nsx-t-mockapi:latest`
- If Docker CLI startup is simpler and already available, use a helper that runs the image with Docker and removes the container on cleanup. If Testcontainers keeps the helper smaller, add `github.com/testcontainers/testcontainers-go` and use it only inside the test support boundary.
- Update `compose.yaml` so `nsx-t-mockapi` uses the public GHCR image and a repo-local compose config file instead of `build.context: ../nsx-t-mockapi`.
- Update `docs/compose-stack.md` to remove sibling-checkout requirements and document the public image.

## TDD Execution Plan

1. [x] RED tracer bullet: add `TestNetworkCloudAddAndRemoveLifecycleAgainstPublicMockAPI` in `internal/stateoperator/manager_pipeline_test.go`.
   - Use envtest-backed clients through the existing typed kubeapi/controller-runtime clients.
   - Start public mockapi through the new test helper interface before the helper exists, so the initial failure is compile-time or startup-path failure.
   - Seed a remote group in mockapi through `nsxclient.Client`.
   - Create an `NSXNetworkCloud` with the typed Kubernetes client.
   - Start `stateoperator.New` with default manager sweep using a factory that returns an `nsxclient.Client` pointed at the mockapi URL.
   - Assert through public interfaces:
     - the cloud gets a `Swept=True` or `Reachable=True` condition,
     - a remote group is imported/observed as an `NSXGroup`,
     - deleting the `NSXNetworkCloud` removes the cloud object,
     - a later operator sweep does not construct an NSX client for the deleted cloud,
     - existing child groups are left alone unless their own lifecycle says otherwise.
2. [x] GREEN: implement the smallest public mockapi test helper using `ghcr.io/djosh34/nsx-t-mockapi:latest`.
   - Generate a temp config with `server.listen_addr: "0.0.0.0:8080"`, temp DB path inside the container, zero realization delays, and large page size.
   - Wait for `/policy/api/v1/eula/acceptance` with basic auth before returning.
   - Capture logs and include them in test failure messages.
   - Handle every returned error; do not discard errors with `_`.
3. [x] RED: migrate `internal/nsxclient` contract tests to the shared helper and run only the contract test package.
   - Behavior stays the same: the typed NSX client must satisfy the mockapi route contract.
   - Expected failure before GREEN: old local helper references or compile errors.
4. [x] GREEN: remove the duplicated nsxclient local checkout helper and use the shared public-image helper.
5. [x] RED: migrate existing stateoperator mockapi lifecycle/write-semantics tests to the shared helper and run the focused stateoperator mockapi tests.
   - Behavior stays the same: observe/manage deletion semantics and write controls work against mockapi.
6. [x] GREEN: remove the duplicated stateoperator local checkout helper and use the shared public-image helper.
7. [x] RED: update compose live path to the public image and verify by executing `docker compose config`.
   - This is not a brittle string test; it is a real compose configuration parse.
8. [x] GREEN: add repo-local mockapi compose config if needed, update `compose.yaml`, and update `docs/compose-stack.md`.
9. [x] RED: add or extend live verification documentation for kubectl + ingest script.
   - The doc/task evidence must show:
     - `docker compose up -d` or equivalent live stack startup,
     - `printf ... | ./scripts/ingest-networkcloud.sh`,
     - `kubectl get nsxnetworkclouds.nsx.ing.com`,
     - evidence that the operator logs are zap JSONL on stderr,
     - mockapi URL/image evidence using `ghcr.io/djosh34/nsx-t-mockapi:latest`.
10. [x] GREEN: run the documented live commands and record concrete outputs in the task file.

## Focused Verification Commands During Execution

Run these after each corresponding slice, not only at the end:

```bash
KUBEBUILDER_ASSETS="$$(./.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run TestNetworkCloudAddAndRemoveLifecycleAgainstPublicMockAPI -count=1
go test ./internal/nsxclient -run 'Test(MockAPIRouteInventoryIsSupportedAndContracted|TypedClientContractsAgainstMockAPI|SharedRateLimitedClientConcurrencyAgainstMockAPI)' -count=1
KUBEBUILDER_ASSETS="$$(./.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run 'TestLifecycle.*MockAPI|Test.*AgainstMockAPI|TestNetworkCloudAddAndRemoveLifecycleAgainstPublicMockAPI' -count=1
docker compose config
```

Required final gates:

```bash
make check
make test
make test-coverage
```

Coverage must remain at least 80%, and new code must have focused behavioral coverage.

## Manual Verification Plan

Record concrete evidence in the task file after execution.

Preferred live path:

```bash
docker compose down --volumes --remove-orphans
docker compose config
docker compose up -d
docker compose ps
export KUBECONFIG=config/compose/generated/kubeconfig-host.yaml
kubectl wait --for=condition=Established crd/nsxnetworkclouds.nsx.ing.com --timeout=60s
printf '%s\n' '{"name":"cloud-live","fqdn":"nsx-t-mockapi:8080","id":"cloud-live"}' | ./scripts/ingest-networkcloud.sh
kubectl get nsxnetworkclouds.nsx.ing.com cloud-live -o yaml
kubectl delete nsxnetworkclouds.nsx.ing.com cloud-live
kubectl get nsxnetworkclouds.nsx.ing.com cloud-live
docker compose logs --no-color operator
docker image inspect ghcr.io/djosh34/nsx-t-mockapi:latest --format '{{.RepoTags}} {{.Id}}'
docker compose down --volumes --remove-orphans
```

Expected evidence:

- CRD `nsxnetworkclouds.nsx.ing.com` is Established.
- `cloud-live` is created by the ingest script.
- Operator logs are structured JSONL on stderr and include NetworkCloud reconcile/sweep fields.
- `cloud-live` deletion returns NotFound on a follow-up get.
- Mockapi image evidence names `ghcr.io/djosh34/nsx-t-mockapi:latest`, not a local `../nsx-t-mockapi` checkout.

## Design Recheck Rule

If execution proves NetworkCloud deletion needs a finalizer, new status field, new API type, or a different public mockapi image contract, change this plan's final marker back to `TO BE VERIFIED`, document the proposed type/interface change above, update the task plan pointer if needed, and quit immediately.

NOW EXECUTE

## Task: Create Scratch Docker Compose Stack <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Create a scratch-based Dockerfile for the `nsx-operator` binary and a Docker Compose stack that uses that Dockerfile while also starting kine in in-memory mode, a Kubernetes API server backed by kine, and the sibling `../nsx-t-mockapi` service.

The higher order goal is to make local end-to-end operator execution reproducible with real process boundaries: the operator should run from a minimal container image, talk to a live kube-apiserver, and use the NSX-T mock API as its NSX endpoint.

In scope: add a Dockerfile or clearly named Dockerfile variant that builds the Go operator and produces a final `scratch` runtime image; add a Docker Compose file that builds the operator image from that Dockerfile; include services for kine using in-memory storage, kube-apiserver configured to use kine as storage, the operator, and `../nsx-t-mockapi`; wire service networking, ports, health checks or readiness waits, volumes/config, and environment variables needed for the operator to reach both Kubernetes and NSX-T mockapi; document the exact commands needed to build, start, inspect, and stop the stack.

The Compose stack must not rely on a developer's host Kubernetes context to make the core flow work. It should create or mount only the kubeconfig/config artifacts it needs, and those generated artifacts must be treated as generated output. The operator container must use the repo's zap JSONL logging behavior to stderr and keep errors handled explicitly.

Out of scope: deploying to a real NSX-T manager, publishing images to a registry, replacing the existing Go test/envtest path, or changing production reconciliation behavior except where a small configuration hook is genuinely required to run in the Compose environment.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] A scratch-based operator Dockerfile builds successfully from a clean repo checkout.
- [x] Docker Compose builds the operator image from that Dockerfile.
- [x] Docker Compose starts kine in in-memory mode.
- [x] Docker Compose starts kube-apiserver and kube-apiserver is reachable via `kubectl` or an equivalent concrete API call.
- [x] Docker Compose starts the sibling `../nsx-t-mockapi` service and the operator can reach it by service name.
- [x] Operator logs are emitted as structured zap JSONL to stderr inside the Compose stack.
- [x] Verification evidence includes successful `docker compose build`, `docker compose up`, service health/status output, kube-apiserver API evidence, nsx-t-mockapi API evidence, and operator log evidence.
- [x] `docker compose down` or the documented cleanup command removes the stack cleanly.
</acceptance_criteria>

<verification_evidence>
Commands executed successfully on 2026-05-19:

- `./hack/compose/generate-kube-assets.sh`
  - Wrote generated Kubernetes assets under `hack/compose/generated`.
- `docker compose config`
  - Rendered the Compose stack successfully after command values were quoted.
- `docker compose build`
  - Built `nsx-operator:scratch` from `Dockerfile.scratch`.
  - Built `nsx-t-mockapi:scratch` from sibling `../nsx-t-mockapi/Dockerfile`.
- `docker compose up -d`
  - Started kine using `ghcr.io/k3s-io/kine:v0.14.6-k3s1.32` with in-memory SQLite DSN `sqlite://file::memory:?cache=shared`.
  - Started `registry.k8s.io/kube-apiserver:v1.32.0` backed by kine.
  - Ran `crd-init` to apply CRDs and sample resources.
  - Started `nsx-t-mockapi` and the scratch `operator` service.
- `docker compose ps`
  - Showed `kine`, `kube-apiserver`, `nsx-t-mockapi`, and `operator` all running.
  - Kube-apiserver exposed `0.0.0.0:16443->6443/tcp`.
  - Mock API exposed `0.0.0.0:18080->8080/tcp`.
- `kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get --raw=/readyz`
  - Returned `ok`.
- `kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get nsxnetworkclouds.nsx.ing.com`
  - Showed `mockapi` with `FQDN=nsx-t-mockapi:8080`, `Reachable=True`, and `Swept=True`.
- `kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get nsxgroups.nsx.ing.com`
  - Showed `compose-managed-web` with `FQDN=nsx-t-mockapi:8080`, `GROUPID=compose-managed-web`, and `MODE=Manage`.
- `docker compose run --rm --entrypoint curl crd-init -fsS -u nsx_admin:nsx_password http://nsx-t-mockapi:8080/policy/api/v1/infra/domains/default/groups`
  - Returned JSON containing `result_count: 1` and group `compose-managed-web`, proving service-name access to the sibling mock API inside the Compose network.
- `docker compose logs --no-color operator`
  - Emitted structured zap JSONL lines to Docker logs from the operator process.
  - Evidence included `loaded startup config`, `constructed nsx manager client` with `baseURL=http://nsx-t-mockapi:8080`, `sending nsx list request`, `completed nsx request` with `statusCode=200`, `default manager gather completed`, and `submitted manage group apply`.
- `docker compose down --volumes --remove-orphans`
  - Stopped and removed the operator, crd-init, kube-apiserver, kine, and nsx-t-mockapi containers.
  - Removed `nsx-operator-scratch_nsx-mockapi-data` and the `nsx-operator-scratch_default` network.

Automated checks executed successfully:

- `make check`
  - Passed fmt, vet, golangci-lint, normal tests, race tests, contract tests, e2e tests, large-chaos tests, and coverage.
  - Coverage result: `coverage 82.7% meets 80.0% threshold`.
- `make test`
  - Passed all Go packages with envtest assets.
- `make test-coverage`
  - Passed all package coverage checks and total coverage threshold.
  - Coverage result: `coverage 82.7% meets 80.0% threshold`.

Design/boundary evidence:

- `nsx.urlScheme` is parsed, defaulted, and validated in `internal/config`.
- Startup URL rendering remains in `internal/startup`, next to NSX client construction.
- Compose-generated TLS and kubeconfig artifacts are isolated under ignored `hack/compose/generated/`.
- Compose bootstrap files do not add reconciliation-specific code paths.
</verification_evidence>

<execution_plan>
.ralph/tasks/19-story-scratch-compose-stack/01-task-create-scratch-docker-compose-stack_plans/01-compose-stack-plan.md
NOW EXECUTE
</execution_plan>

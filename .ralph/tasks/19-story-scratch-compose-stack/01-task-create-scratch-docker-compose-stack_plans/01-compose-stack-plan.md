# Plan: Scratch Docker Compose Stack

## Current Context

- Task file: `.ralph/tasks/19-story-scratch-compose-stack/01-task-create-scratch-docker-compose-stack.md`.
- The operator entrypoint is `cmd/nsx-operator/main.go` and requires `-config`.
- Runtime startup loads validated config in `internal/config` and constructs the controller-runtime manager in `internal/startup`.
- `internal/startup/manager.go` currently builds every NSX manager URL as `https://` plus the normalized `NSXNetworkCloud.spec.networkCloudFQDN`.
- The sibling mock API exists at `../nsx-t-mockapi`, already has a scratch Dockerfile, and its Compose service exposes plain HTTP on port `8080`.
- The Compose stack must use real process boundaries and must not depend on a developer host Kubernetes context.

## Boundary Plan

Use `$improve-code-boundaries` during implementation to keep the change from becoming bootstrap string soup:

- Put any new operator runtime configuration in `internal/config`; parse and validate it once there.
- Add only the minimum hook needed for the Compose environment: an NSX manager URL scheme with a production-safe default of `https` and an explicit Compose value of `http`.
- Keep URL construction in `internal/startup`, close to the manager client factory that owns NSX client creation.
- Do not add ad-hoc environment parsing in `cmd/nsx-operator` or Compose-only code paths in reconciliation logic.
- Keep generated TLS, kubeconfig, and runtime sample manifests under a dedicated generated directory, and ignore that directory rather than scattering generated files through the repo.

## TDD / Verification Plan

Use `$tdd` for code changes and executable validation for infrastructure. For the Dockerfile and Compose files, do not write brittle tests that assert file text. Verify by actually building and running the stack.

1. RED: add a config behavior test proving `nsx.urlScheme: http` loads as valid config and the default remains `https`.
2. GREEN: implement `NSXConfig.URLScheme` in `internal/config`, default to `https`, and validate only `http` or `https`.
3. RED: add a startup/manager behavior test proving the manager client factory builds an HTTP mock API URL when config selects `http`.
4. GREEN: change `internal/startup/manager.go` to use the validated config scheme instead of hard-coded `https`.
5. Refactor: check for boundary smells after tests pass; keep validation in config and avoid duplicate URL rendering.
6. Infrastructure executable checks:
   - Build the scratch operator image with the new Dockerfile.
   - Bring up kine with in-memory storage.
   - Bring up kube-apiserver against kine and prove it answers an API call.
   - Bring up `../nsx-t-mockapi` by service name and prove it answers an authenticated API call.
   - Start the operator from the scratch image and prove zap JSONL logs are on stderr.
   - Apply the CRDs and a small `NSXNetworkCloud`/`NSXGroup` sample through the Compose kube-apiserver to prove the operator can reach the mock API.

## Implementation Steps

- Add `.dockerignore` for build context hygiene.
- Add `Dockerfile.scratch`:
  - build the static `nsx-operator` binary from Go with `CGO_ENABLED=0`;
  - copy CA certificates from a small builder/runtime source so HTTPS Kubernetes and optional HTTPS NSX endpoints work from `scratch`;
  - run the final image with only the binary and required certificate bundle.
- Add Compose-owned runtime artifacts:
  - `hack/compose/nsx-operator-config.yaml` with debug logging, explicit `nsx.urlScheme: http`, and mock API credentials `nsx_admin/nsx_password`;
  - `hack/compose/manifests/` for CRDs and minimal sample resources;
  - `hack/compose/generated/` as generated output for kube-apiserver certificates and kubeconfig.
- Add or choose a small script only if needed to keep commands reliable:
  - generate a local CA, kube-apiserver serving cert, service account keys, and kubeconfig under `hack/compose/generated/`;
  - explicitly check every command error; do not ignore errors.
- Add `compose.yaml`:
  - `kine` service in memory mode;
  - `kube-apiserver` service using `registry.k8s.io/kube-apiserver:v1.32.0` and `--etcd-servers=http://kine:2379`;
  - `nsx-t-mockapi` service built from `../nsx-t-mockapi`;
  - `operator` service built from `Dockerfile.scratch`, mounting only config and kubeconfig/cert artifacts needed at runtime;
  - health checks/readiness gates where the image has a usable probe binary; otherwise document concrete readiness commands run from the host or a short-lived kubectl/curl container.
- Document exact commands in a repo-local doc, likely `docs/compose-stack.md`:
  - generate artifacts;
  - `docker compose build`;
  - `docker compose up -d`;
  - service status;
  - kube-apiserver API proof with `kubectl --kubeconfig hack/compose/generated/kubeconfig.yaml`;
  - mock API proof by service name from inside the Compose network;
  - operator log proof using `docker compose logs operator` with JSONL examples;
  - cleanup with `docker compose down --volumes --remove-orphans`.
- Record concrete verification evidence back into the task file once execution passes.

## Required Final Checks

- `make check`
- `make test`
- `make test-coverage` with total coverage at or above 80%
- Manual Compose evidence:
  - successful `docker compose build`;
  - successful `docker compose up`;
  - `docker compose ps` or equivalent service status;
  - kube-apiserver API evidence;
  - mock API service-name evidence;
  - operator JSONL stderr evidence;
  - clean `docker compose down`.
- Final `$improve-code-boundaries` pass: specifically re-check that URL scheme validation is not duplicated, generated artifacts are isolated, and Compose bootstrap work does not leak into production reconciliation.

NOW EXECUTE

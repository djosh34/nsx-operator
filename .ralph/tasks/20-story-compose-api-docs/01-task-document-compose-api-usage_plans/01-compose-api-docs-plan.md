# Plan: Document Compose API Usage

## Task

Document the existing Docker Compose API/operator workflow so a developer with this repository and sibling `../nsx-t-mockapi` checkout can build, start, inspect, and cleanly remove the local stack.

## Current State

- `compose.yaml` defines the `nsx-operator-scratch` project with `kine`, `kube-apiserver`, `nsx-t-mockapi`, `crd-init`, and `operator` services.
- `hack/compose/generate-kube-assets.sh` creates generated TLS assets plus `kubeconfig-host.yaml` and `kubeconfig-operator.yaml` under `hack/compose/generated/`.
- `hack/compose/nsx-operator-config.yaml` configures debug logging, HTTP NSX access, and mock API credentials.
- `hack/compose/manifests/sample.yaml` creates one `NSXNetworkCloud` and one managed `NSXGroup` targeting `nsx-t-mockapi:8080`.
- `docs/compose-stack.md` already exists but is too sparse for this task's full acceptance criteria.
- `README.md` exists but is empty, so the Compose docs are not easy to discover from the repository root.

## TDD / Verification Strategy

This is a documentation-only task, so the TDD exception applies: do not add tests that assert Markdown strings or file contents. Instead, use the TDD mindset by treating each documented command as the public interface and proving it through execution.

- [x] Update one documentation slice at a time.
- [x] Run the corresponding documented command.
- [x] If the command output or behavior differs, fix the documentation immediately before moving to the next slice.
- [x] Record concrete evidence in the task file, not only in terminal history.

## Documentation Interface

- [x] Use `docs/compose-stack.md` as the detailed README-style entry point for the Compose API stack.
- [x] Use root `README.md` only as a discoverability entry point that links to `docs/compose-stack.md`, avoiding duplicated command blocks that can drift.
- [x] Keep generated credential details out of docs; mention only paths and regeneration/removal commands.
- [x] Document the stack as self-contained and not dependent on a host Kubernetes context for the core local flow.

## Behaviors To Document And Verify

- [x] Prerequisites: Docker, Docker Compose plugin, `kubectl`, `openssl`, `base64`, and sibling `../nsx-t-mockapi` checkout with its `Dockerfile` and `config/config.yaml`.
- [x] Build/start path:
  - `./hack/compose/generate-kube-assets.sh`
  - `docker compose config`
  - `docker compose build`
  - `docker compose up -d`
  - `docker compose ps`
- [x] Kubernetes API proof:
  - `kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get --raw=/readyz`
  - `kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get nsxnetworkclouds.nsx.ing.com`
  - `kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get nsxgroups.nsx.ing.com`
- [x] NSX-T mock API proof:
  - From inside the Compose network with `docker compose run --rm --entrypoint curl crd-init ... http://nsx-t-mockapi:8080/...`.
  - From the host through the published default port `18080`, if available.
- [x] Operator log proof:
  - `docker compose logs --no-color operator`
  - Mention structured zap JSONL on stderr, with Docker logs merging stdout and stderr.
- [x] Cleanup:
  - `docker compose down --volumes --remove-orphans`
  - `rm -rf hack/compose/generated`
- [x] Troubleshooting:
  - Missing sibling `../nsx-t-mockapi` checkout or config.
  - Port conflicts on `NSX_OPERATOR_KUBE_APISERVER_PORT` default `16443` and `NSX_T_MOCKAPI_PORT` default `18080`.
  - Unhealthy or exited services, especially `crd-init`.
  - Stale generated kubeconfig/cert artifacts.

## Improve-Code-Boundaries Review

- [x] Keep detailed Compose workflow knowledge in one docs module, `docs/compose-stack.md`.
- [x] Keep root `README.md` shallow so there is one authoritative command surface.
- [x] Do not introduce code or config changes unless verification proves the existing Compose workflow is wrong.
- [x] If documentation exposes a confusing code/config boundary, note it and only refactor if needed for the docs to be truthful and executable.
- [x] Before completion, reread the changed docs for muddy boundaries, duplicate instructions, secret leakage, or host-context assumptions.

## Execution Steps

- [x] Expand `docs/compose-stack.md` with prerequisites, exact service/port/artifact names, commands, expected evidence, logs, cleanup, and troubleshooting.
- [x] Add a short root `README.md` entry point linking to `docs/compose-stack.md`.
- [x] Run the documented Compose flow from a clean-ish state:
  - [x] `docker compose down --volumes --remove-orphans`
  - [x] `rm -rf hack/compose/generated`
  - [x] `./hack/compose/generate-kube-assets.sh`
  - [x] `docker compose config`
  - [x] `docker compose build`
  - [x] `docker compose up -d`
  - [x] `docker compose ps`
  - [x] Kubernetes API checks.
  - [x] NSX-T mock API checks.
  - [x] Operator log check.
  - [x] Cleanup commands.
- [x] Update the task file acceptance criteria and add `<verification_evidence>` with concrete command results.
- [x] Run `make check`.
- [x] Run `make test`.
- [x] Run `make test-coverage` and confirm total coverage remains at least 80%.
- [x] Perform final improve-code-boundaries review.
- [x] Set `<passes>true</passes>` only after all checks pass.
- [x] Run `/bin/bash .ralph/task_switch.sh`.
- [ ] Commit all files with `task finished 01-task-document-compose-api-usage: document compose api workflow`.
- [ ] Push.

NOW EXECUTE

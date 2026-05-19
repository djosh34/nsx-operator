## Task: Rationalize Config CRDs Scripts And Dockerfile Layout <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Make repository directories and file names more sensible. Move configuration material to `./config`, move CRD manifests that currently live under paths such as `./config/crds/base/...` to `./crds`, move scripts to `./scripts`, and rename `dockerfile.scratch` so the Dockerfile is named `Dockerfile`.

The task must update all references that are affected by the moves, including Makefile targets, documentation, compose files, CI workflows, tests, scripts, manifests, and any embedded paths. The resulting layout must be easy to understand from the repo root: operator config under `config`, CRDs under `crds`, executable helper scripts under `scripts`, and the scratch Dockerfile available as `Dockerfile`.

In scope: move files/directories; update references; preserve executable bits for scripts; update tests and docs that mention old paths; verify build/test commands still work after the move.

Out of scope: changing CRD schema content; changing runtime behavior unrelated to path references; broad repository cleanup beyond the requested path moves.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Config files are located under `./config` in the final layout.
- [x] CRD manifests previously under `./config/crds/base/...` or equivalent nested config CRD paths are located under `./crds`.
- [x] Repository helper scripts are located under `./scripts` and remain executable where applicable.
- [x] `dockerfile.scratch` is renamed or replaced so the intended Dockerfile path is `Dockerfile`.
- [x] All Makefile, CI, compose, docs, tests, and script references are updated to the new paths.
- [x] There are no stale references to the old paths except in intentional historical notes or task evidence.
- [x] Build, test, CRD install/apply, and Docker build verification commands pass with the new layout.
</acceptance_criteria>

<verification_evidence>
Performed 2026-05-19 from repository root.

- Baseline old-layout evidence before moves:
  - `rg -n "Dockerfile\\.scratch|config/crd|hack/compose" -S . --glob '!**/.git/**'` found old references in `compose.yaml`, `docs/compose-stack.md`, and `hack/compose/generate-kube-assets.sh`.
  - `find config hack -maxdepth 4 -type f | sort` showed CRDs under `config/crd/bases/`, compose config/manifests/script under `hack/compose/`.
  - `find config hack -maxdepth 5 -printf '%M %p\n' | sort` showed `hack/compose/generate-kube-assets.sh` was executable.
- Final layout evidence:
  - `find crds config scripts -maxdepth 4 -printf '%M %p\n' | sort` showed:
    - `crds/nsx.ing.com_nsxgroups.yaml`
    - `crds/nsx.ing.com_nsxnetworkclouds.yaml`
    - `config/compose/nsx-operator-config.yaml`
    - `config/compose/manifests/sample.yaml`
    - executable `scripts/generate-compose-kube-assets.sh` with mode `-rwxr-xr-x`.
  - `ls -la Dockerfile Dockerfile.scratch dockerfile.scratch` showed `Dockerfile` exists and both old names are absent.
  - Active-source stale sweep passed: `rg -n "config/crd/bases|config/crd|hack/compose|Dockerfile\\.scratch|dockerfile\\.scratch" -S api internal cmd compose.yaml docs .github .gitignore .dockerignore Makefile scripts crds config` returned no matches.
- Generated asset and Compose config evidence:
  - `./scripts/generate-compose-kube-assets.sh` exited 0 and wrote assets under `config/compose/generated`.
  - `find config/compose/generated -maxdepth 2 -printf '%M %p\n' | sort` showed private keys and kubeconfigs with `-rw-------`.
  - `docker compose config` exited 0 and resolved bind mounts to `crds`, `config/compose/manifests`, `config/compose/nsx-operator-config.yaml`, and `config/compose/generated`.
- Build and test evidence:
  - `docker build -f Dockerfile -t nsx-operator:layout-check .` exited 0.
  - `docker compose build` exited 0 and built `nsx-operator:scratch` plus `nsx-t-mockapi:scratch`.
  - `make check` exited 0, including fmt, vet, lint, normal tests, race tests, contract tests, e2e tests, large chaos tests, and coverage.
  - `make test` exited 0.
  - `make test-coverage` exited 0 with `coverage 83.6% meets 80.0% threshold`.
- Compose CRD install/apply and runtime evidence:
  - `docker compose down --volumes --remove-orphans && rm -rf config/compose/generated && ./scripts/generate-compose-kube-assets.sh && docker compose up -d` exited 0.
  - `docker compose ps && docker compose ps -a crd-init` showed `kine`, `kube-apiserver`, `nsx-t-mockapi`, and `operator` running, and `crd-init` `Exited (0)`.
  - `kubectl --kubeconfig config/compose/generated/kubeconfig-host.yaml get --raw=/readyz` returned `ok`.
  - `kubectl --kubeconfig config/compose/generated/kubeconfig-host.yaml get nsxnetworkclouds.nsx.ing.com` listed `mockapi` with `REACHABLE True` and `SWEPT True`.
  - `kubectl --kubeconfig config/compose/generated/kubeconfig-host.yaml get nsxgroups.nsx.ing.com` listed `compose-managed-web`.
  - `docker compose logs --no-color --tail=80 crd-init` showed CRDs created from `/crds`, both CRDs reached condition met, and sample resources were created.
  - `docker compose logs --no-color --tail=80 operator` showed zap JSON logs for `starting default manager sweep`, `constructed nsx manager client`, `completed nsx request`, and `completed default manager sweep`.
  - `docker compose run --rm --entrypoint curl crd-init -fsS -u nsx_admin:nsx_password http://nsx-t-mockapi:8080/policy/api/v1/infra/domains/default/groups` returned JSON containing `compose-managed-web`.
  - `docker compose down --volumes --remove-orphans` exited 0 and removed containers, volume, and network.
- Boundary review:
  - Re-read the `improve-code-boundaries` skill and confirmed there are no compatibility wrappers or duplicated old-path interfaces. Ownership is now: `Dockerfile` for image build, `crds/` for CRD manifests, `config/compose/` for compose config and manifests, `scripts/` for executable helper workflows.
</verification_evidence>

<plan>
.ralph/tasks/26-story-repository-layout/01-task-rationalize-config-crds-scripts-and-dockerfile_plans/2026-05-19-layout-rationalization.md
</plan>

NOW EXECUTE

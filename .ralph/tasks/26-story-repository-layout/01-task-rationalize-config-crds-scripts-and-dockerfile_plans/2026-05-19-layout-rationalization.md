# Plan: Rationalize Config, CRDs, Scripts, And Dockerfile Layout

Task: `.ralph/tasks/26-story-repository-layout/01-task-rationalize-config-crds-scripts-and-dockerfile.md`

## Current Findings

- `Dockerfile.scratch` is the operator image Dockerfile and is referenced by `compose.yaml`, `.github/workflows/ci-cd.yml`, and `docs/compose-stack.md`.
- CRD YAMLs currently live under `config/crd/bases/`.
- Compose-only material currently lives under `hack/compose/`:
  - executable helper script: `hack/compose/generate-kube-assets.sh`
  - generated local TLS/kubeconfig output: `hack/compose/generated/`
  - operator runtime config: `hack/compose/nsx-operator-config.yaml`
  - sample custom resources: `hack/compose/manifests/sample.yaml`
- Ignore files currently ignore `hack/compose/generated`.
- The task is mostly repository layout and path wiring, so the TDD exception applies: do not add brittle tests that assert strings inside YAML, Dockerfiles, workflow files, or docs. Use the `tdd` skill as a red-green mindset by making one observable command fail on the old path expectation, moving the smallest slice, and verifying the command now passes.

## Target Layout

- Rename `Dockerfile.scratch` to `Dockerfile`.
- Move CRD manifests from `config/crd/bases/*.yaml` to `crds/*.yaml`.
- Move `hack/compose/generate-kube-assets.sh` to `scripts/generate-compose-kube-assets.sh` and preserve executable mode.
- Move `hack/compose/nsx-operator-config.yaml` to `config/compose/nsx-operator-config.yaml`.
- Move `hack/compose/manifests/sample.yaml` to `config/compose/manifests/sample.yaml`.
- Update the script so generated TLS and kubeconfig output goes to `config/compose/generated/`.
- Remove the now-empty `hack/` tree after moves.

## Boundary Cleanup

Use the `improve-code-boundaries` skill to flatten the layout boundary:

- `config/` owns operator and compose configuration only.
- `crds/` owns Kubernetes CRD definitions directly from the repository root.
- `scripts/` owns executable helper workflows.
- `Dockerfile` is the canonical image build interface for Compose and CI.
- Avoid compatibility shims such as duplicate old-path wrapper scripts or duplicate Dockerfiles; those keep the old boundary alive and make later tasks muddier.

## Red-Green Execution Slices

1. Baseline path evidence:
   - Run `rg -n "Dockerfile\\.scratch|config/crd|hack/compose" -S . --glob '!**/.git/**'`.
   - Run `find config hack -maxdepth 4 -type f | sort` and `find . -maxdepth 2 -type f -perm -111 | sort`.
   - Record this as old-layout evidence in progress/task notes.

2. Dockerfile slice:
   - Rename `Dockerfile.scratch` to `Dockerfile`.
   - Update `.github/workflows/ci-cd.yml`, `compose.yaml`, and docs to build from `Dockerfile`.
   - Verify with `docker build -f Dockerfile -t nsx-operator:layout-check .` when Docker is available.

3. CRD slice:
   - Move `config/crd/bases/nsx.ing.com_nsxgroups.yaml` and `config/crd/bases/nsx.ing.com_nsxnetworkclouds.yaml` to `crds/`.
   - Update `compose.yaml` so `crd-init` mounts `./crds`.
   - Verify with `kubectl apply --dry-run=client -f crds` if `kubectl` is available, and with the full Compose CRD install path later.

4. Compose config and sample manifest slice:
   - Move `hack/compose/nsx-operator-config.yaml` to `config/compose/nsx-operator-config.yaml`.
   - Move `hack/compose/manifests/sample.yaml` to `config/compose/manifests/sample.yaml`.
   - Update `compose.yaml` mounts and `docs/compose-stack.md` references.
   - Verify `docker compose config` resolves the new bind paths.

5. Script/generated output slice:
   - Move `hack/compose/generate-kube-assets.sh` to `scripts/generate-compose-kube-assets.sh`.
   - Update the script's `repo_root` calculation for the new depth.
   - Change `generated_dir` to `${repo_root}/config/compose/generated`.
   - Update `.gitignore`, `.dockerignore`, docs, and any compose bind paths to `config/compose/generated`.
   - Run `./scripts/generate-compose-kube-assets.sh`.
   - Verify expected generated files exist under `config/compose/generated/` and private files retain `0600`.

6. Stale reference sweep:
   - Run `rg -n "Dockerfile\\.scratch|config/crd|hack/compose" -S . --glob '!**/.git/**'`.
   - Resolve every stale reference outside intentional Ralph task/history/progress evidence.
   - Run `find . -maxdepth 3 -type d | sort` to confirm `hack/` is gone and the root layout is understandable.

7. Full verification:
   - Run `make check`.
   - Run `make test`.
   - Run `make test-coverage` and confirm total coverage is at least `80.0%`.
   - Run `docker compose config`.
   - Run `docker compose build`.
   - Run `docker compose down --volumes --remove-orphans`, `./scripts/generate-compose-kube-assets.sh`, `docker compose up -d`, `docker compose ps`, `docker compose ps -a crd-init`, host `kubectl --kubeconfig config/compose/generated/kubeconfig-host.yaml get --raw=/readyz`, resource `kubectl` checks, operator logs check, then `docker compose down --volumes --remove-orphans`.
   - If Docker, Compose, kubectl, or sibling `../nsx-t-mockapi` are unavailable, record the exact command failure as a blocker instead of marking passes true.

8. Final boundary review:
   - Re-read the `improve-code-boundaries` skill.
   - Inspect changed files for duplicated path compatibility, old directory concepts, or mixed ownership.
   - Remove any compatibility leftovers before completing.

## Completion Steps

- Update the task file with concrete verification evidence.
- Set `<passes>true</passes>` only after all required checks pass.
- Run `/bin/bash .ralph/task_switch.sh`.
- `git add -A`.
- Commit with `task finished 01-task-rationalize-config-crds-scripts-and-dockerfile: rationalize repository layout` and include evidence for tests/checks in the commit message.
- Push.

NOW EXECUTE

# Delete All CRs Script Plan

Plan path: `.ralph/tasks/27-story-cr-cleanup-script/01-task-add-delete-all-crs-script_plans/01-delete-all-crs-script-plan.md`

## Required Skills Read

- `$tdd`: use vertical red-green slices, behavior tests through public interfaces, and do not add brittle source-string tests for shell content.
- `$improve-code-boundaries`: keep the solution at the script/process boundary, avoid a needless Go cleanup CLI, and do not introduce duplicate Kubernetes resource DTOs or helper layers.

## Current Facts

- The repo already has `scripts/`, containing focused executable helpers and a Go test file that runs scripts as subprocesses with fake tools.
- The actual NSX group CRD is `crds/nsx.ing.com_nsxgroups.yaml`.
- The actual CRD name is `nsxgroups.nsx.ing.com`.
- The actual resource plural is `nsxgroups`, singular `nsxgroup`, kind `NSXGroup`, list kind `NSXGroupList`.
- The target resource is namespaced, so `kubectl patch` must include both `--namespace <namespace>` and the object name.
- `make test` runs `go test ./...`, so behavior tests under `scripts/` are included in the normal suite.

## Public Interface

- Add executable script: `scripts/delete-all-crs.sh`.
- The script takes no arguments.
- It uses `kubectl` from `PATH`.
- It targets exactly `nsxgroups.nsx.ing.com` to avoid ambiguous short/plural resource resolution and to document the project-confirmed CRD group.
- It lists targets across all namespaces using:
  - `kubectl get nsxgroups.nsx.ing.com -A -o jsonpath=...`
- For each listed resource, it clears finalizers with:
  - `kubectl patch nsxgroups.nsx.ing.com <name> --namespace <namespace> --type=merge -p '{"metadata":{"finalizers":[]}}'`
- After patching, it deletes all targets with:
  - `kubectl delete nsxgroups.nsx.ing.com -A --all`
- It must keep `set -euo pipefail`.
- It should fail with a useful missing-tool message if `kubectl` is absent.
- It should not fail only because GNU `parallel` is absent.

## Boundary Design

- The shell script is the deep module for this task: Kubernetes discovery, finalizer clearing, and delete-all in one focused operational workflow.
- Keep shell helpers minimal. A single `require_command` helper is acceptable because the repo already uses it for script tool checks.
- Do not add a Go package, typed CR cleanup API, generic kubectl wrapper, or configurable resource abstraction for one operational script.
- Keep tests at the process boundary by executing `scripts/delete-all-crs.sh` and observing fake `kubectl` and fake `parallel` behavior.
- Avoid source-code substring tests. Verify expected calls through executable fixtures and captured argv/stdin.

## TDD Execution Plan

1. [x] RED tracer bullet: add a behavior test in `scripts/delete_all_crs_test.go` that runs `./delete-all-crs.sh` with a temporary `PATH` containing fake `kubectl` and no `parallel`.
   - Fake `kubectl get` returns two tab-separated namespace/name rows for different namespaces.
   - Fake `kubectl patch` records argv and requires resource, name, namespace, merge type, and finalizer patch payload.
   - Fake `kubectl delete` records argv and requires `delete nsxgroups.nsx.ing.com -A --all`.
   - The test proves the serial fallback works when `parallel` is unavailable.
   - Expected initial failure: script does not exist.
2. [x] GREEN: add `scripts/delete-all-crs.sh` with `#!/usr/bin/env bash`, `set -euo pipefail`, `require_command kubectl`, `RESOURCE="nsxgroups.nsx.ing.com"`, serial patch loop, and final delete.
3. [x] RED: add a second behavior test that places fake GNU `parallel` ahead of `kubectl` in `PATH`.
   - Fake `parallel` must require a max concurrency of 20, such as `-j 20`.
   - It should execute the supplied shell snippet once per input row so the test still observes the actual `kubectl patch` commands.
   - The test proves the script uses GNU `parallel` when available.
4. [x] GREEN: update the script to detect `parallel` with `command -v parallel` and run patching through `parallel -j 20` when present.
5. [x] RED/GREEN if needed: add a focused missing-`kubectl` test that runs the script with a `PATH` containing bash but no `kubectl` and verifies it fails with `missing required command: kubectl`.
   - No separate test was added because the minimal script already introduced `require_command kubectl` during the first GREEN slice; adding a green-only test afterward would violate the TDD rule.
6. [x] Refactor pass using `$improve-code-boundaries`:
   - keep the shell workflow readable in one file.
   - keep fake command setup local to the script test file unless duplication becomes meaningful.
   - do not add production options or test-only production flags.
7. [x] Set executable bit on `scripts/delete-all-crs.sh`.

## Manual Verification Plan

Record concrete evidence in the task file after execution.

Preferred verification:

- Run the script against the compose/envtest Kubernetes API if a usable cluster is available and confirm `nsxgroups.nsx.ing.com` resources with finalizers are patched and deleted.

Fallback verification if a live cluster is not available:

- Run `go test ./scripts -run TestDeleteAllCRs -count=1` to prove serial and parallel behavior through fake executable tools.
- Run an additional manual command with temporary fake `kubectl` and fake `parallel` fixtures, then show captured calls proving:
  - cross-namespace list was requested.
  - each patch included the correct namespace and name.
  - finalizer payload was exactly the merge patch object.
  - GNU `parallel` used max concurrency 20.
  - delete-all was invoked after patching.

## Required Final Checks

- [x] `make check`
- [x] `make test`
- [x] `make test-coverage` reports at least 80%.
- [x] Final `$improve-code-boundaries` pass confirms no unnecessary CLI/package/resource abstraction was introduced and no new muddy boundary exists.
- [x] Update task acceptance criteria and verification evidence.
- [x] Set `<passes>true</passes>` only after all required checks pass.
- [x] Run `/bin/bash .ralph/task_switch.sh`.
- [x] Commit all files, including `.ralph`, with `task finished 01-task-add-delete-all-crs-script: ...`.
- [ ] Push.

If implementation proves the script name, resource name, namespace handling, parallel invocation contract, or verification boundary is wrong, replace the final marker with `TO BE VERIFIED`, document the proposed design change here, and quit immediately.

NOW EXECUTE

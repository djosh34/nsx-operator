# NetworkCloud Ingest Script Plan

Plan path: `.ralph/tasks/29-story-networkcloud-ingest-script/01-task-add-networkcloud-ingest-script_plans/01-networkcloud-ingest-script-plan.md`

## Required Skills Read

- `$tdd`: use vertical red-green slices, behavior tests through public interfaces, and avoid brittle implementation assertions.
- `$improve-code-boundaries`: keep the script boundary small and avoid introducing a needless Go CLI, duplicate DTO, or manifest-rendering abstraction.

## Current Facts

- The actual NetworkCloud API is `apiVersion: nsx.ing.com/v1alpha` from `api/v1alpha/types.go`.
- The actual kind is `NSXNetworkCloud`.
- The required spec fields are `networkCloudFQDN`, `networkCloudId`, and `name`.
- The CRD confirms the resource is cluster-scoped and requires exactly those NetworkCloud spec fields.
- `scripts/` currently contains `generate-compose-kube-assets.sh`; add the new executable there.
- `make test` runs `go test ./...`, so a Go smoke test under `scripts/` is a simple way to include script behavior in the normal test suite.

## Public Interface

- Add executable script: `scripts/ingest-networkcloud.sh`.
- It reads one JSON object from stdin with:
  - `.name`
  - `.fqdn`
  - `.id`
- It emits the transformed object through the requested command shape:
  - `jq` renders JSON.
  - `yq -p=json` converts JSON to YAML.
  - `kubectl apply -f -` receives the manifest on stdin.
- Keep tool validation minimal and useful:
  - fail early if `jq`, `yq`, or `kubectl` are unavailable.
  - do not add prompts, bulk import, alternate formats, or a management CLI.

## Boundary Design

- The shell script is the deep module for this task: stdin JSON to Kubernetes apply.
- Do not add Go CR constructors, duplicate `NSXNetworkCloudSpec` structs, or a rendering package for one simple pipeline.
- Keep tests at the process boundary by executing the script and observing fake command behavior plus parsed manifest content.
- Keep the CRD/API type layer authoritative for field names; the script should hard-code only the confirmed public manifest shape.

## TDD Execution Plan

1. [x] RED tracer bullet: add `scripts/ingest_networkcloud_test.go` that executes `scripts/ingest-networkcloud.sh` with a sample JSON object and a temporary `PATH` containing fake `yq` and fake `kubectl`.
   - Fake `yq` must require `-p=json`, forward stdin to stdout, and exit nonzero on wrong args.
   - Fake `kubectl` must require `apply -f -`, capture stdin to a temp file, and exit nonzero on wrong args.
   - The test must parse captured stdin as YAML using `gopkg.in/yaml.v3` and assert behavior fields, not raw string containment:
     - `apiVersion == nsx.ing.com/v1alpha`
     - `kind == NSXNetworkCloud`
     - `metadata.name == input.name`
     - `spec.name == input.name`
     - `spec.networkCloudFQDN == input.fqdn`
     - `spec.networkCloudId == input.id`
   - Expected initial failure: script does not exist.
2. [x] GREEN: add `scripts/ingest-networkcloud.sh` with `#!/usr/bin/env bash`, `set -euo pipefail`, minimal `require_command`, the `jq` transform, `yq -p=json`, and `kubectl apply -f -`.
3. [x] RED: extend the process-boundary test with missing-tool behavior by running with a temp `PATH` where one required tool is absent and asserting the process fails with a useful stderr message.
   - Do not assert broad script source text.
   - Keep this as command execution behavior only.
4. [x] GREEN: ensure required tool validation covers `jq`, `yq`, and `kubectl` before the pipeline starts.
5. [x] Refactor pass using `$improve-code-boundaries`:
   - keep only one helper if it removes repeated command checks.
   - avoid adding generic manifest functions or test-only production flags.
   - keep fake command creation inside the test file.
6. [x] Set executable bit on `scripts/ingest-networkcloud.sh`.

## Manual Verification Plan

Record concrete evidence in the task file after execution.

Commands to run after implementation:

```bash
printf '%s\n' '{"name":"cloud-a","fqdn":"nsx-a.example.net","id":"cloud-a-id"}' \
  | ./scripts/ingest-networkcloud.sh
```

Apply-path verification options:

- Preferred if a local compose Kubernetes API is available:
  - generate/start compose assets if needed.
  - set `KUBECONFIG=config/compose/generated/kubeconfig-host.yaml`.
  - run the script and then `kubectl get nsxnetworkcloud cloud-a -o yaml`.
  - record the relevant command outputs.
- If no live compose API is available during execution:
  - run the smoke test proving `kubectl apply -f -` receives the generated manifest.
  - run an additional manual command with a temporary fake `kubectl` and real `jq`/`yq`, capture the YAML file, parse/show it with `yq`, and record that evidence.

## Required Final Checks

- [x] `make check`
- [x] `make test`
- [x] `make test-coverage` reports at least 80%.
- [x] Final `$improve-code-boundaries` pass confirms no needless DTO, package, or CLI layer was introduced.
- [x] Update task acceptance criteria and evidence.
- [x] Set `<passes>true</passes>` only after all required checks pass.
- [x] Run `/bin/bash .ralph/task_switch.sh`.
- [x] Commit all files, including `.ralph`, with `task finished 01-task-add-networkcloud-ingest-script: ...`.
- [x] Push.

If implementation proves the script interface, test boundary, API version/kind, or field mapping is wrong, replace the final marker with `TO BE VERIFIED`, document the proposed design change here, and quit immediately.

NOW EXECUTE

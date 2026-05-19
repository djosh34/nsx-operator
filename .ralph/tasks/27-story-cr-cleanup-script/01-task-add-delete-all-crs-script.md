## Task: Add Delete All CRs Script <status>completed</status> <passes>true</passes>

<plan>
.ralph/tasks/27-story-cr-cleanup-script/01-task-add-delete-all-crs-script_plans/01-delete-all-crs-script-plan.md
NOW EXECUTE
</plan>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add a helper script that removes all NSX group custom resources by clearing finalizers and deleting the resources across all namespaces. The script must live in the repository scripts directory after the repository layout task is completed or in the current script location if that layout task has not landed yet. If GNU `parallel` is available, the finalizer patching step must use it with a maximum of 20 concurrent processes; if GNU `parallel` is not available, the script must fall back to a simple serial loop.

The requested script behavior is:

```bash
#!/bin/bash

set -euo pipefail

RESOURCE=NSXGroups

kubectl get "$RESOURCE" -A \
  -o jsonpath='{range .items[*]}{.metadata.namespace}{"\t"}{.metadata.name}{"\n"}{end}' | while IFS=$'\t' read -r name; do
  echo "Patching $name"
  kubectl patch "$RESOURCE" "$name" \
    --type=merge \
    -p '{"metadata":{"finalizers":[]}}'
done

kubectl delete "$RESOURCE" -A --all
```

Implement the intent of the script and correct any obvious argument handling issue required for namespaced resources. Keep the script simple and focused. It should be safe to run from the repository root, use `kubectl`, clear finalizers, and then delete all resources. If GNU `parallel` is installed, patch finalizers with `parallel -j 20` or equivalent max-20 concurrency. If GNU `parallel` is missing, use a serial loop without failing solely because `parallel` is absent. If the resource kind or plural used by the project differs from `NSXGroups`, use the correct kubectl resource name and document the choice in evidence.

In scope: add the executable script; ensure namespace and name are handled correctly for `kubectl patch`; use GNU `parallel` when available with at most 20 processes; provide a serial fallback when GNU `parallel` is unavailable; add shellcheck or script-level tests if available; verify against a test cluster/envtest/kind cluster or mockable kubectl fixture.

Out of scope: building a full cleanup CLI; deleting unrelated CRD instances; changing finalizer behavior in the operator.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] An executable delete-all-CRs helper script exists under the repo's scripts directory.
- [x] The script uses `set -euo pipefail`.
- [x] The script lists all target custom resources across namespaces.
- [x] The script patches each target resource to clear finalizers before deletion.
- [x] When GNU `parallel` is available, the patching step uses it with a maximum of 20 concurrent processes.
- [x] When GNU `parallel` is unavailable, the script falls back to serial patching and still works.
- [x] The script correctly passes namespace and resource name to `kubectl patch` for namespaced resources.
- [x] The script deletes all target resources across namespaces after clearing finalizers.
- [x] Script verification proves the expected `kubectl` calls are made or proves the behavior against a real test cluster.
</acceptance_criteria>

<verification_evidence>
Resource choice:
- Used `nsxgroups.nsx.ing.com` because `crds/nsx.ing.com_nsxgroups.yaml` defines the namespaced NSXGroup CRD as `nsxgroups.nsx.ing.com` with plural `nsxgroups` and kind `NSXGroup`.

Manual/process-boundary script verification:
- `go test ./scripts -run TestDeleteAllCRs -count=1` passed.
- The serial fallback test ran `scripts/delete-all-crs.sh` with a temporary `PATH` containing fake `bash` and fake `kubectl`, and no `parallel`.
- The fake `kubectl` validated and recorded:
  - `kubectl get nsxgroups.nsx.ing.com -A -o jsonpath=...`
  - `kubectl patch nsxgroups.nsx.ing.com group-a --namespace ns-a --type=merge -p '{"metadata":{"finalizers":[]}}'`
  - `kubectl patch nsxgroups.nsx.ing.com group-b --namespace ns-b --type=merge -p '{"metadata":{"finalizers":[]}}'`
  - `kubectl delete nsxgroups.nsx.ing.com -A --all`
- The GNU parallel test ran the same script with fake `parallel` in `PATH`; fake `parallel` required `parallel -j 20 --colsep <tab>`, replayed the command template once per listed namespace/name row, and the same fake `kubectl` validated the resulting patch/delete calls.

Required checks:
- `make test` passed.
- `make test-coverage` passed with `coverage 83.7% meets 80.0% threshold`.
- `make check` passed, including gofumpt, go vet, golangci-lint (`0 issues`), normal tests, race tests, contract tests, e2e tests, large chaos tests, and coverage.

Final boundary check:
- Kept the implementation as one focused script plus process-boundary script tests.
- Did not add a generic cleanup CLI, Go Kubernetes DTO layer, production flags, or test-only production hooks.
- Extracted only `list_resources` and `patch_resource` shell helpers to remove duplicated command construction while keeping behavior local to `scripts/delete-all-crs.sh`.
</verification_evidence>

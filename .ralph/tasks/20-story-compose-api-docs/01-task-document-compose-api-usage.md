## Task: Document Docker Compose API Usage <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add Markdown documentation that explains how a developer can use Docker Compose to spin up this repository's local API/operator stack, validate that the API is reachable, inspect logs, and cleanly shut the stack down.

The higher order goal is to make the local API environment discoverable and repeatable for a developer who has only this repository and the sibling `../nsx-t-mockapi` checkout. The documentation should turn the Compose work into an executable workflow, not just describe that a stack exists.

In scope: create or update a clear README-style Markdown entry point, such as the repository root `README.md` if appropriate or a focused docs README under `docs/`; document prerequisites including Docker, Docker Compose, and the sibling `../nsx-t-mockapi` service requirement; show the exact commands to build the stack, start it, wait for readiness, call or inspect the local Kubernetes/API endpoint, call or inspect the NSX-T mock API endpoint, view structured zap JSONL logs from the operator, and stop/clean up the stack; include troubleshooting notes for common failures such as missing sibling mock API checkout, port conflicts, unhealthy services, and generated kubeconfig/config artifacts.

The README/docs must reference the actual Compose file names, service names, ports, generated artifacts, and commands present in this repo after the Compose stack exists. If the docs depend on the scratch Compose stack from `.ralph/tasks/19-story-scratch-compose-stack/01-task-create-scratch-docker-compose-stack.md`, verify the implementation first and document the real behavior rather than guessing. The docs must not instruct users to rely on a host Kubernetes context for the core local flow unless that is explicitly how the implemented Compose stack works and is proven by verification evidence.

Out of scope: implementing the Docker Compose stack itself, changing production reconciliation behavior, publishing images, deploying to a real NSX-T manager, or replacing existing Go tests/envtest workflows.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the documentation works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] A Markdown README or docs entry point exists and is easy to find from the repository root.
- [x] The docs list prerequisites for Docker Compose usage, including the sibling `../nsx-t-mockapi` checkout when required.
- [x] The docs show exact commands to build, start, inspect, and stop the Docker Compose API stack.
- [x] The docs show at least one concrete command that proves the local Kubernetes/API endpoint is reachable.
- [x] The docs show at least one concrete command that proves the NSX-T mock API endpoint is reachable.
- [x] The docs explain how to inspect operator logs and mention that logs should be structured zap JSONL on stderr.
- [x] The docs include cleanup instructions for stopping the stack and removing generated artifacts if applicable.
- [x] The docs include troubleshooting notes for missing mock API checkout, port conflicts, unhealthy services, and stale generated config.
- [x] Verification evidence includes running the documented build/start/inspection/cleanup commands, or a recorded explanation for any command that cannot be run in the current environment.
</acceptance_criteria>

<verification_evidence>
Performed on 2026-05-19 from repository root.

- `docker compose down --volumes --remove-orphans`: exited 0 from clean start.
- `rm -rf hack/compose/generated`: exited 0.
- `./hack/compose/generate-kube-assets.sh`: exited 0 and printed `wrote generated Kubernetes assets under /home/joshazimullah.linux/work_mounts/vmware/nsx/nsx-operator/hack/compose/generated`.
- `docker compose config`: exited 0, rendered 143 lines, and resolved `name: nsx-operator-scratch`.
- `docker compose build`: exited 0 and built `nsx-t-mockapi:scratch` and `nsx-operator:scratch`.
- `docker compose up -d`: exited 0 after starting `kine`, `kube-apiserver`, `nsx-t-mockapi`, successful one-shot `crd-init`, and `operator`.
- `docker compose ps`: showed `kine`, `kube-apiserver`, `nsx-t-mockapi`, and `operator` running; `kube-apiserver` was published on `0.0.0.0:16443->6443/tcp` and `nsx-t-mockapi` on `0.0.0.0:18080->8080/tcp`.
- `docker compose ps -a crd-init`: showed `nsx-operator-scratch-crd-init-1` as `Exited (0)`.
- `kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get --raw=/readyz`: exited 0 with output `ok`.
- `kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get nsxnetworkclouds.nsx.ing.com`: exited 0 and listed `mockapi` with `FQDN nsx-t-mockapi:8080`, `REACHABLE True`, and `SWEPT True`.
- `kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get nsxgroups.nsx.ing.com`: exited 0 and listed `compose-managed-web` with `FQDN nsx-t-mockapi:8080`, `GROUPID compose-managed-web`, and `MODE Manage`.
- `docker compose run --rm --entrypoint curl crd-init -fsS -u nsx_admin:nsx_password http://nsx-t-mockapi:8080/policy/api/v1/infra/domains/default/groups`: exited 0 and returned JSON with `result_count:1` and group `compose-managed-web`.
- `curl -fsS -u nsx_admin:nsx_password http://127.0.0.1:18080/policy/api/v1/infra/domains/default/groups`: exited 0 and returned successful JSON with `result_count:0`, proving the published host endpoint was reachable.
- `docker compose logs --no-color operator`: exited 0 and showed zap JSON entries including `loaded startup config`, `constructed nsx manager client`, `completed default manager sweep`, and `completed global sweep`.
- `docker compose down --volumes --remove-orphans && rm -rf hack/compose/generated`: exited 0, removed containers/network/volume, and left `hack/compose/generated` absent.
- Final post-edit `make check`: exited 0; fmt, vet, lint, normal tests, race tests, contract tests, e2e subset, largechaos subset, and coverage passed; coverage reported `82.7% meets 80.0% threshold`.
- Final post-edit `make test`: exited 0 for all packages.
- Final post-edit `make test-coverage`: exited 0 and reported `coverage 82.7% meets 80.0% threshold`.
- Final improve-code-boundaries review: workflow knowledge is centralized in `docs/compose-stack.md`, root `README.md` is a shallow entry point, generated credential contents are not documented, and the hardcoded host kubeconfig port boundary is documented in troubleshooting.
</verification_evidence>

<execution_plan>
.ralph/tasks/20-story-compose-api-docs/01-task-document-compose-api-usage_plans/01-compose-api-docs-plan.md
NOW EXECUTE
</execution_plan>

## Task: Document Docker Compose API Usage <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add Markdown documentation that explains how a developer can use Docker Compose to spin up this repository's local API/operator stack, validate that the API is reachable, inspect logs, and cleanly shut the stack down.

The higher order goal is to make the local API environment discoverable and repeatable for a developer who has only this repository and the sibling `../nsx-t-mockapi` checkout. The documentation should turn the Compose work into an executable workflow, not just describe that a stack exists.

In scope: create or update a clear README-style Markdown entry point, such as the repository root `README.md` if appropriate or a focused docs README under `docs/`; document prerequisites including Docker, Docker Compose, and the sibling `../nsx-t-mockapi` service requirement; show the exact commands to build the stack, start it, wait for readiness, call or inspect the local Kubernetes/API endpoint, call or inspect the NSX-T mock API endpoint, view structured zap JSONL logs from the operator, and stop/clean up the stack; include troubleshooting notes for common failures such as missing sibling mock API checkout, port conflicts, unhealthy services, and generated kubeconfig/config artifacts.

The README/docs must reference the actual Compose file names, service names, ports, generated artifacts, and commands present in this repo after the Compose stack exists. If the docs depend on the scratch Compose stack from `.ralph/tasks/19-story-scratch-compose-stack/01-task-create-scratch-docker-compose-stack.md`, verify the implementation first and document the real behavior rather than guessing. The docs must not instruct users to rely on a host Kubernetes context for the core local flow unless that is explicitly how the implemented Compose stack works and is proven by verification evidence.

Out of scope: implementing the Docker Compose stack itself, changing production reconciliation behavior, publishing images, deploying to a real NSX-T manager, or replacing existing Go tests/envtest workflows.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the documentation works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] A Markdown README or docs entry point exists and is easy to find from the repository root.
- [ ] The docs list prerequisites for Docker Compose usage, including the sibling `../nsx-t-mockapi` checkout when required.
- [ ] The docs show exact commands to build, start, inspect, and stop the Docker Compose API stack.
- [ ] The docs show at least one concrete command that proves the local Kubernetes/API endpoint is reachable.
- [ ] The docs show at least one concrete command that proves the NSX-T mock API endpoint is reachable.
- [ ] The docs explain how to inspect operator logs and mention that logs should be structured zap JSONL on stderr.
- [ ] The docs include cleanup instructions for stopping the stack and removing generated artifacts if applicable.
- [ ] The docs include troubleshooting notes for missing mock API checkout, port conflicts, unhealthy services, and stale generated config.
- [ ] Verification evidence includes running the documented build/start/inspection/cleanup commands, or a recorded explanation for any command that cannot be run in the current environment.
</acceptance_criteria>

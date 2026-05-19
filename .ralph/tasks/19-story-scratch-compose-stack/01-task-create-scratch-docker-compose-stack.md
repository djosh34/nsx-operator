## Task: Create Scratch Docker Compose Stack <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Create a scratch-based Dockerfile for the `nsx-operator` binary and a Docker Compose stack that uses that Dockerfile while also starting kine in in-memory mode, a Kubernetes API server backed by kine, and the sibling `../nsx-t-mockapi` service.

The higher order goal is to make local end-to-end operator execution reproducible with real process boundaries: the operator should run from a minimal container image, talk to a live kube-apiserver, and use the NSX-T mock API as its NSX endpoint.

In scope: add a Dockerfile or clearly named Dockerfile variant that builds the Go operator and produces a final `scratch` runtime image; add a Docker Compose file that builds the operator image from that Dockerfile; include services for kine using in-memory storage, kube-apiserver configured to use kine as storage, the operator, and `../nsx-t-mockapi`; wire service networking, ports, health checks or readiness waits, volumes/config, and environment variables needed for the operator to reach both Kubernetes and NSX-T mockapi; document the exact commands needed to build, start, inspect, and stop the stack.

The Compose stack must not rely on a developer's host Kubernetes context to make the core flow work. It should create or mount only the kubeconfig/config artifacts it needs, and those generated artifacts must be treated as generated output. The operator container must use the repo's zap JSONL logging behavior to stderr and keep errors handled explicitly.

Out of scope: deploying to a real NSX-T manager, publishing images to a registry, replacing the existing Go test/envtest path, or changing production reconciliation behavior except where a small configuration hook is genuinely required to run in the Compose environment.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] A scratch-based operator Dockerfile builds successfully from a clean repo checkout.
- [ ] Docker Compose builds the operator image from that Dockerfile.
- [ ] Docker Compose starts kine in in-memory mode.
- [ ] Docker Compose starts kube-apiserver and kube-apiserver is reachable via `kubectl` or an equivalent concrete API call.
- [ ] Docker Compose starts the sibling `../nsx-t-mockapi` service and the operator can reach it by service name.
- [ ] Operator logs are emitted as structured zap JSONL to stderr inside the Compose stack.
- [ ] Verification evidence includes successful `docker compose build`, `docker compose up`, service health/status output, kube-apiserver API evidence, nsx-t-mockapi API evidence, and operator log evidence.
- [ ] `docker compose down` or the documented cleanup command removes the stack cleanly.
</acceptance_criteria>

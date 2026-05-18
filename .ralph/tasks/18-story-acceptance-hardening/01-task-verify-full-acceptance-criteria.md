## Task: Verify Full NSX Operator Acceptance Criteria <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Perform final acceptance hardening for the whole NSX operator against the design in `.ralph/designs/PLAN.md`. This task is the final integration gate after implementation stories are complete.

In scope: verify CRDs install on Kubernetes >= 1.32; API version is `nsx.ing.com/v1alpha`; CRDs include schemas, status subresources, and selectableFields; `domainId` is absent from CRD specs; `NSXGroup` spec/status and `NSXNetworkCloud` status match the design; each `cidrs` item maps one-to-one and unchanged to one NSX `IPAddressExpression.ip_addresses` item; Observe import/update/delete behavior works; Manage patch/delete behavior works through documented PATCH endpoints; Manage delete waits for confirmed absence; controller-runtime manager and Reconcile are registered; `Start` ticker is non-overlapping and skips elapsed ticks; one goroutine per cloud per sweep; gather/process/apply pipeline exists and process stage has zero client calls; kube client exposes field filters; HTTP limiter is generic per host+port and blocks; NSX client uses global Basic Auth, stream-decodes list results, paginates, and does not auto refetch/reapply; zap logs JSONL to stderr; `go test -race ./...` passes; 80%+ coverage; large e2e and chaos scenarios pass.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] A final acceptance evidence bundle or linked artifact records commands, versions, logs, and pass/fail outputs.
- [ ] Any unmet acceptance item is turned into a follow-up Ralph task before this task is marked passing.
</acceptance_criteria>

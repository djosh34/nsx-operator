## Task: Verify Full NSX Operator Acceptance Criteria <status>done</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Perform final acceptance hardening for the whole NSX operator against the design in `.ralph/designs/PLAN.md`. This task is the final integration gate after implementation stories are complete.

In scope: verify CRDs install on Kubernetes >= 1.32; API version is `nsx.ing.com/v1alpha`; CRDs include schemas, status subresources, and selectableFields; `domainId` is absent from CRD specs; `NSXGroup` spec/status and `NSXNetworkCloud` status match the design; each `cidrs` item maps one-to-one and unchanged to one NSX `IPAddressExpression.ip_addresses` item; Observe import/update/delete behavior works; Manage patch/delete behavior works through documented PATCH endpoints; Manage delete waits for confirmed absence; controller-runtime manager and Reconcile are registered; `Start` ticker is non-overlapping and skips elapsed ticks; one goroutine per cloud per sweep; gather/process/apply pipeline exists and process stage has zero client calls; kube client exposes field filters; HTTP limiter is generic per host+port and blocks; NSX client uses global Basic Auth, stream-decodes list results, paginates, and does not auto refetch/reapply; zap logs JSONL to stderr; `go test -race ./...` passes; 80%+ coverage; large e2e and chaos scenarios pass.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] A final acceptance evidence bundle or linked artifact records commands, versions, logs, and pass/fail outputs.
- [x] Any unmet acceptance item is turned into a follow-up Ralph task before this task is marked passing.
</acceptance_criteria>

<verification_evidence>
Final evidence bundle: `.ralph/tasks/18-story-acceptance-hardening/evidence/20260519T032638Z/`

Primary matrix: `.ralph/tasks/18-story-acceptance-hardening/evidence/20260519T032638Z/acceptance-matrix.md`

Required checks completed:

- `make test` passed; see `make-test.log`.
- Explicit `go test -race ./...` passed with envtest assets; see `race.log`.
- `make test-coverage` passed with total coverage `82.7%`, meeting the `80.0%` threshold; see `make-test-coverage.log`.
- `make check` passed; see `make-check.log`.

Additional acceptance evidence:

- CRD install/schema/status/selectable-field verification passed against envtest Kubernetes `1.32.0`; see `crd-verification.log`.
- NSX mockapi contract and Observe/Manage lifecycle verification passed against `../nsx-t-mockapi`; see `mockapi-contract.log`.
- Runtime/controller/pipeline verification passed; one evidence collection command initially missed `KUBEBUILDER_ASSETS`, and the corrected rerun is recorded in `runtime-verification-envtest-correction.log`.
- Large/chaos scenarios passed; verbose evidence includes 2000 remote groups, 10000 real CRs, 5000 managed writes, 5000 observe deletes, and limiter/unavailable chaos evidence; see `large-chaos.log` and `large-chaos-verbose.log`.

No production code was changed for this verification task, and no unmet acceptance item required a follow-up Ralph task.
</verification_evidence>

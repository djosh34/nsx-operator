# Final Acceptance Evidence Plan

Plan path: `.ralph/tasks/18-story-acceptance-hardening/01-task-verify-full-acceptance-criteria_plans/01-plan-final-acceptance-evidence.md`

Task path: `.ralph/tasks/18-story-acceptance-hardening/01-task-verify-full-acceptance-criteria.md`

## Goal

Produce a concrete acceptance evidence bundle for the NSX operator against `.ralph/designs/PLAN.md`, then mark the task passing only when every required command and manual verification item has evidence. This is a verification task, not a speculative implementation task.

## TDD And Boundary Rules

- Use the `$tdd` skill mindset for any code changes: one public behavior test first, confirm it fails for the actual gap, implement the smallest fix, run it green, then refactor.
- Do not add brittle tests that assert strings in manifests, Dockerfiles, workflows, or generated text. For CRDs and workflows, verify by installing/serving/parsing them through Kubernetes/envtest or by executing the workflow command.
- Because the main task is acceptance verification, the first execution path is command-driven evidence collection. TDD becomes mandatory only if verification reveals a code defect or missing behavior.
- Use the `$improve-code-boundaries` skill before final completion: inspect whether any fix introduced muddy boundaries, duplicate DTOs, stringly rendering, bootstrap leakage, or process-stage client calls. If so, refactor boldly behind the existing public interfaces and rerun checks.

## Evidence Bundle Layout

Create one timestamped evidence directory under:

`.ralph/tasks/18-story-acceptance-hardening/evidence/<UTC_TIMESTAMP>/`

Record at least:

- `environment.txt`: date, hostname, git commit, git status, Go version, Docker version if available, Kubernetes/envtest version, mockapi location.
- `make-check.log`: full `make check` output.
- `make-test.log`: full `make test` output.
- `make-test-coverage.log`: full `make test-coverage` output and total percentage.
- `race.log`: explicit `go test -race ./...` output, even though `make check` also runs race.
- `crd-verification.log`: Kubernetes/envtest-backed verification for CRD install, API version, schemas, status subresources, selectable fields, and absence of `domainId` in specs.
- `mockapi-contract.log`: NSX mockapi-backed contract/e2e evidence, including Observe and Manage behavior.
- `large-chaos.log`: large and chaos scenario output.
- `acceptance-matrix.md`: each design acceptance item mapped to command output, log line, test name, or follow-up task.

## Execution Plan

- [x] Create the evidence directory and write `environment.txt` before changing code.
- [x] Run `git status --short` and preserve dirty-worktree evidence so unrelated user/Ralph changes are not lost.
- [x] Inspect `../nsx-t-mockapi` enough to document how the mock API is being used by existing contract tests.
- [x] Run the fastest public-interface verification first:
  - [x] `make test`
  - [x] Save full output to `make-test.log`.
  - [x] If this fails from a real behavior gap, enter a vertical TDD cycle for only that behavior. No behavior gap was found.
- [x] Verify CRD acceptance through envtest-backed tests and/or a small executable verification command:
  - [x] Kubernetes >= 1.32 envtest assets are used.
  - [x] API version is `nsx.ing.com/v1alpha`.
  - [x] CRDs install successfully.
  - [x] CRDs include schemas.
  - [x] CRDs include status subresources.
  - [x] CRDs include selectable fields.
  - [x] `domainId` is absent from CRD specs.
  - [x] `NSXGroup` spec/status and `NSXNetworkCloud` status match the design.
- [x] Verify NSX client and reconciliation behavior through public tests and mockapi:
  - [x] CIDR items map one-to-one and unchanged to `IPAddressExpression.ip_addresses`.
  - [x] Observe import, update, and delete behavior works.
  - [x] Manage patch and delete behavior uses documented PATCH endpoints.
  - [x] Manage delete waits for confirmed absence.
  - [x] NSX client uses global Basic Auth.
  - [x] NSX client stream-decodes list results.
  - [x] NSX client paginates.
  - [x] NSX client does not auto-refetch or auto-reapply.
- [x] Verify runtime and controller behavior:
  - [x] controller-runtime manager is built and controllers are registered.
  - [x] `Reconcile` is registered for both CRDs.
  - [x] `Start` ticker is non-overlapping and skips elapsed ticks.
  - [x] one goroutine per cloud per sweep.
  - [x] gather/process/apply pipeline exists.
  - [x] process stage has zero Kubernetes or NSX client calls.
  - [x] Kubernetes client exposes field filters.
- [x] Verify infrastructure behavior:
  - [x] HTTP limiter is generic per host+port and blocks.
  - [x] zap logs JSONL to stderr.
- [x] Run required final commands and save full output:
  - [x] `make check`
  - [x] `make test`
  - [x] `make test-coverage`
  - [x] explicit `go test -race ./...` with the same envtest assets as the Makefile.
- [x] Confirm coverage:
  - [x] New code touched during this task has 80%+ meaningful behavioral coverage. No production code was added; existing touched runtime packages remain at or above 80%.
  - [x] Repository `make test-coverage` reports at least 80%; total was `82.7%`.
- [x] Build `acceptance-matrix.md` linking every acceptance/design item to concrete evidence.
- [x] If any item cannot be verified or fails:
  - [x] No unmet acceptance item remained after corrected evidence collection, so no follow-up Ralph task was required.
  - [x] The task was marked passing only after required checks and evidence completed.
- [x] Final boundary pass using `$improve-code-boundaries`:
  - [x] Look for duplicate type layers, DTO conversion churn, stringly manifest checks, bootstrap/request spaghetti, or process-stage client calls.
  - [x] Refactor any muddy code introduced by this task. No production code was changed and no muddy boundary was introduced.
  - [x] Rerun affected tests plus the required final commands after refactor. No refactor was needed; required final commands passed.
- [x] Update the task file:
  - [x] Record or link the final evidence bundle.
  - [x] Check off acceptance criteria.
  - [x] Set `<passes>true</passes>` only after all required checks pass and follow-up handling is complete.
- [x] Run `/bin/bash .ralph/task_switch.sh`.
- [x] Stage all files, including `.ralph` changes and evidence.
- [x] Commit with subject `task finished 01-task-verify-full-acceptance-criteria: final acceptance evidence`.
- [x] Include command evidence, coverage result, and any challenges/follow-ups in the commit body.
- [x] Push.
- [x] Quit immediately.

## Interface Design

No public API or type changes are planned for the verification path. If implementation gaps appear, preserve the current public interfaces unless the failing acceptance item proves the interface is wrong. If an interface/type/enumeration change is required, switch this plan back to `TO BE VERIFIED`, document the proposed design change here, and quit immediately.

## Expected Public Behaviors To Test If A Fix Is Needed

- CRDs can be installed and queried through Kubernetes APIs, not string-matched from YAML.
- NSX mockapi behavior is verified through the typed NSX client and state operator public functions, not private helpers.
- Reconciliation semantics are verified by observable Kubernetes CR state and mockapi HTTP calls.
- Runtime behavior is verified through manager/startup/state-operator public construction and observable scheduling behavior.

NOW EXECUTE

## Task: Implement NSXStateOperator Runtime <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `NSXStateOperator` with `Start(ctx)` and `Reconcile(ctx, req)` integrated into controller-runtime manager. `Start` must run periodic global sweeps and `Reconcile` must exist for CR event handling.

In scope: controller-runtime manager registration for NSXNetworkCloud and NSXGroup controllers; fixed tick interval from validated config; non-overlapping ticker algorithm that skips elapsed ticks and waits until the next future tick after long sweeps; per-tick list of all NSXNetworkCloud CRs; one goroutine per cloud; wait for all cloud goroutines; structured sweep logging with `sweepID`; no worker pool, request queue, or manager queue. Out of scope: hidden retry loops outside the documented sweep/reconcile model.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] Tests prove global sweeps never overlap and elapsed ticks are skipped.
- [x] Logs or tests prove one goroutine per cloud and healthy clouds continue while another cloud fails.
</acceptance_criteria>

<plan>
.ralph/tasks/07-story-state-operator-runtime/01-task-implement-state-operator-runtime_plans/2026-05-19-state-operator-runtime-plan.md
</plan>

<verification_evidence>
- `go test ./internal/stateoperator -count=1` passed. Tests cover immediate global sweep, one goroutine per cloud through per-cloud fanout, failed-cloud isolation, non-overlapping sweeps, elapsed tick skipping, structured `sweepID` and `networkCloudFQDN` logs, and `Reconcile` returning `reconcile.Result{}` without explicit requeue.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/startup -count=1` passed. Envtest-backed manager test constructs the controller-runtime manager, registers both `NSXNetworkCloud` and `NSXGroup` controllers, starts the runnable, creates real CRs through the API server, observes reconcile logs for both kinds, and observes the periodic sweeper listing the real cloud.
- `make check` passed on the final code: `gofumpt -w .`, `golangci-lint run ./...` with `0 issues`, `go test ./...`, and `go test -cover ./...`.
- `make test` passed separately.
- `make test-coverage` passed separately with all packages at or above 80%: `api/v1alpha 100.0%`, `cmd/nsx-operator 80.8%`, `internal/buildinfo 100.0%`, `internal/config 82.9%`, `internal/httpratelimit 87.8%`, `internal/kubeapi 80.6%`, `internal/logging 96.2%`, `internal/nsxclient 80.3%`, `internal/startup 82.3%`, `internal/stateoperator 86.8%`.
- Final boundary review using `$improve-code-boundaries`: sweep scheduling, sweep IDs, per-cloud fanout, and structured logging live in `internal/stateoperator`; controller-runtime manager/bootstrap wiring lives in `internal/startup`; no worker pool, request queue, hidden retry loop, duplicate cloud/group DTO, or extra config validation outside `internal/config` was introduced.
</verification_evidence>

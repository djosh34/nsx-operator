## Task: Implement NSXStateOperator Runtime <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `NSXStateOperator` with `Start(ctx)` and `Reconcile(ctx, req)` integrated into controller-runtime manager. `Start` must run periodic global sweeps and `Reconcile` must exist for CR event handling.

In scope: controller-runtime manager registration for NSXNetworkCloud and NSXGroup controllers; fixed tick interval from validated config; non-overlapping ticker algorithm that skips elapsed ticks and waits until the next future tick after long sweeps; per-tick list of all NSXNetworkCloud CRs; one goroutine per cloud; wait for all cloud goroutines; structured sweep logging with `sweepID`; no worker pool, request queue, or manager queue. Out of scope: hidden retry loops outside the documented sweep/reconcile model.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Tests prove global sweeps never overlap and elapsed ticks are skipped.
- [ ] Logs or tests prove one goroutine per cloud and healthy clouds continue while another cloud fails.
</acceptance_criteria>

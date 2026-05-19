## Plan: NSXStateOperator Runtime

Task: `.ralph/tasks/07-story-state-operator-runtime/01-task-implement-state-operator-runtime.md`

### Current State

- `api/v1alpha` already provides `NSXNetworkCloud`, `NSXNetworkCloudList`, `NSXGroup`, `NSXGroupList`, condition constants, and `AddToScheme`.
- `internal/config` already validates `operator.tickInterval` into `config.Config.Operator.TickInterval`; do not re-validate the interval in the runtime package.
- `internal/logging` already provides structured zap fields for `component`, `networkCloudFQDN`, `sweepID`, and `reconcileKey`.
- `internal/kubeapi` has a typed REST client, but this task specifically requires controller-runtime manager registration. Use controller-runtime client/cache for the operator runtime.
- `cmd/nsx-operator/main.go` currently loads config and constructs optional startup clients, but does not create or start a controller-runtime manager.
- Stories 08 and 09 own the detailed gather/process/apply pipeline and detailed event reconcile behavior. This task owns the runtime shell: manager wiring, periodic global sweeps, per-cloud fanout, non-overlapping timing, and a no-explicit-requeue `Reconcile` entrypoint.

### Public Interface

Create package `internal/stateoperator`.

Primary runtime type:

```go
type NSXStateOperator struct {
	// fields private
}
```

Constructor:

```go
type Options struct {
	Client       client.Client
	TickInterval time.Duration
	Logger       *zap.Logger
	SweepCloud   CloudSweepFunc
	Clock        Clock
	IDGenerator  SweepIDGenerator
}

type CloudSweepFunc func(ctx context.Context, cloud nsxv1alpha.NSXNetworkCloud, sweep SweepContext) error

type SweepContext struct {
	ID string
}

func New(options Options) (*NSXStateOperator, error)
```

Controller-runtime interfaces:

```go
func (o *NSXStateOperator) Start(ctx context.Context) error
func (o *NSXStateOperator) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error)
```

Manager registration:

```go
type ManagerOptions struct {
	Config       config.Config
	RestConfig   *rest.Config
	Logger       *zap.Logger
	SweepCloud   stateoperator.CloudSweepFunc
	Clock        stateoperator.Clock
	IDGenerator  stateoperator.SweepIDGenerator
}

func NewManager(options ManagerOptions) (manager.Manager, error)
```

`NewManager` should live in `internal/startup` or a small `internal/runtime` package only if startup gets too crowded. Prefer keeping the construction boundary near `startup.Run`, because the manager is process bootstrap, not reconciliation logic.

Clock abstraction for deterministic timing tests:

```go
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
}

type Timer interface {
	C() <-chan time.Time
	Stop() bool
}
```

Use a one-shot timer loop instead of `time.Ticker` so long sweeps naturally schedule the next future tick after completion:

1. Run a sweep immediately when `Start` begins.
2. After the sweep finishes, compute the next tick strictly after the current time using the original schedule anchor and `TickInterval`.
3. Sleep until that future tick.
4. Repeat until context cancellation.

This design satisfies non-overlap and elapsed tick skipping with a small interface and without a worker pool, request queue, or hidden retry loop.

### Behavior Boundaries

- `Start(ctx)` is the global periodic sweeper and must block until `ctx` is cancelled or an unrecoverable list error occurs.
- Each sweep lists all `NSXNetworkCloud` CRs through the controller-runtime client.
- For each cloud in the list, start exactly one goroutine.
- Wait for all cloud goroutines before scheduling the next sweep.
- If one cloud sweep returns an error, log it and continue waiting for other clouds. Do not cancel healthy cloud goroutines because another cloud failed.
- `Reconcile(ctx, req)` must exist and log the event key, but for this story it should not perform NSX mutation, status mutation, or explicit requeue. Return `reconcile.Result{}` with a nil error unless context cancellation or future story logic requires otherwise.
- Controller registrations must create two controllers: one watching `NSXNetworkCloud` and one watching `NSXGroup`, both using the same `NSXStateOperator` reconciler.
- Manager setup must register `api/v1alpha` scheme before controller setup.
- Keep the fixed tick interval exclusively from validated `config.Config.Operator.TickInterval`.

### Boundary Plan Using `$improve-code-boundaries`

- Deep module: keep scheduling, sweep IDs, cloud fanout, and logging inside `NSXStateOperator`; expose only `Start` and `Reconcile` plus a constructor.
- Avoid wrong-placeism: manager/bootstrap construction belongs near startup; sweep orchestration belongs in `internal/stateoperator`; future NSX gather/process/apply logic stays out of this task.
- Avoid duplicate API shapes: pass `nsxv1alpha.NSXNetworkCloud` directly into `CloudSweepFunc`; do not create a parallel cloud DTO.
- Avoid overengineering: no worker pools, request queues, background retry queues, persistent scheduler state machine, or second ticker abstraction.
- Avoid validation outside config: `New` may reject nil dependencies and non-positive intervals defensively, but it must not parse or reinterpret raw config.
- Keep fields and helper types as private as possible. Export only the small seams needed by startup and behavior tests.
- Final boundary review after green: scan for duplicate cloud/group representations, test-only production exports, manager construction leaking reconciliation internals, and any hidden retry loop outside the documented sweep/reconcile model.

### TDD Execution Plan Using `$tdd`

Follow vertical red-green cycles. Do not write all tests first.

1. [x] Tracer bullet: `Start` performs one immediate sweep that lists all clouds and invokes one cloud sweep per cloud.
   - [x] RED: using a controller-runtime fake client loaded with two `NSXNetworkCloud` objects, call `Start` with a cancellable context and a `SweepCloud` func that records both clouds and cancels after the second call.
   - [x] GREEN: add `internal/stateoperator` with `NSXStateOperator`, constructor, immediate sweep, list call, per-cloud goroutines, and wait group.

2. [x] Healthy clouds continue while another cloud fails.
   - [x] RED: `SweepCloud` returns an error for one cloud while another blocks until released; assert both cloud sweeps were started and `Start` does not cancel the healthy cloud early.
   - [x] GREEN: collect per-cloud errors for logging only and wait for all goroutines before the sweep completes.

3. [x] Sweeps never overlap.
   - [x] RED: use a manual clock and blocking `SweepCloud` to prove a second sweep cannot begin while the first sweep is still running even when virtual time advances past multiple intervals.
   - [x] GREEN: keep the scheduling loop single-threaded around `runSweep`; do not use a ticker callback that can fire concurrently.

4. [x] Elapsed ticks are skipped after long sweeps.
   - [x] RED: with a 10s interval, anchor at 00:00:00, make the first sweep finish at 00:00:35, and assert the next timer waits until 00:00:40 rather than immediately running catch-up sweeps for 00:00:10, 00:00:20, or 00:00:30.
   - [x] GREEN: calculate the next future tick from the anchor and interval after each sweep completes.

5. [x] Structured sweep logs include `sweepID` and per-cloud fields.
   - [x] RED: use zap observer logs at debug/info level; assert sweep start/completion logs contain `sweepID`, and cloud start/failure/completion logs contain `sweepID` plus `networkCloudFQDN`.
   - [x] GREEN: use existing `internal/logging` helpers and zap fields only; no unstructured string logs.

6. [x] `Reconcile` exists and does not explicitly requeue.
   - [x] RED: call `Reconcile` with a sample request and assert `reconcile.Result{}` and nil error, plus structured log with `reconcileKey`.
   - [x] GREEN: implement minimal reconcile entrypoint.

7. [x] Manager registration wires both controllers and the runnable.
   - [x] RED: start envtest with CRDs, create a manager through the new constructor, start it in a goroutine, create one `NSXNetworkCloud` and one `NSXGroup`, and assert logs or injected reconciler evidence shows both controller registrations accept events. Also assert the periodic `Start` runnable can list the cloud.
   - [x] GREEN: create controller-runtime manager with scheme registration, add `NSXStateOperator` as a manager runnable, and register builders for `NSXNetworkCloud` and `NSXGroup`.

8. [x] Startup integrates manager construction and start.
   - [x] RED: add a startup test using injected `ManagerFactory`/`ManagerStarter` or equivalent small interface proving valid config creates and starts the manager with the validated tick interval.
   - [x] GREEN: extend `startup.Run` without spreading config validation or controller details through `cmd`.

9. [x] Boundary/refactor pass after green.
   - [x] Remove speculative exported types and helpers.
   - [x] Keep production fields private where tests can use public behavior.
   - [x] Run focused package tests after each refactor.

### Manual Verification Plan

Record concrete evidence in the task before setting `<passes>true</passes>`:

- Focused `go test ./internal/stateoperator -count=1 -v` output showing non-overlap, elapsed tick skipping, per-cloud fanout, failure isolation, `sweepID` logging, and no explicit reconcile requeue.
- Envtest-backed manager/startup test output showing both `NSXNetworkCloud` and `NSXGroup` controllers are registered and the runnable lists real CRs from the API server.
- Full `make check`, `make test`, and `make test-coverage` output.
- Coverage evidence proving new code is at least 80% covered and every package in `make test-coverage` remains 80%+.
- Final `$improve-code-boundaries` review notes confirming no worker pool, no request queue, no duplicate CR DTOs, and no hidden retry loop.

### Completion Steps

- Tick plan checkboxes as each TDD slice is executed.
- If implementation proves this interface or timing design wrong, replace the final marker with `TO BE VERIFIED` and quit immediately.
- Only after all required checks pass, update the task acceptance criteria, record verification evidence, set `<passes>true</passes>`, run `.ralph/task_switch.sh`, commit all files including `.ralph`, push, and quit immediately.

NOW EXECUTE

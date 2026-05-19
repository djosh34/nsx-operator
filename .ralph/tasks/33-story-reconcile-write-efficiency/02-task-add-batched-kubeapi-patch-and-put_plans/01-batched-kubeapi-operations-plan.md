Plan path: `.ralph/tasks/33-story-reconcile-write-efficiency/02-task-add-batched-kubeapi-patch-and-put_plans/01-batched-kubeapi-operations-plan.md`

# Add Generic Batched Kube API Operations

## Startup Context

- Required skills read for this plan: `$tdd` and `$improve-code-boundaries`.
- Current task file: `.ralph/tasks/33-story-reconcile-write-efficiency/02-task-add-batched-kubeapi-patch-and-put.md`.
- Existing typed kube-api client lives in `internal/kubeapi/client.go`.
  - `Client` owns `NetworkCloudClient` and `GroupClient`.
  - `typedResource[Object, List]` already owns the shared single-object REST behavior for `Create`, `Update`, server-side `Apply`, status `PUT /status`, `Delete`, `Get`, `List`, and `Watch`.
  - Existing single-operation methods already log with zap and preserve typed resource stamping.
- Existing config lives in `internal/config/config.go`.
  - There is currently no top-level `kubeAPI` section.
  - `HTTPRateLimiterConfig` is specific to NSX manager HTTP clients and must not be reused as kube API batch config because the field names and semantics differ.
- Existing startup wiring constructs the typed client in `internal/startup/manager.go` and should pass the validated kube API batch config there.
- Existing kubeapi tests use envtest in `internal/kubeapi/client_test.go`; pure batch executor tests can be normal unit tests that do not need envtest.

## Public Interface And Type Design

Add exported batch types in `internal/kubeapi`, matching the task's public shape unless implementation proves a compile-time type issue:

```go
type BatchConfig struct {
	NumParallelWorkers   int
	MaxRequestsPerSecond int
	MaxRequestsInFlight  int
}

type BatchKey struct {
	Operation   string
	Resource    string
	Subresource string
	Name        string
}

type BatchItemError struct {
	Key BatchKey
	Err error
}

func (e BatchItemError) Error() string
func (e BatchItemError) Unwrap() error

type BatchError struct {
	Operation string
	Resource  string
	Items     map[BatchKey]error
}

func (e BatchError) Error() string
func (e BatchError) Errors() map[BatchKey]error

type JSONPatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	From  string `json:"from,omitempty"`
	Value any    `json:"value,omitempty"`
}

type BatchOperation[Request any, Result any] struct {
	Operation string
	Resource  string
	Execute   func(context.Context, Request) (Result, error)
}

func ExecuteBatch[Request any, Result any](
	ctx context.Context,
	cfg BatchConfig,
	log *zap.Logger,
	operation BatchOperation[Request, Result],
	requests map[BatchKey]Request,
) (map[BatchKey]Result, map[BatchKey]error, error)
```

Typed request structs are public because the task requires callers to build maps of typed requests:

- `GroupApplyRequest`, `GroupUpdateRequest`, `GroupFinalizerPatchRequest`, `GroupStatusUpdateRequest`, `GroupCreateRequest`, `GroupDeleteRequest`
- `NetworkCloudApplyRequest`, `NetworkCloudUpdateRequest`, `NetworkCloudFinalizerPatchRequest`, `NetworkCloudStatusUpdateRequest`, `NetworkCloudCreateRequest`, `NetworkCloudDeleteRequest`

Each struct should contain exactly the current single-operation arguments plus metadata needed by that call:

- Apply/update/create requests carry `Object` and options.
- Status requests carry `Name`, `Status`, and `StatusUpdateOptions`.
- Delete requests carry `Name` and `metav1.DeleteOptions`.
- Finalizer patch requests carry `Name`, `ResourceVersion`, full desired `Finalizers`, and `metav1.PatchOptions`.

Add public batch methods on `GroupClient` and `NetworkCloudClient` exactly as requested:

- `ApplyBatch`, `UpdateBatch`, `PatchFinalizersBatch`, `UpdateStatusBatch`, `CreateBatch`, `DeleteBatch`.
- Delete batch returns `map[BatchKey]struct{}` for successful deletes.
- Preserve all existing single-operation methods.

Add config types:

```go
type KubeAPIConfig struct {
	NumParallelWorkers   int
	MaxRequestsPerSecond int
	MaxRequestsInFlight  int
}
```

Validated defaults:

- `kubeAPI.numParallelWorkers` defaults to `1` when omitted or zero.
- `kubeAPI.maxRequestsPerSecond` defaults to `100` when omitted or zero.
- `kubeAPI.maxRequestsInFlight` defaults to `100` when omitted or zero.
- Negative values are validation errors.
- The normal repo config file `config/compose/nsx-operator-config.yaml` explicitly sets:

```yaml
kubeAPI:
  numParallelWorkers: 20
  maxRequestsPerSecond: 100
  maxRequestsInFlight: 100
```

## Batch Executor Behavior

- Normalize `BatchConfig` inside `ExecuteBatch` so direct callers also get default worker/rate-limit behavior.
- Validate `BatchOperation.Execute` is non-nil before scheduling work.
- For an empty request map, return empty result/error maps and `nil` aggregate error.
- Sort `BatchKey` values before scheduling to keep logs and tests deterministic even though caller input is a map.
- Use one coordinator loop to schedule keyed requests over an unbuffered work channel, receive keyed results from workers, and assemble result and error maps.
- Start `NumParallelWorkers` workers.
- Apply rate limiting inside workers immediately before executing the operation:
  - Use `golang.org/x/time/rate` for `MaxRequestsPerSecond`.
  - Use a buffered channel as the `MaxRequestsInFlight` semaphore.
  - Log debug fields for waiting/acquired rate permits and in-flight slots.
- Keep processing queued work after item errors.
- Do not retry.
- On per-item error, wrap the cause as `BatchItemError{Key: key, Err: err}` in the returned error map.
- If any item fails, return a `BatchError` whose `Errors()` exposes a copy of the item error map.
- Respect context cancellation:
  - Stop scheduling new items after `ctx.Done()`.
  - Return useful `BatchItemError` entries for unscheduled keys with the context error.
  - Workers waiting on rate/in-flight limits should return context errors for their item.
  - Operations receive the caller's context.
- Use structured zap logging:
  - Info for batch start and completion with operation, resource, item count, worker count, `maxRequestsPerSecond`, and `maxRequestsInFlight`.
  - Debug for item scheduling, worker execution start, success, failure, rate-limit wait/acquire, in-flight wait/acquire/release.

## Kube API Client Behavior

- Add `BatchConfig BatchConfig` to `kubeapi.Options`.
- Store normalized config on each `typedResource` so typed clients do not need duplicate batch plumbing.
- Add small typedResource batch methods where the behavior is generic:
  - `createBatch`
  - `updateBatch`
  - `applyBatch`
  - `updateStatusBatch`
  - `deleteBatch`
  - `patchFinalizersBatch`
- Keep `client.go` from becoming a large mixed-responsibility file:
  - Put executor, errors, and rate-limit machinery in `internal/kubeapi/batch.go`.
  - Put request structs and public resource batch methods in `internal/kubeapi/batch_methods.go`.
  - Keep the existing single-operation client in `client.go` with only the minimal `BatchConfig` field/wiring.
- Batched apply must call the existing `apply` path so server-side apply payload semantics, field manager validation, resourceVersion clearing, and managed fields clearing remain unchanged.
- Batched update must call the existing `update` path and must not perform a fresh `Get`.
- Batched status update must call the existing `updateStatus` path and therefore issue `PUT .../status`.
- Batched create and delete must call existing `create` and `delete` paths.
- Finalizer patch is the one new single-resource operation. Add it to `typedResource` and expose only the batch methods unless execution proves a single-operation public method is needed.
  - Use JSON Patch (`types.JSONPatchType`).
  - Write the full desired finalizers array with an `add` operation at `/metadata/finalizers`; RFC6902 add on an existing object member replaces that member, which gives "full desired array" semantics.
  - If `ResourceVersion` is non-empty, include a leading `test` operation against `/metadata/resourceVersion` to prevent stale finalizer writes.
  - Return the updated typed object and stamp its GVK.

## Boundary Cleanup From `$improve-code-boundaries`

- Avoid smell 8, "too much in one file": do not add all generic executor, rate limiter, request structs, and typed methods to `client.go`.
- Avoid smell 13, "functions with wrong overlap": shared worker-pool, result accounting, logging, rate-limit, and error aggregation must exist only in `ExecuteBatch`, not repeated in each typed batch method.
- Avoid smell 5, "one shared shape": use the one `BatchKey` and one `BatchConfig` shape for every batched kube-api operation.
- Avoid smell 3, "wrong place-ism": config validation and defaulting belongs in `internal/config`; kubeapi only normalizes direct `Options.BatchConfig` for callers that bypass app config.
- Avoid smell 7, "stop overengineering": no retry engine, no cross-batch persistence, no future reconcile rewrite in this task.
- Avoid smell 14, "too public": only export the task-required API types/methods. Keep worker structs, normalized config helpers, rate limiter helpers, and typedResource internals private.

## Behaviors To Prove

- Config loading accepts omitted `kubeAPI` and defaults to workers `1`, requests/sec `100`, in-flight `100`.
- Config loading accepts explicit `kubeAPI` values and the compose config uses workers `20`.
- Negative kubeAPI numeric values fail validation with field-specific errors.
- `ExecuteBatch` defaults to one worker when `NumParallelWorkers` is unset.
- `ExecuteBatch` honors configured worker count `20`.
- `ExecuteBatch` handles at least 10,000 resources in normal `go test ./...`.
- `ExecuteBatch` honors `MaxRequestsInFlight`.
- `ExecuteBatch` honors `MaxRequestsPerSecond`.
- `ExecuteBatch` gathers per-item errors, keeps processing after item errors, returns successful item results, returns `BatchItemError` values per key, and returns a `BatchError`.
- `ExecuteBatch` performs zero retries.
- `ExecuteBatch` returns useful per-item context cancellation errors for queued or rate-limited work.
- Batch logs include start/completion info and per-item debug execution decisions with structured fields.
- Fake kube-api server tests observe actual request counts for group and network cloud apply/patch, update/PUT, status PUT, create POST, delete DELETE, and finalizer JSON Patch.
- Status update batch uses `PUT` on the `/status` subresource.
- Finalizer batch sends JSON Patch with the full desired finalizer array and resourceVersion test when provided.
- Update batch performs no GET and sends the supplied object's resourceVersion.


1. [x] RED: Add `internal/config` behavior tests for omitted `kubeAPI` defaults and explicit compose-style `kubeAPI` values.
2. [x] GREEN: Add `KubeAPIConfig`, raw YAML parsing, defaulting, validation, startup summary logging, and compose config values.
3. [x] RED: Add a pure `internal/kubeapi` batch executor test proving default worker count is one by measuring max concurrent executions with unset `BatchConfig`.
4. [x] GREEN: Implement minimal `BatchConfig`, `BatchKey`, `BatchOperation`, and `ExecuteBatch` worker pool with default worker normalization.
5. [x] RED: Add configured worker count `20` and 10,000+ item executor tests that assert all results are present.
6. [x] GREEN: Complete deterministic scheduling, result collection, and enough logging for start/completion and item execution.
7. [x] RED: Add max in-flight test where many workers block inside execution but observed concurrent executions never exceed `MaxRequestsInFlight`.
8. [x] GREEN: Add in-flight semaphore to the executor and debug logs for wait/acquire/release.
9. [x] RED: Add max requests per second test using a small limit and timestamped executions, with enough tolerance to avoid flakes.
10. [x] GREEN: Add `rate.Limiter` wait before execution and debug logs for rate-limit waits/acquires.
11. [x] RED: Add per-item error/no-retry test: selected keys fail once, selected keys succeed, every key is attempted once, the error map values are `BatchItemError`, and aggregate error is `BatchError`.
12. [x] GREEN: Implement `BatchItemError`, `BatchError`, `Errors()`, and no-retry accounting.
13. [x] RED: Add context cancellation tests for queued work and rate-limited work, asserting unprocessed/canceled keys get useful item errors.
14. [x] GREEN: Stop scheduling on cancellation, fill missing keyed errors, and return aggregate failure.
15. [x] RED: Add fake kube-api server batch method test for `GroupClient` create, update, apply, status update, delete, and finalizer patch counts and request shapes.
16. [x] GREEN: Add typed request structs, `Options.BatchConfig`, typedResource batch wrappers, `GroupClient` batch methods, and `patchFinalizers` JSON Patch.
17. [x] RED: Add equivalent fake kube-api server test coverage for `NetworkCloudClient` batch methods.
18. [x] GREEN: Add `NetworkCloudClient` batch methods by reusing the same typedResource batch wrappers.
19. [x] RED: Add focused tests proving update batch performs no GET and sends the supplied resourceVersion, status batch uses `PUT /status`, and finalizer patch sends the full desired finalizers array.
20. [x] GREEN: Adjust request construction only; do not introduce any reconcile-loop changes.
21. [x] REFACTOR: Run the `$improve-code-boundaries` review on touched code. Split files or inline helpers if the implementation starts duplicating batch mechanics or turning `client.go` into a catch-all.
22. [x] VERIFY focused tests:
    - `go test ./internal/config ./internal/kubeapi -count=1`
    - `go test -race ./internal/kubeapi -run 'TestExecuteBatch.*10000|TestExecuteBatchHandles10000' -count=1`
23. [x] VERIFY required gates:
    - `make check`
    - `make test`
    - `make test-coverage`
24. [x] MANUAL EVIDENCE: Record concrete evidence in this task file, including exact commands and outputs for focused tests, 10,000+ race test, full gates, fake server request counts, status PUT path, finalizer JSON Patch payload, and coverage percentage.
25. [x] DONE: Only after all checks pass and coverage is 80%+, set `<passes>true</passes>`, run `/bin/bash .ralph/task_switch.sh`, commit all files with `task finished 02-task-add-batched-kubeapi-patch-and-put: ...`, push, then quit immediately.


- If implementation requires changing the requested public batch type names or method signatures, replace the final marker with `TO BE VERIFIED`, document the proposed API here, update the task marker, and quit immediately.
- If finalizer JSON Patch with `add /metadata/finalizers` does not work against the fake or envtest kube API for existing and absent finalizer arrays, switch back to `TO BE VERIFIED` and propose the exact JSON Patch operation contract before continuing.
- If rate-limit timing tests are flaky under normal `go test ./...`, switch back to `TO BE VERIFIED` and redesign the limiter test seam without weakening behavior coverage.
- If `ExecuteBatch` cannot both preserve deterministic result accounting and satisfy the "one coordinator" requirement cleanly, switch back to `TO BE VERIFIED` and document the concurrency design choice.
- If startup/config wiring starts leaking raw config into kubeapi internals, stop and redesign the config reduction boundary before implementation.

NOW EXECUTE

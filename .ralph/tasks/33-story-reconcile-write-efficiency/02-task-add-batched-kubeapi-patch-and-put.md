## Task: Add Generic Batched Kube API Operations <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Add generic kube-api operation batching so reconcile can process many Kubernetes operations efficiently without one slow serial call per resource. All kube-api methods used by reconcile must be batchable, including apply/patch, put/update, status update, create, delete, and finalizer patch operations. The batch engine must also be reusable for future kube-api methods without rewriting worker-pool, rate-limit, logging, result-mapping, or error-gathering logic. Callers must build a map of typed request structs keyed by resource operation identity and pass that map to a batch function. The batch processor must execute those requests with a configurable worker pool, apply rate limiting, and return complete per-item results without dropping errors.

Configuration requirements:
- Add configuration for the maximum number of parallel kube-api batch workers.
- The configured value must default to `1` when unset or omitted.
- The repository's normal config should set kube-api batch workers to `20`.
- Add kube-api rate limiting configuration with exactly `maxRequestsPerSecond` and `maxRequestsInFlight`.
- Kube-api rate limiting defaults must be high enough not to throttle normal usage aggressively, while still making limits explicit and testable.
- Add a top-level `kubeAPI` config section:

```yaml
kubeAPI:
  numParallelWorkers: 20
  maxRequestsPerSecond: 100
  maxRequestsInFlight: 100
```

API shape requirements:
- Implement one public generic batch executor that can run any kube-api method by accepting typed requests plus a typed execution function.
- Add typed request structs for every batched operation. Each request must contain the same arguments the single-operation call already needs, plus enough operation metadata for logging and result matching.
- Add public batch methods for every kube-api operation used by reconcile: apply/patch, put/update, status update, create, delete, and finalizer patch.
- Future kube-api methods must be able to become batchable by defining a request struct and calling the public generic executor.
- Preserve the current single-operation methods for non-batch callers.
- Keep caller changes small: callers should build `map[BatchKey]RequestType` and pass that map to the matching batch method.
- Batched patch must execute the existing patch/apply operation shape with the same payload semantics as the current single-operation client.
- Finalizer changes must use a batched JSON Patch operation that writes the full desired finalizers array.
- Batched put/update must execute the existing put/update operation shape and must use object `resourceVersion` values from the gather pass rather than performing a fresh get.
- Batched status update must support the existing PUT `/status` operation.
- Implement the generic executor and public batch API in the typed kube-api client with this shape, adjusted only for exact package naming after implementation:

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

type GroupApplyRequest struct {
	Object  *nsxv1alpha.NSXGroup
	Options ApplyOptions
}

type GroupUpdateRequest struct {
	Object  *nsxv1alpha.NSXGroup
	Options metav1.UpdateOptions
}

type GroupFinalizerPatchRequest struct {
	Name            string
	ResourceVersion string
	Finalizers      []string
	Options         metav1.PatchOptions
}

type GroupStatusUpdateRequest struct {
	Name    string
	Status  nsxv1alpha.NSXGroupStatus
	Options StatusUpdateOptions
}

type GroupCreateRequest struct {
	Object  *nsxv1alpha.NSXGroup
	Options metav1.CreateOptions
}

type GroupDeleteRequest struct {
	Name    string
	Options metav1.DeleteOptions
}

type NetworkCloudApplyRequest struct {
	Object  *nsxv1alpha.NSXNetworkCloud
	Options ApplyOptions
}

type NetworkCloudUpdateRequest struct {
	Object  *nsxv1alpha.NSXNetworkCloud
	Options metav1.UpdateOptions
}

type NetworkCloudFinalizerPatchRequest struct {
	Name            string
	ResourceVersion string
	Finalizers      []string
	Options         metav1.PatchOptions
}

type NetworkCloudStatusUpdateRequest struct {
	Name    string
	Status  nsxv1alpha.NSXNetworkCloudStatus
	Options StatusUpdateOptions
}

type NetworkCloudCreateRequest struct {
	Object  *nsxv1alpha.NSXNetworkCloud
	Options metav1.CreateOptions
}

type NetworkCloudDeleteRequest struct {
	Name    string
	Options metav1.DeleteOptions
}

func (c *GroupClient) ApplyBatch(ctx context.Context, requests map[BatchKey]GroupApplyRequest) (map[BatchKey]*nsxv1alpha.NSXGroup, map[BatchKey]error, error)
func (c *GroupClient) UpdateBatch(ctx context.Context, requests map[BatchKey]GroupUpdateRequest) (map[BatchKey]*nsxv1alpha.NSXGroup, map[BatchKey]error, error)
func (c *GroupClient) PatchFinalizersBatch(ctx context.Context, requests map[BatchKey]GroupFinalizerPatchRequest) (map[BatchKey]*nsxv1alpha.NSXGroup, map[BatchKey]error, error)
func (c *GroupClient) UpdateStatusBatch(ctx context.Context, requests map[BatchKey]GroupStatusUpdateRequest) (map[BatchKey]*nsxv1alpha.NSXGroup, map[BatchKey]error, error)
func (c *GroupClient) CreateBatch(ctx context.Context, requests map[BatchKey]GroupCreateRequest) (map[BatchKey]*nsxv1alpha.NSXGroup, map[BatchKey]error, error)
func (c *GroupClient) DeleteBatch(ctx context.Context, requests map[BatchKey]GroupDeleteRequest) (map[BatchKey]struct{}, map[BatchKey]error, error)

func (c *NetworkCloudClient) ApplyBatch(ctx context.Context, requests map[BatchKey]NetworkCloudApplyRequest) (map[BatchKey]*nsxv1alpha.NSXNetworkCloud, map[BatchKey]error, error)
func (c *NetworkCloudClient) UpdateBatch(ctx context.Context, requests map[BatchKey]NetworkCloudUpdateRequest) (map[BatchKey]*nsxv1alpha.NSXNetworkCloud, map[BatchKey]error, error)
func (c *NetworkCloudClient) PatchFinalizersBatch(ctx context.Context, requests map[BatchKey]NetworkCloudFinalizerPatchRequest) (map[BatchKey]*nsxv1alpha.NSXNetworkCloud, map[BatchKey]error, error)
func (c *NetworkCloudClient) UpdateStatusBatch(ctx context.Context, requests map[BatchKey]NetworkCloudStatusUpdateRequest) (map[BatchKey]*nsxv1alpha.NSXNetworkCloud, map[BatchKey]error, error)
func (c *NetworkCloudClient) CreateBatch(ctx context.Context, requests map[BatchKey]NetworkCloudCreateRequest) (map[BatchKey]*nsxv1alpha.NSXNetworkCloud, map[BatchKey]error, error)
func (c *NetworkCloudClient) DeleteBatch(ctx context.Context, requests map[BatchKey]NetworkCloudDeleteRequest) (map[BatchKey]struct{}, map[BatchKey]error, error)
```
- `BatchKey` must identify the operation, resource, optional subresource, and resource name.
- `BatchItemError` must implement `error`; each entry in the returned error map must be a `BatchItemError` carrying the `BatchKey` and cause.
- `BatchError` must implement `error` and expose `Errors() map[BatchKey]error` containing the item errors.

Batch behavior requirements:
- Support batched apply/patch calls.
- Support batched put/update calls.
- Support batched status update calls.
- Support batched create calls.
- Support batched delete calls.
- Support batched finalizer patch calls.
- Execute batch items through multiple parallel workers when configured above `1`.
- Preserve enough result detail to identify which resource operation failed.
- Gather all errors. Return a result map, an error map, and an aggregate error indicating that the batch failed.
- Keep processing queued work after item errors and gather all item errors unless context cancellation prevents further execution.
- The batch executor and public batch methods perform no retries.
- Respect context cancellation and deadlines.
- Use one coordinator goroutine to iterate over the request map, send keyed requests over a synchronous work channel to workers, receive keyed responses/errors over a result channel, and assemble result/error maps.
- Use zap structured debug logs for item scheduling, execution, success, failure, rate-limit waits if present, and batch completion.
- Use info logs for larger batch actions, including batch size, operation type, worker count, and rate-limit settings.
- Keep kube-api client behavior deterministic enough for tests to assert concurrency limits, rate limiting, result accounting, and error propagation.

Testing requirements:
- Add normal package tests that exercise the batch processor with 10,000+ resources.
- The 10,000+ resource batch tests must run as part of normal `go test ./...`.
- Run the 10,000+ resource batch tests with `go test -race`.
- Test configured worker limits, default worker count `1`, configured worker count `20`, max requests per second, max requests in flight, context cancellation, and per-item error reporting.
- Include at least one integration-style test using a fake kube-api server or test environment that observes actual apply/patch, put/update, status update, create, delete, and finalizer patch request counts.
- Tests must prove status update batching uses the existing PUT `/status` operation.
- Tests must prove finalizer batch patch writes the full desired finalizers array.

In scope:
- Config structs/defaults/loading/validation, public generic kube-api batch executor, public typed kube-api batch APIs, rate limiter integration, worker pool implementation, tests, and manual verification evidence.

Out of scope:
- Rewriting every reconcile loop to use the batch API; that is the next task in this story.
- Changing NSX-T manager rate limiting semantics except as needed to share a generic limiter utility.


</description>


<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Config exposes kube-api batch max parallel workers, defaults to `1`, and normal repo config sets it to `20`.
- [ ] Config exposes top-level `kubeAPI.numParallelWorkers`, `kubeAPI.maxRequestsPerSecond`, and `kubeAPI.maxRequestsInFlight` with explicit high request-rate and in-flight defaults.
- [ ] One public generic batch executor supports current and future kube-api methods by accepting keyed typed requests and a typed execution function.
- [ ] Batch APIs accept maps of typed request structs that map directly to existing single-operation arguments.
- [ ] Batched patch preserves the existing patch/apply semantics.
- [ ] Batched put/update uses resource versions supplied by the request objects and performs no pre-update get.
- [ ] Batched finalizer changes use JSON Patch to write the full desired finalizers array.
- [ ] Batched status update supports the existing PUT `/status` operation.
- [ ] Batched apply/patch operations execute through the configured worker pool and report per-item success/failure.
- [ ] Batched put/update operations execute through the configured worker pool and report per-item success/failure.
- [ ] Batched status update, create, delete, and finalizer patch operations execute through the configured worker pool and report per-item success/failure.
- [ ] Batch execution gathers all item errors, returns result and error maps, and returns a `BatchError` aggregate exposing `Errors() map[BatchKey]error` for failed batches.
- [ ] Batch execution performs zero retries.
- [ ] Context cancellation returns useful errors for unprocessed or canceled items.
- [ ] Normal `go test ./...` includes 10,000+ resource batch coverage.
- [ ] Tests cover default worker count, configured worker count `20`, 10,000+ resources, max requests per second, max requests in flight, cancellation, no-retry behavior, and per-item error handling.
- [ ] `go test -race` is run for the 10,000+ resource batch tests and the command plus result are recorded.
- [ ] Structured zap logs prove batch start/completion and per-item execution decisions without ignoring any errors.
</acceptance_criteria>

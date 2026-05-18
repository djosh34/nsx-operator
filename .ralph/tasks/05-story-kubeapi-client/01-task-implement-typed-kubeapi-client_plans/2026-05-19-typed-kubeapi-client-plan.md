## Plan: Typed Kubernetes CRD Client

Task: `.ralph/tasks/05-story-kubeapi-client/01-task-implement-typed-kubeapi-client.md`

### Current State

- `api/v1alpha` already defines `NSXNetworkCloud`, `NSXNetworkCloudList`, `NSXNetworkCloudStatus`, `NSXGroup`, `NSXGroupList`, `NSXGroupStatus`, scheme registration, and handwritten deepcopy methods.
- CRD manifests already exist under `config/crd/bases` and are cluster-scoped.
- Both CRDs already expose a `/status` subresource.
- `NSXNetworkCloud` selectable fields are `.spec.networkCloudFQDN` and `.spec.networkCloudId`.
- `NSXGroup` selectable fields are `.spec.networkCloudFQDN`, `.spec.groupID`, and `.spec.mode`.
- Existing CRD integration tests use envtest with a real Kubernetes API server. This is the right test boundary for field selectors, server-side apply, watch behavior, and status subresource semantics.
- No `internal/kubeapi` package exists yet.

### Public Interface

Create package `internal/kubeapi`.

```go
type Options struct {
	Config *rest.Config
	Logger *zap.Logger
}

func NewClient(options Options) (*Client, error)

type Client struct {
	// private fields only
}

func (c *Client) NetworkClouds() *NetworkCloudClient
func (c *Client) Groups() *GroupClient
```

Rules:

- `Config` is required and must be copied with `rest.CopyConfig` before mutation.
- `Logger == nil` uses `zap.NewNop()`.
- `NewClient` registers `api/v1alpha` into a private scheme and configures a `rest.Interface` with `SchemeGroupVersion`, `runtime.NegotiatedSerializer`, and `APIPath`.
- The package owns client-go REST mechanics. Callers do not pass raw resource strings, raw JSON bytes, or unstructured objects.
- All methods accept `context.Context` first and return typed `api/v1alpha` objects or status structs plus `error`.
- All larger actions log at `Info`, and request-level details log at `Debug` with structured zap fields.

Resource clients:

```go
type NetworkCloudClient struct {
	// private fields only
}

func (c *NetworkCloudClient) List(ctx context.Context, options ListOptions) (*v1alpha.NSXNetworkCloudList, error)
func (c *NetworkCloudClient) Get(ctx context.Context, name string, options metav1.GetOptions) (*v1alpha.NSXNetworkCloud, error)
func (c *NetworkCloudClient) Create(ctx context.Context, object *v1alpha.NSXNetworkCloud, options metav1.CreateOptions) (*v1alpha.NSXNetworkCloud, error)
func (c *NetworkCloudClient) Update(ctx context.Context, object *v1alpha.NSXNetworkCloud, options metav1.UpdateOptions) (*v1alpha.NSXNetworkCloud, error)
func (c *NetworkCloudClient) Apply(ctx context.Context, object *v1alpha.NSXNetworkCloud, options ApplyOptions) (*v1alpha.NSXNetworkCloud, error)
func (c *NetworkCloudClient) UpdateStatus(ctx context.Context, name string, status v1alpha.NSXNetworkCloudStatus, options StatusUpdateOptions) (*v1alpha.NSXNetworkCloud, error)
func (c *NetworkCloudClient) Delete(ctx context.Context, name string, options metav1.DeleteOptions) error
func (c *NetworkCloudClient) Watch(ctx context.Context, options ListOptions) (watch.Interface, error)

type GroupClient struct {
	// private fields only
}

func (c *GroupClient) List(ctx context.Context, options ListOptions) (*v1alpha.NSXGroupList, error)
func (c *GroupClient) Get(ctx context.Context, name string, options metav1.GetOptions) (*v1alpha.NSXGroup, error)
func (c *GroupClient) Create(ctx context.Context, object *v1alpha.NSXGroup, options metav1.CreateOptions) (*v1alpha.NSXGroup, error)
func (c *GroupClient) Update(ctx context.Context, object *v1alpha.NSXGroup, options metav1.UpdateOptions) (*v1alpha.NSXGroup, error)
func (c *GroupClient) Apply(ctx context.Context, object *v1alpha.NSXGroup, options ApplyOptions) (*v1alpha.NSXGroup, error)
func (c *GroupClient) UpdateStatus(ctx context.Context, name string, status v1alpha.NSXGroupStatus, options StatusUpdateOptions) (*v1alpha.NSXGroup, error)
func (c *GroupClient) Delete(ctx context.Context, name string, options metav1.DeleteOptions) error
func (c *GroupClient) Watch(ctx context.Context, options ListOptions) (watch.Interface, error)
```

### Options And Filters

Use typed package options instead of leaking raw `metav1.ListOptions` as the main caller interface:

```go
type ListOptions struct {
	ResourceVersion string
	Filters         []FieldFilter
	Limit           int64
	Continue        string
}

type FieldSelectorField string

const (
	FieldNetworkCloudFQDN FieldSelectorField = "spec.networkCloudFQDN"
	FieldNetworkCloudID   FieldSelectorField = "spec.networkCloudId"
	FieldGroupID          FieldSelectorField = "spec.groupID"
	FieldGroupMode        FieldSelectorField = "spec.mode"
)

type FieldFilter struct {
	// private fields only
}

func FilterBy(field FieldSelectorField, value string) FieldFilter
```

Rules:

- `FilterBy` is the public Go form of the requested `filterBy(field, value)` abstraction.
- `FieldFilter` stores field and value privately so selector rendering is centralized.
- `NetworkCloudClient` accepts only `FieldNetworkCloudFQDN` and `FieldNetworkCloudID`.
- `GroupClient` accepts only `FieldNetworkCloudFQDN`, `FieldGroupID`, and `FieldGroupMode`.
- Invalid fields return an error from `List` or `Watch`; do not silently drop filters.
- Render filters through `k8s.io/apimachinery/pkg/fields`, not string concatenation in each method.

Apply and status options:

```go
type ApplyOptions struct {
	FieldManager string
	Force        bool
}

type StatusUpdateOptions struct {
	ResourceVersion string
	UpdateOptions   metav1.UpdateOptions
}
```

Rules:

- `ApplyOptions.FieldManager` is required. Empty field manager returns an error before the request.
- `Apply` uses server-side apply with `types.ApplyPatchType`, not merge patch or raw status patch APIs.
- `Update` requires `object.ResourceVersion != ""`; empty resourceVersion returns an error before the request.
- `UpdateStatus` accepts only `name`, typed `status`, and metadata options. The method creates a minimal typed object containing `metadata.name`, optional `metadata.resourceVersion`, and `status`. There is no status API that accepts a full caller-provided object with spec fields.

### Internal Design

Use one private generic resource wrapper to keep the package deep:

```go
type typedResource[Object clientObject, List clientObject] struct {
	restClient rest.Interface
	resource   string
	kind       string
	log        *zap.Logger
	allowed    map[FieldSelectorField]struct{}
	newObject  func() Object
	newList    func() List
}
```

Implementation notes:

- Keep resource plural strings (`nsxnetworkclouds`, `nsxgroups`) private.
- Keep kind strings private.
- Keep TypeMeta preparation private. Before create/update/apply/status, deep-copy or construct the typed object and set `APIVersion`/`Kind` if missing.
- Convert `ListOptions` to `metav1.ListOptions` in one private method.
- Convert `ApplyOptions` to `metav1.PatchOptions` in one private method.
- Use `rest.Interface` methods directly: `Get`, `Post`, `Put`, `Patch`, and `Delete`.
- `Watch` sets `Watch: true`, uses the typed field selectors, and calls `.Watch(ctx)`.
- Never ignore returned errors, including response decoding and watcher creation errors.

### Boundary Plan Using `$improve-code-boundaries`

- Keep `internal/kubeapi` as the only production boundary for typed Kubernetes CR clients. Do not scatter REST resource strings or field-selector string rendering into callers.
- Do not add separate DTOs mirroring `api/v1alpha` specs or statuses. The CR Go types are already the correct API contract.
- Do not expose unstructured, dynamic, or raw patch APIs from `internal/kubeapi`.
- Use a small exported surface: `Client`, two resource clients, list/filter/apply/status option types, field constants, and `FilterBy`.
- Keep the generic REST wrapper private. Public callers should see typed clients, not generic type parameters or Kubernetes REST plumbing.
- Make fields private unless callers need them for object construction. `FieldFilter` should only be constructible through `FilterBy`.
- After green tests, scan for duplicate list/get/create/update/apply/status/watch code across both resources and move shared code behind the private generic wrapper.
- Final boundary review: verify no raw JSON status patch APIs, no duplicated CRD DTOs, no public resource plural helpers, no direct `fields.ParseSelector` usage outside the package internals, and no logging of credentials or full object payloads.

### TDD Execution Plan Using `$tdd`

Follow vertical red-green cycles. Write one behavior test, confirm it fails for the expected reason, implement the minimum production code, then continue. Do not write all tests first.

- [x] Tracer bullet: typed network cloud create/get/list through envtest.
  - RED: add an envtest-backed test that installs CRDs, constructs `kubeapi.NewClient`, creates an `NSXNetworkCloud`, gets it by name, and lists it with `FilterBy(FieldNetworkCloudFQDN, value)`.
  - GREEN: add `internal/kubeapi`, `Options`, `NewClient`, private REST client setup, `NetworkCloudClient.Create`, `Get`, and `List`.
- [x] Typed group create/get/list through envtest.
  - RED: create two `NSXGroup` objects and prove `FilterBy(FieldGroupID, value)`, `FilterBy(FieldGroupMode, string(v1alpha.NSXGroupModeManage))`, and `FilterBy(FieldNetworkCloudFQDN, value)` return the expected typed list items.
  - GREEN: add `GroupClient.Create`, `Get`, and `List` through the same private resource wrapper.
- [x] Update requires resourceVersion.
  - RED: prove `Update` on a typed network cloud or group with empty `ResourceVersion` returns a local validation error and does not send a Kubernetes request; then prove updating an object fetched from the API succeeds.
  - GREEN: add resourceVersion validation and typed update methods.
- [x] Status updates are status-only and cannot mutate spec.
  - RED: create a group, call `UpdateStatus` with a typed `NSXGroupStatus`, then get the group and prove status changed while spec stayed identical. Also prove there is no API path that accepts a full spec-bearing object for status update.
  - GREEN: implement minimal-object `/status` updates for both resources.
- [x] Apply requires field manager and uses server-side apply.
  - RED: prove empty `ApplyOptions.FieldManager` fails locally. Then apply a typed group with a non-empty manager, get it back, apply a changed spec with the same manager, and prove the API stores the change.
  - GREEN: implement typed server-side apply with `types.ApplyPatchType`, `metav1.PatchOptions{FieldManager, Force}`, and TypeMeta preparation.
- [x] Delete removes typed objects.
  - RED: create a network cloud or group, delete it through the typed client, and prove `Get` returns a Kubernetes not-found error.
  - GREEN: implement delete on the private resource wrapper.
- [x] Watch emits typed events.
  - RED: start `Watch` with a typed field filter, create a matching object, and prove the event object is the expected typed CR kind and name.
  - GREEN: implement watch using converted typed list options and the REST client's `Watch`.
- [x] Filter validation rejects fields that are not selectable for that resource.
  - RED: use `NetworkCloudClient.List` with `FieldGroupID` and prove it returns a validation error before making a request; use `GroupClient.List` with `FieldNetworkCloudID` and prove the same.
  - GREEN: add per-resource allowed field validation in one private method.
- [x] Constructor validation and logging.
  - RED: prove nil config returns an error, nil logger is accepted, and actions emit structured zap log entries to a test sink.
  - GREEN: add constructor validation and structured debug/info logging.
- [x] Refactor after green.
  - Run focused tests after each refactor.
  - Collapse duplicated resource methods behind private generic helpers.
  - Keep tests behavior-focused through the public `internal/kubeapi` API.

### Test Harness

- Reuse the envtest pattern from `api/v1alpha/crd_integration_test.go`.
- Create `internal/kubeapi/envtest_test.go` helpers:
  - require `KUBEBUILDER_ASSETS`,
  - install CRDs from `config/crd/bases`,
  - start/stop envtest with checked errors,
  - construct a typed `kubeapi.Client`,
  - use short context timeouts for watch tests.
- Keep tests in `internal/kubeapi` or `internal/kubeapi_test` depending on whether private request-count instrumentation is needed. Prefer `kubeapi_test` for public-interface behavior tests.
- Use `apierrors.IsNotFound` and other Kubernetes error predicates instead of string matching error messages.
- Use `zaptest/observer` or an in-memory zapcore sink for logging assertions, without requiring exact full log text.

### Verification

Focused commands during implementation:

```bash
KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/kubeapi -count=1 -v
KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test -race ./internal/kubeapi -count=1
KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test -coverprofile=/tmp/kubeapi.cover ./internal/kubeapi -count=1 && go tool cover -func=/tmp/kubeapi.cover
```

Required final checks:

```bash
make check
make test
make test-coverage
```

Coverage requirement:

- `internal/kubeapi` must report at least 80% coverage.
- `make test-coverage` must also report at least 80% for all new code and keep the whole repo above the required bar.

Manual evidence to record in the task file before setting `<passes>true</passes>`:

- Focused `go test ./internal/kubeapi -count=1 -v` output showing create, get, list field selectors, update, apply, watch, status update, and delete behavior passed against envtest.
- Explicit evidence that `UpdateStatus` changed status and preserved spec.
- Explicit evidence that invalid resource-specific filters fail before request execution.
- Focused race and coverage output for `internal/kubeapi`.
- Full `make check`, `make test`, and `make test-coverage` output.
- Final boundary review notes confirming no raw status patch API and no duplicated CR DTO layer.

### Completion Steps

- Mark checklist items in this plan complete as each red-green cycle is implemented.
- If implementation proves the public interface, type design, or option design is wrong, replace the final marker with `TO BE VERIFIED` and quit immediately.
- If design remains valid and all checks pass:
  - update the task file with concrete verification evidence,
  - set `<passes>true</passes>`,
  - run `/bin/bash .ralph/task_switch.sh`,
  - add all files,
  - commit with `task finished 01-task-implement-typed-kubeapi-client: implement typed Kubernetes CRD client`,
  - include summary and test evidence in the commit message,
  - push,
  - quit immediately.

Plan path: `.ralph/tasks/05-story-kubeapi-client/01-task-implement-typed-kubeapi-client_plans/2026-05-19-typed-kubeapi-client-plan.md`

NOW EXECUTE

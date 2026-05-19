## Plan: Per-Manager Gather Process Apply Pipeline

Task: `.ralph/tasks/08-story-manager-sweep-pipeline/01-task-implement-manager-sweep-pipeline.md`

### Current State

- `internal/stateoperator` already owns periodic global sweeps, one goroutine per `NSXNetworkCloud`, sweep IDs, and structured sweep logs. It currently calls an injected `SweepCloud` callback that defaults to no-op.
- `internal/nsxclient` already exposes typed group methods and paginated list helpers. `ListGroups(ctx)` follows NSX cursors internally.
- `internal/kubeapi` already exposes typed CRD clients with field selectors, server-side apply, status update, delete, and watch operations.
- `api/v1alpha` already provides `NSXNetworkCloud`, `NSXGroup`, mode constants, condition constants, and status structs using conditions only.
- Future stories own controller reconcile details, finalizers, detailed Observe and Manage behavior, status condition semantics, names package extraction, and strict NSX write-semantics hardening. This task should build the sweep pipeline and the pure planner without embedding those future concerns too deeply.

### Public Interface

Keep the new behavior in `internal/stateoperator`; do not create a new top-level service package unless the implementation becomes clearly too crowded.

Core pure planning types:

```go
type BindingKey struct {
	NetworkCloudFQDN string
	GroupID          string
}

type RemoteGroup struct {
	Key                   BindingKey
	DisplayName           string
	CIDRs                 []string
	SegmentPath           *string
	IPAddressExpressionID string
	PathExpressionID      string
	UnsupportedExpression bool
	Raw                   nsxclient.Group
}

type ManagerSnapshot struct {
	Cloud            nsxv1alpha.NSXNetworkCloud
	NetworkCloudFQDN string
	LocalGroups      []nsxv1alpha.NSXGroup
	RemoteGroups     []RemoteGroup
	GatherError      error
}

type ManagerPlan struct {
	ObserveUpserts []nsxv1alpha.NSXGroup
	ManagedWrites  []ManagedGroupWrite
	ManagedDeletes []ManagedGroupDelete
	GroupStatuses  []GroupStatusPlan
	ObserveDeletes []string
	CloudStatus    *CloudStatusPlan
}

func BuildBindings(snapshot ManagerSnapshot) (ManagerBindings, error)
func ProcessManagerSnapshot(snapshot ManagerSnapshot, now time.Time) (ManagerPlan, error)
```

`ProcessManagerSnapshot` is the main public behavior for tests. It must be deterministic, must only inspect the value snapshot passed to it, and must not accept Kubernetes, NSX, HTTP, logger, clock, or client interfaces. `now` is an explicit value so condition timestamps are deterministic in unit tests.

Pipeline shell:

```go
type ManagerClient interface {
	ListGroups(ctx context.Context) ([]*nsxclient.Group, error)
	PatchGroup(ctx context.Context, groupID string, group *nsxclient.Group) error
	PatchGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.IPAddressExpression) error
	AddGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.IPAddressExpression) error
	DeleteGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string) error
	PatchGroupPathExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.PathExpression) error
	DeleteGroup(ctx context.Context, groupID string) error
}

type ManagerClientFactory func(ctx context.Context, cloud nsxv1alpha.NSXNetworkCloud) (ManagerClient, error)
```

Extend `stateoperator.Options` with a typed CRD client and manager-client factory:

```go
KubeClient            *kubeapi.Client
ManagerClientFactory  ManagerClientFactory
Now                   func() time.Time // optional; default uses the existing Clock.Now
```

If `SweepCloud` is supplied, keep honoring it for tests and future overrides. If `SweepCloud` is nil, construct the default cloud sweep from `KubeClient`, `ManagerClientFactory`, logger, and clock.

Startup wiring:

- In `internal/startup.NewManager`, construct `kubeapi.NewClient` with the manager `restConfig` and pass it into `stateoperator.New`.
- Add a default manager-client factory that builds an `nsxclient.Client` for the cloud using global Basic Auth from validated config and an HTTPS base URL derived from normalized `cloud.Spec.NetworkCloudFQDN`.
- Keep HTTP rate-limiter wiring out of this story unless the current startup path already exposes a shared `http.Client`; story 15 owns full concurrency validation. If a plain client is needed now, make that explicit and leave the factory small enough for story 15 to replace.

### Normalization And Keys

- Normalize `cloud.Spec.NetworkCloudFQDN` once at gather start. Use the normalized value for:
  - local `NSXGroup` field selector,
  - all `BindingKey.NetworkCloudFQDN` values,
  - generated Observe CR specs.
- Implement a private normalization helper compatible with the future `internal/names` story: trim whitespace, lowercase the host, preserve an explicit port, and remove accidental URL scheme/trailing slash if present. Do not mutate existing CR specs during gather.
- `BindingKey` identity is exactly normalized `<networkCloudFQDN>/<groupID>`.
- For Observe upsert metadata names, use a small private deterministic helper matching the future names story examples: `nsx-a.example.net/app-foo -> nsx-a.example.net--app-foo` and `nsx-a.example.net:8443/app-foo -> nsx-a.example.net-8443--app-foo`. Keep it private so story 16 can extract/replace it without public API churn.

### Gather Design

Gather must do all external reads and produce only in-memory values:

1. Normalize the cloud FQDN.
2. List local `NSXGroup` CRs with `kubeapi.Groups().List(ctx, kubeapi.ListOptions{Filters: []kubeapi.FieldFilter{kubeapi.FilterBy(kubeapi.FieldNetworkCloudFQDN, normalizedFQDN)}})`.
3. Construct the manager client for the cloud.
4. Call `ListGroups(ctx)` once. Rely on `nsxclient` pagination for all remote pages.
5. Convert each `nsxclient.Group` into `RemoteGroup` by parsing group expressions in memory:
   - one `IPAddressExpression` maps to `CIDRs` and records its expression ID,
   - zero or one `PathExpression` with one path maps to `SegmentPath` and records its expression ID,
   - unknown expression resource types, multiple represented expressions, extended expressions, or malformed JSON set `UnsupportedExpression=true` while preserving the best representable spec.
6. On any gather error, return `ManagerSnapshot{Cloud: cloud, NetworkCloudFQDN: normalizedFQDN, GatherError: err}`. Do not fabricate empty remote groups in a way that would cause child mass-marking.

Debug log each read boundary and parsed count. Info log larger sweep actions and failures with `component`, `sweepID`, `networkCloudFQDN`, and where applicable `groupID`.

### Process Design

`ProcessManagerSnapshot(snapshot, now)` implements the pure policy:

- If `snapshot.GatherError != nil`, return a plan with only `CloudStatus` setting `Reachable=False` and `Swept=False` or Unknown, using the gather error as message. No group status, Observe delete, Observe upsert, or Manage write may be planned in this path.
- Build deterministic bindings from local and remote groups:
  - sort local groups by `metadata.name`,
  - sort remote groups by `BindingKey`,
  - build maps by `BindingKey`,
  - return an error for duplicate local logical identities or duplicate remote logical identities, rather than silently picking a winner.
- Remote-only binding:
  - plan an Observe upsert with deterministic metadata name, `mode=Observe`, normalized FQDN, remote group ID, display name, CIDRs, and segment path.
  - plan group status conditions reflecting remote present, spec matches remote, unsupported expression, and synced/realized as far as this story can determine.
- Existing Observe with remote present:
  - if the remote spec differs, plan an Observe upsert that replaces the spec from remote while keeping `mode=Observe`.
  - plan status updates for matched or unsupported remote expressions.
- Existing Observe with remote missing after successful gather:
  - plan Kubernetes CR delete in `ObserveDeletes`.
- Existing Manage with remote missing:
  - plan `ManagedWrite` that creates or patches the NSX group shell and represented expressions from the Kubernetes spec.
  - plan status reflecting Applying/Unknown rather than falsely marking synced.
- Existing Manage with remote present but drifted:
  - plan `ManagedWrite` using expression IDs discovered in the snapshot.
  - do not rewrite the CR spec from remote.
- Existing Manage with remote present and matching:
  - plan only a status update.
- `ManagedDeletes` exists to preserve the apply order surface, but story 13 owns finalizer-driven delete confirmation. For this story, only populate it for a local Manage group that already has deletion timestamp/finalizer state if the existing Kubernetes API makes that observable without adding lifecycle policy. If that proves design-heavy, leave it empty and document that Manage deletion is story 09/11/13 scope.

### Apply Design

Apply executes a completed plan in exactly this order:

1. Observe upserts via `kubeapi.Groups().Apply`.
2. Managed NSX writes/deletes:
   - `PatchGroup` for the group shell,
   - `PatchGroupIPAddressExpression` when an ID exists,
   - `AddGroupIPAddressExpression` with stable ID `cidrs` when no ID exists,
   - `DeleteGroupIPAddressExpression` when a selected represented IP expression exists but spec CIDRs are empty,
   - `PatchGroupPathExpression` or `DeleteGroupIPAddressExpression` equivalent path cleanup only where `nsxclient` already exposes the exact route. Do not invent full group replacement.
   - `DeleteGroup` only for explicit managed delete plan entries.
3. Group status updates via `kubeapi.Groups().UpdateStatus`.
4. Observe deletes via `kubeapi.Groups().Delete`.
5. Cloud status update via `kubeapi.NetworkClouds().UpdateStatus`.

Abort on the first failed apply operation and return an annotated error. Do not set cloud `Swept=True` after a partial apply failure. Log every planned action at debug level and larger mutating actions at info level with structured zap fields.

### Boundary Plan Using `$improve-code-boundaries`

- Keep `ProcessManagerSnapshot` a deep pure module: values in, `ManagerPlan` out, no clients, no logging, no context, no global time.
- Keep gather/apply as the only client boundaries. No Kubernetes or NSX call should be reachable from `BuildBindings` or `ProcessManagerSnapshot`.
- Use existing CRD and NSX client structs directly. Do not introduce mirror DTOs for `NSXGroupSpec`, `NSXGroupStatus`, or NSX group payloads unless they remove real duplication.
- Keep startup limited to dependency construction. Do not spread gather/process/apply logic into `internal/startup`.
- Avoid request queues, worker pools, hidden retry loops, or manager-side reconciliation queues. Existing global sweep goroutines remain the concurrency model.
- Do not solve future stories by accident: finalizer lifecycle, exact status derivation, strict write-semantics contract inspection, and exported `internal/names` stay replaceable.
- Final boundary review after green: scan for duplicate spec/status shapes, stringly condition assembly spread across files, NSX raw JSON parsing mixed into apply, Kubernetes clients passed into process functions, and startup/bootstrap logic leaking reconciliation policy.

### TDD Execution Plan Using `$tdd`

Follow vertical red-green cycles. Write one behavior test, watch it fail, implement the minimum, then continue.

1. [x] Tracer bullet: gather failure only plans cloud status.
   - RED: call `ProcessManagerSnapshot` with `GatherError` and local Manage/Observe children; assert no child updates/deletes/writes and only cloud status is planned.
   - GREEN: add `ManagerSnapshot`, `ManagerPlan`, and the gather-error branch.

2. [x] Deterministic binding and duplicate detection.
   - RED: pass shuffled local/remote groups and assert sorted deterministic plan output; pass duplicate logical identities and assert an error.
   - GREEN: add `BindingKey`, `BuildBindings`, sorting, and duplicate detection.

3. [x] Remote-only groups are imported as Observe upserts.
   - RED: snapshot with one remote group and no local group; assert Observe upsert spec, deterministic name, and status plan.
   - GREEN: implement remote-only planning and private name helper.

4. [x] Observe existing groups mirror remote spec and delete when remote is gone after successful gather.
   - RED: one existing Observe with changed remote spec plans an upsert; one existing Observe with no remote plans delete.
   - GREEN: add Observe-present and Observe-missing branches.

5. [x] Manage missing/drift/match behavior.
   - RED: missing Manage remote plans a managed write; drifted remote plans a managed write with existing expression IDs; matching remote plans only status.
   - GREEN: implement represented-spec comparison and `ManagedGroupWrite`.

6. [x] Remote expression parsing is pure and preserves expression IDs.
   - RED: parse raw NSX group expressions for IP addresses and segment path; assert IDs are retained; assert unsupported extra expressions set `UnsupportedExpression` without panics.
   - GREEN: implement `RemoteGroupFromNSXGroup` or equivalent pure helper used by gather.

7. [x] Default gather lists local groups by normalized FQDN and uses NSX pagination.
   - RED: with envtest CRDs and the sibling mockapi or an `httptest` NSX server returning cursor pages, run the gather step and assert only normalized-FQDN local groups are included and remote groups from every page are present.
   - GREEN: wire `kubeapi.Groups().List` field selector, manager-client factory, and `nsxclient.ListGroups`.

8. [x] Apply order is exact.
   - RED: use recording fake kube and NSX clients with a plan containing every action category; assert operation order is Observe upserts, managed NSX writes/deletes, group statuses, Observe deletes, cloud status.
   - GREEN: implement `ApplyManagerPlan`.

9. [x] Startup default sweep wiring.
   - RED: manager/startup test proves `NewManager` constructs the default typed kube client and manager-client factory when no custom `SweepCloud` is supplied, while existing custom `SweepCloud` tests still pass.
   - GREEN: extend `stateoperator.Options` and `startup.NewManager`.

10. [x] Focused refactor pass.
   - Run focused tests after each cleanup.
   - Collapse duplicate condition/status constructors.
   - Keep NSX expression parsing private and test through behavior where possible.
   - Confirm all errors are checked; no `_ := err` patterns.

### Integration And Manual Verification Plan

Record concrete evidence in the task before setting `<passes>true</passes>`:

- Focused unit output for `go test ./internal/stateoperator -run 'ProcessManagerSnapshot|BuildBindings|RemoteGroup|ApplyManagerPlan' -count=1 -v`, proving pure processing determinism and gather-failure child safety.
- Integration output proving pagination, preferably through sibling `../nsx-t-mockapi` if it can be configured for multiple group pages, otherwise through an `httptest` NSX server exercising the real `nsxclient.ListGroups` cursor loop.
- Envtest output proving local `NSXGroup` gather uses the `spec.networkCloudFQDN` field selector and status/apply/delete operations work against real CRDs.
- Apply-order test output showing the exact operation sequence.
- Full `make check`, `make test`, and `make test-coverage` output.
- Coverage evidence proving new code is 80%+ and `make test-coverage` remains 80%+ package-wide.
- Final `$improve-code-boundaries` review notes confirming pure processing has no client dependencies and gather/apply are the only external I/O boundaries.

### Completion Steps

- Tick plan checkboxes as each TDD slice is executed.
- If implementation proves these interfaces, type shapes, or boundaries wrong, replace the final marker with `TO BE VERIFIED` and quit immediately.
- If design remains valid and all checks pass:
  - update the task acceptance criteria,
  - record verification evidence,
  - set `<passes>true</passes>`,
  - run `/bin/bash .ralph/task_switch.sh`,
  - add all files including `.ralph`,
  - commit with `task finished 01-task-implement-manager-sweep-pipeline: implement manager sweep pipeline`,
  - include summary and test evidence in the commit message,
  - push,
  - quit immediately.

Plan path: `.ralph/tasks/08-story-manager-sweep-pipeline/01-task-implement-manager-sweep-pipeline_plans/2026-05-19-manager-sweep-pipeline-plan.md`

NOW EXECUTE

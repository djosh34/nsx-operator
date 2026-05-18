## Plan: API Go Types And CRD Manifests

Task: `.ralph/tasks/06-story-api-types-crds/01-task-add-api-types-and-crd-manifests.md`

### Current State

- The repository has no `api/v1alpha` package, no Kubernetes API dependencies, and no CRD manifests.
- The task is a prerequisite for the typed Kubernetes client task because later code needs concrete `NSXNetworkCloud` and `NSXGroup` object types.
- The design source of truth is `.ralph/designs/PLAN.md`, sections 4 and 5.
- Required CRD API group/version: `nsx.ing.com/v1alpha`.
- Both CRDs are cluster-scoped.
- Kubernetes `selectableFields` are required and must be verified against a real Kubernetes v1.32+ API server.
- `NSXGroup` must not expose `domainId` anywhere in its spec. The NSX domain remains hidden elsewhere as `default`.

### Public Interface

Create package `api/v1alpha` with Kubernetes API types and scheme registration:

```go
const (
	GroupName = "nsx.ing.com"
	Version   = "v1alpha"
)

var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}

func AddToScheme(scheme *runtime.Scheme) error
```

Top-level resource types:

```go
type NSXNetworkCloud struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NSXNetworkCloudSpec   `json:"spec,omitempty"`
	Status            NSXNetworkCloudStatus `json:"status,omitempty"`
}

type NSXNetworkCloudList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NSXNetworkCloud `json:"items"`
}

type NSXGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NSXGroupSpec   `json:"spec,omitempty"`
	Status            NSXGroupStatus `json:"status,omitempty"`
}

type NSXGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NSXGroup `json:"items"`
}
```

Spec and status types:

```go
type NSXNetworkCloudSpec struct {
	NetworkCloudFQDN string `json:"networkCloudFQDN"`
	NetworkCloudID   string `json:"networkCloudId"`
	Name             string `json:"name"`
}

type NSXNetworkCloudStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type NSXGroupMode string

const (
	NSXGroupModeObserve NSXGroupMode = "Observe"
	NSXGroupModeManage  NSXGroupMode = "Manage"
)

type NSXGroupSpec struct {
	NetworkCloudFQDN string       `json:"networkCloudFQDN"`
	GroupID          string       `json:"groupID"`
	DisplayName      string       `json:"display_name"`
	Mode             NSXGroupMode `json:"mode"`
	CIDRs            []string     `json:"cidrs"`
	SegmentPath      *string      `json:"segment_path,omitempty"`
}

type NSXGroupStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

Condition constants:

```go
const (
	ConditionReachable             = "Reachable"
	ConditionSwept                 = "Swept"
	ConditionRemotePresent         = "RemotePresent"
	ConditionSpecMatchesRemote     = "SpecMatchesRemote"
	ConditionUnsupportedExpression = "UnsupportedExpression"
	ConditionRealized              = "Realized"
	ConditionSynced                = "Synced"
	ConditionApplying              = "Applying"
	ConditionDeleting              = "Deleting"
)
```

Implementation rules:

- Do not use the official Kubernetes code generator for this task.
- Add manual `DeepCopy`, `DeepCopyInto`, and `DeepCopyObject` methods required by `runtime.Object`.
- Keep CRD schemas and Go structs aligned with the design. Avoid parallel DTOs or conversion layers.
- Keep status conditions-only: no synced booleans, fingerprints, revisions, timestamps, remote objects, or extra status fields.
- Keep `domainId` absent from Go specs, CRD schemas, examples, and tests.

### CRD Manifests

Add manifests under `config/crd/bases`:

- `config/crd/bases/nsx.ing.com_nsxnetworkclouds.yaml`
- `config/crd/bases/nsx.ing.com_nsxgroups.yaml`

`NSXNetworkCloud` CRD requirements:

- `metadata.name: nsxnetworkclouds.nsx.ing.com`
- `spec.group: nsx.ing.com`
- `spec.scope: Cluster`
- `kind: NSXNetworkCloud`
- `listKind: NSXNetworkCloudList`
- `shortNames: [nsxnc]`
- served/storage version `v1alpha`
- `subresources.status`
- `openAPIV3Schema`
- `selectableFields`:
  - `.spec.networkCloudFQDN`
  - `.spec.networkCloudId`
- printer columns for FQDN, CloudID, DisplayName, Reachable, and Swept
- spec fields exactly `networkCloudFQDN`, `networkCloudId`, and `name`
- status fields exactly `conditions`

`NSXGroup` CRD requirements:

- `metadata.name: nsxgroups.nsx.ing.com`
- `spec.group: nsx.ing.com`
- `spec.scope: Cluster`
- `kind: NSXGroup`
- `listKind: NSXGroupList`
- `shortNames: [nsxg]`
- served/storage version `v1alpha`
- `subresources.status`
- `openAPIV3Schema`
- `selectableFields`:
  - `.spec.networkCloudFQDN`
  - `.spec.groupID`
  - `.spec.mode`
- printer columns for FQDN, GroupID, Mode, Synced, Realized, and Unsupported
- spec fields exactly `networkCloudFQDN`, `groupID`, `display_name`, `mode`, `cidrs`, and nullable `segment_path`
- status fields exactly `conditions`
- `mode` enum exactly `Observe` and `Manage`
- condition enum exactly `RemotePresent`, `SpecMatchesRemote`, `UnsupportedExpression`, `Realized`, `Synced`, `Applying`, and `Deleting`

### Boundary Plan Using `$improve-code-boundaries`

- Keep the Kubernetes API boundary deep in `api/v1alpha`: callers import one package for types, constants, and scheme registration.
- Do not add a second internal representation of CRD specs. The API structs are the contract used by future kube clients, controllers, and state processing.
- Keep CRD YAML as installable Kubernetes artifacts, not as strings embedded in Go code.
- Keep Kubernetes test helpers in tests, not production packages.
- Keep future operator concepts out of this task. Do not implement names, typed kube clients, controllers, reconciliation, finalizers, status condition helpers, or NSX conversion logic here.
- Avoid stringly-rendered YAML generation. Manifests should be static Kubernetes resources validated by a real API server.
- Final review after checks: scan for duplicate API shapes, hidden `domainId` leaks, extra status/spec fields, generated-code scaffolding that was not needed, and manifest/schema drift from Go types.

### TDD Execution Plan Using `$tdd`

Follow vertical red-green cycles. Write one behavior test, see it fail, implement only enough code to pass, then continue.

1. [x] Tracer bullet: API types register into a Kubernetes runtime scheme.
   - [x] RED: test `api/v1alpha.AddToScheme` with a fresh scheme, then use `scheme.New` for both `NSXNetworkCloud` and `NSXGroup` GVKs.
   - [x] GREEN: add Kubernetes dependencies, `api/v1alpha` type skeletons, `SchemeGroupVersion`, builder registration, and `AddToScheme`.

2. [x] Manual deepcopy behavior for top-level objects.
   - [x] RED: create populated `NSXGroup` and `NSXNetworkCloud` objects with labels, CIDRs, `segment_path`, and conditions; call `DeepCopyObject`; mutate the copy; assert original slices, maps, pointers, and conditions remain unchanged through public fields.
   - [x] GREEN: implement manual deepcopy methods without ignored errors.

3. [x] JSON/API shape behavior.
   - [x] RED: marshal and unmarshal representative `NSXGroup` and `NSXNetworkCloud` objects through `encoding/json` and assert public fields survive with the expected JSON names, including `display_name`, nullable/absent `segment_path`, and `conditions`.
   - [x] GREEN: add exact JSON tags and constants.

4. [x] CRD install and object acceptance against Kubernetes v1.32+.
   - [x] RED: integration test starts a real kube-apiserver/etcd using controller-runtime envtest assets for Kubernetes 1.32+, applies both CRD manifests through the apiextensions client, waits for Established, then creates representative cluster-scoped `NSXNetworkCloud` and `NSXGroup` objects through the dynamic client.
   - [x] GREEN: add static CRD manifests and test support needed for them to install.

5. [x] Status subresource behavior.
   - [x] RED: after creating objects through the dynamic client, update `/status` with conditions and prove spec remains unchanged.
   - [x] GREEN: ensure CRDs define `subresources.status` and conditions schemas only.

6. [x] Selectable field behavior.
   - [x] RED: create multiple clouds/groups and list with field selectors for:
     - `spec.networkCloudFQDN` on both CRDs,
     - `spec.networkCloudId` on `NSXNetworkCloud`,
     - `spec.groupID` and `spec.mode` on `NSXGroup`.
     Assert only matching objects return from the real API server.
   - [x] GREEN: add CRD `selectableFields` exactly as designed.

7. [x] Schema rejection behavior.
   - [x] RED: attempt to create invalid objects against the real API server:
     - `NSXGroup.spec.mode: Invalid`
     - `NSXGroup.spec.segment_path` with a non-string value
     - `NSXGroup` missing required `cidrs`
     - `NSXNetworkCloud` missing required `networkCloudFQDN`
     Assert the API server rejects each request.
   - [x] GREEN: tighten CRD schemas.

8. [x] Boundary/refactor pass after green.
   - [x] Run focused package tests after each refactor.
   - [x] Remove duplicate helper types if they appear.
   - [x] Keep all returned errors checked; do not use `_ :=` for errors.

### Test Harness And Makefile Plan

- [x] Add Makefile support so `make test`, `make check`, and `make test-coverage` provision and use Kubernetes 1.32+ envtest assets instead of relying on developer-local state.
- [x] Prefer `setup-envtest` from the controller-runtime project for the test API server assets.
- [x] Keep the API-server integration tests unskipped. If assets cannot be provisioned, tests must fail with a useful error instead of silently skipping.
- [x] Keep the real API server verification in normal `make test` and `make test-coverage`, because the task acceptance requires CRD install and selectable field proof.

### Verification

Run focused commands during implementation:

```bash
go test ./api/v1alpha -count=1 -v
go test ./api/v1alpha -count=1 -run 'TestCRDsInstall|TestSelectableFields|TestStatusSubresource' -v
go test -cover ./api/v1alpha -count=1
```

Then run all required checks:

```bash
make check
make test
make test-coverage
```

Coverage requirement:

- `api/v1alpha` coverage must be 80%+.
- Overall `make test-coverage` must also report 80%+ for all packages.

Manual evidence to record in the task file before setting `<passes>true</passes>`:

- Focused test output proving scheme registration, deepcopy independence, JSON shape, CRD install, object creation, status subresource behavior, selectable field selectors, and schema rejection behavior.
- Exact command output proving the API server is Kubernetes v1.32+ or newer.
- Full `make check`, `make test`, and `make test-coverage` output.
- Note that `domainId` is absent from CRD specs and API Go specs after final boundary review.

### Completion Steps

- Mark TDD checklist items complete in this plan as they are implemented.
- If implementation proves the public interface, CRD schema, test harness, or type design is wrong, replace the final marker with `TO BE VERIFIED` and quit immediately.
- If design remains valid and all checks pass:
  - update the task file with concrete verification evidence,
  - set `<passes>true</passes>`,
  - run `/bin/bash .ralph/task_switch.sh`,
  - add all files,
  - commit with `task finished 01-task-add-api-types-and-crd-manifests: add api types and CRD manifests`,
  - include summary and test evidence in the commit message,
  - push,
  - quit immediately.

Plan path: `.ralph/tasks/06-story-api-types-crds/01-task-add-api-types-and-crd-manifests_plans/2026-05-19-api-types-crd-manifests-plan.md`

NOW EXECUTE

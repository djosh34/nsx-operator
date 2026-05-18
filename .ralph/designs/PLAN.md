Grounding used: uploaded NSX-T 4.2.3 OpenAPI, especially group list/patch APIs, group `state`, paginated list-result `cursor`, `IPAddressExpression`, `PathExpression`, and `marked_for_delete` behavior. The uploaded OpenAPI defines NSX API version `4.2.3`, group list under `/policy/api/v1/infra/domains/{domain-id}/groups`, `GroupListResult.cursor`, group realization state `IN_PROGRESS | SUCCESS | FAILURE`, IP elements as address/range/subnet strings, and PATCH endpoints for group, IP-address expression, and path expression.     Kubernetes CRDs use `openAPIV3Schema`; Kubernetes documents CRD `selectableFields` as stable in v1.32 and shows `.spec.versions[*].selectableFields` with JSON paths. ([Kubernetes][1]) Kubernetes also documents `/status` subresources, status/spec separation, and generation behavior for status-only updates. ([Kubernetes][1])

# NSX Operator specification

## 1. Project gist

Build one Go operator that connects one Kubernetes API server to multiple NSX-T managers / network clouds.

The operator continuously compares:

```text
desired/observed state in Kubernetes CRDs
against
actual NSX Policy Group state in NSX-T managers
```

The operator manages only NSX Policy Groups under NSX domain:

```text
default
```

The NSX domain ID is a hard-coded constant. It is not exposed in CRDs, config, status, or user-facing APIs.

The operator has two core modes per `NSXGroup`:

```text
Observe:
  Kubernetes mirrors NSX.
  Operator never mutates NSX for this group.
  Remote NSX deletion deletes the Kubernetes CR.

Manage:
  Kubernetes spec is authoritative.
  Operator mutates NSX to match the CR spec.
  Operator never rewrites the CR spec from remote NSX.
  Remote NSX deletion or drift is repaired from Kubernetes.
```

Main topology:

```text
                       ┌────────────────────┐
                       │   Kubernetes API   │
                       │                    │
                       │  NSXNetworkClouds  │
                       │      NSXGroups     │
                       └─────────▲──────────┘
                                 │
                                 │ client-go + controller-runtime
                                 │
                       ┌─────────┴──────────┐
                       │    nsx-operator    │
                       │                    │
                       │ config/logging     │
                       │ kube typed client  │
                       │ nsx typed client   │
                       │ rate-limited HTTP  │
                       │ state loop         │
                       │ reconcile hooks    │
                       └───────▲─────▲──────┘
                               │     │
                 HTTPS JSON API│     │HTTPS JSON API
                               │     │
              ┌────────────────┘     └────────────────┐
              │                                       │
     ┌────────┴────────┐                     ┌────────┴────────┐
     │ NSX manager A   │                     │ NSX manager B   │
     │ fqdn-a:443      │                     │ fqdn-b:443      │
     │ domain default  │                     │ domain default  │
     └─────────────────┘                     └─────────────────┘
```

Inner architecture:

```text
main
 └─ parse config once
 └─ validate config once
 └─ build zap logger
 └─ build shared rate-limited HTTP transport
 └─ build Kubernetes typed CRD client
 └─ build NSX client factory
 └─ start controller-runtime manager
     ├─ registers NSXNetworkCloud controller
     ├─ registers NSXGroup controller
     └─ runs NSXStateOperator.Start(ctx)
          └─ periodic global sweep
              └─ one goroutine per NSXNetworkCloud
                  ├─ gather all info
                  ├─ process all info, pure function
                  └─ apply planned Kubernetes/NSX changes
```

---

## 2. Hard constants and naming

```go
const APIGroup = "nsx.ing.com"
const APIVersion = "v1alpha"
const NSXDomainID = "default"
const Finalizer = "nsx.ing.com/finalizer"
```

CRDs:

```text
nsxnetworkclouds.nsx.ing.com
nsxgroups.nsx.ing.com
```

Kinds:

```text
NSXNetworkCloud
NSXGroup
```

Scope:

```text
Cluster
```

Logical identity for `NSXGroup`:

```text
<networkCloudFQDN>/<groupID>
```

Examples:

```text
nsx-a.example.net/app-foo
nsx-a.example.net/group-123
nsx-a.example.net:8443/app-foo
```

Kubernetes `metadata.name` cannot be a URL path containing `/`, so implementation must use a readable non-hash deterministic name derived from the logical identity.

Required name function:

```go
func NSXGroupName(networkCloudFQDN, groupID string) string
```

Rules:

```text
input logical identity: nsx-a.example.net/app-foo
metadata.name:          nsx-a.example.net--app-foo

input logical identity: nsx-a.example.net:8443/app-foo
metadata.name:          nsx-a.example.net-8443--app-foo

no SHA/hash IDs
no random suffix
stable across restarts
same input always same output
output must be valid Kubernetes DNS-subdomain name
spec.networkCloudFQDN and spec.groupID remain source of truth
```

---

## 3. Core behavior summary

### Observe mode

```text
Remote NSX group exists, CR missing:
  create NSXGroup CR
  mode = Observe
  spec = remote projection
  status.conditions = current observed state

Remote NSX group exists, CR exists, mode Observe:
  replace CR spec from remote projection
  keep mode = Observe
  update status.conditions

Remote NSX group absent, CR exists, mode Observe:
  delete Kubernetes CR

User deletes Observe CR:
  remove finalizer
  do not call NSX DELETE
  if remote still exists, next successful sweep recreates Observe CR
```

### Manage mode

```text
CR exists, mode Manage:
  CR spec is authoritative

Remote NSX group missing:
  set RemotePresent=False
  set Synced=False
  apply managed create/patch to NSX

Remote NSX group differs:
  set SpecMatchesRemote=False
  set Synced=False
  apply managed patch to NSX

Remote NSX group matches and NSX state SUCCESS:
  set RemotePresent=True
  set SpecMatchesRemote=True
  set Realized=True
  set Synced=True

Remote NSX group matches and NSX state IN_PROGRESS:
  set Realized=Unknown
  set Synced=False

Remote NSX group matches and NSX state FAILURE:
  set Realized=False
  set Synced=False
```

### Manager failure isolation

```text
NSXNetworkCloud A unreachable:
  update only A status conditions
  do not mass-mark all A child groups false/missing unless A gather succeeded
  do not block B/C/D manager goroutines

NSXNetworkCloud B reachable:
  B sweep proceeds normally
```

---

## 4. CRD manifests

These CRDs are part of the deliverable. They contain `openAPIV3Schema`, status subresources, and `selectableFields`.

### 4.1 NSXNetworkCloud CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: nsxnetworkclouds.nsx.ing.com
spec:
  group: nsx.ing.com
  scope: Cluster
  names:
    plural: nsxnetworkclouds
    singular: nsxnetworkcloud
    kind: NSXNetworkCloud
    listKind: NSXNetworkCloudList
    shortNames:
      - nsxnc
  versions:
    - name: v1alpha
      served: true
      storage: true
      selectableFields:
        - jsonPath: .spec.networkCloudFQDN
        - jsonPath: .spec.networkCloudId
      additionalPrinterColumns:
        - name: FQDN
          type: string
          jsonPath: .spec.networkCloudFQDN
        - name: CloudID
          type: string
          jsonPath: .spec.networkCloudId
        - name: DisplayName
          type: string
          jsonPath: .spec.name
        - name: Reachable
          type: string
          jsonPath: .status.conditions[?(@.type=="Reachable")].status
        - name: Swept
          type: string
          jsonPath: .status.conditions[?(@.type=="Swept")].status
      subresources:
        status: {}
      schema:
        openAPIV3Schema:
          type: object
          description: NSXNetworkCloud represents one NSX manager/network cloud swept by the operator.
          required:
            - spec
          properties:
            apiVersion:
              type: string
            kind:
              type: string
            metadata:
              type: object
            spec:
              type: object
              required:
                - networkCloudFQDN
                - networkCloudId
                - name
              properties:
                networkCloudFQDN:
                  type: string
                  description: DNS name or DNS name with port of the NSX manager/network cloud endpoint. Used for identity and HTTPS base URL.
                networkCloudId:
                  type: string
                  description: Stable network-cloud identifier from the surrounding platform.
                name:
                  type: string
                  description: Human-readable display name.
            status:
              type: object
              properties:
                conditions:
                  type: array
                  description: Kubernetes-style conditions. No status fields other than conditions are used.
                  x-kubernetes-list-type: map
                  x-kubernetes-list-map-keys:
                    - type
                  items:
                    type: object
                    required:
                      - type
                      - status
                      - lastTransitionTime
                      - reason
                      - message
                    properties:
                      type:
                        type: string
                        enum:
                          - Reachable
                          - Swept
                      status:
                        type: string
                        enum:
                          - "True"
                          - "False"
                          - Unknown
                      observedGeneration:
                        type: integer
                        format: int64
                        minimum: 0
                      lastTransitionTime:
                        type: string
                        format: date-time
                      reason:
                        type: string
                      message:
                        type: string
```

### 4.2 NSXGroup CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: nsxgroups.nsx.ing.com
spec:
  group: nsx.ing.com
  scope: Cluster
  names:
    plural: nsxgroups
    singular: nsxgroup
    kind: NSXGroup
    listKind: NSXGroupList
    shortNames:
      - nsxg
  versions:
    - name: v1alpha
      served: true
      storage: true
      selectableFields:
        - jsonPath: .spec.networkCloudFQDN
        - jsonPath: .spec.groupID
        - jsonPath: .spec.mode
      additionalPrinterColumns:
        - name: FQDN
          type: string
          jsonPath: .spec.networkCloudFQDN
        - name: GroupID
          type: string
          jsonPath: .spec.groupID
        - name: Mode
          type: string
          jsonPath: .spec.mode
        - name: Synced
          type: string
          jsonPath: .status.conditions[?(@.type=="Synced")].status
        - name: Realized
          type: string
          jsonPath: .status.conditions[?(@.type=="Realized")].status
        - name: Unsupported
          type: string
          jsonPath: .status.conditions[?(@.type=="UnsupportedExpression")].status
      subresources:
        status: {}
      schema:
        openAPIV3Schema:
          type: object
          description: NSXGroup represents one NSX Policy Group in NSX domain default.
          required:
            - spec
          properties:
            apiVersion:
              type: string
            kind:
              type: string
            metadata:
              type: object
            spec:
              type: object
              required:
                - networkCloudFQDN
                - groupID
                - display_name
                - mode
                - cidrs
              properties:
                networkCloudFQDN:
                  type: string
                  description: NSXNetworkCloud FQDN. This plus groupID is the NSXGroup logical identity.
                groupID:
                  type: string
                  description: NSX Policy Group id under domain default.
                display_name:
                  type: string
                  description: NSX Group display_name. This is not derived from groupID.
                mode:
                  type: string
                  enum:
                    - Observe
                    - Manage
                  description: Observe mirrors NSX into Kubernetes. Manage makes NSX match Kubernetes spec.
                cidrs:
                  type: array
                  description: Array of static IP membership strings. Each item maps one-to-one and unchanged to NSX IPAddressExpression.ip_addresses.
                  x-kubernetes-list-type: set
                  items:
                    type: string
                segment_path:
                  type: string
                  nullable: true
                  description: Nullable NSX segment policy path. When set, it maps to one NSX PathExpression with one segment path. Valid only together with non-empty cidrs.
            status:
              type: object
              properties:
                conditions:
                  type: array
                  description: Kubernetes-style conditions. Status is conditions only; no remote objects, fingerprints, revisions, or duplicate synced fields.
                  x-kubernetes-list-type: map
                  x-kubernetes-list-map-keys:
                    - type
                  items:
                    type: object
                    required:
                      - type
                      - status
                      - lastTransitionTime
                      - reason
                      - message
                    properties:
                      type:
                        type: string
                        enum:
                          - RemotePresent
                          - SpecMatchesRemote
                          - UnsupportedExpression
                          - Realized
                          - Synced
                          - Applying
                          - Deleting
                      status:
                        type: string
                        enum:
                          - "True"
                          - "False"
                          - Unknown
                      observedGeneration:
                        type: integer
                        format: int64
                        minimum: 0
                      lastTransitionTime:
                        type: string
                        format: date-time
                      reason:
                        type: string
                      message:
                        type: string
```

---

## 5. Example custom resources

### 5.1 NSXNetworkCloud CR

```yaml
apiVersion: nsx.ing.com/v1alpha
kind: NSXNetworkCloud
metadata:
  name: nsx-a.example.net
spec:
  networkCloudFQDN: nsx-a.example.net
  networkCloudId: nc-prod-a
  name: Production NSX A
status:
  conditions: []
```

With explicit port for test/mock/lab setup:

```yaml
apiVersion: nsx.ing.com/v1alpha
kind: NSXNetworkCloud
metadata:
  name: nsx-mock.local-8443
spec:
  networkCloudFQDN: nsx-mock.local:8443
  networkCloudId: nc-mock
  name: Mock NSX
status:
  conditions: []
```

### 5.2 Observe group with no expressions

This represents an NSX group whose expression list is empty.

```yaml
apiVersion: nsx.ing.com/v1alpha
kind: NSXGroup
metadata:
  name: nsx-a.example.net--empty-group
  finalizers:
    - nsx.ing.com/finalizer
spec:
  networkCloudFQDN: nsx-a.example.net
  groupID: empty-group
  display_name: Empty Group
  mode: Observe
  cidrs: []
  segment_path: null
status:
  conditions:
    - type: RemotePresent
      status: "True"
      reason: Observed
      message: Remote group exists in NSX.
      lastTransitionTime: "2026-05-18T10:00:00Z"
    - type: UnsupportedExpression
      status: "False"
      reason: Supported
      message: Remote expression shape is supported.
      lastTransitionTime: "2026-05-18T10:00:00Z"
```

### 5.3 Observe group with IP membership values

Each `cidrs` item maps one-to-one and unchanged to one NSX `IPAddressExpression.ip_addresses` item.

```yaml
apiVersion: nsx.ing.com/v1alpha
kind: NSXGroup
metadata:
  name: nsx-a.example.net--app-foo
  finalizers:
    - nsx.ing.com/finalizer
spec:
  networkCloudFQDN: nsx-a.example.net
  groupID: app-foo
  display_name: App Foo
  mode: Observe
  cidrs:
    - 10.10.0.0/24
    - 10.10.0.17
  segment_path: null
status:
  conditions: []
```

### 5.4 Manage group with IP membership values plus segment path

This maps to one NSX `IPAddressExpression`, an OR conjunction, and one NSX `PathExpression`.

```yaml
apiVersion: nsx.ing.com/v1alpha
kind: NSXGroup
metadata:
  name: nsx-a.example.net--app-foo
  finalizers:
    - nsx.ing.com/finalizer
spec:
  networkCloudFQDN: nsx-a.example.net
  groupID: app-foo
  display_name: App Foo
  mode: Manage
  cidrs:
    - 10.10.0.0/24
    - 10.10.0.17
  segment_path: /infra/segments/seg-app-foo
status:
  conditions: []
```

---

## 6. Static IP membership mapping

The field is named:

```yaml
spec.cidrs
```

Each list item is a static IP membership string.

```text
spec.cidrs[n] == IPAddressExpression.ip_addresses[n]
```

Rules:

```text
one item in spec.cidrs becomes one item in NSX IPAddressExpression.ip_addresses
one item in NSX IPAddressExpression.ip_addresses becomes one item in spec.cidrs
items are copied exactly
item order is preserved
```

Reason for the field name:

```text
cidrs is legacy/project vocabulary.
It means static IP membership strings.
```

---

## 7. NSX expression model

Only these NSX group expression shapes are supported by the CRD projection.

### Case 1 — no expression

NSX remote:

```json
{
  "id": "empty-group",
  "display_name": "Empty Group",
  "resource_type": "Group",
  "expression": []
}
```

CR spec:

```yaml
networkCloudFQDN: nsx-a.example.net
groupID: empty-group
display_name: Empty Group
mode: Observe
cidrs: []
segment_path: null
```

Condition result:

```text
UnsupportedExpression=False
```

### Case 2 — one IPAddressExpression

NSX remote:

```json
{
  "id": "app-foo",
  "display_name": "App Foo",
  "resource_type": "Group",
  "expression": [
    {
      "id": "expr-ip",
      "resource_type": "IPAddressExpression",
      "ip_addresses": [
        "10.10.0.0/24",
        "10.10.0.17"
      ]
    }
  ],
  "state": "SUCCESS"
}
```

CR spec:

```yaml
networkCloudFQDN: nsx-a.example.net
groupID: app-foo
display_name: App Foo
mode: Observe
cidrs:
  - 10.10.0.0/24
  - 10.10.0.17
segment_path: null
```

Condition result:

```text
UnsupportedExpression=False
Realized=True
```

### Case 3 — IPAddressExpression OR PathExpression

NSX remote:

```json
{
  "id": "app-foo",
  "display_name": "App Foo",
  "resource_type": "Group",
  "expression": [
    {
      "id": "expr-ip",
      "resource_type": "IPAddressExpression",
      "ip_addresses": [
        "10.10.0.0/24",
        "10.10.0.17"
      ]
    },
    {
      "resource_type": "ConjunctionOperator",
      "conjunction_operator": "OR"
    },
    {
      "id": "expr-segment",
      "resource_type": "PathExpression",
      "paths": [
        "/infra/segments/seg-app-foo"
      ]
    }
  ],
  "state": "SUCCESS"
}
```

CR spec:

```yaml
networkCloudFQDN: nsx-a.example.net
groupID: app-foo
display_name: App Foo
mode: Observe
cidrs:
  - 10.10.0.0/24
  - 10.10.0.17
segment_path: /infra/segments/seg-app-foo
```

Condition result:

```text
UnsupportedExpression=False
Realized=True
```

### Unsupported expression shape

Examples:

```text
multiple IPAddressExpression objects
PathExpression with multiple paths
PathExpression without IPAddressExpression
IPAddressExpression AND PathExpression
NestedExpression
Condition
MACAddressExpression
ExternalIDExpression
IdentityGroupExpression
mixed conjunctions
IPv6 values
range values
```

Behavior:

```text
UnsupportedExpression=True
Synced=False unless explicitly impossible to determine, then Synced=Unknown
Manage write still only patches/deletes the represented IPAddressExpression and represented PathExpression
other expressions are not altered
```

---

## 8. Projection functions

Package:

```text
internal/nsxgroup
```

Required API:

```go
func RemoteGroupToCRSpec(
    networkCloudFQDN string,
    group *nsx.Group,
) (*v1alpha.NSXGroupSpec, *ConversionReport)

func CRSpecToGroupPatch(
    spec *v1alpha.NSXGroupSpec,
) (*nsx.Group, error)

func CRSpecToIPAddressExpression(
    spec *v1alpha.NSXGroupSpec,
) (*nsx.IPAddressExpression, error)

func CRSpecToPathExpression(
    spec *v1alpha.NSXGroupSpec,
) (*nsx.PathExpression, error)

func CompareCRSpecToRemote(
    spec *v1alpha.NSXGroupSpec,
    remote *nsx.Group,
) (*GroupComparison, error)
```

Conversion report:

```go
type ConversionReport struct {
    UnsupportedExpression bool
    IPAddressExpressionID *string
    PathExpressionID      *string
    NSXGroupState         *string
}
```

Comparison result:

```go
type GroupComparison struct {
    RemotePresent         bool
    SpecMatchesRemote     bool
    UnsupportedExpression bool
    Realized              metav1.ConditionStatus
}
```

### Remote-to-CR rules

```text
spec.networkCloudFQDN = sweep context FQDN
spec.groupID = remote.id
spec.display_name = remote.display_name
spec.mode = Observe for remote-only groups
spec.cidrs = IPAddressExpression.ip_addresses exactly
spec.segment_path = single PathExpression path when supported
domain default is implicit and never written into spec
```

### CR-to-NSX group patch rules

```json
{
  "id": "<spec.groupID>",
  "display_name": "<spec.display_name>",
  "resource_type": "Group"
}
```

No tags are added.

No ownership markers are added.

The NSX manager must not know this object is represented by a Kubernetes CR.

### CR-to-IPAddressExpression rules

```json
{
  "resource_type": "IPAddressExpression",
  "ip_addresses": ["<each spec.cidrs value exactly>"]
}
```

### CR-to-PathExpression rules

```json
{
  "resource_type": "PathExpression",
  "paths": ["<spec.segment_path>"]
}
```

### Expression apply rules

```text
cidrs empty, segment_path null:
  desired represented shape = no IP expression and no segment path expression
  delete selected IPAddressExpression if present
  delete selected PathExpression if present
  leave unrelated/unsupported expressions untouched

cidrs non-empty, segment_path null:
  patch selected IPAddressExpression
  delete selected PathExpression if present
  leave unrelated/unsupported expressions untouched

cidrs non-empty, segment_path set:
  patch selected IPAddressExpression
  patch selected PathExpression
  desired relation = OR
  leave unrelated/unsupported expressions untouched
```

Expression IDs:

```text
Expression IDs are not CRD fields.
Expression IDs are resolved from the current remote NSX group during gather/reconcile reads.
When creating a new expression and no existing expression ID is available:
  IPAddressExpression expression-id = cidrs
  PathExpression expression-id = segment
```

---

## 9. Projection examples

### Example A — no remote expression

Remote NSX:

```json
{
  "id": "g-empty",
  "display_name": "G Empty",
  "resource_type": "Group",
  "expression": [],
  "state": "SUCCESS"
}
```

Generated CR:

```yaml
apiVersion: nsx.ing.com/v1alpha
kind: NSXGroup
metadata:
  name: nsx-a.example.net--g-empty
spec:
  networkCloudFQDN: nsx-a.example.net
  groupID: g-empty
  display_name: G Empty
  mode: Observe
  cidrs: []
  segment_path: null
status:
  conditions:
    - type: RemotePresent
      status: "True"
      reason: Observed
      message: Remote group exists.
      lastTransitionTime: "2026-05-18T10:00:00Z"
    - type: SpecMatchesRemote
      status: "True"
      reason: Compared
      message: Supported remote projection matches spec.
      lastTransitionTime: "2026-05-18T10:00:00Z"
    - type: UnsupportedExpression
      status: "False"
      reason: Supported
      message: Remote expression shape is supported.
      lastTransitionTime: "2026-05-18T10:00:00Z"
    - type: Realized
      status: "True"
      reason: NSXState
      message: NSX group state is SUCCESS.
      lastTransitionTime: "2026-05-18T10:00:00Z"
    - type: Synced
      status: "True"
      reason: Compared
      message: Remote is present, supported, matching, and realized.
      lastTransitionTime: "2026-05-18T10:00:00Z"
```

### Example B — IP membership values

Remote NSX:

```json
{
  "id": "g-web",
  "display_name": "Web",
  "resource_type": "Group",
  "expression": [
    {
      "id": "expr-100",
      "resource_type": "IPAddressExpression",
      "ip_addresses": [
        "10.20.0.0/24",
        "10.20.0.42"
      ]
    }
  ],
  "state": "SUCCESS"
}
```

Generated CR spec:

```yaml
networkCloudFQDN: nsx-a.example.net
groupID: g-web
display_name: Web
mode: Observe
cidrs:
  - 10.20.0.0/24
  - 10.20.0.42
segment_path: null
```

Managed write from this CR patches:

```http
PATCH /policy/api/v1/infra/domains/default/groups/g-web
PATCH /policy/api/v1/infra/domains/default/groups/g-web/ip-address-expressions/expr-100
```

IPAddressExpression body:

```json
{
  "resource_type": "IPAddressExpression",
  "ip_addresses": [
    "10.20.0.0/24",
    "10.20.0.42"
  ]
}
```

### Example C — IP membership values OR segment

Remote NSX:

```json
{
  "id": "g-web",
  "display_name": "Web",
  "resource_type": "Group",
  "expression": [
    {
      "id": "expr-100",
      "resource_type": "IPAddressExpression",
      "ip_addresses": [
        "10.20.0.0/24",
        "10.20.0.42"
      ]
    },
    {
      "resource_type": "ConjunctionOperator",
      "conjunction_operator": "OR"
    },
    {
      "id": "expr-200",
      "resource_type": "PathExpression",
      "paths": [
        "/infra/segments/seg-web"
      ]
    }
  ],
  "state": "SUCCESS"
}
```

Generated CR spec:

```yaml
networkCloudFQDN: nsx-a.example.net
groupID: g-web
display_name: Web
mode: Observe
cidrs:
  - 10.20.0.0/24
  - 10.20.0.42
segment_path: /infra/segments/seg-web
```

Managed write patches:

```http
PATCH /policy/api/v1/infra/domains/default/groups/g-web
PATCH /policy/api/v1/infra/domains/default/groups/g-web/ip-address-expressions/expr-100
PATCH /policy/api/v1/infra/domains/default/groups/g-web/path-expressions/expr-200
```

### Example D — unsupported expression

Remote NSX:

```json
{
  "id": "g-unsupported",
  "display_name": "Unsupported",
  "resource_type": "Group",
  "expression": [
    {
      "resource_type": "MACAddressExpression",
      "mac_addresses": ["00:50:56:aa:bb:cc"]
    }
  ],
  "state": "SUCCESS"
}
```

Generated CR spec:

```yaml
networkCloudFQDN: nsx-a.example.net
groupID: g-unsupported
display_name: Unsupported
mode: Observe
cidrs: []
segment_path: null
```

Status condition:

```yaml
- type: UnsupportedExpression
  status: "True"
  reason: Unsupported
  message: Remote expression shape cannot be represented by NSXGroup spec fields.
  lastTransitionTime: "2026-05-18T10:00:00Z"
```


[1]: https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/ "Extend the Kubernetes API with CustomResourceDefinitions | Kubernetes"

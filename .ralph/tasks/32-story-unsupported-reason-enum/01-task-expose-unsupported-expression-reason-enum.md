## Task: Expose Unsupported Expression Reason Enum in NSXGroup Status <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Make unsupported remote expression decisions visible and machine-readable in the `NSXGroup` CRD status. Today several code paths set a boolean `UnsupportedExpression` to true for vague internal reasons, and the CR only exposes the resulting `UnsupportedExpression` condition without a constrained reason taxonomy. Operators need the CR to show why a group is unsupported, using an enum of reasons rather than invisible bool assignments or free-form messages.

CRD/API changes required:
- Add a status field to `NSXGroupStatus`: `UnsupportedReason string `json:"unsupportedReason,omitempty"``.
- Add a Go enum type in `api/v1alpha`, for example `type UnsupportedExpressionReason string`, and constants for every supported reason.
- The CRD schema for `NSXGroup.status` must add `unsupportedReason` as a string enum. It must be omitted/empty when `UnsupportedExpression=False`.
- Keep `status.conditions` unchanged and still include the existing `UnsupportedExpression` condition type.
- Update `UnsupportedExpression` condition `reason` to use the same enum value when status is `True`, so humans can see the reason in standard condition tooling too.
- Add an additional printer column named `UnsupportedReason` with JSONPath `.status.unsupportedReason`.

Minimum enum values required:
- `UnsupportedExpressionType`: a remote expression has an unsupported `resource_type` or cannot be decoded into a known expression shape.
- `MultipleIPAddressExpressions`: more than one NSX `IPAddressExpression` exists for the group.
- `MultiplePathExpressions`: more than one NSX `PathExpression` exists for the group.
- `InvalidIPAddressExpression`: the NSX `IPAddressExpression` is malformed, undecodable, or missing fields needed by the operator.
- `InvalidPathExpression`: the NSX `PathExpression` is malformed, undecodable, or missing fields needed by the operator.
- `UnsupportedIPAddressExpressionFields`: the NSX `IPAddressExpression` uses fields outside the operator-supported one-to-one CIDR mapping.
- `UnsupportedPathExpressionFields`: the NSX `PathExpression` uses fields outside the operator-supported one-to-one segment path mapping.
- `UnsupportedNestedExpression`: the remote expression contains nested/conjunction/exclusion/operator structure that cannot be represented by the CRD spec.

Implementation requirements:
- Replace internal `UnsupportedExpression bool`-only state with a structured result that carries both unsupported status and `UnsupportedExpressionReason`.
- Every place that currently sets unsupported to true must choose exactly one enum reason. Do not leave any vague `true` assignment without a reason.
- Preserve representable fields when possible: for unsupported remote expressions, keep any CIDRs and segment paths that are safely representable, but also set `status.unsupportedReason` and `UnsupportedExpression=True`.
- When the remote expression is fully supported, `status.unsupportedReason` must be empty/omitted, `UnsupportedExpression=False`, and the condition reason should be a supported/success reason such as `SupportedExpression`.
- `Synced` must remain false when `UnsupportedExpression=True`, and its reason/message must reference the same unsupported enum reason or clearly include it.
- All logs for unsupported detection must use zap structured fields including `networkCloudFQDN`, `groupID`, and `unsupportedReason`.

In scope:
- API structs, deepcopy, CRD schema/status subresource schema, printer columns, status condition builders, manager pipeline projection/status logic, tests covering every enum reason, and sample/evidence updates as needed.

Out of scope:
- Expanding the CRD spec to support nested NSX expression trees.
- Removing the `UnsupportedExpression` condition.
- Changing unrelated condition types or NSX write-disable behavior.


</description>


<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] The served `NSXGroup` CRD exposes `status.unsupportedReason` as an optional string enum and adds an `UnsupportedReason` printer column.
- [ ] `api/v1alpha.NSXGroupStatus` contains `UnsupportedReason UnsupportedExpressionReason `json:"unsupportedReason,omitempty"`` or an equivalent strongly typed enum field.
- [ ] Every current unsupported decision path records one of the enum reasons; no unsupported path remains a bare bool assignment with no reason.
- [ ] `UnsupportedExpression=True` conditions use the enum value as the condition reason, and `status.unsupportedReason` contains the same value.
- [ ] Supported/representable remote expressions leave `status.unsupportedReason` empty and set `UnsupportedExpression=False`.
- [ ] Unit tests cover each enum value and prove condition/status consistency.
- [ ] Full relevant tests are run and recorded, including status condition tests, manager pipeline tests, API/CRD integration tests, and NSX-facing verification against `../nsx-t-mockapi` or equivalent testcontainers evidence where the remote expression shape matters.
</acceptance_criteria>

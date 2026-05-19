# Plan: Typed Unsupported Expression Reason Status

Task: `.ralph/tasks/32-story-unsupported-reason-enum/01-task-expose-unsupported-expression-reason-enum.md`

## Current Shape

- `api/v1alpha.NSXGroupStatus` only exposes `Conditions []metav1.Condition`.
- `crds/nsx.ing.com_nsxgroups.yaml` status schema only exposes `conditions`; printer columns include `Unsupported` but not `UnsupportedReason`.
- `internal/stateoperator.RemoteGroup` carries `UnsupportedExpression bool`, losing the cause at the projection boundary.
- `RemoteGroupFromNSXGroup` has several bare `remote.UnsupportedExpression = true` assignments:
  - non-empty `extended_expression`
  - malformed raw expression header
  - duplicate `IPAddressExpression`
  - undecodable `IPAddressExpression`
  - duplicate `PathExpression`
  - undecodable `PathExpression`
  - undecodable `ConjunctionOperator`
  - non-OR conjunction operator
  - unknown `resource_type`
- `unsupportedExpressionCondition` can only emit reason `"UnsupportedExpression"`.
- `syncedRemoteStatus` can only emit reason `"UnsupportedExpression"` for unsupported remote expressions.

## Boundary Design

Use `$improve-code-boundaries`: replace the bool-only boundary with a small typed result carried from NSX projection through status.

1. Add API enum in `api/v1alpha/types.go`:
   - `type UnsupportedExpressionReason string`
   - constants:
     - `UnsupportedExpressionReasonSupportedExpression = "SupportedExpression"` for condition success reason only, not a status enum value.
     - `UnsupportedExpressionReasonUnsupportedExpressionType = "UnsupportedExpressionType"`
     - `UnsupportedExpressionReasonMultipleIPAddressExpressions = "MultipleIPAddressExpressions"`
     - `UnsupportedExpressionReasonMultiplePathExpressions = "MultiplePathExpressions"`
     - `UnsupportedExpressionReasonInvalidIPAddressExpression = "InvalidIPAddressExpression"`
     - `UnsupportedExpressionReasonInvalidPathExpression = "InvalidPathExpression"`
     - `UnsupportedExpressionReasonUnsupportedIPAddressExpressionFields = "UnsupportedIPAddressExpressionFields"`
     - `UnsupportedExpressionReasonUnsupportedPathExpressionFields = "UnsupportedPathExpressionFields"`
     - `UnsupportedExpressionReasonUnsupportedNestedExpression = "UnsupportedNestedExpression"`
2. Add `UnsupportedReason UnsupportedExpressionReason `json:"unsupportedReason,omitempty"` to `NSXGroupStatus`.
3. Replace `RemoteGroup.UnsupportedExpression bool` with `UnsupportedReason nsxv1alpha.UnsupportedExpressionReason`.
   - Add `func (r RemoteGroup) HasUnsupportedExpression() bool` or equivalent small method if it improves readability.
   - This removes the weak bool as source of truth and makes unsupported state derived from the enum.
4. In `RemoteGroupFromNSXGroup`, use one local helper like `remote.markUnsupported(reason)` so every unsupported path chooses exactly one reason and the first cause wins deterministically while representable fields continue to be preserved.
5. Map reasons as follows:
   - non-empty `ExtendedExpression` -> `UnsupportedNestedExpression`
   - malformed raw expression header or missing/unknown `resource_type` -> `UnsupportedExpressionType`
   - duplicate `IPAddressExpression` -> `MultipleIPAddressExpressions`
   - undecodable `IPAddressExpression` or missing usable `ip_addresses` field -> `InvalidIPAddressExpression`
   - duplicate `PathExpression` -> `MultiplePathExpressions`
   - undecodable `PathExpression` or missing usable `paths` field -> `InvalidPathExpression`
   - `IPAddressExpression` containing unsupported fields outside the one-to-one CIDR mapping -> `UnsupportedIPAddressExpressionFields`
   - `PathExpression` containing unsupported fields outside the one-to-one segment path mapping -> `UnsupportedPathExpressionFields`
   - non-OR/invalid conjunction/operator/nested expression shapes -> `UnsupportedNestedExpression`
6. Add helper validators close to projection code:
   - decode expression first into `map[string]json.RawMessage` or a small envelope to distinguish missing fields from empty arrays.
   - Allow normal NSX metadata inherited from `nsxclient.Resource`.
   - For `IPAddressExpression`, allow `resource_type`, id/resource metadata, and `ip_addresses`; reject fields like `mac_addresses`, `external_ids`, nested structures, or other unsupported expression fields with `UnsupportedIPAddressExpressionFields`.
   - For `PathExpression`, allow `resource_type`, id/resource metadata, and `paths`; reject unsupported fields with `UnsupportedPathExpressionFields`.
7. Extend `statuscondition.BuildGroupStatus` without duplicating status structs:
   - either add `unsupportedReason nsxv1alpha.UnsupportedExpressionReason` as a named parameter, or add an option/result struct if it keeps the call sites clearer.
   - It should return `NSXGroupStatus{Conditions: conditions, UnsupportedReason: unsupportedReason}`.
   - When the unsupported condition is false, pass empty unsupported reason.
8. Update status builders in `manager_pipeline.go`:
   - `unsupportedExpressionCondition(remote)` returns `True`, enum string, message including enum when unsupported; otherwise `False`, `SupportedExpression`, supported message.
   - `Synced` reason/message for unsupported remote expressions use/include the same enum value.
   - `matchingManageStatus`, `missingManageStatus`, `deletedManageStatus` pass empty unsupported reason.
9. Add zap debug logs for unsupported detection in the sweep path:
   - Log after projection or after snapshot gather for each unsupported remote group.
   - Include `networkCloudFQDN`, `groupID`, and `unsupportedReason`.
   - Use existing `logging.NetworkCloudFQDN`, `logging.GroupID`, and `zap.String("unsupportedReason", string(reason))`.

## CRD/API Work

1. Update `api/v1alpha/deepcopy.go` only if the new enum field requires manual handling. Since it is a string alias, `*out = *in` is sufficient.
2. Update `crds/nsx.ing.com_nsxgroups.yaml`:
   - Add printer column:
     - `name: UnsupportedReason`
     - `type: string`
     - `jsonPath: .status.unsupportedReason`
   - Add `status.properties.unsupportedReason` as string enum with only unsupported reason values:
     - `UnsupportedExpressionType`
     - `MultipleIPAddressExpressions`
     - `MultiplePathExpressions`
     - `InvalidIPAddressExpression`
     - `InvalidPathExpression`
     - `UnsupportedIPAddressExpressionFields`
     - `UnsupportedPathExpressionFields`
     - `UnsupportedNestedExpression`
   - Do not include `SupportedExpression` in the status enum because the field must be omitted/empty when supported.

## TDD Execution

Use `$tdd`: execute in vertical red-green slices, one behavior at a time.

1. RED: API JSON behavior.
   - Add/extend `api/v1alpha/types_test.go` so `NSXGroupStatus.UnsupportedReason` marshals to `status.unsupportedReason` when set and is omitted when empty.
   - GREEN: add enum type/constants/status field.
2. RED: CRD schema behavior.
   - Replace `requireStatusSchemaConditionsOnly` with a helper that still checks network cloud status is conditions-only but checks NSXGroup status exposes exactly `conditions` and `unsupportedReason`.
   - Add assertions for enum values and `UnsupportedReason` printer column.
   - GREEN: update CRD YAML.
3. RED: projection reason behavior.
   - Replace/extend `TestRemoteGroupFromNSXGroupFlagsUnsupportedAndPreservesRepresentableFields` into focused subtests covering each enum reason through public `RemoteGroupFromNSXGroup` output.
   - Keep one test proving supported empty/IP/IP-or-path expression yields no unsupported reason and preserves represented fields.
   - GREEN: implement typed projection and validators.
4. RED: status consistency behavior.
   - Update `TestProcessManagerSnapshotRemoteOnlyUnsupportedExpressionMarksUnsynced` and add table coverage proving:
     - `status.unsupportedReason` equals the enum.
     - `UnsupportedExpression=True` condition reason equals the enum.
     - `Synced=False` reason/message references the enum.
     - supported expressions leave `UnsupportedReason` empty and `UnsupportedExpression=False`.
   - GREEN: update status builders and call sites.
5. RED: logs behavior.
   - Add a pipeline/sweep test with zap observer or buffer core that feeds one unsupported remote group and asserts a debug log record has `networkCloudFQDN`, `groupID`, and `unsupportedReason`.
   - GREEN: add structured log call in the sweep path.
6. Refactor only after green:
   - Remove remaining `UnsupportedExpression` bool assignments and searches for `UnsupportedExpression: true`.
   - Prefer a single projection helper over duplicated `reason != ""` checks.
   - Keep condition strings close to `api/v1alpha` enum constants to avoid string drift.

## Verification Commands

Run the focused loop during implementation:

```bash
go test ./api/v1alpha -run 'Test(JSONShapeUsesPublicAPIFieldNames|CRDsInstallStatusSubresourceSelectableFieldsAndSchema)' -count=1
go test ./internal/stateoperator -run 'Test(RemoteGroupFromNSXGroup|ProcessManagerSnapshotRemoteOnlyUnsupportedExpressionMarksUnsynced|DefaultManagerSweep)' -count=1
go test ./internal/statuscondition -count=1
```

Final required commands, all must pass:

```bash
make check
make test
make test-coverage
```

If `make check` already includes `test` and `test-coverage`, still run the explicit `make test` and `make test-coverage` again because the task requires them separately.

## Completion Updates

After all checks pass:

1. Record concrete verification evidence in the task file.
2. Set `<passes>true</passes>` in the task file.
3. Run `/bin/bash .ralph/task_switch.sh`.
4. Stage all files, including `.ralph` updates.
5. Commit with `task finished 01-task-expose-unsupported-expression-reason-enum: expose unsupported expression reason enum`, with evidence and challenges in the message.
6. Push.

NOW EXECUTE

## Task: Remove Observe Finalizers And Autodelete Missing Observe Groups <status>completed</status> <passes>true</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Change NSXGroup lifecycle handling so any `NSXGroup` in `Observe` mode has no Kubernetes finalizer at all, while preserving correct cleanup when the observed NSX object disappears outside this operator.

Today finalizers are finicky for observed resources. Observe-mode resources are observational only and must not be held in Kubernetes deletion by `nsx.ing.com/finalizer` or any replacement finalizer. The operator must remove the finalizer from existing Observe-mode `NSXGroup` objects during reconciliation if one is already present, must not add it to newly observed groups, and must not require a finalizer to clean up Observe-mode CRs.

The operator must still work when the backing NSX group is deleted without this operator. If an `NSXGroup` CR is in `Observe` mode and the corresponding NSX group is absent from NSX-T during a verified manager sweep or reconcile path, the operator must automatically delete the Kubernetes CR from the kube-api. This deletion must not attempt to delete anything from NSX, and it must not depend on a finalizer being present.

In scope: finalizer rules for Observe-mode `NSXGroup`; migration behavior for existing Observe CRs that already have `nsx.ing.com/finalizer`; auto-deleting Observe CRs when the backing NSX group is missing from NSX-T; focused unit/integration/e2e tests using envtest and `../nsx-t-mockapi` where appropriate; structured zap logging for finalizer removal and Observe auto-delete decisions.

Out of scope: changing Manage-mode finalizer semantics except where needed to keep existing tests passing; adding finalizers to `NSXNetworkCloud`; deleting NSX objects for Observe-mode resources; broad redesign of reconciliation unrelated to Observe lifecycle behavior.

</description>

<acceptance_criteria>
- [x] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [x] The verification evidence is recorded in the task or linked artifact.
- [x] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [x] A newly created or reconciled Observe-mode `NSXGroup` never receives `nsx.ing.com/finalizer` or any other lifecycle-blocking finalizer.
- [x] An existing Observe-mode `NSXGroup` that already has `nsx.ing.com/finalizer` has that finalizer removed by reconciliation without deleting the backing NSX group.
- [x] Deleting an Observe-mode `NSXGroup` from Kubernetes completes without the operator using a finalizer and without deleting the backing NSX group from NSX-T.
- [x] If the backing NSX group is deleted outside this operator and the Kubernetes `NSXGroup` is in `Observe` mode, the operator automatically deletes the Kubernetes CR from the kube-api after it verifies the NSX group is absent.
- [x] The Observe missing-from-NSX auto-delete behavior is tested against `../nsx-t-mockapi` using envtest or testcontainers-backed integration coverage, not only mocks.
- [x] Existing Manage-mode finalizer/delete behavior remains covered and passing.
- [x] `make check`, `make test`, and `make test-coverage` pass, with no skipped tests.
</acceptance_criteria>

<plan>
.ralph/tasks/13-story-object-lifecycle/02-task-remove-observe-finalizers-and-autodelete-missing-observe-groups_plans/2026-05-19-observe-finalizer-removal-plan.md
</plan>

NOW EXECUTE

<verification_evidence>
Implementation evidence:
- `GroupReconciler` now removes `nsx.ing.com/finalizer` from Observe-mode `NSXGroup` objects on both deleting and non-deleting reconcile paths, preserves unrelated finalizers, and does not construct an NSX manager client for Observe reconcile/delete.
- Manager sweep imports and repairs Observe-mode groups with no finalizers. Legacy Observe finalizer cleanup is represented explicitly as `ManagerPlan.ObserveFinalizerRemovals` and applied before `ObserveDeletes`, so legacy finalizers cannot block missing-remote CR auto-delete.
- Structured zap logging now records Observe finalizer removal fields in reconcile (`component`, `reconcileKey`, `groupName`, `networkCloudFQDN`, `groupID`, `mode`) and manager sweep plan summary (`observeFinalizerRemovalCount`, `observeFinalizerRemovalNames`, `observeDeleteCount`, `observeDeleteNames`).
- Boundary review: Observe/Manage lifecycle decisions stayed in `internal/stateoperator`; `internal/kubeapi` remains a mode-agnostic typed transport; no CRD schema, cloud finalizer, owner reference, startup, or NSX client boundary changes were introduced.

Focused red/green and integration commands run:
- `go test ./internal/stateoperator -run TestGroupReconcileObserveDoesNotMutateNSXOrRequeue -count=1 -v` first failed with `finalizers = [nsx.ing.com/finalizer], want no "nsx.ing.com/finalizer"`, then passed after removing the non-deleting Observe finalizer add.
- `go test ./internal/stateoperator -run TestGroupReconcileObserveRemovesLegacyFinalizerWithoutNSXMutation -count=1 -v` first failed with `finalizers = [nsx.ing.com/finalizer example.test/keep], want no "nsx.ing.com/finalizer"`, then passed after adding Observe finalizer removal.
- `go test ./internal/stateoperator -run TestProcessManagerSnapshotImportsRemoteOnlyGroupsAsObserveUpserts -count=1 -v` first failed with `Observe upsert finalizers = [nsx.ing.com/finalizer], want none`, then passed after `observeGroupFromRemote` stopped setting finalizers.
- `go test ./internal/stateoperator -run TestApplyManagerPlanRunsOperationsInExactOrder -count=1 -v` first failed because `remove-finalizer:observe-missing:nsx.ing.com/finalizer` was missing before `delete-group-cr:observe-missing`, then passed after applying `ObserveFinalizerRemovals`.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run 'TestDefaultManagerSweepAppliesObserveUpsertStatusAndDeleteThroughTypedKubeAPI|TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI|TestLifecycleObserveMissingRemoteDeletesCRAgainstMockAPIWithoutNSXDelete' -count=1 -v` passed. This verified typed kube-api imports/repairs have no Observe finalizer, Observe Kubernetes delete leaves the backing mockapi group present, Manage delete still removes the backing mockapi group and keeps the finalizer until sweep confirmation, and a mockapi backing group deleted outside the operator causes the Observe CR to be auto-deleted without any operator NSX delete call.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -run 'TestDefaultManagerSweepAppliesObserveUpsertStatusAndDeleteThroughTypedKubeAPI|TestLifecycleObserveMissingRemoteDeletesCRAgainstMockAPIWithoutNSXDelete' -count=1 -v` passed after the final telemetry change. The debug log evidence included `observeFinalizerRemovalCount:2`, `observeFinalizerRemovalNames:["observe-drifted","observe-stale"]`, `observeDeleteCount:1`, and `observeDeleteNames:["observe-stale"]`.
- `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./internal/stateoperator -count=1` passed in 42.008s before the telemetry change.

Required final checks after all code changes:
- `make check` passed. It ran `gofumpt -w .`, `go vet ./...`, `golangci-lint run ./...` with `0 issues`, `go test ./...`, `go test -race ./...`, mockapi contract tests, lifecycle mockapi test, selected envtest packages, largechaos tests, and coverage. Final coverage output included `internal/stateoperator 80.4%` and `coverage 83.2% meets 80.0% threshold`.
- `make test` passed: `KUBEBUILDER_ASSETS="$(.bin/setup-envtest use 1.32.x -p path)" go test ./...` returned `ok` for all packages.
- `make test-coverage` passed: package coverage included `internal/stateoperator 80.4%` and total `coverage 83.2% meets 80.0% threshold`.
</verification_evidence>

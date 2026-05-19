## Task: Remove Observe Finalizers And Autodelete Missing Observe Groups <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Change NSXGroup lifecycle handling so any `NSXGroup` in `Observe` mode has no Kubernetes finalizer at all, while preserving correct cleanup when the observed NSX object disappears outside this operator.

Today finalizers are finicky for observed resources. Observe-mode resources are observational only and must not be held in Kubernetes deletion by `nsx.ing.com/finalizer` or any replacement finalizer. The operator must remove the finalizer from existing Observe-mode `NSXGroup` objects during reconciliation if one is already present, must not add it to newly observed groups, and must not require a finalizer to clean up Observe-mode CRs.

The operator must still work when the backing NSX group is deleted without this operator. If an `NSXGroup` CR is in `Observe` mode and the corresponding NSX group is absent from NSX-T during a verified manager sweep or reconcile path, the operator must automatically delete the Kubernetes CR from the kube-api. This deletion must not attempt to delete anything from NSX, and it must not depend on a finalizer being present.

In scope: finalizer rules for Observe-mode `NSXGroup`; migration behavior for existing Observe CRs that already have `nsx.ing.com/finalizer`; auto-deleting Observe CRs when the backing NSX group is missing from NSX-T; focused unit/integration/e2e tests using envtest and `../nsx-t-mockapi` where appropriate; structured zap logging for finalizer removal and Observe auto-delete decisions.

Out of scope: changing Manage-mode finalizer semantics except where needed to keep existing tests passing; adding finalizers to `NSXNetworkCloud`; deleting NSX objects for Observe-mode resources; broad redesign of reconciliation unrelated to Observe lifecycle behavior.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] A newly created or reconciled Observe-mode `NSXGroup` never receives `nsx.ing.com/finalizer` or any other lifecycle-blocking finalizer.
- [ ] An existing Observe-mode `NSXGroup` that already has `nsx.ing.com/finalizer` has that finalizer removed by reconciliation without deleting the backing NSX group.
- [ ] Deleting an Observe-mode `NSXGroup` from Kubernetes completes without the operator using a finalizer and without deleting the backing NSX group from NSX-T.
- [ ] If the backing NSX group is deleted outside this operator and the Kubernetes `NSXGroup` is in `Observe` mode, the operator automatically deletes the Kubernetes CR from the kube-api after it verifies the NSX group is absent.
- [ ] The Observe missing-from-NSX auto-delete behavior is tested against `../nsx-t-mockapi` using envtest or testcontainers-backed integration coverage, not only mocks.
- [ ] Existing Manage-mode finalizer/delete behavior remains covered and passing.
- [ ] `make check`, `make test`, and `make test-coverage` pass, with no skipped tests.
</acceptance_criteria>

// Package stateoperator reconciles Kubernetes NSX custom resources with NSX manager state.
package stateoperator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/logging"
	"github.com/djosh34/nsx-operator/internal/names"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
	"github.com/djosh34/nsx-operator/internal/operatormetrics"
	"github.com/djosh34/nsx-operator/internal/statuscondition"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BindingKey identifies one NSX group within a normalized network cloud.
type BindingKey struct {
	NetworkCloudFQDN string
	GroupID          string
}

// RemoteGroup is the NSX manager representation normalized for planning.
type RemoteGroup struct {
	Key                   BindingKey
	DisplayName           string
	CIDRs                 []string
	SegmentPaths          []string
	IPAddressExpressionID string
	PathExpressionID      string
	UnsupportedReason     nsxv1alpha.UnsupportedExpressionReason
	Raw                   nsxclient.Group
}

// HasUnsupportedExpression reports whether the remote group contains unsupported expression shape.
func (r *RemoteGroup) HasUnsupportedExpression() bool {
	return r.UnsupportedReason != ""
}

func (r *RemoteGroup) markUnsupported(reason nsxv1alpha.UnsupportedExpressionReason) {
	if r.UnsupportedReason == "" {
		r.UnsupportedReason = reason
	}
}

// GroupListFunc lists local group custom resources for manager planning.
type GroupListFunc func(context.Context, kubeapi.ListOptions) (*nsxv1alpha.NSXGroupList, error)

// ManagerClient is the NSX manager API surface required by group reconciliation.
type ManagerClient interface {
	managerGroupReader
	managerGroupWriter
	managerIPAddressExpressionWriter
	managerPathExpressionWriter
}

type managerGroupReader interface {
	ListGroups(ctx context.Context) ([]*nsxclient.Group, error)
}

type managerGroupWriter interface {
	PatchGroup(ctx context.Context, groupID string, group *nsxclient.GroupPatch) error
	PutGroup(ctx context.Context, groupID string, group *nsxclient.Group) (*nsxclient.Group, error)
	DeleteGroup(ctx context.Context, groupID string) error
}

type managerIPAddressExpressionWriter interface {
	PatchGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.IPAddressExpressionPatch) error
	AddGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.IPAddressExpressionPatch) error
	DeleteGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string) error
}

type managerPathExpressionWriter interface {
	PatchGroupPathExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.PathExpressionPatch) error
	AddGroupPathExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.PathExpressionPatch) error
	DeleteGroupPathExpression(ctx context.Context, groupID string, expressionID string) error
}

type managerManagedWriteClient interface {
	managerGroupWriter
	managerIPAddressExpressionWriter
	managerPathExpressionWriter
}

// ManagerClientFactory creates an NSX manager client for a network cloud.
type ManagerClientFactory func(context.Context, nsxv1alpha.NSXNetworkCloud) (ManagerClient, error)

// ManagerKubeApplier applies Kubernetes writes produced by a manager plan.
type ManagerKubeApplier interface {
	ApplyManagerKubeWrites(ctx context.Context, writes ManagerKubeWritePlan) (*ManagerKubeApplyResult, error)
}

var _ ManagerClient = (*nsxclient.Client)(nil)

// ManagerSnapshot contains local and remote group state for one network cloud sweep.
type ManagerSnapshot struct {
	Cloud            nsxv1alpha.NSXNetworkCloud
	NetworkCloudFQDN string
	LocalGroups      []nsxv1alpha.NSXGroup
	RemoteGroups     []RemoteGroup
	GatherError      error
}

// ManagerPlan contains Kubernetes and NSX writes required to reconcile one manager snapshot.
type ManagerPlan struct {
	ObserveUpserts           []nsxv1alpha.NSXGroup
	ManagedWrites            []ManagedGroupWrite
	ManagedDeletes           []ManagedGroupDelete
	ObserveFinalizerRemovals []string
	ManagedFinalizerRemovals []string
	GroupStatuses            []GroupStatusPlan
	ObserveDeletes           []string
	CloudStatus              *CloudStatusPlan
	KubeWrites               ManagerKubeWritePlan
	NSXWrites                ManagerNSXWritePlan
	statusWriteDecisions     []statusWriteLogDecision
}

// ManagerBindings indexes local and remote groups by binding key.
type ManagerBindings struct {
	Local       []LocalBinding
	Remote      []RemoteBinding
	LocalByKey  map[BindingKey]nsxv1alpha.NSXGroup
	RemoteByKey map[BindingKey]RemoteGroup
}

// LocalBinding pairs a local group with its manager binding key.
type LocalBinding struct {
	Key   BindingKey
	Group nsxv1alpha.NSXGroup
}

// RemoteBinding pairs a remote group with its manager binding key.
type RemoteBinding struct {
	Key    BindingKey
	Remote RemoteGroup
}

// ManagedGroupWrite describes a desired NSX group write.
type ManagedGroupWrite struct {
	Name                  string
	Key                   BindingKey
	DisplayName           string
	CIDRs                 []string
	SegmentPaths          []string
	IPAddressExpressionID string
	PathExpressionID      string
}

// ManagedGroupPatch describes a patchable NSX group update.
type ManagedGroupPatch = ManagedGroupWrite

// ManagedGroupPut describes a full NSX group put/upsert.
type ManagedGroupPut = ManagedGroupWrite

// ManagedGroupDelete describes a desired NSX group delete.
type ManagedGroupDelete struct {
	GroupID string
}

// ManagerNSXWritePlan classifies NSX manager writes for one manager pass.
type ManagerNSXWritePlan struct {
	Patches map[BindingKey]ManagedGroupPatch
	Puts    map[BindingKey]ManagedGroupPut
	Deletes map[BindingKey]ManagedGroupDelete
}

// Empty reports whether the plan contains no NSX writes.
func (writes *ManagerNSXWritePlan) Empty() bool {
	return len(writes.Patches) == 0 && len(writes.Puts) == 0 && len(writes.Deletes) == 0
}

// GroupStatusPlan describes a local group status update.
type GroupStatusPlan struct {
	Name            string
	ResourceVersion string
	Status          nsxv1alpha.NSXGroupStatus
}

// CloudStatusPlan describes a network cloud status update.
type CloudStatusPlan struct {
	Name            string
	ResourceVersion string
	Status          nsxv1alpha.NSXNetworkCloudStatus
}

type statusWriteLogDecision struct {
	ResourceKind     string
	ResourceName     string
	NetworkCloudFQDN string
	GroupID          string
	Decision         statuscondition.StatusWriteDecision
}

// BuildBindings indexes local and remote groups from a manager snapshot.
//
//nolint:gocritic // public planning API keeps value snapshots so tests and callers can pass literals.
func BuildBindings(snapshot ManagerSnapshot) (*ManagerBindings, error) {
	localGroups := append([]nsxv1alpha.NSXGroup(nil), snapshot.LocalGroups...)
	sort.Slice(localGroups, func(i int, j int) bool {
		return localGroups[i].Name < localGroups[j].Name
	})

	remoteGroups := append([]RemoteGroup(nil), snapshot.RemoteGroups...)
	sort.Slice(remoteGroups, func(i int, j int) bool {
		return compareBindingKeys(remoteGroups[i].Key, remoteGroups[j].Key) < 0
	})

	bindings := ManagerBindings{
		Local:       make([]LocalBinding, 0, len(localGroups)),
		Remote:      make([]RemoteBinding, 0, len(remoteGroups)),
		LocalByKey:  make(map[BindingKey]nsxv1alpha.NSXGroup, len(localGroups)),
		RemoteByKey: make(map[BindingKey]RemoteGroup, len(remoteGroups)),
	}
	for groupIndex := range localGroups {
		group := localGroups[groupIndex]
		key := BindingKey{NetworkCloudFQDN: group.Spec.NetworkCloudFQDN, GroupID: group.Spec.GroupID}
		if _, exists := bindings.LocalByKey[key]; exists {
			return nil, fmt.Errorf("duplicate local binding %s/%s", key.NetworkCloudFQDN, key.GroupID)
		}
		bindings.Local = append(bindings.Local, LocalBinding{Key: key, Group: group})
		bindings.LocalByKey[key] = group
	}
	for remoteIndex := range remoteGroups {
		remote := remoteGroups[remoteIndex]
		if _, exists := bindings.RemoteByKey[remote.Key]; exists {
			return nil, fmt.Errorf("duplicate remote binding %s/%s", remote.Key.NetworkCloudFQDN, remote.Key.GroupID)
		}
		bindings.Remote = append(bindings.Remote, RemoteBinding{Key: remote.Key, Remote: remote})
		bindings.RemoteByKey[remote.Key] = remote
	}
	return &bindings, nil
}

// RemoteGroupFromNSXGroup normalizes an NSX group into planning state.
//
//nolint:gocritic // public conversion API keeps value DTOs so callers can pass decoded literals.
func RemoteGroupFromNSXGroup(networkCloudFQDN string, group nsxclient.Group) RemoteGroup {
	remote := RemoteGroup{
		Key: BindingKey{
			NetworkCloudFQDN: networkCloudFQDN,
			GroupID:          group.ID,
		},
		DisplayName: group.DisplayName,
		Raw:         group,
	}
	if len(group.ExtendedExpression) > 0 {
		remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression)
	}
	seenIPExpression := false
	seenPathExpression := false
	for _, raw := range group.Expression {
		var header struct {
			ResourceType string `json:"resource_type"`
		}
		err := json.Unmarshal(raw, &header)
		if err != nil {
			remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonUnsupportedExpressionType)
			continue
		}
		switch header.ResourceType {
		case "IPAddressExpression":
			if seenIPExpression {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonMultipleIPAddressExpressions)
				continue
			}
			seenIPExpression = true
			fields, fieldsErr := expressionFields(raw)
			if fieldsErr != nil {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonInvalidIPAddressExpression)
				continue
			}
			if _, ok := fields["ip_addresses"]; !ok {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonInvalidIPAddressExpression)
				continue
			}
			var expression nsxclient.IPAddressExpression
			err = json.Unmarshal(raw, &expression)
			if err != nil {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonInvalidIPAddressExpression)
				continue
			}
			remote.CIDRs = append([]string(nil), expression.IPAddresses...)
			remote.IPAddressExpressionID = expression.ID
			if hasUnsupportedExpressionFields(fields, allowedIPAddressExpressionFields) {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonUnsupportedIPAddressExpressionFields)
			}
		case "PathExpression":
			if seenPathExpression {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonMultiplePathExpressions)
				continue
			}
			seenPathExpression = true
			fields, fieldsErr := expressionFields(raw)
			if fieldsErr != nil {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonInvalidPathExpression)
				continue
			}
			if _, ok := fields["paths"]; !ok {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonInvalidPathExpression)
				continue
			}
			var expression nsxclient.PathExpression
			err = json.Unmarshal(raw, &expression)
			if err != nil {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonInvalidPathExpression)
				continue
			}
			remote.SegmentPaths = copyStringSlice(expression.Paths)
			remote.PathExpressionID = expression.ID
			if hasUnsupportedExpressionFields(fields, allowedPathExpressionFields) {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonUnsupportedPathExpressionFields)
			}
		case "ConjunctionOperator":
			var expression struct {
				ConjunctionOperator string `json:"conjunction_operator"`
			}
			err = json.Unmarshal(raw, &expression)
			if err != nil {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression)
				continue
			}
			if expression.ConjunctionOperator != "OR" {
				remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression)
			}
		default:
			remote.markUnsupported(nsxv1alpha.UnsupportedExpressionReasonUnsupportedExpressionType)
		}
	}
	return remote
}

var allowedIPAddressExpressionFields = map[string]struct{}{
	"id":                  {},
	"display_name":        {},
	"description":         {},
	"resource_type":       {},
	"path":                {},
	"parent_path":         {},
	"relative_path":       {},
	"_revision":           {},
	"_create_user":        {},
	"_last_modified_user": {},
	"_create_time":        {},
	"_last_modified_time": {},
	"ip_addresses":        {},
}

var allowedPathExpressionFields = map[string]struct{}{
	"id":                  {},
	"display_name":        {},
	"description":         {},
	"resource_type":       {},
	"path":                {},
	"parent_path":         {},
	"relative_path":       {},
	"_revision":           {},
	"_create_user":        {},
	"_last_modified_user": {},
	"_create_time":        {},
	"_last_modified_time": {},
	"paths":               {},
}

func expressionFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	err := json.Unmarshal(raw, &fields)
	if err != nil {
		return nil, err
	}
	return fields, nil
}

func hasUnsupportedExpressionFields(fields map[string]json.RawMessage, allowed map[string]struct{}) bool {
	for field := range fields {
		if _, ok := allowed[field]; !ok {
			return true
		}
	}
	return false
}

// GatherManagerSnapshot reads local groups and remote NSX groups for one cloud.
//
//nolint:gocritic // public planning API keeps value cloud objects so callers can pass informer values.
func GatherManagerSnapshot(
	ctx context.Context,
	cloud nsxv1alpha.NSXNetworkCloud,
	listGroups GroupListFunc,
	managerClientFactory ManagerClientFactory,
) (*ManagerSnapshot, error) {
	normalizedFQDN := names.NormalizeNetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN)
	snapshot := ManagerSnapshot{
		Cloud:            cloud,
		NetworkCloudFQDN: normalizedFQDN,
	}
	if listGroups == nil {
		return nil, errors.New("group list function is required")
	}
	if managerClientFactory == nil {
		return nil, errors.New("manager client factory is required")
	}
	localGroups, err := listGroups(ctx, kubeapi.ListOptions{
		Filters: []kubeapi.FieldFilter{
			kubeapi.FilterBy(kubeapi.FieldNetworkCloudFQDN, normalizedFQDN),
		},
	})
	if err != nil {
		snapshot.GatherError = fmt.Errorf("list local nsx groups for %q: %w", normalizedFQDN, err)
		return &snapshot, nil
	}
	snapshot.LocalGroups = append([]nsxv1alpha.NSXGroup(nil), localGroups.Items...)

	managerClient, err := managerClientFactory(ctx, cloud)
	if err != nil {
		snapshot.GatherError = fmt.Errorf("construct nsx manager client for %q: %w", normalizedFQDN, err)
		return &snapshot, nil
	}
	remoteGroups, err := managerClient.ListGroups(ctx)
	if err != nil {
		snapshot.GatherError = fmt.Errorf("list remote nsx groups for %q: %w", normalizedFQDN, err)
		return &snapshot, nil
	}
	snapshot.RemoteGroups = make([]RemoteGroup, 0, len(remoteGroups))
	for _, remoteGroup := range remoteGroups {
		if remoteGroup == nil {
			continue
		}
		snapshot.RemoteGroups = append(snapshot.RemoteGroups, RemoteGroupFromNSXGroup(normalizedFQDN, *remoteGroup))
	}
	return &snapshot, nil
}

// ProcessManagerSnapshot converts gathered state into Kubernetes and NSX writes.
//
//nolint:gocritic // public planning API keeps value snapshots so tests and callers can pass literals.
func ProcessManagerSnapshot(snapshot ManagerSnapshot, now time.Time) (*ManagerPlan, error) {
	if snapshot.GatherError != nil {
		cloudStatus, err := statuscondition.BuildNetworkCloudStatus(
			snapshot.Cloud.Status,
			snapshot.Cloud.Generation,
			now,
			statuscondition.Reachable(metav1.ConditionFalse, "GatherFailed", snapshot.GatherError.Error()),
			statuscondition.Swept(metav1.ConditionFalse, "GatherFailed", snapshot.GatherError.Error()),
		)
		if err != nil {
			return nil, fmt.Errorf("build gather failure cloud status: %w", err)
		}
		plan := ManagerPlan{}
		setCloudStatusPlanIfNeeded(&plan, snapshot.Cloud.Name, snapshot.Cloud.ResourceVersion, snapshot.Cloud.Spec.NetworkCloudFQDN, snapshot.Cloud.Status, *cloudStatus)
		return &plan, nil
	}
	bindings, err := BuildBindings(snapshot)
	if err != nil {
		return nil, err
	}
	plan := ManagerPlan{}
	cloudStatus, err := statuscondition.BuildNetworkCloudStatus(
		snapshot.Cloud.Status,
		snapshot.Cloud.Generation,
		now,
		statuscondition.Reachable(metav1.ConditionTrue, "GatherSucceeded", "NSX manager gather completed"),
		statuscondition.Swept(metav1.ConditionTrue, "SweepPlanned", "manager snapshot was processed"),
	)
	if err != nil {
		return nil, fmt.Errorf("build successful cloud status: %w", err)
	}
	setCloudStatusPlanIfNeeded(&plan, snapshot.Cloud.Name, snapshot.Cloud.ResourceVersion, snapshot.Cloud.Spec.NetworkCloudFQDN, snapshot.Cloud.Status, *cloudStatus)
	for remoteIndex := range bindings.Remote {
		remoteBinding := bindings.Remote[remoteIndex]
		if _, exists := bindings.LocalByKey[remoteBinding.Key]; exists {
			continue
		}
		name := names.NSXGroupName(names.NSXGroupLogicalID{
			NetworkCloudFQDN: remoteBinding.Key.NetworkCloudFQDN,
			GroupID:          remoteBinding.Key.GroupID,
		})
		group := observeGroupFromRemote(name, &remoteBinding.Remote)
		plan.ObserveUpserts = append(plan.ObserveUpserts, group)
		createKey := groupBatchKey("create", "", name)
		ensureManagerKubeWrites(&plan.KubeWrites)
		plan.KubeWrites.GroupCreates[createKey] = kubeapi.GroupCreateRequest{Object: group.DeepCopy()}
		status, statusErr := syncedRemoteStatus(nsxv1alpha.NSXGroupStatus{}, 0, &remoteBinding.Remote, now)
		if statusErr != nil {
			return nil, fmt.Errorf("build observe import status %q: %w", name, statusErr)
		}
		appendGroupStatusPlanIfNeeded(&plan, name, "", remoteBinding.Key, nsxv1alpha.NSXGroupStatus{}, *status, &createKey)
	}
	for localIndex := range bindings.Local {
		localBinding := bindings.Local[localIndex]
		remote, exists := bindings.RemoteByKey[localBinding.Key]
		switch localBinding.Group.Spec.Mode {
		case nsxv1alpha.NSXGroupModeObserve:
			var groupWriteKey *kubeapi.BatchKey
			if slices.Contains(localBinding.Group.Finalizers, GroupFinalizer) {
				plan.ObserveFinalizerRemovals = append(plan.ObserveFinalizerRemovals, localBinding.Group.Name)
			}
			if !exists {
				plan.ObserveDeletes = append(plan.ObserveDeletes, localBinding.Group.Name)
				addGroupDeleteRequest(&plan, localBinding.Group.Name)
				if slices.Contains(localBinding.Group.Finalizers, GroupFinalizer) {
					addGroupFinalizerPatchRequest(&plan, &localBinding.Group, nil)
				}
				continue
			}
			remoteSpec := observeSpecFromRemote(&remote)
			if !groupSpecsEqual(&localBinding.Group.Spec, &remoteSpec) {
				plan.ObserveUpserts = append(plan.ObserveUpserts, observeGroupFromRemote(localBinding.Group.Name, &remote))
				update := localBinding.Group.DeepCopy()
				update.Spec = remoteSpec
				key := groupBatchKey("update", "", localBinding.Group.Name)
				ensureManagerKubeWrites(&plan.KubeWrites)
				plan.KubeWrites.GroupUpdates[key] = kubeapi.GroupUpdateRequest{Object: update}
				groupWriteKey = &key
			}
			status, statusErr := syncedRemoteStatus(localBinding.Group.Status, localBinding.Group.Generation, &remote, now)
			if statusErr != nil {
				return nil, fmt.Errorf("build observe status %q: %w", localBinding.Group.Name, statusErr)
			}
			statusKey := appendGroupStatusPlanIfNeeded(&plan, localBinding.Group.Name, localBinding.Group.ResourceVersion, localBinding.Key, localBinding.Group.Status, *status, groupWriteKey)
			if slices.Contains(localBinding.Group.Finalizers, GroupFinalizer) {
				addGroupFinalizerPatchRequest(&plan, &localBinding.Group, groupWriteKeyOrStatusKey(groupWriteKey, statusKey))
			}
		case nsxv1alpha.NSXGroupModeManage:
			if localBinding.Group.DeletionTimestamp != nil {
				if exists {
					deletion := ManagedGroupDelete{GroupID: localBinding.Key.GroupID}
					plan.ManagedDeletes = append(plan.ManagedDeletes, deletion)
					ensureManagerNSXWrites(&plan.NSXWrites)
					plan.NSXWrites.Deletes[localBinding.Key] = deletion
					status, statusErr := deletingManageStatus(localBinding.Group.Status, localBinding.Group.Generation, &remote, now)
					if statusErr != nil {
						return nil, fmt.Errorf("build deleting managed status %q: %w", localBinding.Group.Name, statusErr)
					}
					appendGroupStatusPlanIfNeeded(&plan, localBinding.Group.Name, localBinding.Group.ResourceVersion, localBinding.Key, localBinding.Group.Status, *status, nil)
					continue
				}
				if slices.Contains(localBinding.Group.Finalizers, GroupFinalizer) {
					plan.ManagedFinalizerRemovals = append(plan.ManagedFinalizerRemovals, localBinding.Group.Name)
				}
				status, statusErr := deletedManageStatus(localBinding.Group.Status, localBinding.Group.Generation, now)
				if statusErr != nil {
					return nil, fmt.Errorf("build deleted managed status %q: %w", localBinding.Group.Name, statusErr)
				}
				statusKey := appendGroupStatusPlanIfNeeded(&plan, localBinding.Group.Name, localBinding.Group.ResourceVersion, localBinding.Key, localBinding.Group.Status, *status, nil)
				if slices.Contains(localBinding.Group.Finalizers, GroupFinalizer) {
					addGroupFinalizerPatchRequest(&plan, &localBinding.Group, statusKey)
				}
				continue
			}
			if !exists {
				write := managedWriteFromLocal(&localBinding.Group, &RemoteGroup{})
				plan.ManagedWrites = append(plan.ManagedWrites, write)
				ensureManagerNSXWrites(&plan.NSXWrites)
				plan.NSXWrites.Puts[localBinding.Key] = write
				status, statusErr := missingManageStatus(localBinding.Group.Status, localBinding.Group.Generation, now)
				if statusErr != nil {
					return nil, fmt.Errorf("build missing managed status %q: %w", localBinding.Group.Name, statusErr)
				}
				appendGroupStatusPlanIfNeeded(&plan, localBinding.Group.Name, localBinding.Group.ResourceVersion, localBinding.Key, localBinding.Group.Status, *status, nil)
				continue
			}
			if !managedSpecMatchesRemote(&localBinding.Group.Spec, &remote) {
				write := managedWriteFromLocal(&localBinding.Group, &remote)
				plan.ManagedWrites = append(plan.ManagedWrites, write)
				ensureManagerNSXWrites(&plan.NSXWrites)
				plan.NSXWrites.Patches[localBinding.Key] = write
				status, statusErr := applyingManageStatus(localBinding.Group.Status, localBinding.Group.Generation, &remote, now)
				if statusErr != nil {
					return nil, fmt.Errorf("build applying managed status %q: %w", localBinding.Group.Name, statusErr)
				}
				appendGroupStatusPlanIfNeeded(&plan, localBinding.Group.Name, localBinding.Group.ResourceVersion, localBinding.Key, localBinding.Group.Status, *status, nil)
				continue
			}
			status, statusErr := matchingManageStatus(localBinding.Group.Status, localBinding.Group.Generation, &remote, now)
			if statusErr != nil {
				return nil, fmt.Errorf("build matching managed status %q: %w", localBinding.Group.Name, statusErr)
			}
			appendGroupStatusPlanIfNeeded(&plan, localBinding.Group.Name, localBinding.Group.ResourceVersion, localBinding.Key, localBinding.Group.Status, *status, nil)
		}
	}
	return &plan, nil
}

func appendGroupStatusPlanIfNeeded(
	plan *ManagerPlan,
	name string,
	resourceVersion string,
	key BindingKey,
	current nsxv1alpha.NSXGroupStatus,
	desired nsxv1alpha.NSXGroupStatus,
	afterGroupWrite *kubeapi.BatchKey,
) *kubeapi.BatchKey {
	decision := statuscondition.CompareGroupStatus(current, desired)
	plan.statusWriteDecisions = append(plan.statusWriteDecisions, statusWriteLogDecision{
		ResourceKind:     "NSXGroup",
		ResourceName:     name,
		NetworkCloudFQDN: key.NetworkCloudFQDN,
		GroupID:          key.GroupID,
		Decision:         decision,
	})
	if !decision.Needed {
		return nil
	}
	plan.GroupStatuses = append(plan.GroupStatuses, GroupStatusPlan{
		Name:            name,
		ResourceVersion: resourceVersion,
		Status:          desired,
	})
	ensureManagerKubeWrites(&plan.KubeWrites)
	statusKey := groupBatchKey("updateStatus", "status", name)
	if afterGroupWrite != nil {
		plan.KubeWrites.GroupStatusesAfterGroupWrite[*afterGroupWrite] = GroupStatusAfterGroupWrite{
			Name:   name,
			Status: desired,
		}
		return &statusKey
	}
	plan.KubeWrites.GroupStatusUpdates[statusKey] = kubeapi.GroupStatusUpdateRequest{
		Name:   name,
		Status: desired,
		Options: kubeapi.StatusUpdateOptions{
			ResourceVersion: resourceVersion,
		},
	}
	return &statusKey
}

func setCloudStatusPlanIfNeeded(
	plan *ManagerPlan,
	name string,
	resourceVersion string,
	networkCloudFQDN string,
	current nsxv1alpha.NSXNetworkCloudStatus,
	desired nsxv1alpha.NSXNetworkCloudStatus,
) {
	decision := statuscondition.CompareNetworkCloudStatus(current, desired)
	plan.statusWriteDecisions = append(plan.statusWriteDecisions, statusWriteLogDecision{
		ResourceKind:     "NSXNetworkCloud",
		ResourceName:     name,
		NetworkCloudFQDN: networkCloudFQDN,
		Decision:         decision,
	})
	if !decision.Needed {
		return
	}
	plan.CloudStatus = &CloudStatusPlan{Name: name, ResourceVersion: resourceVersion, Status: desired}
	ensureManagerKubeWrites(&plan.KubeWrites)
	plan.KubeWrites.CloudStatusUpdates[networkCloudBatchKey("updateStatus", "status", name)] = kubeapi.NetworkCloudStatusUpdateRequest{
		Name:   name,
		Status: desired,
		Options: kubeapi.StatusUpdateOptions{
			ResourceVersion: resourceVersion,
		},
	}
}

func ensureManagerNSXWrites(writes *ManagerNSXWritePlan) {
	if writes.Patches == nil {
		writes.Patches = map[BindingKey]ManagedGroupPatch{}
	}
	if writes.Puts == nil {
		writes.Puts = map[BindingKey]ManagedGroupPut{}
	}
	if writes.Deletes == nil {
		writes.Deletes = map[BindingKey]ManagedGroupDelete{}
	}
}

// ApplyManagerPlan applies NSX writes first and then Kubernetes writes from a manager plan.
//
//nolint:gocritic // public planning API keeps value plans so tests and callers can pass literals.
func ApplyManagerPlan(ctx context.Context, kubeApplier ManagerKubeApplier, managerClient ManagerClient, plan ManagerPlan) error {
	if kubeApplier == nil {
		return errors.New("kubernetes manager applier is required")
	}
	if plan.KubeWrites.Empty() {
		plan.KubeWrites = legacyManagerKubeWrites(&plan)
	}
	if plan.NSXWrites.Empty() {
		plan.NSXWrites = legacyManagerNSXWrites(&plan)
	}
	if managerClient == nil && !plan.NSXWrites.Empty() {
		return errors.New("manager client is required for managed writes or deletes")
	}
	if !plan.NSXWrites.Empty() {
		err := ApplyManagerNSXWrites(ctx, managerClient, plan.NSXWrites)
		if err != nil {
			return err
		}
	}
	if !plan.KubeWrites.Empty() {
		_, err := kubeApplier.ApplyManagerKubeWrites(ctx, plan.KubeWrites)
		if err != nil {
			return fmt.Errorf("apply manager kubernetes writes: %w", err)
		}
	}
	return nil
}

func legacyManagerNSXWrites(plan *ManagerPlan) ManagerNSXWritePlan {
	writes := ManagerNSXWritePlan{}
	ensureManagerNSXWrites(&writes)
	for writeIndex := range plan.ManagedWrites {
		write := plan.ManagedWrites[writeIndex]
		writes.Patches[write.Key] = write
	}
	for deleteIndex := range plan.ManagedDeletes {
		deletion := plan.ManagedDeletes[deleteIndex]
		key := BindingKey{GroupID: deletion.GroupID}
		writes.Deletes[key] = deletion
	}
	return writes
}

// ApplyManagerNSXWrites applies classified NSX manager writes for one manager pass.
func ApplyManagerNSXWrites(ctx context.Context, managerClient managerManagedWriteClient, writes ManagerNSXWritePlan) error {
	for key := range writes.Patches {
		if _, exists := writes.Puts[key]; exists {
			return fmt.Errorf("managed nsx group %q/%q cannot be both patched and put", key.NetworkCloudFQDN, key.GroupID)
		}
	}
	nsxWritesSkipped := false
	for _, key := range sortedBindingKeys(writes.Patches) {
		write := writes.Patches[key]
		err := applyManagedWrite(ctx, managerClient, &write)
		if err != nil {
			if isWriteDisabled(err) {
				nsxWritesSkipped = true
				break
			}
			return err
		}
	}
	if !nsxWritesSkipped {
		for _, key := range sortedBindingKeys(writes.Puts) {
			write := writes.Puts[key]
			err := applyManagedPut(ctx, managerClient, &write)
			if err != nil {
				if isWriteDisabled(err) {
					nsxWritesSkipped = true
					break
				}
				return err
			}
		}
	}
	if !nsxWritesSkipped {
		for _, key := range sortedBindingKeys(writes.Deletes) {
			deletion := writes.Deletes[key]
			err := managerClient.DeleteGroup(ctx, deletion.GroupID)
			if err != nil {
				if isWriteDisabled(err) {
					break
				}
				return fmt.Errorf("delete managed nsx group %q: %w", deletion.GroupID, err)
			}
		}
	}
	return nil
}

func sortedBindingKeys[T any](items map[BindingKey]T) []BindingKey {
	keys := make([]BindingKey, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, compareBindingKeys)
	return keys
}

func isWriteDisabled(err error) bool {
	var writeDisabled *nsxclient.WriteDisabledError
	return errors.As(err, &writeDisabled)
}

func defaultManagerSweep(
	kubeClient *kubeapi.Client,
	managerClientFactory ManagerClientFactory,
	logger *zap.Logger,
	clock Clock,
	recorder operatormetrics.Recorder,
) CloudSweepFunc {
	if logger == nil {
		logger = zap.NewNop()
	}
	if recorder == nil {
		recorder = &operatormetrics.NopRecorder{}
	}
	return func(ctx context.Context, cloud nsxv1alpha.NSXNetworkCloud, sweep SweepContext) error {
		normalizedFQDN := names.NormalizeNetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN)
		fields := []zap.Field{
			logging.Component("stateoperator"),
			logging.SweepID(sweep.ID),
			logging.NetworkCloudFQDN(normalizedFQDN),
			zap.String("networkCloudName", cloud.Name),
		}
		logger.Info("starting default manager sweep", fields...)
		snapshot, err := GatherManagerSnapshot(ctx, cloud, kubeClient.Groups().List, managerClientFactory)
		if err != nil {
			logger.Info("default manager gather setup failed", append(fields, zap.Error(err))...)
			return err
		}
		logger.Debug("default manager gather completed", append(
			fields,
			zap.Int("localGroupCount", len(snapshot.LocalGroups)),
			zap.Int("remoteGroupCount", len(snapshot.RemoteGroups)),
			zap.Bool("gatherFailed", snapshot.GatherError != nil),
		)...)
		for remoteIndex := range snapshot.RemoteGroups {
			remote := &snapshot.RemoteGroups[remoteIndex]
			if !remote.HasUnsupportedExpression() {
				continue
			}
			logger.Debug("default manager remote group has unsupported expression", append(
				fields,
				logging.GroupID(remote.Key.GroupID),
				zap.String("unsupportedReason", string(remote.UnsupportedReason)),
			)...)
		}
		plan, err := ProcessManagerSnapshot(*snapshot, clock.Now())
		if err != nil {
			logger.Info("default manager processing failed", append(fields, zap.Error(err))...)
			return err
		}
		logger.Debug("default manager processing completed", append(
			fields,
			zap.Int("observeUpsertCount", len(plan.ObserveUpserts)),
			zap.Int("managedWriteCount", len(plan.ManagedWrites)),
			zap.Int("managedDeleteCount", len(plan.ManagedDeletes)),
			zap.Int("observeFinalizerRemovalCount", len(plan.ObserveFinalizerRemovals)),
			zap.Strings("observeFinalizerRemovalNames", plan.ObserveFinalizerRemovals),
			zap.Int("managedFinalizerRemovalCount", len(plan.ManagedFinalizerRemovals)),
			zap.Int("groupStatusCount", len(plan.GroupStatuses)),
			zap.Int("observeDeleteCount", len(plan.ObserveDeletes)),
			zap.Strings("observeDeleteNames", plan.ObserveDeletes),
			zap.Bool("cloudStatusPlanned", plan.CloudStatus != nil),
		)...)
		logManagerStatusWriteDecisions(logger, fields, plan.statusWriteDecisions)
		if snapshot.GatherError == nil {
			metricsSnapshot, metricsErr := managerMetricsSnapshot(snapshot, plan)
			if metricsErr != nil {
				logger.Info("default manager metrics summary failed", append(fields, zap.Error(metricsErr))...)
				return metricsErr
			}
			recorder.SetManagerGroupSnapshot(normalizedFQDN, *metricsSnapshot)
		}
		var managerClient ManagerClient
		if len(plan.ManagedWrites) > 0 || len(plan.ManagedDeletes) > 0 {
			managerClient, err = managerClientFactory(ctx, cloud)
			if err != nil {
				logger.Info("default manager apply client construction failed", append(fields, zap.Error(err))...)
				return fmt.Errorf("construct nsx manager client for apply: %w", err)
			}
		}
		err = ApplyManagerPlan(ctx, &kubeAPIAdapter{client: kubeClient, logger: logger}, managerClient, *plan)
		if err != nil {
			logger.Info("default manager apply failed", append(fields, zap.Error(err))...)
			return err
		}
		logger.Info("completed default manager sweep", fields...)
		return nil
	}
}

func logManagerStatusWriteDecisions(logger *zap.Logger, baseFields []zap.Field, decisions []statusWriteLogDecision) {
	for decisionIndex := range decisions {
		statusDecision := &decisions[decisionIndex]
		fields := appendStatusWriteDecisionFields(baseFields, statusDecision)
		logger.Debug("manager status write decision", fields...)
	}
}

func managerMetricsSnapshot(snapshot *ManagerSnapshot, plan *ManagerPlan) (*operatormetrics.ManagerGroupSnapshot, error) {
	bindings, err := BuildBindings(*snapshot)
	if err != nil {
		return nil, fmt.Errorf("build manager metrics bindings: %w", err)
	}

	remoteOnlyCreates := 0
	for remoteIndex := range bindings.Remote {
		remote := bindings.Remote[remoteIndex]
		if _, exists := bindings.LocalByKey[remote.Key]; !exists {
			remoteOnlyCreates++
		}
	}

	observeGroups := 0
	manageGroups := 0
	for localIndex := range bindings.Local {
		local := bindings.Local[localIndex]
		switch local.Group.Spec.Mode {
		case nsxv1alpha.NSXGroupModeObserve:
			observeGroups++
		case nsxv1alpha.NSXGroupModeManage:
			manageGroups++
		}
	}
	observeGroups += remoteOnlyCreates

	return &operatormetrics.ManagerGroupSnapshot{
		ListedGroups:         len(snapshot.RemoteGroups),
		ObserveGroups:        observeGroups,
		ManageGroups:         manageGroups,
		ObserveUpdatesNeeded: len(plan.ObserveUpserts) + len(plan.ObserveDeletes) + len(plan.ObserveFinalizerRemovals) - remoteOnlyCreates,
		ManageUpdatesNeeded:  len(plan.ManagedWrites) + len(plan.ManagedDeletes) + len(plan.ManagedFinalizerRemovals),
		CreatesNeeded:        remoteOnlyCreates,
	}, nil
}

func appendStatusWriteDecisionFields(baseFields []zap.Field, statusDecision *statusWriteLogDecision) []zap.Field {
	fields := append([]zap.Field{}, baseFields...)
	if len(baseFields) == 0 {
		fields = append(fields, logging.Component("stateoperator"))
	}
	fields = append(
		fields,
		zap.String("resourceKind", statusDecision.ResourceKind),
		zap.String("resourceName", statusDecision.ResourceName),
		logging.NetworkCloudFQDN(statusDecision.NetworkCloudFQDN),
		zap.Bool("statusWriteNeeded", statusDecision.Decision.Needed),
		zap.String("statusWriteReason", statusDecision.Decision.Reason),
		zap.Strings("statusDriftFields", statusDecision.Decision.DriftFields),
	)
	switch statusDecision.ResourceKind {
	case "NSXGroup":
		fields = append(
			fields,
			zap.String("groupName", statusDecision.ResourceName),
			logging.GroupID(statusDecision.GroupID),
		)
	case "NSXNetworkCloud":
		fields = append(fields, zap.String("networkCloudName", statusDecision.ResourceName))
	}
	return fields
}

func compareBindingKeys(left BindingKey, right BindingKey) int {
	if left.NetworkCloudFQDN < right.NetworkCloudFQDN {
		return -1
	}
	if left.NetworkCloudFQDN > right.NetworkCloudFQDN {
		return 1
	}
	if left.GroupID < right.GroupID {
		return -1
	}
	if left.GroupID > right.GroupID {
		return 1
	}
	return 0
}

func applyManagedWrite(ctx context.Context, managerClient managerManagedWriteClient, write *ManagedGroupWrite) error {
	group := &nsxclient.GroupPatch{
		ID:           write.Key.GroupID,
		DisplayName:  write.DisplayName,
		ResourceType: "Group",
	}
	err := managerClient.PatchGroup(ctx, write.Key.GroupID, group)
	if err != nil {
		return fmt.Errorf("patch managed nsx group %q: %w", write.Key.GroupID, err)
	}
	err = applyManagedIPAddressExpression(ctx, managerClient, write)
	if err != nil {
		return err
	}
	err = applyManagedPathExpression(ctx, managerClient, write)
	if err != nil {
		return err
	}
	return nil
}

func applyManagedPut(ctx context.Context, managerClient managerManagedWriteClient, write *ManagedGroupPut) error {
	group, err := managedGroupPutPayload(write)
	if err != nil {
		return err
	}
	_, err = managerClient.PutGroup(ctx, write.Key.GroupID, group)
	if err != nil {
		return fmt.Errorf("put managed nsx group %q: %w", write.Key.GroupID, err)
	}
	return nil
}

func managedGroupPutPayload(write *ManagedGroupPut) (*nsxclient.Group, error) {
	group := &nsxclient.Group{
		Resource: nsxclient.Resource{
			ID:           write.Key.GroupID,
			DisplayName:  write.DisplayName,
			ResourceType: "Group",
		},
		Expression: []json.RawMessage{},
	}
	if len(write.CIDRs) > 0 {
		raw, err := json.Marshal(nsxclient.IPAddressExpression{
			Resource: nsxclient.Resource{
				ID:           "cidrs",
				ResourceType: "IPAddressExpression",
			},
			IPAddresses: copyStringSlice(write.CIDRs),
		})
		if err != nil {
			return nil, fmt.Errorf("encode managed nsx group %q ip expression: %w", write.Key.GroupID, err)
		}
		group.Expression = append(group.Expression, raw)
	}
	if len(write.CIDRs) > 0 && len(write.SegmentPaths) > 0 {
		raw, err := json.Marshal(map[string]string{
			"resource_type":        "ConjunctionOperator",
			"conjunction_operator": "OR",
		})
		if err != nil {
			return nil, fmt.Errorf("encode managed nsx group %q conjunction expression: %w", write.Key.GroupID, err)
		}
		group.Expression = append(group.Expression, raw)
	}
	if len(write.SegmentPaths) > 0 {
		raw, err := json.Marshal(nsxclient.PathExpression{
			Resource: nsxclient.Resource{
				ID:           "segment",
				ResourceType: "PathExpression",
			},
			Paths: copyStringSlice(write.SegmentPaths),
		})
		if err != nil {
			return nil, fmt.Errorf("encode managed nsx group %q path expression: %w", write.Key.GroupID, err)
		}
		group.Expression = append(group.Expression, raw)
	}
	return group, nil
}

func applyManagedIPAddressExpression(ctx context.Context, managerClient managerIPAddressExpressionWriter, write *ManagedGroupWrite) error {
	if len(write.CIDRs) == 0 {
		if write.IPAddressExpressionID != "" {
			err := managerClient.DeleteGroupIPAddressExpression(ctx, write.Key.GroupID, write.IPAddressExpressionID)
			if err != nil {
				return fmt.Errorf("delete managed nsx group %q ip expression %q: %w", write.Key.GroupID, write.IPAddressExpressionID, err)
			}
		}
	} else {
		expression := &nsxclient.IPAddressExpressionPatch{
			ID:           write.IPAddressExpressionID,
			ResourceType: "IPAddressExpression",
			IPAddresses:  append([]string(nil), write.CIDRs...),
		}
		if write.IPAddressExpressionID != "" {
			err := managerClient.PatchGroupIPAddressExpression(ctx, write.Key.GroupID, write.IPAddressExpressionID, expression)
			if err != nil {
				return fmt.Errorf("patch managed nsx group %q ip expression %q: %w", write.Key.GroupID, write.IPAddressExpressionID, err)
			}
		} else {
			expression.ID = "cidrs"
			err := managerClient.AddGroupIPAddressExpression(ctx, write.Key.GroupID, expression.ID, expression)
			if err != nil {
				return fmt.Errorf("add managed nsx group %q ip expression %q: %w", write.Key.GroupID, expression.ID, err)
			}
		}
	}
	return nil
}

func applyManagedPathExpression(ctx context.Context, managerClient managerPathExpressionWriter, write *ManagedGroupWrite) error {
	if len(write.SegmentPaths) == 0 {
		if write.PathExpressionID != "" {
			err := managerClient.DeleteGroupPathExpression(ctx, write.Key.GroupID, write.PathExpressionID)
			if err != nil {
				return fmt.Errorf("delete managed nsx group %q path expression %q: %w", write.Key.GroupID, write.PathExpressionID, err)
			}
		}
		return nil
	}
	if write.PathExpressionID != "" {
		expression := &nsxclient.PathExpressionPatch{
			ID:           write.PathExpressionID,
			ResourceType: "PathExpression",
			Paths:        copyStringSlice(write.SegmentPaths),
		}
		err := managerClient.PatchGroupPathExpression(ctx, write.Key.GroupID, write.PathExpressionID, expression)
		if err != nil {
			return fmt.Errorf("patch managed nsx group %q path expression %q: %w", write.Key.GroupID, write.PathExpressionID, err)
		}
	}
	if write.PathExpressionID == "" {
		expression := &nsxclient.PathExpressionPatch{
			ID:           "segment",
			ResourceType: "PathExpression",
			Paths:        copyStringSlice(write.SegmentPaths),
		}
		err := managerClient.AddGroupPathExpression(ctx, write.Key.GroupID, expression.ID, expression)
		if err != nil {
			return fmt.Errorf("add managed nsx group %q path expression %q: %w", write.Key.GroupID, expression.ID, err)
		}
	}
	return nil
}

func observeGroupFromRemote(name string, remote *RemoteGroup) nsxv1alpha.NSXGroup {
	return nsxv1alpha.NSXGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: observeSpecFromRemote(remote),
	}
}

func observeSpecFromRemote(remote *RemoteGroup) nsxv1alpha.NSXGroupSpec {
	return nsxv1alpha.NSXGroupSpec{
		NetworkCloudFQDN: names.NormalizeNetworkCloudFQDN(remote.Key.NetworkCloudFQDN),
		GroupID:          strings.TrimSpace(remote.Key.GroupID),
		DisplayName:      remote.DisplayName,
		Mode:             nsxv1alpha.NSXGroupModeObserve,
		CIDRs:            copyStringSlice(remote.CIDRs),
		SegmentPaths:     copyStringSlice(remote.SegmentPaths),
	}
}

func groupSpecsEqual(left *nsxv1alpha.NSXGroupSpec, right *nsxv1alpha.NSXGroupSpec) bool {
	return left.NetworkCloudFQDN == right.NetworkCloudFQDN &&
		left.GroupID == right.GroupID &&
		left.DisplayName == right.DisplayName &&
		left.Mode == right.Mode &&
		reflect.DeepEqual(left.CIDRs, right.CIDRs) &&
		stringSetEqual(left.SegmentPaths, right.SegmentPaths)
}

func managedSpecMatchesRemote(local *nsxv1alpha.NSXGroupSpec, remote *RemoteGroup) bool {
	return local.NetworkCloudFQDN == remote.Key.NetworkCloudFQDN &&
		local.GroupID == remote.Key.GroupID &&
		local.DisplayName == remote.DisplayName &&
		reflect.DeepEqual(local.CIDRs, remote.CIDRs) &&
		stringSetEqual(local.SegmentPaths, remote.SegmentPaths)
}

func managedWriteFromLocal(group *nsxv1alpha.NSXGroup, remote *RemoteGroup) ManagedGroupWrite {
	return ManagedGroupWrite{
		Name:                  group.Name,
		Key:                   BindingKey{NetworkCloudFQDN: group.Spec.NetworkCloudFQDN, GroupID: group.Spec.GroupID},
		DisplayName:           group.Spec.DisplayName,
		CIDRs:                 copyStringSlice(group.Spec.CIDRs),
		SegmentPaths:          copyStringSlice(group.Spec.SegmentPaths),
		IPAddressExpressionID: remote.IPAddressExpressionID,
		PathExpressionID:      remote.PathExpressionID,
	}
}

func missingManageStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, now time.Time) (*nsxv1alpha.NSXGroupStatus, error) {
	return statuscondition.BuildGroupStatus(
		previous,
		observedGeneration,
		now,
		statuscondition.RemotePresent(metav1.ConditionFalse, "RemoteMissing", "remote NSX group is missing"),
		statuscondition.SpecMatchesRemote(metav1.ConditionFalse, "RemoteMissing", "remote NSX group is missing"),
		statuscondition.UnsupportedExpression(metav1.ConditionFalse, "SupportedExpression", "no unsupported remote expression is present"),
		statuscondition.Realized(metav1.ConditionUnknown, "RemoteMissing", "remote realization is unknown because the group is missing"),
		statuscondition.Synced(metav1.ConditionFalse, metav1.ConditionFalse, metav1.ConditionFalse, metav1.ConditionUnknown, "Applying", "managed NSX group create is planned"),
		statuscondition.Applying(metav1.ConditionTrue, "Applying", "managed NSX group create is planned"),
		statuscondition.Deleting(metav1.ConditionFalse, "NotDeleting", "no NSX delete is planned"),
	)
}

func applyingManageStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, remote *RemoteGroup, now time.Time) (*nsxv1alpha.NSXGroupStatus, error) {
	unsupportedStatus, unsupportedReason, unsupportedMessage := unsupportedExpressionCondition(remote)
	realizedStatus, realizedReason, realizedMessage := realizedCondition(remote)
	return statuscondition.BuildGroupStatus(
		previous,
		observedGeneration,
		now,
		statuscondition.RemotePresent(metav1.ConditionTrue, "RemoteFound", "remote NSX group is present"),
		statuscondition.SpecMatchesRemote(metav1.ConditionFalse, "SpecDrifted", "local group spec does not match remote NSX group"),
		statuscondition.UnsupportedExpression(unsupportedStatus, unsupportedReason, unsupportedMessage),
		statuscondition.Realized(realizedStatus, realizedReason, realizedMessage),
		statuscondition.Synced(metav1.ConditionTrue, metav1.ConditionFalse, unsupportedStatus, realizedStatus, "Applying", "managed NSX group update is planned"),
		statuscondition.Applying(metav1.ConditionTrue, "Applying", "managed NSX group update is planned"),
		statuscondition.Deleting(metav1.ConditionFalse, "NotDeleting", "no NSX delete is planned"),
	)
}

func matchingManageStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, remote *RemoteGroup, now time.Time) (*nsxv1alpha.NSXGroupStatus, error) {
	realizedStatus, realizedReason, realizedMessage := realizedCondition(remote)
	syncedReason := "Synced"
	syncedMessage := "local group matches remote NSX group"
	if realizedStatus == metav1.ConditionUnknown {
		syncedReason = "RealizationPending"
		syncedMessage = "remote realization is still pending"
	}
	if realizedStatus == metav1.ConditionFalse {
		syncedReason = "RealizationFailed"
		syncedMessage = "remote NSX group is not realized"
	}
	return statuscondition.BuildGroupStatus(
		previous,
		observedGeneration,
		now,
		statuscondition.RemotePresent(metav1.ConditionTrue, "RemoteFound", "remote NSX group is present"),
		statuscondition.SpecMatchesRemote(metav1.ConditionTrue, "SpecMatches", "local group matches remote NSX group"),
		statuscondition.UnsupportedExpression(metav1.ConditionFalse, "SupportedExpression", "remote NSX group expression is representable"),
		statuscondition.Realized(realizedStatus, realizedReason, realizedMessage),
		statuscondition.Synced(metav1.ConditionTrue, metav1.ConditionTrue, metav1.ConditionFalse, realizedStatus, syncedReason, syncedMessage),
		statuscondition.Applying(metav1.ConditionFalse, "NotApplying", "no NSX write is planned"),
		statuscondition.Deleting(metav1.ConditionFalse, "NotDeleting", "no NSX delete is planned"),
	)
}

func deletingManageStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, remote *RemoteGroup, now time.Time) (*nsxv1alpha.NSXGroupStatus, error) {
	unsupportedStatus, unsupportedReason, unsupportedMessage := unsupportedExpressionCondition(remote)
	realizedStatus, realizedReason, realizedMessage := realizedCondition(remote)
	return statuscondition.BuildGroupStatus(
		previous,
		observedGeneration,
		now,
		statuscondition.RemotePresent(metav1.ConditionTrue, "RemoteFound", "remote NSX group is present"),
		statuscondition.SpecMatchesRemote(metav1.ConditionUnknown, "Deleting", "managed NSX group is being deleted"),
		statuscondition.UnsupportedExpression(unsupportedStatus, unsupportedReason, unsupportedMessage),
		statuscondition.Realized(realizedStatus, realizedReason, realizedMessage),
		statuscondition.Synced(metav1.ConditionUnknown, metav1.ConditionUnknown, unsupportedStatus, realizedStatus, "Deleting", "managed NSX group delete is planned"),
		statuscondition.Applying(metav1.ConditionFalse, "NotApplying", "no NSX write is planned"),
		statuscondition.Deleting(metav1.ConditionTrue, "Deleting", "managed NSX group delete is planned"),
	)
}

func deletedManageStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, now time.Time) (*nsxv1alpha.NSXGroupStatus, error) {
	return statuscondition.BuildGroupStatus(
		previous,
		observedGeneration,
		now,
		statuscondition.RemotePresent(metav1.ConditionFalse, "RemoteDeleted", "remote NSX group deletion is confirmed"),
		statuscondition.SpecMatchesRemote(metav1.ConditionTrue, "Deleted", "managed NSX group deletion is confirmed"),
		statuscondition.UnsupportedExpression(metav1.ConditionFalse, "SupportedExpression", "no unsupported remote expression is present"),
		statuscondition.Realized(metav1.ConditionTrue, "Deleted", "managed NSX group deletion is confirmed"),
		statuscondition.ConditionUpdate{Type: nsxv1alpha.ConditionSynced, Status: metav1.ConditionTrue, Reason: "Deleted", Message: "managed NSX group deletion is confirmed"},
		statuscondition.Applying(metav1.ConditionFalse, "NotApplying", "no NSX write is planned"),
		statuscondition.Deleting(metav1.ConditionFalse, "Deleted", "managed NSX group deletion is confirmed"),
	)
}

func syncedRemoteStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, remote *RemoteGroup, now time.Time) (*nsxv1alpha.NSXGroupStatus, error) {
	unsupportedStatus, unsupportedReason, unsupportedMessage := unsupportedExpressionCondition(remote)
	realizedStatus, realizedReason, realizedMessage := realizedCondition(remote)
	syncedReason := "Synced"
	syncedMessage := "local group reflects remote NSX group"
	if remote.HasUnsupportedExpression() {
		syncedReason = string(remote.UnsupportedReason)
		syncedMessage = fmt.Sprintf("remote NSX group expression needs operator support before it can be synced: %s", remote.UnsupportedReason)
	}
	if realizedStatus == metav1.ConditionUnknown && unsupportedStatus != metav1.ConditionTrue {
		syncedReason = "RealizationPending"
		syncedMessage = "remote realization is still pending"
	}
	if realizedStatus == metav1.ConditionFalse && unsupportedStatus != metav1.ConditionTrue {
		syncedReason = "RealizationFailed"
		syncedMessage = "remote NSX group is not realized"
	}
	return statuscondition.BuildGroupStatus(
		previous,
		observedGeneration,
		now,
		statuscondition.RemotePresent(metav1.ConditionTrue, "RemoteFound", "remote NSX group is present"),
		statuscondition.SpecMatchesRemote(metav1.ConditionTrue, "SpecMatches", "local group spec matches remote NSX group"),
		statuscondition.UnsupportedExpression(unsupportedStatus, unsupportedReason, unsupportedMessage),
		statuscondition.Realized(realizedStatus, realizedReason, realizedMessage),
		statuscondition.Synced(metav1.ConditionTrue, metav1.ConditionTrue, unsupportedStatus, realizedStatus, syncedReason, syncedMessage),
		statuscondition.Applying(metav1.ConditionFalse, "NotApplying", "no NSX write is planned"),
		statuscondition.Deleting(metav1.ConditionFalse, "NotDeleting", "no NSX delete is planned"),
	)
}

func unsupportedExpressionCondition(remote *RemoteGroup) (metav1.ConditionStatus, string, string) {
	if remote.HasUnsupportedExpression() {
		return metav1.ConditionTrue, string(remote.UnsupportedReason), fmt.Sprintf("remote NSX group expression is not fully representable: %s", remote.UnsupportedReason)
	}
	return metav1.ConditionFalse, string(nsxv1alpha.UnsupportedExpressionReasonSupportedExpression), "remote NSX group expression is representable"
}

func realizedCondition(remote *RemoteGroup) (metav1.ConditionStatus, string, string) {
	switch strings.ToUpper(strings.TrimSpace(remote.Raw.State)) {
	case "", "REALIZED", "SUCCESS":
		return metav1.ConditionTrue, "Realized", "remote NSX group is realized"
	case "UNREALIZED", "ERROR", "FAILURE", "FAILED":
		return metav1.ConditionFalse, "RealizationFailed", "remote NSX group is not realized"
	default:
		return metav1.ConditionUnknown, "RealizationPending", "remote realization is still pending"
	}
}

func copyStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}

func stringSetEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	if len(left) == 0 {
		return true
	}
	counts := make(map[string]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	for _, value := range right {
		count, ok := counts[value]
		if !ok {
			return false
		}
		if count == 1 {
			delete(counts, value)
			continue
		}
		counts[value] = count - 1
	}
	return len(counts) == 0
}

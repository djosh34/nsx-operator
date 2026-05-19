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
	"github.com/djosh34/nsx-operator/internal/statuscondition"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

type GroupListFunc func(context.Context, kubeapi.ListOptions) (*nsxv1alpha.NSXGroupList, error)

type ManagerClient interface {
	ListGroups(ctx context.Context) ([]*nsxclient.Group, error)
	PatchGroup(ctx context.Context, groupID string, group *nsxclient.GroupPatch) error
	PatchGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.IPAddressExpressionPatch) error
	AddGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.IPAddressExpressionPatch) error
	DeleteGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string) error
	PatchGroupPathExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.PathExpressionPatch) error
	AddGroupPathExpression(ctx context.Context, groupID string, expressionID string, expression *nsxclient.PathExpressionPatch) error
	DeleteGroupPathExpression(ctx context.Context, groupID string, expressionID string) error
	DeleteGroup(ctx context.Context, groupID string) error
}

type ManagerClientFactory func(context.Context, nsxv1alpha.NSXNetworkCloud) (ManagerClient, error)

type ManagerKubeApplier interface {
	ApplyGroup(ctx context.Context, group nsxv1alpha.NSXGroup) error
	UpdateGroupStatus(ctx context.Context, name string, status nsxv1alpha.NSXGroupStatus) error
	DeleteGroupCR(ctx context.Context, name string) error
	RemoveGroupFinalizer(ctx context.Context, name string, finalizer string) error
	UpdateCloudStatus(ctx context.Context, name string, status nsxv1alpha.NSXNetworkCloudStatus) error
}

type ManagerSnapshot struct {
	Cloud            nsxv1alpha.NSXNetworkCloud
	NetworkCloudFQDN string
	LocalGroups      []nsxv1alpha.NSXGroup
	RemoteGroups     []RemoteGroup
	GatherError      error
}

type ManagerPlan struct {
	ObserveUpserts           []nsxv1alpha.NSXGroup
	ManagedWrites            []ManagedGroupWrite
	ManagedDeletes           []ManagedGroupDelete
	ManagedFinalizerRemovals []string
	GroupStatuses            []GroupStatusPlan
	ObserveDeletes           []string
	CloudStatus              *CloudStatusPlan
}

type ManagerBindings struct {
	Local       []LocalBinding
	Remote      []RemoteBinding
	LocalByKey  map[BindingKey]nsxv1alpha.NSXGroup
	RemoteByKey map[BindingKey]RemoteGroup
}

type LocalBinding struct {
	Key   BindingKey
	Group nsxv1alpha.NSXGroup
}

type RemoteBinding struct {
	Key    BindingKey
	Remote RemoteGroup
}

type ManagedGroupWrite struct {
	Name                  string
	Key                   BindingKey
	DisplayName           string
	CIDRs                 []string
	SegmentPath           *string
	IPAddressExpressionID string
	PathExpressionID      string
}

type ManagedGroupDelete struct {
	GroupID string
}

type GroupStatusPlan struct {
	Name   string
	Status nsxv1alpha.NSXGroupStatus
}

type CloudStatusPlan struct {
	Name   string
	Status nsxv1alpha.NSXNetworkCloudStatus
}

func BuildBindings(snapshot ManagerSnapshot) (ManagerBindings, error) {
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
	for _, group := range localGroups {
		key := BindingKey{NetworkCloudFQDN: group.Spec.NetworkCloudFQDN, GroupID: group.Spec.GroupID}
		if _, exists := bindings.LocalByKey[key]; exists {
			return ManagerBindings{}, fmt.Errorf("duplicate local binding %s/%s", key.NetworkCloudFQDN, key.GroupID)
		}
		bindings.Local = append(bindings.Local, LocalBinding{Key: key, Group: group})
		bindings.LocalByKey[key] = group
	}
	for _, remote := range remoteGroups {
		if _, exists := bindings.RemoteByKey[remote.Key]; exists {
			return ManagerBindings{}, fmt.Errorf("duplicate remote binding %s/%s", remote.Key.NetworkCloudFQDN, remote.Key.GroupID)
		}
		bindings.Remote = append(bindings.Remote, RemoteBinding{Key: remote.Key, Remote: remote})
		bindings.RemoteByKey[remote.Key] = remote
	}
	return bindings, nil
}

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
		remote.UnsupportedExpression = true
	}
	seenIPExpression := false
	seenPathExpression := false
	for _, raw := range group.Expression {
		var header struct {
			ResourceType string `json:"resource_type"`
		}
		if err := json.Unmarshal(raw, &header); err != nil {
			remote.UnsupportedExpression = true
			continue
		}
		switch header.ResourceType {
		case "IPAddressExpression":
			if seenIPExpression {
				remote.UnsupportedExpression = true
				continue
			}
			seenIPExpression = true
			var expression nsxclient.IPAddressExpression
			if err := json.Unmarshal(raw, &expression); err != nil {
				remote.UnsupportedExpression = true
				continue
			}
			remote.CIDRs = append([]string(nil), expression.IPAddresses...)
			remote.IPAddressExpressionID = expression.ID
		case "PathExpression":
			if seenPathExpression {
				remote.UnsupportedExpression = true
				continue
			}
			seenPathExpression = true
			var expression nsxclient.PathExpression
			if err := json.Unmarshal(raw, &expression); err != nil {
				remote.UnsupportedExpression = true
				continue
			}
			if len(expression.Paths) != 1 {
				remote.UnsupportedExpression = true
				if len(expression.Paths) == 0 {
					continue
				}
			}
			remote.SegmentPath = copyStringPointer(&expression.Paths[0])
			remote.PathExpressionID = expression.ID
		case "ConjunctionOperator":
			var expression struct {
				ConjunctionOperator string `json:"conjunction_operator"`
			}
			if err := json.Unmarshal(raw, &expression); err != nil {
				remote.UnsupportedExpression = true
				continue
			}
			if expression.ConjunctionOperator != "OR" {
				remote.UnsupportedExpression = true
			}
		default:
			remote.UnsupportedExpression = true
		}
	}
	return remote
}

func GatherManagerSnapshot(
	ctx context.Context,
	cloud nsxv1alpha.NSXNetworkCloud,
	listGroups GroupListFunc,
	managerClientFactory ManagerClientFactory,
) (ManagerSnapshot, error) {
	normalizedFQDN := names.NormalizeNetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN)
	snapshot := ManagerSnapshot{
		Cloud:            cloud,
		NetworkCloudFQDN: normalizedFQDN,
	}
	if listGroups == nil {
		return ManagerSnapshot{}, errors.New("group list function is required")
	}
	if managerClientFactory == nil {
		return ManagerSnapshot{}, errors.New("manager client factory is required")
	}
	localGroups, err := listGroups(ctx, kubeapi.ListOptions{
		Filters: []kubeapi.FieldFilter{
			kubeapi.FilterBy(kubeapi.FieldNetworkCloudFQDN, normalizedFQDN),
		},
	})
	if err != nil {
		snapshot.GatherError = fmt.Errorf("list local nsx groups for %q: %w", normalizedFQDN, err)
		return snapshot, nil
	}
	snapshot.LocalGroups = append([]nsxv1alpha.NSXGroup(nil), localGroups.Items...)

	managerClient, err := managerClientFactory(ctx, cloud)
	if err != nil {
		snapshot.GatherError = fmt.Errorf("construct nsx manager client for %q: %w", normalizedFQDN, err)
		return snapshot, nil
	}
	remoteGroups, err := managerClient.ListGroups(ctx)
	if err != nil {
		snapshot.GatherError = fmt.Errorf("list remote nsx groups for %q: %w", normalizedFQDN, err)
		return snapshot, nil
	}
	snapshot.RemoteGroups = make([]RemoteGroup, 0, len(remoteGroups))
	for _, remoteGroup := range remoteGroups {
		if remoteGroup == nil {
			continue
		}
		snapshot.RemoteGroups = append(snapshot.RemoteGroups, RemoteGroupFromNSXGroup(normalizedFQDN, *remoteGroup))
	}
	return snapshot, nil
}

func ProcessManagerSnapshot(snapshot ManagerSnapshot, now time.Time) (ManagerPlan, error) {
	if snapshot.GatherError != nil {
		cloudStatus, err := statuscondition.BuildNetworkCloudStatus(
			snapshot.Cloud.Status,
			snapshot.Cloud.Generation,
			now,
			statuscondition.Reachable(metav1.ConditionFalse, "GatherFailed", snapshot.GatherError.Error()),
			statuscondition.Swept(metav1.ConditionFalse, "GatherFailed", snapshot.GatherError.Error()),
		)
		if err != nil {
			return ManagerPlan{}, fmt.Errorf("build gather failure cloud status: %w", err)
		}
		return ManagerPlan{
			CloudStatus: &CloudStatusPlan{
				Name:   snapshot.Cloud.Name,
				Status: cloudStatus,
			},
		}, nil
	}
	bindings, err := BuildBindings(snapshot)
	if err != nil {
		return ManagerPlan{}, err
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
		return ManagerPlan{}, fmt.Errorf("build successful cloud status: %w", err)
	}
	plan.CloudStatus = &CloudStatusPlan{Name: snapshot.Cloud.Name, Status: cloudStatus}
	for _, remoteBinding := range bindings.Remote {
		if _, exists := bindings.LocalByKey[remoteBinding.Key]; exists {
			continue
		}
		name := names.NSXGroupName(names.NSXGroupLogicalID{
			NetworkCloudFQDN: remoteBinding.Key.NetworkCloudFQDN,
			GroupID:          remoteBinding.Key.GroupID,
		})
		plan.ObserveUpserts = append(plan.ObserveUpserts, observeGroupFromRemote(name, remoteBinding.Remote))
		status, err := syncedRemoteStatus(nsxv1alpha.NSXGroupStatus{}, 0, remoteBinding.Remote, now)
		if err != nil {
			return ManagerPlan{}, fmt.Errorf("build observe import status %q: %w", name, err)
		}
		plan.GroupStatuses = append(plan.GroupStatuses, GroupStatusPlan{
			Name:   name,
			Status: status,
		})
	}
	for _, localBinding := range bindings.Local {
		remote, exists := bindings.RemoteByKey[localBinding.Key]
		switch localBinding.Group.Spec.Mode {
		case nsxv1alpha.NSXGroupModeObserve:
			if !exists {
				plan.ObserveDeletes = append(plan.ObserveDeletes, localBinding.Group.Name)
				continue
			}
			remoteSpec := observeSpecFromRemote(remote)
			if !groupSpecsEqual(localBinding.Group.Spec, remoteSpec) {
				plan.ObserveUpserts = append(plan.ObserveUpserts, observeGroupFromRemote(localBinding.Group.Name, remote))
			}
			status, err := syncedRemoteStatus(localBinding.Group.Status, localBinding.Group.Generation, remote, now)
			if err != nil {
				return ManagerPlan{}, fmt.Errorf("build observe status %q: %w", localBinding.Group.Name, err)
			}
			plan.GroupStatuses = append(plan.GroupStatuses, GroupStatusPlan{
				Name:   localBinding.Group.Name,
				Status: status,
			})
		case nsxv1alpha.NSXGroupModeManage:
			if localBinding.Group.DeletionTimestamp != nil {
				if exists {
					plan.ManagedDeletes = append(plan.ManagedDeletes, ManagedGroupDelete{GroupID: localBinding.Key.GroupID})
					status, err := deletingManageStatus(localBinding.Group.Status, localBinding.Group.Generation, remote, now)
					if err != nil {
						return ManagerPlan{}, fmt.Errorf("build deleting managed status %q: %w", localBinding.Group.Name, err)
					}
					plan.GroupStatuses = append(plan.GroupStatuses, GroupStatusPlan{
						Name:   localBinding.Group.Name,
						Status: status,
					})
					continue
				}
				if slices.Contains(localBinding.Group.Finalizers, GroupFinalizer) {
					plan.ManagedFinalizerRemovals = append(plan.ManagedFinalizerRemovals, localBinding.Group.Name)
				}
				status, err := deletedManageStatus(localBinding.Group.Status, localBinding.Group.Generation, now)
				if err != nil {
					return ManagerPlan{}, fmt.Errorf("build deleted managed status %q: %w", localBinding.Group.Name, err)
				}
				plan.GroupStatuses = append(plan.GroupStatuses, GroupStatusPlan{
					Name:   localBinding.Group.Name,
					Status: status,
				})
				continue
			}
			if !exists {
				plan.ManagedWrites = append(plan.ManagedWrites, managedWriteFromLocal(localBinding.Group, RemoteGroup{}))
				status, err := missingManageStatus(localBinding.Group.Status, localBinding.Group.Generation, now)
				if err != nil {
					return ManagerPlan{}, fmt.Errorf("build missing managed status %q: %w", localBinding.Group.Name, err)
				}
				plan.GroupStatuses = append(plan.GroupStatuses, GroupStatusPlan{
					Name:   localBinding.Group.Name,
					Status: status,
				})
				continue
			}
			if !managedSpecMatchesRemote(localBinding.Group.Spec, remote) {
				plan.ManagedWrites = append(plan.ManagedWrites, managedWriteFromLocal(localBinding.Group, remote))
				status, err := applyingManageStatus(localBinding.Group.Status, localBinding.Group.Generation, remote, now)
				if err != nil {
					return ManagerPlan{}, fmt.Errorf("build applying managed status %q: %w", localBinding.Group.Name, err)
				}
				plan.GroupStatuses = append(plan.GroupStatuses, GroupStatusPlan{
					Name:   localBinding.Group.Name,
					Status: status,
				})
				continue
			}
			status, err := matchingManageStatus(localBinding.Group.Status, localBinding.Group.Generation, remote, now)
			if err != nil {
				return ManagerPlan{}, fmt.Errorf("build matching managed status %q: %w", localBinding.Group.Name, err)
			}
			plan.GroupStatuses = append(plan.GroupStatuses, GroupStatusPlan{
				Name:   localBinding.Group.Name,
				Status: status,
			})
		}
	}
	return plan, nil
}

func ApplyManagerPlan(ctx context.Context, kubeApplier ManagerKubeApplier, managerClient ManagerClient, plan ManagerPlan) error {
	if kubeApplier == nil {
		return errors.New("kubernetes manager applier is required")
	}
	if managerClient == nil && (len(plan.ManagedWrites) > 0 || len(plan.ManagedDeletes) > 0) {
		return errors.New("manager client is required for managed writes or deletes")
	}
	for _, group := range plan.ObserveUpserts {
		if err := kubeApplier.ApplyGroup(ctx, group); err != nil {
			return fmt.Errorf("apply observe group %q: %w", group.Name, err)
		}
	}
	nsxWritesSkipped := false
	for _, write := range plan.ManagedWrites {
		if err := applyManagedWrite(ctx, managerClient, write); err != nil {
			if isWriteDisabled(err) {
				nsxWritesSkipped = true
				break
			}
			return err
		}
	}
	if !nsxWritesSkipped {
		for _, deletion := range plan.ManagedDeletes {
			if err := managerClient.DeleteGroup(ctx, deletion.GroupID); err != nil {
				if isWriteDisabled(err) {
					break
				}
				return fmt.Errorf("delete managed nsx group %q: %w", deletion.GroupID, err)
			}
		}
	}
	for _, status := range plan.GroupStatuses {
		if err := kubeApplier.UpdateGroupStatus(ctx, status.Name, status.Status); err != nil {
			return fmt.Errorf("update group status %q: %w", status.Name, err)
		}
	}
	for _, name := range plan.ManagedFinalizerRemovals {
		if err := kubeApplier.RemoveGroupFinalizer(ctx, name, GroupFinalizer); err != nil {
			return fmt.Errorf("remove managed group finalizer %q: %w", name, err)
		}
	}
	for _, name := range plan.ObserveDeletes {
		if err := kubeApplier.DeleteGroupCR(ctx, name); err != nil {
			return fmt.Errorf("delete observe group cr %q: %w", name, err)
		}
	}
	if plan.CloudStatus != nil {
		if err := kubeApplier.UpdateCloudStatus(ctx, plan.CloudStatus.Name, plan.CloudStatus.Status); err != nil {
			return fmt.Errorf("update cloud status %q: %w", plan.CloudStatus.Name, err)
		}
	}
	return nil
}

func isWriteDisabled(err error) bool {
	var writeDisabled nsxclient.WriteDisabledError
	return errors.As(err, &writeDisabled)
}

func defaultManagerSweep(
	kubeClient *kubeapi.Client,
	managerClientFactory ManagerClientFactory,
	logger *zap.Logger,
	clock Clock,
) CloudSweepFunc {
	if logger == nil {
		logger = zap.NewNop()
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
		plan, err := ProcessManagerSnapshot(snapshot, clock.Now())
		if err != nil {
			logger.Info("default manager processing failed", append(fields, zap.Error(err))...)
			return err
		}
		logger.Debug("default manager processing completed", append(
			fields,
			zap.Int("observeUpsertCount", len(plan.ObserveUpserts)),
			zap.Int("managedWriteCount", len(plan.ManagedWrites)),
			zap.Int("managedDeleteCount", len(plan.ManagedDeletes)),
			zap.Int("managedFinalizerRemovalCount", len(plan.ManagedFinalizerRemovals)),
			zap.Int("groupStatusCount", len(plan.GroupStatuses)),
			zap.Int("observeDeleteCount", len(plan.ObserveDeletes)),
			zap.Bool("cloudStatusPlanned", plan.CloudStatus != nil),
		)...)
		var managerClient ManagerClient
		if len(plan.ManagedWrites) > 0 || len(plan.ManagedDeletes) > 0 {
			managerClient, err = managerClientFactory(ctx, cloud)
			if err != nil {
				logger.Info("default manager apply client construction failed", append(fields, zap.Error(err))...)
				return fmt.Errorf("construct nsx manager client for apply: %w", err)
			}
		}
		if err := ApplyManagerPlan(ctx, kubeAPIAdapter{client: kubeClient}, managerClient, plan); err != nil {
			logger.Info("default manager apply failed", append(fields, zap.Error(err))...)
			return err
		}
		logger.Info("completed default manager sweep", fields...)
		return nil
	}
}

type kubeAPIAdapter struct {
	client *kubeapi.Client
}

func (a kubeAPIAdapter) ApplyGroup(ctx context.Context, group nsxv1alpha.NSXGroup) error {
	current, err := a.client.Groups().Get(ctx, group.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := a.client.Groups().Create(ctx, &group, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	current.Spec = group.Spec
	for _, finalizer := range group.Finalizers {
		if !slices.Contains(current.Finalizers, finalizer) {
			current.Finalizers = append(current.Finalizers, finalizer)
		}
	}
	_, err = a.client.Groups().Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (a kubeAPIAdapter) UpdateGroupStatus(ctx context.Context, name string, status nsxv1alpha.NSXGroupStatus) error {
	current, err := a.client.Groups().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	_, err = a.client.Groups().UpdateStatus(ctx, name, status, kubeapi.StatusUpdateOptions{ResourceVersion: current.ResourceVersion})
	if err != nil {
		return err
	}
	return nil
}

func (a kubeAPIAdapter) DeleteGroupCR(ctx context.Context, name string) error {
	return a.client.Groups().Delete(ctx, name, metav1.DeleteOptions{})
}

func (a kubeAPIAdapter) RemoveGroupFinalizer(ctx context.Context, name string, finalizer string) error {
	current, err := a.client.Groups().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !slices.Contains(current.Finalizers, finalizer) {
		return nil
	}
	current.Finalizers = slices.DeleteFunc(current.Finalizers, func(existing string) bool {
		return existing == finalizer
	})
	_, err = a.client.Groups().Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (a kubeAPIAdapter) UpdateCloudStatus(ctx context.Context, name string, status nsxv1alpha.NSXNetworkCloudStatus) error {
	current, err := a.client.NetworkClouds().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	_, err = a.client.NetworkClouds().UpdateStatus(ctx, name, status, kubeapi.StatusUpdateOptions{ResourceVersion: current.ResourceVersion})
	if err != nil {
		return err
	}
	return nil
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

func applyManagedWrite(ctx context.Context, managerClient ManagerClient, write ManagedGroupWrite) error {
	group := &nsxclient.GroupPatch{
		ID:           write.Key.GroupID,
		DisplayName:  write.DisplayName,
		ResourceType: "Group",
	}
	if err := managerClient.PatchGroup(ctx, write.Key.GroupID, group); err != nil {
		return fmt.Errorf("patch managed nsx group %q: %w", write.Key.GroupID, err)
	}
	if err := applyManagedIPAddressExpression(ctx, managerClient, write); err != nil {
		return err
	}
	if err := applyManagedPathExpression(ctx, managerClient, write); err != nil {
		return err
	}
	return nil
}

func applyManagedIPAddressExpression(ctx context.Context, managerClient ManagerClient, write ManagedGroupWrite) error {
	if len(write.CIDRs) == 0 {
		if write.IPAddressExpressionID != "" {
			if err := managerClient.DeleteGroupIPAddressExpression(ctx, write.Key.GroupID, write.IPAddressExpressionID); err != nil {
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
			if err := managerClient.PatchGroupIPAddressExpression(ctx, write.Key.GroupID, write.IPAddressExpressionID, expression); err != nil {
				return fmt.Errorf("patch managed nsx group %q ip expression %q: %w", write.Key.GroupID, write.IPAddressExpressionID, err)
			}
		} else {
			expression.ID = "cidrs"
			if err := managerClient.AddGroupIPAddressExpression(ctx, write.Key.GroupID, expression.ID, expression); err != nil {
				return fmt.Errorf("add managed nsx group %q ip expression %q: %w", write.Key.GroupID, expression.ID, err)
			}
		}
	}
	return nil
}

func applyManagedPathExpression(ctx context.Context, managerClient ManagerClient, write ManagedGroupWrite) error {
	if write.SegmentPath == nil {
		if write.PathExpressionID != "" {
			if err := managerClient.DeleteGroupPathExpression(ctx, write.Key.GroupID, write.PathExpressionID); err != nil {
				return fmt.Errorf("delete managed nsx group %q path expression %q: %w", write.Key.GroupID, write.PathExpressionID, err)
			}
		}
		return nil
	}
	if write.SegmentPath != nil && write.PathExpressionID != "" {
		expression := &nsxclient.PathExpressionPatch{
			ID:           write.PathExpressionID,
			ResourceType: "PathExpression",
			Paths:        []string{*write.SegmentPath},
		}
		if err := managerClient.PatchGroupPathExpression(ctx, write.Key.GroupID, write.PathExpressionID, expression); err != nil {
			return fmt.Errorf("patch managed nsx group %q path expression %q: %w", write.Key.GroupID, write.PathExpressionID, err)
		}
	}
	if write.SegmentPath != nil && write.PathExpressionID == "" {
		expression := &nsxclient.PathExpressionPatch{
			ID:           "segment",
			ResourceType: "PathExpression",
			Paths:        []string{*write.SegmentPath},
		}
		if err := managerClient.AddGroupPathExpression(ctx, write.Key.GroupID, expression.ID, expression); err != nil {
			return fmt.Errorf("add managed nsx group %q path expression %q: %w", write.Key.GroupID, expression.ID, err)
		}
	}
	return nil
}

func observeGroupFromRemote(name string, remote RemoteGroup) nsxv1alpha.NSXGroup {
	return nsxv1alpha.NSXGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Finalizers: []string{GroupFinalizer},
		},
		Spec: observeSpecFromRemote(remote),
	}
}

func observeSpecFromRemote(remote RemoteGroup) nsxv1alpha.NSXGroupSpec {
	return nsxv1alpha.NSXGroupSpec{
		NetworkCloudFQDN: names.NormalizeNetworkCloudFQDN(remote.Key.NetworkCloudFQDN),
		GroupID:          strings.TrimSpace(remote.Key.GroupID),
		DisplayName:      remote.DisplayName,
		Mode:             nsxv1alpha.NSXGroupModeObserve,
		CIDRs:            copyStringSlice(remote.CIDRs),
		SegmentPath:      copyStringPointer(remote.SegmentPath),
	}
}

func groupSpecsEqual(left nsxv1alpha.NSXGroupSpec, right nsxv1alpha.NSXGroupSpec) bool {
	return left.NetworkCloudFQDN == right.NetworkCloudFQDN &&
		left.GroupID == right.GroupID &&
		left.DisplayName == right.DisplayName &&
		left.Mode == right.Mode &&
		reflect.DeepEqual(left.CIDRs, right.CIDRs) &&
		reflect.DeepEqual(left.SegmentPath, right.SegmentPath)
}

func managedSpecMatchesRemote(local nsxv1alpha.NSXGroupSpec, remote RemoteGroup) bool {
	return local.NetworkCloudFQDN == remote.Key.NetworkCloudFQDN &&
		local.GroupID == remote.Key.GroupID &&
		local.DisplayName == remote.DisplayName &&
		reflect.DeepEqual(local.CIDRs, remote.CIDRs) &&
		reflect.DeepEqual(local.SegmentPath, remote.SegmentPath)
}

func managedWriteFromLocal(group nsxv1alpha.NSXGroup, remote RemoteGroup) ManagedGroupWrite {
	return ManagedGroupWrite{
		Name:                  group.Name,
		Key:                   BindingKey{NetworkCloudFQDN: group.Spec.NetworkCloudFQDN, GroupID: group.Spec.GroupID},
		DisplayName:           group.Spec.DisplayName,
		CIDRs:                 copyStringSlice(group.Spec.CIDRs),
		SegmentPath:           copyStringPointer(group.Spec.SegmentPath),
		IPAddressExpressionID: remote.IPAddressExpressionID,
		PathExpressionID:      remote.PathExpressionID,
	}
}

func missingManageStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, now time.Time) (nsxv1alpha.NSXGroupStatus, error) {
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

func applyingManageStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, remote RemoteGroup, now time.Time) (nsxv1alpha.NSXGroupStatus, error) {
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

func matchingManageStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, remote RemoteGroup, now time.Time) (nsxv1alpha.NSXGroupStatus, error) {
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

func deletingManageStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, remote RemoteGroup, now time.Time) (nsxv1alpha.NSXGroupStatus, error) {
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

func deletedManageStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, now time.Time) (nsxv1alpha.NSXGroupStatus, error) {
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

func syncedRemoteStatus(previous nsxv1alpha.NSXGroupStatus, observedGeneration int64, remote RemoteGroup, now time.Time) (nsxv1alpha.NSXGroupStatus, error) {
	unsupportedStatus, unsupportedReason, unsupportedMessage := unsupportedExpressionCondition(remote)
	realizedStatus, realizedReason, realizedMessage := realizedCondition(remote)
	syncedReason := "Synced"
	syncedMessage := "local group reflects remote NSX group"
	if remote.UnsupportedExpression {
		syncedReason = "UnsupportedExpression"
		syncedMessage = "remote NSX group expression needs operator support before it can be synced"
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

func unsupportedExpressionCondition(remote RemoteGroup) (metav1.ConditionStatus, string, string) {
	if remote.UnsupportedExpression {
		return metav1.ConditionTrue, "UnsupportedExpression", "remote NSX group expression is not fully representable"
	}
	return metav1.ConditionFalse, "SupportedExpression", "remote NSX group expression is representable"
}

func realizedCondition(remote RemoteGroup) (metav1.ConditionStatus, string, string) {
	switch strings.ToUpper(strings.TrimSpace(remote.Raw.State)) {
	case "", "REALIZED", "SUCCESS":
		return metav1.ConditionTrue, "Realized", "remote NSX group is realized"
	case "UNREALIZED", "ERROR", "FAILURE", "FAILED":
		return metav1.ConditionFalse, "RealizationFailed", "remote NSX group is not realized"
	default:
		return metav1.ConditionUnknown, "RealizationPending", "remote realization is still pending"
	}
}

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func copyStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}

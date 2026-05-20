// Package stateoperator plans and applies Kubernetes writes for manager reconciliation.
package stateoperator

import (
	"context"
	"fmt"
	"slices"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/logging"
	"go.uber.org/zap"
)

// ManagerKubeWritePlan groups Kubernetes writes and their ordering dependencies.
type ManagerKubeWritePlan struct {
	GroupCreates          map[kubeapi.BatchKey]kubeapi.GroupCreateRequest
	GroupUpdates          map[kubeapi.BatchKey]kubeapi.GroupUpdateRequest
	GroupDeletes          map[kubeapi.BatchKey]kubeapi.GroupDeleteRequest
	GroupStatusUpdates    map[kubeapi.BatchKey]kubeapi.GroupStatusUpdateRequest
	GroupFinalizerPatches map[kubeapi.BatchKey]kubeapi.GroupFinalizerPatchRequest
	CloudStatusUpdates    map[kubeapi.BatchKey]kubeapi.NetworkCloudStatusUpdateRequest

	GroupStatusesAfterGroupWrite    map[kubeapi.BatchKey]GroupStatusAfterGroupWrite
	GroupFinalizersAfterGroupWrite  map[kubeapi.BatchKey]GroupFinalizerAfterGroupWrite
	GroupFinalizersAfterStatusWrite map[kubeapi.BatchKey]GroupFinalizerAfterStatusWrite
}

// GroupStatusAfterGroupWrite is a status update that waits for a group write.
type GroupStatusAfterGroupWrite struct {
	Name   string
	Status nsxv1alpha.NSXGroupStatus
}

// GroupFinalizerAfterGroupWrite is a finalizer patch that waits for a group write.
type GroupFinalizerAfterGroupWrite struct {
	Name       string
	Finalizers []string
}

// GroupFinalizerAfterStatusWrite is a finalizer patch that waits for a status write.
type GroupFinalizerAfterStatusWrite struct {
	Name       string
	Finalizers []string
}

// Empty reports whether the plan contains no Kubernetes writes.
func (writes ManagerKubeWritePlan) Empty() bool {
	return len(writes.GroupCreates) == 0 &&
		len(writes.GroupUpdates) == 0 &&
		len(writes.GroupDeletes) == 0 &&
		len(writes.GroupStatusUpdates) == 0 &&
		len(writes.GroupFinalizerPatches) == 0 &&
		len(writes.CloudStatusUpdates) == 0 &&
		len(writes.GroupStatusesAfterGroupWrite) == 0 &&
		len(writes.GroupFinalizersAfterGroupWrite) == 0 &&
		len(writes.GroupFinalizersAfterStatusWrite) == 0
}

func ensureManagerKubeWrites(writes *ManagerKubeWritePlan) {
	if writes.GroupCreates == nil {
		writes.GroupCreates = map[kubeapi.BatchKey]kubeapi.GroupCreateRequest{}
	}
	if writes.GroupUpdates == nil {
		writes.GroupUpdates = map[kubeapi.BatchKey]kubeapi.GroupUpdateRequest{}
	}
	if writes.GroupDeletes == nil {
		writes.GroupDeletes = map[kubeapi.BatchKey]kubeapi.GroupDeleteRequest{}
	}
	if writes.GroupStatusUpdates == nil {
		writes.GroupStatusUpdates = map[kubeapi.BatchKey]kubeapi.GroupStatusUpdateRequest{}
	}
	if writes.GroupFinalizerPatches == nil {
		writes.GroupFinalizerPatches = map[kubeapi.BatchKey]kubeapi.GroupFinalizerPatchRequest{}
	}
	if writes.CloudStatusUpdates == nil {
		writes.CloudStatusUpdates = map[kubeapi.BatchKey]kubeapi.NetworkCloudStatusUpdateRequest{}
	}
	if writes.GroupStatusesAfterGroupWrite == nil {
		writes.GroupStatusesAfterGroupWrite = map[kubeapi.BatchKey]GroupStatusAfterGroupWrite{}
	}
	if writes.GroupFinalizersAfterGroupWrite == nil {
		writes.GroupFinalizersAfterGroupWrite = map[kubeapi.BatchKey]GroupFinalizerAfterGroupWrite{}
	}
	if writes.GroupFinalizersAfterStatusWrite == nil {
		writes.GroupFinalizersAfterStatusWrite = map[kubeapi.BatchKey]GroupFinalizerAfterStatusWrite{}
	}
}

func managerKubeObjectWrites(writes ManagerKubeWritePlan) ManagerKubeWritePlan {
	return ManagerKubeWritePlan{
		GroupCreates:                    writes.GroupCreates,
		GroupUpdates:                    writes.GroupUpdates,
		GroupStatusesAfterGroupWrite:    writes.GroupStatusesAfterGroupWrite,
		GroupFinalizersAfterGroupWrite:  writes.GroupFinalizersAfterGroupWrite,
		GroupFinalizersAfterStatusWrite: groupFinalizersAfterGeneratedStatuses(writes),
	}
}

func managerKubePostObjectWrites(writes ManagerKubeWritePlan) ManagerKubeWritePlan {
	return ManagerKubeWritePlan{
		GroupDeletes:                    writes.GroupDeletes,
		GroupStatusUpdates:              writes.GroupStatusUpdates,
		GroupFinalizerPatches:           writes.GroupFinalizerPatches,
		GroupFinalizersAfterStatusWrite: groupFinalizersAfterDirectStatuses(writes),
		CloudStatusUpdates:              writes.CloudStatusUpdates,
	}
}

func groupFinalizersAfterGeneratedStatuses(writes ManagerKubeWritePlan) map[kubeapi.BatchKey]GroupFinalizerAfterStatusWrite {
	filtered := map[kubeapi.BatchKey]GroupFinalizerAfterStatusWrite{}
	for groupWriteKey := range writes.GroupStatusesAfterGroupWrite {
		statusKey := groupBatchKey("updateStatus", "status", groupWriteKey.Name)
		if pending, ok := writes.GroupFinalizersAfterStatusWrite[statusKey]; ok {
			filtered[statusKey] = pending
		}
	}
	return filtered
}

func groupFinalizersAfterDirectStatuses(writes ManagerKubeWritePlan) map[kubeapi.BatchKey]GroupFinalizerAfterStatusWrite {
	filtered := map[kubeapi.BatchKey]GroupFinalizerAfterStatusWrite{}
	for statusKey, pending := range writes.GroupFinalizersAfterStatusWrite {
		if _, direct := writes.GroupStatusUpdates[statusKey]; direct {
			filtered[statusKey] = pending
		}
	}
	return filtered
}

func groupBatchKey(operation string, subresource string, name string) kubeapi.BatchKey {
	return kubeapi.BatchKey{Operation: operation, Resource: "nsxgroups", Subresource: subresource, Name: name}
}

func networkCloudBatchKey(operation string, subresource string, name string) kubeapi.BatchKey {
	return kubeapi.BatchKey{Operation: operation, Resource: "nsxnetworkclouds", Subresource: subresource, Name: name}
}

func addGroupDeleteRequest(plan *ManagerPlan, name string) {
	ensureManagerKubeWrites(&plan.KubeWrites)
	plan.KubeWrites.GroupDeletes[groupBatchKey("delete", "", name)] = kubeapi.GroupDeleteRequest{Name: name}
}

func addGroupFinalizerPatchRequest(plan *ManagerPlan, group *nsxv1alpha.NSXGroup, afterWrite *kubeapi.BatchKey) {
	ensureManagerKubeWrites(&plan.KubeWrites)
	finalizers := slices.DeleteFunc(copyStringSlice(group.Finalizers), func(existing string) bool {
		return existing == GroupFinalizer
	})
	if afterWrite == nil {
		key := groupBatchKey("patchFinalizers", "finalizers", group.Name)
		plan.KubeWrites.GroupFinalizerPatches[key] = kubeapi.GroupFinalizerPatchRequest{
			Name:            group.Name,
			ResourceVersion: group.ResourceVersion,
			Finalizers:      finalizers,
		}
		return
	}
	if afterWrite.Operation == "updateStatus" {
		plan.KubeWrites.GroupFinalizersAfterStatusWrite[*afterWrite] = GroupFinalizerAfterStatusWrite{
			Name:       group.Name,
			Finalizers: finalizers,
		}
		return
	}
	plan.KubeWrites.GroupFinalizersAfterGroupWrite[*afterWrite] = GroupFinalizerAfterGroupWrite{
		Name:       group.Name,
		Finalizers: finalizers,
	}
}

func groupWriteKeyOrStatusKey(groupWriteKey *kubeapi.BatchKey, statusKey *kubeapi.BatchKey) *kubeapi.BatchKey {
	if statusKey != nil {
		return statusKey
	}
	return groupWriteKey
}

func legacyManagerKubeWrites(plan *ManagerPlan) ManagerKubeWritePlan {
	writes := ManagerKubeWritePlan{}
	ensureManagerKubeWrites(&writes)
	for groupIndex := range plan.ObserveUpserts {
		groupCopy := plan.ObserveUpserts[groupIndex]
		writes.GroupCreates[groupBatchKey("create", "", groupCopy.Name)] = kubeapi.GroupCreateRequest{Object: &groupCopy}
	}
	for _, status := range plan.GroupStatuses {
		writes.GroupStatusUpdates[groupBatchKey("updateStatus", "status", status.Name)] = kubeapi.GroupStatusUpdateRequest{
			Name:   status.Name,
			Status: status.Status,
			Options: kubeapi.StatusUpdateOptions{
				ResourceVersion: status.ResourceVersion,
			},
		}
	}
	for _, name := range plan.ObserveFinalizerRemovals {
		writes.GroupFinalizerPatches[groupBatchKey("patchFinalizers", "finalizers", name)] = kubeapi.GroupFinalizerPatchRequest{Name: name}
	}
	for _, name := range plan.ManagedFinalizerRemovals {
		writes.GroupFinalizerPatches[groupBatchKey("patchFinalizers", "finalizers", name)] = kubeapi.GroupFinalizerPatchRequest{Name: name}
	}
	for _, name := range plan.ObserveDeletes {
		writes.GroupDeletes[groupBatchKey("delete", "", name)] = kubeapi.GroupDeleteRequest{Name: name}
	}
	if plan.CloudStatus != nil {
		writes.CloudStatusUpdates[networkCloudBatchKey("updateStatus", "status", plan.CloudStatus.Name)] = kubeapi.NetworkCloudStatusUpdateRequest{
			Name:   plan.CloudStatus.Name,
			Status: plan.CloudStatus.Status,
			Options: kubeapi.StatusUpdateOptions{
				ResourceVersion: plan.CloudStatus.ResourceVersion,
			},
		}
	}
	return writes
}

type kubeAPIAdapter struct {
	client *kubeapi.Client
	logger *zap.Logger
}

func (a kubeAPIAdapter) ApplyManagerKubeWrites(ctx context.Context, writes ManagerKubeWritePlan) error {
	logger := a.logger
	if logger == nil {
		logger = zap.NewNop()
	}

	if writes.Empty() {
		logger.Debug("manager kube write plan is empty")
		return nil
	}

	logger.Info(
		"applying manager kubernetes write plan",
		logging.Component("stateoperator"),
		zap.Int("groupCreateCount", len(writes.GroupCreates)),
		zap.Int("groupUpdateCount", len(writes.GroupUpdates)),
		zap.Int("groupStatusCount", len(writes.GroupStatusUpdates)+len(writes.GroupStatusesAfterGroupWrite)),
		zap.Int("groupFinalizerPatchCount", len(writes.GroupFinalizerPatches)+len(writes.GroupFinalizersAfterGroupWrite)+len(writes.GroupFinalizersAfterStatusWrite)),
		zap.Int("groupDeleteCount", len(writes.GroupDeletes)),
		zap.Int("cloudStatusCount", len(writes.CloudStatusUpdates)),
	)

	groupResults := map[kubeapi.BatchKey]*nsxv1alpha.NSXGroup{}
	createResults, _, err := a.client.Groups().CreateBatch(ctx, writes.GroupCreates)
	if err != nil {
		return fmt.Errorf("create manager groups: %w", err)
	}
	for key, result := range createResults {
		groupResults[key] = result
	}
	updateResults, _, err := a.client.Groups().UpdateBatch(ctx, writes.GroupUpdates)
	if err != nil {
		return fmt.Errorf("update manager groups: %w", err)
	}
	for key, result := range updateResults {
		groupResults[key] = result
	}

	statusRequests := copyGroupStatusRequests(writes.GroupStatusUpdates)
	for sourceKey, pendingStatus := range writes.GroupStatusesAfterGroupWrite {
		result, ok := groupResults[sourceKey]
		if !ok || result == nil {
			return fmt.Errorf("manager group status %q waits for missing group write result %#v", pendingStatus.Name, sourceKey)
		}
		statusKey := groupBatchKey("updateStatus", "status", pendingStatus.Name)
		statusRequests[statusKey] = kubeapi.GroupStatusUpdateRequest{
			Name:   pendingStatus.Name,
			Status: pendingStatus.Status,
			Options: kubeapi.StatusUpdateOptions{
				ResourceVersion: result.ResourceVersion,
			},
		}
		logger.Debug(
			"manager group status uses prior group write resource version",
			logging.Component("stateoperator"),
			zap.String("groupName", pendingStatus.Name),
			zap.String("resourceVersion", result.ResourceVersion),
		)
	}

	statusResults, _, err := a.client.Groups().UpdateStatusBatch(ctx, statusRequests)
	if err != nil {
		return fmt.Errorf("update manager group statuses: %w", err)
	}

	finalizerRequests := copyGroupFinalizerPatchRequests(writes.GroupFinalizerPatches)
	for sourceKey, pendingFinalizer := range writes.GroupFinalizersAfterGroupWrite {
		result, ok := groupResults[sourceKey]
		if !ok || result == nil {
			return fmt.Errorf("manager finalizer patch %q waits for missing group write result %#v", pendingFinalizer.Name, sourceKey)
		}
		finalizerRequests[groupBatchKey("patchFinalizers", "finalizers", pendingFinalizer.Name)] = kubeapi.GroupFinalizerPatchRequest{
			Name:            pendingFinalizer.Name,
			ResourceVersion: result.ResourceVersion,
			Finalizers:      pendingFinalizer.Finalizers,
		}
		logger.Debug(
			"manager group finalizer patch uses prior group write resource version",
			logging.Component("stateoperator"),
			zap.String("groupName", pendingFinalizer.Name),
			zap.String("resourceVersion", result.ResourceVersion),
		)
	}
	for sourceKey, pendingFinalizer := range writes.GroupFinalizersAfterStatusWrite {
		result, ok := statusResults[sourceKey]
		if !ok || result == nil {
			return fmt.Errorf("manager finalizer patch %q waits for missing status write result %#v", pendingFinalizer.Name, sourceKey)
		}
		finalizerRequests[groupBatchKey("patchFinalizers", "finalizers", pendingFinalizer.Name)] = kubeapi.GroupFinalizerPatchRequest{
			Name:            pendingFinalizer.Name,
			ResourceVersion: result.ResourceVersion,
			Finalizers:      pendingFinalizer.Finalizers,
		}
		logger.Debug(
			"manager group finalizer patch uses prior status resource version",
			logging.Component("stateoperator"),
			zap.String("groupName", pendingFinalizer.Name),
			zap.String("resourceVersion", result.ResourceVersion),
		)
	}

	_, _, err = a.client.Groups().PatchFinalizersBatch(ctx, finalizerRequests)
	if err != nil {
		return fmt.Errorf("patch manager group finalizers: %w", err)
	}
	_, _, err = a.client.Groups().DeleteBatch(ctx, writes.GroupDeletes)
	if err != nil {
		return fmt.Errorf("delete manager observe groups: %w", err)
	}
	_, _, err = a.client.NetworkClouds().UpdateStatusBatch(ctx, writes.CloudStatusUpdates)
	if err != nil {
		return fmt.Errorf("update manager cloud statuses: %w", err)
	}
	return nil
}

func copyGroupStatusRequests(requests map[kubeapi.BatchKey]kubeapi.GroupStatusUpdateRequest) map[kubeapi.BatchKey]kubeapi.GroupStatusUpdateRequest {
	copied := make(map[kubeapi.BatchKey]kubeapi.GroupStatusUpdateRequest, len(requests))
	for key := range requests {
		copied[key] = requests[key]
	}
	return copied
}

func copyGroupFinalizerPatchRequests(requests map[kubeapi.BatchKey]kubeapi.GroupFinalizerPatchRequest) map[kubeapi.BatchKey]kubeapi.GroupFinalizerPatchRequest {
	copied := make(map[kubeapi.BatchKey]kubeapi.GroupFinalizerPatchRequest, len(requests))
	for key := range requests {
		copied[key] = requests[key]
	}
	return copied
}

package stateoperator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/logging"
	"github.com/djosh34/nsx-operator/internal/names"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
	"github.com/djosh34/nsx-operator/internal/statuscondition"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const GroupFinalizer = "nsx.ing.com/finalizer"

type NetworkCloudReconciler struct {
	Client client.Client
	Logger *zap.Logger
}

type GroupReconciler struct {
	Client               client.Client
	ManagerClientFactory ManagerClientFactory
	Logger               *zap.Logger
	Clock                Clock
}

func (r NetworkCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	if err := ctx.Err(); err != nil {
		return reconcile.Result{}, err
	}
	if r.Client == nil {
		return reconcile.Result{}, fmt.Errorf("network cloud reconciler client is required")
	}
	logger := r.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	var cloud nsxv1alpha.NSXNetworkCloud
	if err := r.Client.Get(ctx, req.NamespacedName, &cloud); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debug(
				"network cloud reconcile skipped missing object",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			)
			return reconcile.Result{}, nil
		}
		logger.Info(
			"network cloud reconcile get failed",
			logging.Component("stateoperator"),
			logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			zap.Error(err),
		)
		return reconcile.Result{}, fmt.Errorf("get nsx network cloud %q: %w", reconcileKey(req.NamespacedName), err)
	}

	logger.Debug(
		"reconciled network cloud",
		logging.Component("stateoperator"),
		logging.ReconcileKey(reconcileKey(req.NamespacedName)),
		zap.String("networkCloudName", cloud.Name),
		logging.NetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN),
	)
	return reconcile.Result{}, nil
}

func (r GroupReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	if err := ctx.Err(); err != nil {
		return reconcile.Result{}, err
	}
	if r.Client == nil {
		return reconcile.Result{}, fmt.Errorf("group reconciler client is required")
	}
	logger := r.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	var group nsxv1alpha.NSXGroup
	if err := r.Client.Get(ctx, req.NamespacedName, &group); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debug(
				"group reconcile skipped missing object",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			)
			return reconcile.Result{}, nil
		}
		logger.Info(
			"group reconcile get failed",
			logging.Component("stateoperator"),
			logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			zap.Error(err),
		)
		return reconcile.Result{}, fmt.Errorf("get nsx group %q: %w", reconcileKey(req.NamespacedName), err)
	}

	logger.Debug(
		"reconciling group",
		logging.Component("stateoperator"),
		logging.ReconcileKey(reconcileKey(req.NamespacedName)),
		zap.String("groupName", group.Name),
		logging.NetworkCloudFQDN(group.Spec.NetworkCloudFQDN),
		zap.String("groupID", group.Spec.GroupID),
		zap.String("mode", string(group.Spec.Mode)),
	)
	if group.Spec.Mode == nsxv1alpha.NSXGroupModeObserve && group.DeletionTimestamp != nil {
		if slices.Contains(group.Finalizers, GroupFinalizer) {
			group.Finalizers = slices.DeleteFunc(group.Finalizers, func(finalizer string) bool {
				return finalizer == GroupFinalizer
			})
			if err := r.Client.Update(ctx, &group); err != nil {
				logger.Info(
					"observe group finalizer removal failed",
					logging.Component("stateoperator"),
					logging.ReconcileKey(reconcileKey(req.NamespacedName)),
					zap.String("groupName", group.Name),
					zap.Error(err),
				)
				return reconcile.Result{}, fmt.Errorf("remove observe nsx group finalizer %q: %w", group.Name, err)
			}
			logger.Info(
				"removed observe group finalizer",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("groupName", group.Name),
			)
		}
		return reconcile.Result{}, nil
	}
	if group.Spec.Mode == nsxv1alpha.NSXGroupModeManage && group.DeletionTimestamp == nil {
		if err := r.ensureGroupFinalizer(ctx, &group, logger, req); err != nil {
			return reconcile.Result{}, err
		}
		cloud, err := r.findNetworkCloud(ctx, group.Spec.NetworkCloudFQDN)
		if err != nil {
			logger.Info(
				"manage group cloud lookup failed",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("groupName", group.Name),
				logging.NetworkCloudFQDN(group.Spec.NetworkCloudFQDN),
				zap.Error(err),
			)
			return reconcile.Result{}, fmt.Errorf("find cloud for nsx group %q: %w", group.Name, err)
		}
		if r.ManagerClientFactory == nil {
			return reconcile.Result{}, fmt.Errorf("group reconciler manager client factory is required")
		}
		managerClient, err := r.ManagerClientFactory(ctx, cloud)
		if err != nil {
			logger.Info(
				"manage group nsx client construction failed",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("groupName", group.Name),
				zap.Error(err),
			)
			return reconcile.Result{}, fmt.Errorf("construct nsx manager client for group %q: %w", group.Name, err)
		}
		if err := applyManagedWrite(ctx, managerClient, managedWriteFromLocal(group, RemoteGroup{})); err != nil {
			if status, classified := r.manageApplyFailureStatus(group, err); classified {
				if statusErr := r.updateGroupStatus(ctx, &group, status); statusErr != nil {
					logger.Info(
						"manage group apply failure status update failed",
						logging.Component("stateoperator"),
						logging.ReconcileKey(reconcileKey(req.NamespacedName)),
						zap.String("groupName", group.Name),
						zap.Error(statusErr),
					)
					return reconcile.Result{}, fmt.Errorf("update manage nsx group apply failure status %q: %w", group.Name, statusErr)
				}
				logger.Info(
					"manage group apply deferred after nsx response",
					logging.Component("stateoperator"),
					logging.ReconcileKey(reconcileKey(req.NamespacedName)),
					zap.String("groupName", group.Name),
					zap.Error(err),
				)
				return reconcile.Result{}, nil
			}
			logger.Info(
				"manage group apply failed",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("groupName", group.Name),
				zap.Error(err),
			)
			return reconcile.Result{}, err
		}
		status, err := r.manageApplySubmittedStatus(group)
		if err != nil {
			return reconcile.Result{}, err
		}
		if err := r.updateGroupStatus(ctx, &group, status); err != nil {
			logger.Info(
				"manage group status update failed",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("groupName", group.Name),
				zap.Error(err),
			)
			return reconcile.Result{}, fmt.Errorf("update manage nsx group status %q: %w", group.Name, err)
		}
		logger.Info(
			"submitted manage group apply",
			logging.Component("stateoperator"),
			logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			zap.String("groupName", group.Name),
			zap.String("groupID", group.Spec.GroupID),
		)
		return reconcile.Result{}, nil
	}
	if group.Spec.Mode == nsxv1alpha.NSXGroupModeManage && group.DeletionTimestamp != nil {
		cloud, err := r.findNetworkCloud(ctx, group.Spec.NetworkCloudFQDN)
		if err != nil {
			logger.Info(
				"manage group delete cloud lookup failed",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("groupName", group.Name),
				logging.NetworkCloudFQDN(group.Spec.NetworkCloudFQDN),
				zap.Error(err),
			)
			return reconcile.Result{}, fmt.Errorf("find cloud for deleting nsx group %q: %w", group.Name, err)
		}
		if r.ManagerClientFactory == nil {
			return reconcile.Result{}, fmt.Errorf("group reconciler manager client factory is required")
		}
		managerClient, err := r.ManagerClientFactory(ctx, cloud)
		if err != nil {
			logger.Info(
				"manage group delete nsx client construction failed",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("groupName", group.Name),
				zap.Error(err),
			)
			return reconcile.Result{}, fmt.Errorf("construct nsx manager client for deleting group %q: %w", group.Name, err)
		}
		if err := managerClient.DeleteGroup(ctx, group.Spec.GroupID); err != nil {
			if status, classified := r.manageDeleteFailureStatus(group, err); classified {
				if statusErr := r.updateGroupStatus(ctx, &group, status); statusErr != nil {
					logger.Info(
						"manage group delete failure status update failed",
						logging.Component("stateoperator"),
						logging.ReconcileKey(reconcileKey(req.NamespacedName)),
						zap.String("groupName", group.Name),
						zap.Error(statusErr),
					)
					return reconcile.Result{}, fmt.Errorf("update manage nsx group delete failure status %q: %w", group.Name, statusErr)
				}
				logger.Info(
					"manage group delete deferred after nsx response",
					logging.Component("stateoperator"),
					logging.ReconcileKey(reconcileKey(req.NamespacedName)),
					zap.String("groupName", group.Name),
					zap.Error(err),
				)
				return reconcile.Result{}, nil
			}
			logger.Info(
				"manage group delete failed",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("groupName", group.Name),
				zap.Error(err),
			)
			return reconcile.Result{}, err
		}
		status, err := r.manageDeleteSubmittedStatus(group)
		if err != nil {
			return reconcile.Result{}, err
		}
		if err := r.updateGroupStatus(ctx, &group, status); err != nil {
			logger.Info(
				"manage group delete status update failed",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("groupName", group.Name),
				zap.Error(err),
			)
			return reconcile.Result{}, fmt.Errorf("update deleting nsx group status %q: %w", group.Name, err)
		}
		logger.Info(
			"submitted manage group delete",
			logging.Component("stateoperator"),
			logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			zap.String("groupName", group.Name),
			zap.String("groupID", group.Spec.GroupID),
		)
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

func (r GroupReconciler) ensureGroupFinalizer(ctx context.Context, group *nsxv1alpha.NSXGroup, logger *zap.Logger, req reconcile.Request) error {
	if slices.Contains(group.Finalizers, GroupFinalizer) {
		return nil
	}
	group.Finalizers = append(group.Finalizers, GroupFinalizer)
	if err := r.Client.Update(ctx, group); err != nil {
		logger.Info(
			"group finalizer add failed",
			logging.Component("stateoperator"),
			logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			zap.String("groupName", group.Name),
			zap.Error(err),
		)
		return fmt.Errorf("add nsx group finalizer %q: %w", group.Name, err)
	}
	logger.Debug(
		"added group finalizer",
		logging.Component("stateoperator"),
		logging.ReconcileKey(reconcileKey(req.NamespacedName)),
		zap.String("groupName", group.Name),
	)
	return nil
}

func (r GroupReconciler) findNetworkCloud(ctx context.Context, networkCloudFQDN string) (nsxv1alpha.NSXNetworkCloud, error) {
	normalizedFQDN := names.NormalizeNetworkCloudFQDN(networkCloudFQDN)
	var clouds nsxv1alpha.NSXNetworkCloudList
	if err := r.Client.List(ctx, &clouds); err != nil {
		return nsxv1alpha.NSXNetworkCloud{}, fmt.Errorf("list nsx network clouds: %w", err)
	}
	for _, cloud := range clouds.Items {
		if names.NormalizeNetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN) == normalizedFQDN {
			return cloud, nil
		}
	}
	return nsxv1alpha.NSXNetworkCloud{}, fmt.Errorf("network cloud %q not found", normalizedFQDN)
}

func (r GroupReconciler) manageApplySubmittedStatus(group nsxv1alpha.NSXGroup) (nsxv1alpha.NSXGroupStatus, error) {
	status, err := statuscondition.BuildGroupStatus(
		group.Status,
		group.Generation,
		r.now(),
		statuscondition.Applying(metav1.ConditionTrue, "Applying", "managed NSX group apply was submitted"),
		statuscondition.Synced(metav1.ConditionUnknown, metav1.ConditionUnknown, metav1.ConditionFalse, metav1.ConditionTrue, "Applying", "managed NSX group apply is awaiting sweep confirmation"),
	)
	if err != nil {
		return nsxv1alpha.NSXGroupStatus{}, fmt.Errorf("build manage apply submitted status: %w", err)
	}
	return status, nil
}

func (r GroupReconciler) manageApplyFailureStatus(group nsxv1alpha.NSXGroup, err error) (nsxv1alpha.NSXGroupStatus, bool) {
	var conflict nsxclient.ConflictError
	if errors.As(err, &conflict) {
		return r.buildManageApplyOutcomeStatus(
			group,
			metav1.ConditionFalse,
			"ApplyConflict",
			"managed NSX group apply was rejected by NSX concurrency control",
			"ApplyConflict",
			"managed NSX group apply needs a later sweep or Kubernetes event",
		)
	}
	var preconditionFailed nsxclient.PreconditionFailedError
	if errors.As(err, &preconditionFailed) {
		return r.buildManageApplyOutcomeStatus(
			group,
			metav1.ConditionFalse,
			"ApplyPreconditionFailed",
			"managed NSX group apply was rejected by NSX precondition checks",
			"ApplyPreconditionFailed",
			"managed NSX group apply needs a later sweep or Kubernetes event",
		)
	}
	var rateLimited nsxclient.RateLimitedError
	if errors.As(err, &rateLimited) {
		return r.buildManageApplyOutcomeStatus(
			group,
			metav1.ConditionUnknown,
			"ApplyRateLimited",
			"managed NSX group apply was rate limited by NSX",
			"ApplyRateLimited",
			"managed NSX group apply needs a later sweep or Kubernetes event",
		)
	}
	var serviceUnavailable nsxclient.ServiceUnavailableError
	if errors.As(err, &serviceUnavailable) {
		return r.buildManageApplyOutcomeStatus(
			group,
			metav1.ConditionUnknown,
			"ApplyUnavailable",
			"managed NSX group apply could not confirm because NSX is unavailable",
			"ApplyUnavailable",
			"managed NSX group apply needs a later sweep or Kubernetes event",
		)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return r.buildManageApplyOutcomeStatus(
			group,
			metav1.ConditionUnknown,
			"ApplyNetworkError",
			"managed NSX group apply could not confirm because of a network error",
			"ApplyNetworkError",
			"managed NSX group apply needs a later sweep or Kubernetes event",
		)
	}
	return nsxv1alpha.NSXGroupStatus{}, false
}

func (r GroupReconciler) buildManageApplyOutcomeStatus(
	group nsxv1alpha.NSXGroup,
	applyingStatus metav1.ConditionStatus,
	applyingReason string,
	applyingMessage string,
	syncedReason string,
	syncedMessage string,
) (nsxv1alpha.NSXGroupStatus, bool) {
	status, err := statuscondition.BuildGroupStatus(
		group.Status,
		group.Generation,
		r.now(),
		statuscondition.Applying(applyingStatus, applyingReason, applyingMessage),
		statuscondition.Synced(metav1.ConditionUnknown, metav1.ConditionUnknown, metav1.ConditionFalse, metav1.ConditionTrue, syncedReason, syncedMessage),
	)
	if err != nil {
		return nsxv1alpha.NSXGroupStatus{}, false
	}
	return status, true
}

func (r GroupReconciler) manageDeleteSubmittedStatus(group nsxv1alpha.NSXGroup) (nsxv1alpha.NSXGroupStatus, error) {
	status, err := statuscondition.BuildGroupStatus(
		group.Status,
		group.Generation,
		r.now(),
		statuscondition.Deleting(metav1.ConditionTrue, "Deleting", "managed NSX group delete was submitted"),
		statuscondition.Synced(metav1.ConditionUnknown, metav1.ConditionUnknown, metav1.ConditionFalse, metav1.ConditionTrue, "Deleting", "managed NSX group delete is awaiting sweep confirmation"),
	)
	if err != nil {
		return nsxv1alpha.NSXGroupStatus{}, fmt.Errorf("build manage delete submitted status: %w", err)
	}
	return status, nil
}

func (r GroupReconciler) manageDeleteFailureStatus(group nsxv1alpha.NSXGroup, err error) (nsxv1alpha.NSXGroupStatus, bool) {
	var conflict nsxclient.ConflictError
	if errors.As(err, &conflict) {
		return r.buildManageDeleteOutcomeStatus(
			group,
			metav1.ConditionFalse,
			"DeleteConflict",
			"managed NSX group delete was rejected by NSX concurrency control",
			"DeleteConflict",
			"managed NSX group delete needs a later sweep or Kubernetes event",
		)
	}
	var preconditionFailed nsxclient.PreconditionFailedError
	if errors.As(err, &preconditionFailed) {
		return r.buildManageDeleteOutcomeStatus(
			group,
			metav1.ConditionFalse,
			"DeletePreconditionFailed",
			"managed NSX group delete was rejected by NSX precondition checks",
			"DeletePreconditionFailed",
			"managed NSX group delete needs a later sweep or Kubernetes event",
		)
	}
	var rateLimited nsxclient.RateLimitedError
	if errors.As(err, &rateLimited) {
		return r.buildManageDeleteOutcomeStatus(
			group,
			metav1.ConditionUnknown,
			"DeleteRateLimited",
			"managed NSX group delete was rate limited by NSX",
			"DeleteRateLimited",
			"managed NSX group delete needs a later sweep or Kubernetes event",
		)
	}
	var serviceUnavailable nsxclient.ServiceUnavailableError
	if errors.As(err, &serviceUnavailable) {
		return r.buildManageDeleteOutcomeStatus(
			group,
			metav1.ConditionUnknown,
			"DeleteUnavailable",
			"managed NSX group delete could not confirm because NSX is unavailable",
			"DeleteUnavailable",
			"managed NSX group delete needs a later sweep or Kubernetes event",
		)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return r.buildManageDeleteOutcomeStatus(
			group,
			metav1.ConditionUnknown,
			"DeleteNetworkError",
			"managed NSX group delete could not confirm because of a network error",
			"DeleteNetworkError",
			"managed NSX group delete needs a later sweep or Kubernetes event",
		)
	}
	return nsxv1alpha.NSXGroupStatus{}, false
}

func (r GroupReconciler) buildManageDeleteOutcomeStatus(
	group nsxv1alpha.NSXGroup,
	deletingStatus metav1.ConditionStatus,
	deletingReason string,
	deletingMessage string,
	syncedReason string,
	syncedMessage string,
) (nsxv1alpha.NSXGroupStatus, bool) {
	status, err := statuscondition.BuildGroupStatus(
		group.Status,
		group.Generation,
		r.now(),
		statuscondition.Deleting(deletingStatus, deletingReason, deletingMessage),
		statuscondition.Synced(metav1.ConditionUnknown, metav1.ConditionUnknown, metav1.ConditionFalse, metav1.ConditionTrue, syncedReason, syncedMessage),
	)
	if err != nil {
		return nsxv1alpha.NSXGroupStatus{}, false
	}
	return status, true
}

func (r GroupReconciler) updateGroupStatus(ctx context.Context, group *nsxv1alpha.NSXGroup, status nsxv1alpha.NSXGroupStatus) error {
	group.Status = status
	if err := r.Client.Status().Update(ctx, group); err != nil {
		return err
	}
	return nil
}

func (r GroupReconciler) now() time.Time {
	clock := r.Clock
	if clock == nil {
		clock = realClock{}
	}
	return clock.Now()
}

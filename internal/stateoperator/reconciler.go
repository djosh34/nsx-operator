// Package stateoperator contains controller-runtime reconcilers for NSX custom resources.
package stateoperator

import (
	"context"

	"github.com/djosh34/nsx-operator/internal/logging"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GroupFinalizer protects managed NSX groups until remote cleanup completes.
const GroupFinalizer = "nsx.ing.com/finalizer"

// NetworkCloudReconciler observes NSXNetworkCloud changes.
type NetworkCloudReconciler struct {
	Logger *zap.Logger
	Runner ReconcilePassRunner
}

// GroupReconciler observes NSXGroup changes.
type GroupReconciler struct {
	Logger *zap.Logger
	Runner ReconcilePassRunner
}

var (
	_ reconcile.Reconciler = (*NetworkCloudReconciler)(nil)
	_ reconcile.Reconciler = (*GroupReconciler)(nil)
)

// Reconcile logs observed network cloud changes.
//
//projectlint:allow struct-error-return controller-runtime reconcile.Reconciler requires reconcile.Result by value
func (r *NetworkCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	err := ctx.Err()
	if err != nil {
		return reconcile.Result{}, err
	}
	logger := r.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	logger.Debug(
		"observed network cloud reconcile event",
		logging.Component("stateoperator"),
		logging.ReconcileKey(reconcileKey(req.NamespacedName)),
		zap.String("networkCloudName", req.Name),
	)
	if r.Runner != nil {
		trigger := ReconcileTrigger{
			Kind: ReconcileTriggerNetworkCloud,
			Name: req.Name,
		}
		logger.Debug(
			"starting network cloud reconcile pass",
			logging.Component("stateoperator"),
			logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			zap.String("triggerKind", string(trigger.Kind)),
			zap.String("networkCloudName", trigger.Name),
		)
		err = r.Runner.RunReconcilePass(ctx, trigger)
		if err != nil {
			logger.Info(
				"network cloud reconcile pass failed",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("triggerKind", string(trigger.Kind)),
				zap.String("networkCloudName", trigger.Name),
				zap.Error(err),
			)
			return reconcile.Result{}, err
		}
		logger.Debug(
			"completed network cloud reconcile pass",
			logging.Component("stateoperator"),
			logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			zap.String("triggerKind", string(trigger.Kind)),
			zap.String("networkCloudName", trigger.Name),
		)
	}
	return reconcile.Result{}, nil
}

// Reconcile applies or cleans up one NSXGroup.
//
//projectlint:allow struct-error-return controller-runtime reconcile.Reconciler requires reconcile.Result by value
func (r *GroupReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	err := ctx.Err()
	if err != nil {
		return reconcile.Result{}, err
	}
	logger := r.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	logger.Debug(
		"observed group reconcile event",
		logging.Component("stateoperator"),
		logging.ReconcileKey(reconcileKey(req.NamespacedName)),
		zap.String("groupName", req.Name),
	)
	if r.Runner != nil {
		trigger := ReconcileTrigger{
			Kind: ReconcileTriggerGroup,
			Name: req.Name,
		}
		logger.Debug(
			"starting group reconcile pass",
			logging.Component("stateoperator"),
			logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			zap.String("triggerKind", string(trigger.Kind)),
			zap.String("groupName", trigger.Name),
		)
		err = r.Runner.RunReconcilePass(ctx, trigger)
		if err != nil {
			logger.Info(
				"group reconcile pass failed",
				logging.Component("stateoperator"),
				logging.ReconcileKey(reconcileKey(req.NamespacedName)),
				zap.String("triggerKind", string(trigger.Kind)),
				zap.String("groupName", trigger.Name),
				zap.Error(err),
			)
			return reconcile.Result{}, err
		}
		logger.Debug(
			"completed group reconcile pass",
			logging.Component("stateoperator"),
			logging.ReconcileKey(reconcileKey(req.NamespacedName)),
			zap.String("triggerKind", string(trigger.Kind)),
			zap.String("groupName", trigger.Name),
		)
	}
	return reconcile.Result{}, nil
}

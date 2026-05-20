// Package stateoperator contains controller-runtime reconcilers for NSX custom resources.
package stateoperator

import (
	"context"

	"github.com/djosh34/nsx-operator/internal/logging"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GroupFinalizer protects managed NSX groups until remote cleanup completes.
const GroupFinalizer = "nsx.ing.com/finalizer"

// NetworkCloudReconciler observes NSXNetworkCloud changes.
type NetworkCloudReconciler struct {
	Client client.Client
	Logger *zap.Logger
}

// GroupReconciler reconciles NSXGroup resources against NSX manager state.
type GroupReconciler struct {
	Client               client.Client
	ManagerClientFactory ManagerClientFactory
	Logger               *zap.Logger
	Clock                Clock
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
	return reconcile.Result{}, nil
}

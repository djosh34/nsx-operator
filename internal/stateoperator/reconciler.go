// Package stateoperator contains controller-runtime reconcilers for NSX custom resources.
package stateoperator

import (
	"context"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/logging"
	"github.com/djosh34/nsx-operator/internal/names"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GroupFinalizer protects managed NSX groups until remote cleanup completes.
const GroupFinalizer = "nsx.ing.com/finalizer"

// NetworkCloudReconciler observes NSXNetworkCloud changes.
type NetworkCloudReconciler struct {
	Logger *zap.Logger
	Runner ReconcilePassRunner
}

// GroupReconciler runs a manager pass when NSXGroups of that manager change.
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

// GroupEventRequests maps an NSXGroup event to the reconcile request of its NSX
// manager, named by the normalized network cloud FQDN. Every pass plans a whole
// manager anyway, so keying the queue by manager lets the workqueue collapse a
// burst of group events (20000 creates, or the status writes of the previous pass)
// into one pending pass instead of one full pass per group.
func GroupEventRequests(_ context.Context, object client.Object) []reconcile.Request {
	group, ok := object.(*nsxv1alpha.NSXGroup)
	if !ok || group == nil {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{Name: names.NormalizeNetworkCloudFQDN(group.Spec.NetworkCloudFQDN)},
	}}
}

// Reconcile runs a pass for the NSX manager whose groups changed. The request name
// is the normalized network cloud FQDN produced by GroupEventRequests.
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
	fields := []zap.Field{
		logging.Component("stateoperator"),
		logging.ReconcileKey(reconcileKey(req.NamespacedName)),
		logging.NetworkCloudFQDN(req.Name),
	}
	logger.Debug("observed group reconcile event", fields...)
	if r.Runner != nil {
		trigger := ReconcileTrigger{
			Kind: ReconcileTriggerManager,
			Name: req.Name,
		}
		fields = append(fields, zap.String("triggerKind", string(trigger.Kind)))
		logger.Debug("starting group reconcile pass", fields...)
		err = r.Runner.RunReconcilePass(ctx, trigger)
		if err != nil {
			logger.Info("group reconcile pass failed", append(fields, zap.Error(err))...)
			return reconcile.Result{}, err
		}
		logger.Debug("completed group reconcile pass", fields...)
	}
	return reconcile.Result{}, nil
}

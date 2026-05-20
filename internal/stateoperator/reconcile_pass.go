package stateoperator

import (
	"context"
	"errors"
	"fmt"
	"sync"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/logging"
	"github.com/djosh34/nsx-operator/internal/names"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
	"github.com/djosh34/nsx-operator/internal/operatormetrics"
	"go.uber.org/zap"
)

// ReconcileTriggerKind identifies the source of one reconcile pass.
type ReconcileTriggerKind string

const (
	// ReconcileTriggerSweep runs a full periodic sweep.
	ReconcileTriggerSweep ReconcileTriggerKind = "sweep"
	// ReconcileTriggerNetworkCloud runs a pass scoped to one NSXNetworkCloud event.
	ReconcileTriggerNetworkCloud ReconcileTriggerKind = "networkCloud"
	// ReconcileTriggerGroup runs a pass scoped to one NSXGroup event.
	ReconcileTriggerGroup ReconcileTriggerKind = "group"
)

// ReconcileTrigger describes the event or sweep that requested a reconcile pass.
type ReconcileTrigger struct {
	Kind  ReconcileTriggerKind
	Name  string
	Sweep SweepContext
}

// ReconcilePassRunner owns gather, process, and apply for one reconciliation pass.
type ReconcilePassRunner interface {
	RunReconcilePass(ctx context.Context, trigger ReconcileTrigger) error
}

// ReconcilePassKubeClient is the Kubernetes boundary for one reconcile pass.
type ReconcilePassKubeClient interface {
	ListNetworkClouds(ctx context.Context) (*nsxv1alpha.NSXNetworkCloudList, error)
	ListGroups(ctx context.Context) (*nsxv1alpha.NSXGroupList, error)
	ApplyManagerKubeWrites(ctx context.Context, writes ManagerKubeWritePlan) (*ManagerKubeApplyResult, error)
}

// ReconcilePassRunnerOptions configures the default gather/process/apply runner.
type ReconcilePassRunnerOptions struct {
	KubeClient           ReconcilePassKubeClient
	ManagerClientFactory ManagerClientFactory
	Logger               *zap.Logger
	Clock                Clock
	Recorder             operatormetrics.Recorder
}

// DefaultReconcilePassRunner executes one gather/process/apply pass for sweeps and events.
type DefaultReconcilePassRunner struct {
	kubeClient           ReconcilePassKubeClient
	managerClientFactory ManagerClientFactory
	logger               *zap.Logger
	clock                Clock
	recorder             operatormetrics.Recorder
}

var _ ReconcilePassRunner = (*DefaultReconcilePassRunner)(nil)

// NewDefaultReconcilePassRunner constructs the production reconcile pass runner.
func NewDefaultReconcilePassRunner(options ReconcilePassRunnerOptions) *DefaultReconcilePassRunner {
	logger := options.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	clock := options.Clock
	if clock == nil {
		clock = &realClock{}
	}
	recorder := options.Recorder
	if recorder == nil {
		recorder = &operatormetrics.NopRecorder{}
	}
	return &DefaultReconcilePassRunner{
		kubeClient:           options.KubeClient,
		managerClientFactory: options.ManagerClientFactory,
		logger:               logger,
		clock:                clock,
		recorder:             recorder,
	}
}

// RunReconcilePass executes one gather/process/apply reconciliation pass.
func (r *DefaultReconcilePassRunner) RunReconcilePass(ctx context.Context, trigger ReconcileTrigger) error {
	if r.kubeClient == nil {
		return errors.New("reconcile pass kubernetes client is required")
	}
	if r.managerClientFactory == nil {
		return errors.New("reconcile pass manager client factory is required")
	}
	fields := reconcileTriggerLogFields(trigger)
	r.logger.Info("starting reconcile pass gather", fields...)
	clouds, err := r.kubeClient.ListNetworkClouds(ctx)
	if err != nil {
		r.logger.Info("reconcile pass cloud gather failed", append(fields, zap.Error(err))...)
		return fmt.Errorf("list reconcile pass network clouds: %w", err)
	}
	groups, err := r.kubeClient.ListGroups(ctx)
	if err != nil {
		r.logger.Info("reconcile pass group gather failed", append(fields, zap.Error(err))...)
		return fmt.Errorf("list reconcile pass groups: %w", err)
	}
	r.logger.Debug(
		"completed reconcile pass kubernetes gather",
		append(fields, zap.Int("cloudCount", len(clouds.Items)), zap.Int("groupCount", len(groups.Items)))...,
	)

	selectedClouds := selectReconcilePassClouds(trigger, clouds.Items, groups.Items, r.logger)
	r.logger.Debug("selected reconcile pass clouds", append(fields, zap.Int("selectedCloudCount", len(selectedClouds)))...)
	if trigger.Kind == ReconcileTriggerSweep {
		var waitGroup sync.WaitGroup
		waitGroup.Add(len(selectedClouds))
		for cloudIndex := range selectedClouds {
			go func(cloud *nsxv1alpha.NSXNetworkCloud) {
				defer waitGroup.Done()
				runErr := r.runCloudPass(ctx, trigger, cloud, groups.Items)
				if runErr != nil {
					r.logger.Info(
						"reconcile pass sweep cloud failed",
						append(
							reconcileTriggerLogFields(trigger),
							zap.String("networkCloudName", cloud.Name),
							logging.NetworkCloudFQDN(names.NormalizeNetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN)),
							zap.Error(runErr),
						)...,
					)
				}
			}(&selectedClouds[cloudIndex])
		}
		waitGroup.Wait()
		r.logger.Info("completed reconcile pass", append(fields, zap.Int("selectedCloudCount", len(selectedClouds)))...)
		return nil
	}
	for cloudIndex := range selectedClouds {
		runErr := r.runCloudPass(ctx, trigger, &selectedClouds[cloudIndex], groups.Items)
		if runErr != nil {
			return runErr
		}
	}
	r.logger.Info("completed reconcile pass", append(fields, zap.Int("selectedCloudCount", len(selectedClouds)))...)
	return nil
}

func (r *DefaultReconcilePassRunner) runCloudPass(
	ctx context.Context,
	trigger ReconcileTrigger,
	cloud *nsxv1alpha.NSXNetworkCloud,
	allGroups []nsxv1alpha.NSXGroup,
) error {
	normalizedFQDN := names.NormalizeNetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN)
	fields := append(
		reconcileTriggerLogFields(trigger),
		logging.NetworkCloudFQDN(normalizedFQDN),
		zap.String("networkCloudName", cloud.Name),
	)
	r.logger.Info("starting reconcile pass cloud processing", fields...)
	localGroups := localGroupsForCloud(allGroups, normalizedFQDN)
	snapshot := ManagerSnapshot{
		Cloud:            *cloud,
		NetworkCloudFQDN: normalizedFQDN,
		LocalGroups:      localGroups,
	}
	managerClient, err := r.managerClientFactory(ctx, *cloud)
	if err != nil {
		snapshot.GatherError = fmt.Errorf("construct nsx manager client for %q: %w", normalizedFQDN, err)
	} else {
		remoteGroups, listErr := managerClient.ListGroups(ctx)
		if listErr != nil {
			snapshot.GatherError = fmt.Errorf("list remote nsx groups for %q: %w", normalizedFQDN, listErr)
		} else {
			snapshot.RemoteGroups = remoteGroupsFromManagerList(normalizedFQDN, remoteGroups)
		}
	}
	r.logger.Debug(
		"completed reconcile pass cloud gather",
		append(
			fields,
			zap.Int("localGroupCount", len(snapshot.LocalGroups)),
			zap.Int("remoteGroupCount", len(snapshot.RemoteGroups)),
			zap.Bool("gatherFailed", snapshot.GatherError != nil),
		)...,
	)
	for remoteIndex := range snapshot.RemoteGroups {
		remote := &snapshot.RemoteGroups[remoteIndex]
		if !remote.HasUnsupportedExpression() {
			continue
		}
		r.logger.Debug("reconcile pass remote group has unsupported expression", append(
			fields,
			logging.GroupID(remote.Key.GroupID),
			zap.String("unsupportedReason", string(remote.UnsupportedReason)),
		)...)
	}
	plan, err := ProcessManagerSnapshot(snapshot, r.clock.Now())
	if err != nil {
		r.logger.Info("reconcile pass cloud processing failed", append(fields, zap.Error(err))...)
		return err
	}
	r.logger.Debug(
		"completed reconcile pass cloud processing",
		append(
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
		)...,
	)
	logManagerStatusWriteDecisions(r.logger, fields, plan.statusWriteDecisions)
	if snapshot.GatherError == nil {
		metricsSnapshot, metricsErr := managerMetricsSnapshot(&snapshot, plan)
		if metricsErr != nil {
			r.logger.Info("reconcile pass metrics summary failed", append(fields, zap.Error(metricsErr))...)
			return metricsErr
		}
		r.recorder.SetManagerGroupSnapshot(normalizedFQDN, *metricsSnapshot)
	}
	err = ApplyManagerPlan(ctx, r.kubeClient, managerClient, *plan)
	if err != nil {
		r.logger.Info("reconcile pass cloud apply failed", append(fields, zap.Error(err))...)
		return err
	}
	r.logger.Info("completed reconcile pass cloud", fields...)
	return nil
}

func selectReconcilePassClouds(
	trigger ReconcileTrigger,
	clouds []nsxv1alpha.NSXNetworkCloud,
	groups []nsxv1alpha.NSXGroup,
	logger *zap.Logger,
) []nsxv1alpha.NSXNetworkCloud {
	switch trigger.Kind {
	case ReconcileTriggerSweep:
		return append([]nsxv1alpha.NSXNetworkCloud(nil), clouds...)
	case ReconcileTriggerNetworkCloud:
		for cloudIndex := range clouds {
			cloud := clouds[cloudIndex]
			if cloud.Name == trigger.Name {
				return []nsxv1alpha.NSXNetworkCloud{cloud}
			}
		}
		logger.Debug("reconcile pass network cloud trigger object missing", reconcileTriggerLogFields(trigger)...)
		return nil
	case ReconcileTriggerGroup:
		for groupIndex := range groups {
			group := groups[groupIndex]
			if group.Name != trigger.Name {
				continue
			}
			groupFQDN := names.NormalizeNetworkCloudFQDN(group.Spec.NetworkCloudFQDN)
			for cloudIndex := range clouds {
				cloud := clouds[cloudIndex]
				if names.NormalizeNetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN) == groupFQDN {
					return []nsxv1alpha.NSXNetworkCloud{cloud}
				}
			}
			logger.Debug("reconcile pass group trigger cloud missing", append(reconcileTriggerLogFields(trigger), logging.NetworkCloudFQDN(groupFQDN))...)
			return nil
		}
		logger.Debug("reconcile pass group trigger object missing", reconcileTriggerLogFields(trigger)...)
		return nil
	default:
		logger.Debug("reconcile pass trigger kind ignored", reconcileTriggerLogFields(trigger)...)
		return nil
	}
}

func localGroupsForCloud(groups []nsxv1alpha.NSXGroup, normalizedFQDN string) []nsxv1alpha.NSXGroup {
	localGroups := make([]nsxv1alpha.NSXGroup, 0)
	for groupIndex := range groups {
		group := groups[groupIndex]
		if names.NormalizeNetworkCloudFQDN(group.Spec.NetworkCloudFQDN) != normalizedFQDN {
			continue
		}
		localGroups = append(localGroups, group)
	}
	return localGroups
}

func remoteGroupsFromManagerList(normalizedFQDN string, remoteGroups []*nsxclient.Group) []RemoteGroup {
	normalized := make([]RemoteGroup, 0, len(remoteGroups))
	for _, remoteGroup := range remoteGroups {
		if remoteGroup == nil {
			continue
		}
		normalized = append(normalized, RemoteGroupFromNSXGroup(normalizedFQDN, *remoteGroup))
	}
	return normalized
}

func reconcileTriggerLogFields(trigger ReconcileTrigger) []zap.Field {
	fields := []zap.Field{
		logging.Component("stateoperator"),
		zap.String("triggerKind", string(trigger.Kind)),
	}
	if trigger.Name != "" {
		fields = append(fields, zap.String("triggerName", trigger.Name))
	}
	if trigger.Sweep.ID != "" {
		fields = append(fields, logging.SweepID(trigger.Sweep.ID))
	}
	return fields
}

type reconcilePassKubeAPIAdapter struct {
	client *kubeapi.Client
	logger *zap.Logger
}

var _ ReconcilePassKubeClient = (*reconcilePassKubeAPIAdapter)(nil)

// NewKubeReconcilePassClient adapts the typed Kubernetes client to the reconcile pass boundary.
func NewKubeReconcilePassClient(client *kubeapi.Client, logger *zap.Logger) ReconcilePassKubeClient {
	return &reconcilePassKubeAPIAdapter{client: client, logger: logger}
}

func (a *reconcilePassKubeAPIAdapter) ListNetworkClouds(ctx context.Context) (*nsxv1alpha.NSXNetworkCloudList, error) {
	if a.client == nil {
		return nil, errors.New("typed kubernetes client is required")
	}
	return a.client.NetworkClouds().List(ctx, kubeapi.ListOptions{})
}

func (a *reconcilePassKubeAPIAdapter) ListGroups(ctx context.Context) (*nsxv1alpha.NSXGroupList, error) {
	if a.client == nil {
		return nil, errors.New("typed kubernetes client is required")
	}
	return a.client.Groups().List(ctx, kubeapi.ListOptions{})
}

func (a *reconcilePassKubeAPIAdapter) ApplyManagerKubeWrites(ctx context.Context, writes ManagerKubeWritePlan) (*ManagerKubeApplyResult, error) {
	return (&kubeAPIAdapter{client: a.client, logger: a.logger}).ApplyManagerKubeWrites(ctx, writes)
}

package stateoperator

import (
	"context"
	"fmt"
	"sync"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/logging"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type CloudSweepFunc func(ctx context.Context, cloud nsxv1alpha.NSXNetworkCloud, sweep SweepContext) error

type SweepContext struct {
	ID string
}

type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

type SweepIDGenerator interface {
	NewSweepID() string
}

type Options struct {
	Client               client.Client
	KubeClient           *kubeapi.Client
	TickInterval         time.Duration
	Logger               *zap.Logger
	SweepCloud           CloudSweepFunc
	ManagerClientFactory ManagerClientFactory
	Clock                Clock
	IDGenerator          SweepIDGenerator
}

type NSXStateOperator struct {
	client       client.Client
	tickInterval time.Duration
	logger       *zap.Logger
	sweepCloud   CloudSweepFunc
	clock        Clock
	idGenerator  SweepIDGenerator
}

func New(options Options) (*NSXStateOperator, error) {
	if options.Client == nil {
		return nil, fmt.Errorf("state operator client is required")
	}
	if options.TickInterval <= 0 {
		return nil, fmt.Errorf("state operator tick interval must be positive")
	}

	logger := options.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	sweepCloud := options.SweepCloud
	clock := options.Clock
	if clock == nil {
		clock = realClock{}
	}
	if sweepCloud == nil {
		if options.KubeClient != nil && options.ManagerClientFactory != nil {
			sweepCloud = defaultManagerSweep(options.KubeClient, options.ManagerClientFactory, logger, clock)
		} else {
			sweepCloud = func(context.Context, nsxv1alpha.NSXNetworkCloud, SweepContext) error {
				return nil
			}
		}
	}

	idGenerator := options.IDGenerator
	if idGenerator == nil {
		idGenerator = timestampSweepIDGenerator{clock: clock}
	}

	return &NSXStateOperator{
		client:       options.Client,
		tickInterval: options.TickInterval,
		logger:       logger,
		sweepCloud:   sweepCloud,
		clock:        clock,
		idGenerator:  idGenerator,
	}, nil
}

func (o *NSXStateOperator) Start(ctx context.Context) error {
	anchor := o.clock.Now()
	o.logger.Info("starting state operator sweeper", logging.Component("stateoperator"), zap.Duration("tick_interval", o.tickInterval))

	for {
		if err := o.runSweep(ctx); err != nil {
			return err
		}

		now := o.clock.Now()
		next := nextFutureTick(anchor, o.tickInterval, now)
		timer := o.clock.NewTimer(next.Sub(now))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C():
				default:
				}
			}
			o.logger.Info("stopping state operator sweeper", logging.Component("stateoperator"), zap.Error(ctx.Err()))
			return nil
		case <-timer.C():
		}
	}
}

func (o *NSXStateOperator) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	if err := ctx.Err(); err != nil {
		return reconcile.Result{}, err
	}
	o.logger.Debug("received reconcile request", logging.Component("stateoperator"), logging.ReconcileKey(reconcileKey(req.NamespacedName)))
	return reconcile.Result{}, nil
}

func (o *NSXStateOperator) runSweep(ctx context.Context) error {
	sweep := SweepContext{ID: o.idGenerator.NewSweepID()}
	o.logger.Info("starting global sweep", logging.Component("stateoperator"), logging.SweepID(sweep.ID))

	var clouds nsxv1alpha.NSXNetworkCloudList
	if err := o.client.List(ctx, &clouds); err != nil {
		o.logger.Info("global sweep list failed", logging.Component("stateoperator"), logging.SweepID(sweep.ID), zap.Error(err))
		return fmt.Errorf("list nsx network clouds: %w", err)
	}

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(clouds.Items))
	for _, cloud := range clouds.Items {
		cloud := cloud
		go func() {
			defer waitGroup.Done()
			o.runCloudSweep(ctx, cloud, sweep)
		}()
	}
	waitGroup.Wait()

	o.logger.Info("completed global sweep", logging.Component("stateoperator"), logging.SweepID(sweep.ID), zap.Int("cloud_count", len(clouds.Items)))
	return nil
}

func (o *NSXStateOperator) runCloudSweep(ctx context.Context, cloud nsxv1alpha.NSXNetworkCloud, sweep SweepContext) {
	fields := []zap.Field{
		logging.Component("stateoperator"),
		logging.SweepID(sweep.ID),
		logging.NetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN),
		zap.String("networkCloudName", cloud.Name),
	}
	o.logger.Debug("starting cloud sweep", fields...)
	if err := o.sweepCloud(ctx, cloud, sweep); err != nil {
		o.logger.Info("cloud sweep failed", append(fields, zap.Error(err))...)
		return
	}
	o.logger.Debug("completed cloud sweep", fields...)
}

func nextFutureTick(anchor time.Time, interval time.Duration, now time.Time) time.Time {
	next := anchor.Add(interval)
	for !next.After(now) {
		next = next.Add(interval)
	}
	return next
}

func reconcileKey(name types.NamespacedName) string {
	if name.Namespace == "" {
		return name.Name
	}
	return name.String()
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

func (realClock) NewTimer(duration time.Duration) Timer {
	return realTimer{Timer: time.NewTimer(duration)}
}

type realTimer struct {
	*time.Timer
}

func (t realTimer) C() <-chan time.Time {
	return t.Timer.C
}

type timestampSweepIDGenerator struct {
	clock Clock
}

func (g timestampSweepIDGenerator) NewSweepID() string {
	return g.clock.Now().UTC().Format("20060102T150405.000000000Z")
}

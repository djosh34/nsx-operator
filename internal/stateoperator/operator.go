// Package stateoperator runs periodic NSX state reconciliation.
package stateoperator

import (
	"context"
	"fmt"
	"sync"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/logging"
	"github.com/djosh34/nsx-operator/internal/operatormetrics"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CloudSweepFunc reconciles one network cloud during a sweep.
type CloudSweepFunc func(ctx context.Context, cloud nsxv1alpha.NSXNetworkCloud, sweep SweepContext) error

// SweepContext carries metadata for one operator sweep.
type SweepContext struct {
	ID string
}

// Clock provides time and timer construction for the operator loop.
type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

// Timer is the timer surface used by the operator loop.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

// SweepIDGenerator creates stable identifiers for sweep log fields.
type SweepIDGenerator interface {
	NewSweepID() string
}

var (
	_ Clock            = (*realClock)(nil)
	_ Timer            = (*realTimer)(nil)
	_ SweepIDGenerator = (*timestampSweepIDGenerator)(nil)
)

// Options configures the NSX state operator.
type Options struct {
	Client               client.Client
	KubeClient           *kubeapi.Client
	TickInterval         time.Duration
	Logger               *zap.Logger
	SweepCloud           CloudSweepFunc
	ManagerClientFactory ManagerClientFactory
	Clock                Clock
	IDGenerator          SweepIDGenerator
	Recorder             operatormetrics.Recorder
}

// NSXStateOperator periodically reconciles NSX state for configured network clouds.
type NSXStateOperator struct {
	client       client.Client
	tickInterval time.Duration
	logger       *zap.Logger
	sweepCloud   CloudSweepFunc
	clock        Clock
	idGenerator  SweepIDGenerator
}

// New constructs an NSX state operator.
//
//nolint:gocritic // public operator API keeps value options so callers can pass literals.
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
	recorder := options.Recorder
	if recorder == nil {
		recorder = &operatormetrics.NopRecorder{}
	}

	sweepCloud := options.SweepCloud
	clock := options.Clock
	if clock == nil {
		clock = &realClock{}
	}
	if sweepCloud == nil {
		if options.KubeClient != nil && options.ManagerClientFactory != nil {
			sweepCloud = defaultManagerSweep(options.KubeClient, options.ManagerClientFactory, logger, clock, recorder)
		} else {
			sweepCloud = func(context.Context, nsxv1alpha.NSXNetworkCloud, SweepContext) error {
				return nil
			}
		}
	}

	idGenerator := options.IDGenerator
	if idGenerator == nil {
		idGenerator = &timestampSweepIDGenerator{clock: clock}
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

// Start runs the periodic operator loop until the context is cancelled.
func (o *NSXStateOperator) Start(ctx context.Context) error {
	anchor := o.clock.Now()
	o.logger.Info("starting state operator sweeper", logging.Component("stateoperator"), zap.Duration("tick_interval", o.tickInterval))

	for {
		err := o.runSweep(ctx)
		if err != nil {
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

func (o *NSXStateOperator) runSweep(ctx context.Context) error {
	sweep := SweepContext{ID: o.idGenerator.NewSweepID()}
	o.logger.Info("starting global sweep", logging.Component("stateoperator"), logging.SweepID(sweep.ID))

	var clouds nsxv1alpha.NSXNetworkCloudList
	err := o.client.List(ctx, &clouds)
	if err != nil {
		o.logger.Info("global sweep list failed", logging.Component("stateoperator"), logging.SweepID(sweep.ID), zap.Error(err))
		return fmt.Errorf("list nsx network clouds: %w", err)
	}

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(clouds.Items))
	for cloudIndex := range clouds.Items {
		cloud := &clouds.Items[cloudIndex]
		go func() {
			defer waitGroup.Done()
			o.runCloudSweep(ctx, cloud, sweep)
		}()
	}
	waitGroup.Wait()

	o.logger.Info("completed global sweep", logging.Component("stateoperator"), logging.SweepID(sweep.ID), zap.Int("cloud_count", len(clouds.Items)))
	return nil
}

func (o *NSXStateOperator) runCloudSweep(ctx context.Context, cloud *nsxv1alpha.NSXNetworkCloud, sweep SweepContext) {
	fields := []zap.Field{
		logging.Component("stateoperator"),
		logging.SweepID(sweep.ID),
		logging.NetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN),
		zap.String("networkCloudName", cloud.Name),
	}
	var current nsxv1alpha.NSXNetworkCloud
	err := o.client.Get(ctx, client.ObjectKey{Name: cloud.Name}, &current)
	if err != nil {
		if apierrors.IsNotFound(err) {
			o.logger.Debug("skipping cloud sweep for missing cloud", fields...)
			return
		}
		o.logger.Info("cloud sweep cloud refresh failed", append(fields, zap.Error(err))...)
		return
	}
	cloud = &current
	fields = []zap.Field{
		logging.Component("stateoperator"),
		logging.SweepID(sweep.ID),
		logging.NetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN),
		zap.String("networkCloudName", cloud.Name),
	}
	o.logger.Debug("starting cloud sweep", fields...)
	err = o.sweepCloud(ctx, *cloud, sweep)
	if err != nil {
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

func (receiver *realClock) Now() time.Time {
	return time.Now()
}

func (receiver *realClock) NewTimer(duration time.Duration) Timer {
	return &realTimer{Timer: time.NewTimer(duration)}
}

type realTimer struct {
	*time.Timer
}

func (t *realTimer) C() <-chan time.Time {
	return t.Timer.C
}

type timestampSweepIDGenerator struct {
	clock Clock
}

func (g *timestampSweepIDGenerator) NewSweepID() string {
	return g.clock.Now().UTC().Format("20060102T150405.000000000Z")
}

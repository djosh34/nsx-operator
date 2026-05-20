//nolint:gocritic,govet,noinlineerr // operator tests use large Kubernetes fixtures, nil fake-client sentinels, and log entries where value assertions keep cases readable.
package stateoperator_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/stateoperator"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestStartImmediatelySweepsAllNetworkClouds(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			networkCloud("cloud-a", "nsx-a.example.test"),
			networkCloud("cloud-b", "nsx-b.example.test"),
		).
		Build()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var swept []string
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       kubeClient,
		TickInterval: time.Hour,
		SweepCloud: func(_ context.Context, cloud nsxv1alpha.NSXNetworkCloud, _ stateoperator.SweepContext) error {
			mu.Lock()
			defer mu.Unlock()
			swept = append(swept, cloud.Spec.NetworkCloudFQDN)
			if len(swept) == 2 {
				cancel()
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = operator.Start(ctx)
	if err != nil && ctx.Err() == nil {
		t.Fatalf("Start() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	slices.Sort(swept)
	want := []string{"nsx-a.example.test", "nsx-b.example.test"}
	if !slices.Equal(swept, want) {
		t.Fatalf("swept clouds = %v, want %v", swept, want)
	}
}

func TestStartRunsSharedRunnerForSweep(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			networkCloud("cloud-a", "nsx-a.example.test"),
			networkCloud("cloud-b", "nsx-b.example.test"),
		).
		Build()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := &recordingReconcilePassRunner{afterRun: cancel}
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       kubeClient,
		TickInterval: time.Hour,
		IDGenerator:  &fixedSweepIDGenerator{id: "sweep-123"},
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = operator.Start(ctx)
	if err != nil && ctx.Err() == nil {
		t.Fatalf("Start() error = %v", err)
	}
	runner.requireSingleTrigger(t, stateoperator.ReconcileTrigger{
		Kind:  stateoperator.ReconcileTriggerSweep,
		Sweep: stateoperator.SweepContext{ID: "sweep-123"},
	})
}

func TestStartWaitsForHealthyCloudWhenAnotherCloudFails(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			networkCloud("cloud-a", "nsx-a.example.test"),
			networkCloud("cloud-b", "nsx-b.example.test"),
		).
		Build()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	failedCloudFinished := make(chan struct{})
	healthyCloudStarted := make(chan struct{})
	releaseHealthyCloud := make(chan struct{})
	healthyCloudContextStillLive := make(chan error, 1)

	operator, err := stateoperator.New(stateoperator.Options{
		Client:       kubeClient,
		TickInterval: time.Hour,
		SweepCloud: func(ctx context.Context, cloud nsxv1alpha.NSXNetworkCloud, _ stateoperator.SweepContext) error {
			switch cloud.Spec.NetworkCloudFQDN {
			case "nsx-a.example.test":
				close(failedCloudFinished)
				return fmt.Errorf("cloud failed")
			case "nsx-b.example.test":
				close(healthyCloudStarted)
				<-releaseHealthyCloud
				healthyCloudContextStillLive <- ctx.Err()
				cancel()
				return nil
			default:
				t.Fatalf("unexpected cloud %q", cloud.Spec.NetworkCloudFQDN)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- operator.Start(ctx)
	}()

	requireClosed(t, failedCloudFinished, "failed cloud to finish")
	requireClosed(t, healthyCloudStarted, "healthy cloud to start")
	requireNotClosed(t, errCh, "Start to remain blocked until healthy cloud finishes")
	close(releaseHealthyCloud)

	err = <-errCh
	if err != nil && ctx.Err() == nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := <-healthyCloudContextStillLive; err != nil {
		t.Fatalf("healthy cloud context error before release = %v, want nil", err)
	}
}

func TestStartSkipsElapsedTicksAfterLongSweep(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(networkCloud("cloud-a", "nsx-a.example.test")).
		Build()
	clock := newManualClock(time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sweepStarted := make(chan struct{})
	releaseSweep := make(chan struct{})
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       kubeClient,
		TickInterval: 10 * time.Second,
		Clock:        clock,
		SweepCloud: func(context.Context, nsxv1alpha.NSXNetworkCloud, stateoperator.SweepContext) error {
			close(sweepStarted)
			<-releaseSweep
			cancel()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- operator.Start(ctx)
	}()

	requireClosed(t, sweepStarted, "first sweep to start")
	clock.Set(time.Date(2026, 5, 19, 0, 0, 35, 0, time.UTC))
	close(releaseSweep)

	duration := clock.RequireNextTimerDuration(t)
	if duration != 5*time.Second {
		t.Fatalf("next timer duration = %v, want 5s", duration)
	}
	err = <-errCh
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestStartDoesNotOverlapSweeps(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(networkCloud("cloud-a", "nsx-a.example.test")).
		Build()
	clock := newManualClock(time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sweepStarted := make(chan struct{}, 1)
	releaseSweep := make(chan struct{})
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       kubeClient,
		TickInterval: 10 * time.Second,
		Clock:        clock,
		SweepCloud: func(context.Context, nsxv1alpha.NSXNetworkCloud, stateoperator.SweepContext) error {
			sweepStarted <- struct{}{}
			<-releaseSweep
			cancel()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- operator.Start(ctx)
	}()

	requireClosed(t, sweepStarted, "first sweep to start")
	clock.Set(time.Date(2026, 5, 19, 0, 0, 45, 0, time.UTC))
	requireNotClosed(t, sweepStarted, "second sweep to start while first sweep is blocked")
	close(releaseSweep)

	err = <-errCh
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestStartLogsSweepAndCloudFields(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(networkCloud("cloud-a", "nsx-a.example.test")).
		Build()
	core, logs := observer.New(zapcore.DebugLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	operator, err := stateoperator.New(stateoperator.Options{
		Client:       kubeClient,
		TickInterval: time.Hour,
		Logger:       zap.New(core),
		IDGenerator:  &fixedSweepIDGenerator{id: "sweep-123"},
		SweepCloud: func(context.Context, nsxv1alpha.NSXNetworkCloud, stateoperator.SweepContext) error {
			cancel()
			return fmt.Errorf("cloud failed")
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = operator.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	requireLogField(t, logs, "starting global sweep", "sweepID", "sweep-123")
	requireLogField(t, logs, "completed global sweep", "sweepID", "sweep-123")
	requireLogField(t, logs, "starting cloud sweep", "networkCloudFQDN", "nsx-a.example.test")
	requireLogField(t, logs, "cloud sweep failed", "networkCloudFQDN", "nsx-a.example.test")
	requireLogField(t, logs, "cloud sweep failed", "sweepID", "sweep-123")
}

func TestStartDoesNotQuerySameCloudTwiceInOneSweep(t *testing.T) {
	scheme := newScheme(t)
	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(networkCloud("cloud-a", "nsx-a.example.test")).
		Build()
	countingClient := newDuplicateQueryDetectingClient(baseClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	operator, err := stateoperator.New(stateoperator.Options{
		Client:       countingClient,
		TickInterval: time.Hour,
		SweepCloud: func(context.Context, nsxv1alpha.NSXNetworkCloud, stateoperator.SweepContext) error {
			cancel()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = operator.Start(ctx)
	if err != nil && ctx.Err() == nil {
		t.Fatalf("Start() error = %v", err)
	}
	if duplicates := countingClient.duplicateQueries(); len(duplicates) != 0 {
		t.Fatalf("duplicate resource queries in one sweep = %v, want none", duplicates)
	}
}

func TestNetworkCloudReconcileObservesEventWithoutClient(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	reconciler := stateoperator.NetworkCloudReconciler{Logger: zap.New(core)}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "cloud-a"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}
	requireLogField(t, logs, "observed network cloud reconcile event", "networkCloudName", "cloud-a")
}

func TestNetworkCloudReconcileRunsSharedPassRunner(t *testing.T) {
	runner := &recordingReconcilePassRunner{}
	reconciler := stateoperator.NetworkCloudReconciler{Runner: runner}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "cloud-a"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}
	runner.requireSingleTrigger(t, stateoperator.ReconcileTrigger{
		Kind: stateoperator.ReconcileTriggerNetworkCloud,
		Name: "cloud-a",
	})
}

func TestNetworkCloudReconcileCanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &recordingReconcilePassRunner{}
	reconciler := stateoperator.NetworkCloudReconciler{Runner: runner}

	_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "cloud-a"}})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want context cancellation")
	}
	if len(runner.triggers) != 0 {
		t.Fatalf("runner trigger count = %d, want 0 after context cancellation", len(runner.triggers))
	}
}

func TestNetworkCloudReconcileReturnsRunnerError(t *testing.T) {
	wantErr := errors.New("runner failed")
	reconciler := stateoperator.NetworkCloudReconciler{Runner: &recordingReconcilePassRunner{err: wantErr}}

	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "cloud-a"},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Reconcile() error = %v, want %v", err, wantErr)
	}
}

func TestGroupReconcileObservesEventWithoutClientOrNSXMutation(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	reconciler := stateoperator.GroupReconciler{Logger: zap.New(core)}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "group-a"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}
	requireLogField(t, logs, "observed group reconcile event", "groupName", "group-a")
}

func TestGroupReconcileRunsSharedPassRunner(t *testing.T) {
	runner := &recordingReconcilePassRunner{}
	reconciler := stateoperator.GroupReconciler{Runner: runner}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "group-a"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}
	runner.requireSingleTrigger(t, stateoperator.ReconcileTrigger{
		Kind: stateoperator.ReconcileTriggerGroup,
		Name: "group-a",
	})
}

func TestGroupReconcileCanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &recordingReconcilePassRunner{}
	reconciler := stateoperator.GroupReconciler{Runner: runner}

	_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "group-a"}})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want context cancellation")
	}
	if len(runner.triggers) != 0 {
		t.Fatalf("runner trigger count = %d, want 0 after context cancellation", len(runner.triggers))
	}
}

func TestGroupReconcileReturnsRunnerError(t *testing.T) {
	wantErr := errors.New("runner failed")
	reconciler := stateoperator.GroupReconciler{Runner: &recordingReconcilePassRunner{err: wantErr}}

	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "group-a"},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Reconcile() error = %v, want %v", err, wantErr)
	}
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := nsxv1alpha.AddToScheme(scheme); err != nil {
		t.Fatalf("add nsx scheme: %v", err)
	}
	return scheme
}

func requireClosed(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func requireNotClosed[T any](t *testing.T, ch <-chan T, name string) {
	t.Helper()

	select {
	case value := <-ch:
		t.Fatalf("%s closed with %v", name, value)
	case <-time.After(50 * time.Millisecond):
	}
}

type manualClock struct {
	mu             sync.Mutex
	now            time.Time
	timerDurations chan time.Duration
}

func newManualClock(now time.Time) *manualClock {
	return &manualClock{
		now:            now,
		timerDurations: make(chan time.Duration, 10),
	}
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) Set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

func (c *manualClock) NewTimer(duration time.Duration) stateoperator.Timer {
	c.timerDurations <- duration
	return &manualTimer{ch: make(chan time.Time)}
}

func (c *manualClock) RequireNextTimerDuration(t *testing.T) time.Duration {
	t.Helper()

	select {
	case duration := <-c.timerDurations:
		return duration
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for timer creation")
	}
	return 0
}

type manualTimer struct {
	ch chan time.Time
}

func (t *manualTimer) C() <-chan time.Time {
	return t.ch
}

func (t *manualTimer) Stop() bool {
	return true
}

type fixedSweepIDGenerator struct {
	id string
}

func (g *fixedSweepIDGenerator) NewSweepID() string {
	return g.id
}

type duplicateQueryDetectingClient struct {
	client.Client
	mu         sync.Mutex
	listed     map[string]struct{}
	duplicates []string
}

func newDuplicateQueryDetectingClient(base client.Client) *duplicateQueryDetectingClient {
	return &duplicateQueryDetectingClient{
		Client: base,
		listed: map[string]struct{}{},
	}
}

func (c *duplicateQueryDetectingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	err := c.Client.List(ctx, list, opts...)
	if err != nil {
		return err
	}
	c.recordListedObjects(list)
	return nil
}

func (c *duplicateQueryDetectingClient) Get(ctx context.Context, key client.ObjectKey, object client.Object, opts ...client.GetOption) error {
	resourceKey := duplicateQueryResourceKey(object, key.Name)
	c.mu.Lock()
	if _, ok := c.listed[resourceKey]; ok {
		c.duplicates = append(c.duplicates, resourceKey)
	}
	c.mu.Unlock()
	return c.Client.Get(ctx, key, object, opts...)
}

func (c *duplicateQueryDetectingClient) recordListedObjects(list client.ObjectList) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch typedList := list.(type) {
	case *nsxv1alpha.NSXNetworkCloudList:
		for itemIndex := range typedList.Items {
			c.listed[duplicateQueryResourceKey(&typedList.Items[itemIndex], typedList.Items[itemIndex].Name)] = struct{}{}
		}
	case *nsxv1alpha.NSXGroupList:
		for itemIndex := range typedList.Items {
			c.listed[duplicateQueryResourceKey(&typedList.Items[itemIndex], typedList.Items[itemIndex].Name)] = struct{}{}
		}
	}
}

func (c *duplicateQueryDetectingClient) duplicateQueries() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.duplicates...)
}

func duplicateQueryResourceKey(object client.Object, name string) string {
	switch object.(type) {
	case *nsxv1alpha.NSXNetworkCloud:
		return "NSXNetworkCloud/" + name
	case *nsxv1alpha.NSXGroup:
		return "NSXGroup/" + name
	default:
		return fmt.Sprintf("%T/%s", object, name)
	}
}

func requireLogField(t *testing.T, logs *observer.ObservedLogs, message string, key string, want string) {
	t.Helper()

	for _, entry := range logs.FilterMessage(message).All() {
		for _, field := range entry.Context {
			if field.Key == key && field.String == want {
				return
			}
		}
	}
	t.Fatalf("log %q did not contain %s=%q; logs: %v", message, key, want, logs.All())
}

func networkCloud(name string, fqdn string) *nsxv1alpha.NSXNetworkCloud {
	return &nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: nsxv1alpha.NSXNetworkCloudSpec{
			NetworkCloudFQDN: fqdn,
			NetworkCloudID:   name,
			Name:             name,
		},
	}
}

type recordingReconcilePassRunner struct {
	err      error
	afterRun func()
	triggers []stateoperator.ReconcileTrigger
}

func (r *recordingReconcilePassRunner) RunReconcilePass(_ context.Context, trigger stateoperator.ReconcileTrigger) error {
	r.triggers = append(r.triggers, trigger)
	if r.afterRun != nil {
		r.afterRun()
	}
	return r.err
}

func (r *recordingReconcilePassRunner) requireSingleTrigger(t *testing.T, want stateoperator.ReconcileTrigger) {
	t.Helper()

	if len(r.triggers) != 1 {
		t.Fatalf("runner trigger count = %d, want 1", len(r.triggers))
	}
	if r.triggers[0] != want {
		t.Fatalf("runner trigger = %#v, want %#v", r.triggers[0], want)
	}
}

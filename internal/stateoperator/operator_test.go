package stateoperator_test

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
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
		IDGenerator:  fixedSweepIDGenerator{id: "sweep-123"},
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

func TestNetworkCloudReconcileLogsCloudAndDoesNotRequeue(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(networkCloud("cloud-a", "nsx-a.example.test")).
		Build()
	core, logs := observer.New(zapcore.DebugLevel)
	reconciler := stateoperator.NetworkCloudReconciler{
		Client: kubeClient,
		Logger: zap.New(core),
	}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "cloud-a"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}
	requireLogField(t, logs, "reconciled network cloud", "networkCloudName", "cloud-a")
	requireLogField(t, logs, "reconciled network cloud", "networkCloudFQDN", "nsx-a.example.test")
}

func TestNetworkCloudReconcileMissingCloudDoesNotRequeue(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := stateoperator.NetworkCloudReconciler{Client: kubeClient}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "missing-cloud"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}
}

func TestNetworkCloudReconcileCanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reconciler := stateoperator.NetworkCloudReconciler{Client: fake.NewClientBuilder().WithScheme(newScheme(t)).Build()}

	_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "cloud-a"}})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want context cancellation")
	}
}

func TestNetworkCloudReconcileRequiresClient(t *testing.T) {
	reconciler := stateoperator.NetworkCloudReconciler{}

	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "cloud-a"}})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want missing client error")
	}
}

func TestGroupReconcileObserveDoesNotMutateNSXOrRequeue(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(managerGroup("group-a", "nsx-a.example.test", "group-a", nsxv1alpha.NSXGroupModeObserve)).
		Build()
	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			t.Fatalf("observe reconcile constructed NSX manager client")
			return nil, nil
		},
	}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "group-a"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}
}

func TestGroupReconcileMissingGroupDoesNotRequeue(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := stateoperator.GroupReconciler{Client: kubeClient}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "missing-group"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}
}

func TestGroupReconcileCanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reconciler := stateoperator.GroupReconciler{Client: fake.NewClientBuilder().WithScheme(newScheme(t)).Build()}

	_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "group-a"}})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want context cancellation")
	}
}

func TestGroupReconcileRequiresClient(t *testing.T) {
	reconciler := stateoperator.GroupReconciler{}

	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "group-a"}})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want missing client error")
	}
}

func TestGroupReconcileObserveDeletionRemovesFinalizerWithoutNSXMutation(t *testing.T) {
	deletionTime := metav1.NewTime(time.Date(2026, 5, 19, 1, 0, 0, 0, time.UTC))
	group := managerGroup("group-a", "nsx-a.example.test", "group-a", nsxv1alpha.NSXGroupModeObserve)
	group.Finalizers = []string{stateoperator.GroupFinalizer, "example.test/keep"}
	group.DeletionTimestamp = &deletionTime

	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(group).
		Build()
	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			t.Fatalf("observe deletion reconcile constructed NSX manager client")
			return nil, nil
		},
	}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "group-a"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}

	var updated nsxv1alpha.NSXGroup
	if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: "group-a"}, &updated); err != nil {
		t.Fatalf("get updated group: %v", err)
	}
	if slices.Contains(updated.Finalizers, stateoperator.GroupFinalizer) {
		t.Fatalf("finalizers = %v, want %q removed", updated.Finalizers, stateoperator.GroupFinalizer)
	}
	if !slices.Contains(updated.Finalizers, "example.test/keep") {
		t.Fatalf("finalizers = %v, want unrelated finalizer kept", updated.Finalizers)
	}
}

func TestGroupReconcileManageAppliesNSXStatusFinalizerAndDoesNotRequeue(t *testing.T) {
	now := time.Date(2026, 5, 19, 1, 15, 0, 0, time.UTC)
	clock := newManualClock(now)
	cloud := networkCloud("cloud-a", "nsx-a.example.test")
	group := managerGroup("group-a", "nsx-a.example.test", "group-a", nsxv1alpha.NSXGroupModeManage)
	group.Generation = 7
	segmentPath := "/infra/segments/web"
	group.Spec.SegmentPath = &segmentPath
	recorder := &operationRecorder{}

	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nsxv1alpha.NSXGroup{}).
		WithObjects(cloud, group).
		Build()
	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(_ context.Context, gotCloud nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			if gotCloud.Name != "cloud-a" {
				t.Fatalf("manager client cloud = %q, want cloud-a", gotCloud.Name)
			}
			return recorder, nil
		},
		Clock: clock,
	}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "group-a"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}
	wantOperations := []string{"patch-group:group-a", "add-ip:group-a:cidrs", "add-path:group-a:segment"}
	if !reflect.DeepEqual(recorder.operations, wantOperations) {
		t.Fatalf("NSX operations = %v, want %v", recorder.operations, wantOperations)
	}
	if got := recorder.pathExpressions["group-a:segment"].Paths; !reflect.DeepEqual(got, []string{segmentPath}) {
		t.Fatalf("path expression paths = %v, want desired segment path", got)
	}

	var updated nsxv1alpha.NSXGroup
	if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: "group-a"}, &updated); err != nil {
		t.Fatalf("get updated group: %v", err)
	}
	if !slices.Contains(updated.Finalizers, stateoperator.GroupFinalizer) {
		t.Fatalf("finalizers = %v, want %q added", updated.Finalizers, stateoperator.GroupFinalizer)
	}
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionApplying, metav1.ConditionTrue, "Applying", "managed NSX group apply was submitted", now)
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown, "Applying", "managed NSX group apply is awaiting sweep confirmation", now)
	requireObservedGeneration(t, updated.Status.Conditions, nsxv1alpha.ConditionApplying, 7)
	requireObservedGeneration(t, updated.Status.Conditions, nsxv1alpha.ConditionSynced, 7)
}

func TestGroupReconcileManageDeletionDeletesNSXStatusKeepsFinalizerAndDoesNotRequeue(t *testing.T) {
	now := time.Date(2026, 5, 19, 1, 30, 0, 0, time.UTC)
	deletionTime := metav1.NewTime(now)
	clock := newManualClock(now)
	cloud := networkCloud("cloud-a", "nsx-a.example.test")
	group := managerGroup("group-a", "nsx-a.example.test", "group-a", nsxv1alpha.NSXGroupModeManage)
	group.Generation = 8
	group.Finalizers = []string{stateoperator.GroupFinalizer}
	group.DeletionTimestamp = &deletionTime
	recorder := &operationRecorder{}

	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nsxv1alpha.NSXGroup{}).
		WithObjects(cloud, group).
		Build()
	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return recorder, nil
		},
		Clock: clock,
	}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "group-a"},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}
	wantOperations := []string{"delete-group:group-a"}
	if !reflect.DeepEqual(recorder.operations, wantOperations) {
		t.Fatalf("NSX operations = %v, want %v", recorder.operations, wantOperations)
	}

	var updated nsxv1alpha.NSXGroup
	if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: "group-a"}, &updated); err != nil {
		t.Fatalf("get updated group: %v", err)
	}
	if !slices.Contains(updated.Finalizers, stateoperator.GroupFinalizer) {
		t.Fatalf("finalizers = %v, want %q kept", updated.Finalizers, stateoperator.GroupFinalizer)
	}
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionDeleting, metav1.ConditionTrue, "Deleting", "managed NSX group delete was submitted", now)
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown, "Deleting", "managed NSX group delete is awaiting sweep confirmation", now)
	requireObservedGeneration(t, updated.Status.Conditions, nsxv1alpha.ConditionDeleting, 8)
	requireObservedGeneration(t, updated.Status.Conditions, nsxv1alpha.ConditionSynced, 8)
}

func TestGroupReconcileManageConflictSetsConditionsAndDoesNotRequeue(t *testing.T) {
	now := time.Date(2026, 5, 19, 1, 45, 0, 0, time.UTC)
	group, kubeClient, recorder := newManageReconcileFixture(t, now)
	recorder.patchGroupErr = nsxclient.ConflictError{StatusError: nsxclient.StatusError{StatusCode: 409, Method: "PATCH", URL: "/policy/api/v1/infra/domains/default/groups/group-a"}}

	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return recorder, nil
		},
		Clock: newManualClock(now),
	}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: group.Name},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}

	var updated nsxv1alpha.NSXGroup
	if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: group.Name}, &updated); err != nil {
		t.Fatalf("get updated group: %v", err)
	}
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionApplying, metav1.ConditionFalse, "ApplyConflict", "managed NSX group apply was rejected by NSX concurrency control", now)
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown, "ApplyConflict", "managed NSX group apply needs a later sweep or Kubernetes event", now)
}

func TestGroupReconcileManagePreconditionFailedSetsConditionsAndDoesNotRequeue(t *testing.T) {
	now := time.Date(2026, 5, 19, 2, 0, 0, 0, time.UTC)
	group, kubeClient, recorder := newManageReconcileFixture(t, now)
	recorder.patchGroupErr = nsxclient.PreconditionFailedError{StatusError: nsxclient.StatusError{StatusCode: 412, Method: "PATCH", URL: "/policy/api/v1/infra/domains/default/groups/group-a"}}

	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return recorder, nil
		},
		Clock: newManualClock(now),
	}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: group.Name},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}

	var updated nsxv1alpha.NSXGroup
	if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: group.Name}, &updated); err != nil {
		t.Fatalf("get updated group: %v", err)
	}
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionApplying, metav1.ConditionFalse, "ApplyPreconditionFailed", "managed NSX group apply was rejected by NSX precondition checks", now)
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown, "ApplyPreconditionFailed", "managed NSX group apply needs a later sweep or Kubernetes event", now)
}

func TestGroupReconcileManageRateLimitedSetsUnknownConditionsAndDoesNotRequeue(t *testing.T) {
	now := time.Date(2026, 5, 19, 2, 15, 0, 0, time.UTC)
	group, kubeClient, recorder := newManageReconcileFixture(t, now)
	recorder.patchGroupErr = nsxclient.RateLimitedError{StatusError: nsxclient.StatusError{StatusCode: 429, Method: "PATCH", URL: "/policy/api/v1/infra/domains/default/groups/group-a"}}

	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return recorder, nil
		},
		Clock: newManualClock(now),
	}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: group.Name},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}

	var updated nsxv1alpha.NSXGroup
	if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: group.Name}, &updated); err != nil {
		t.Fatalf("get updated group: %v", err)
	}
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionApplying, metav1.ConditionUnknown, "ApplyRateLimited", "managed NSX group apply was rate limited by NSX", now)
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown, "ApplyRateLimited", "managed NSX group apply needs a later sweep or Kubernetes event", now)
}

func TestGroupReconcileManageUnavailableSetsUnknownConditionsAndDoesNotRequeue(t *testing.T) {
	now := time.Date(2026, 5, 19, 2, 30, 0, 0, time.UTC)
	group, kubeClient, recorder := newManageReconcileFixture(t, now)
	recorder.patchGroupErr = nsxclient.ServiceUnavailableError{StatusError: nsxclient.StatusError{StatusCode: 503, Method: "PATCH", URL: "/policy/api/v1/infra/domains/default/groups/group-a"}}

	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return recorder, nil
		},
		Clock: newManualClock(now),
	}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: group.Name},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}

	var updated nsxv1alpha.NSXGroup
	if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: group.Name}, &updated); err != nil {
		t.Fatalf("get updated group: %v", err)
	}
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionApplying, metav1.ConditionUnknown, "ApplyUnavailable", "managed NSX group apply could not confirm because NSX is unavailable", now)
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown, "ApplyUnavailable", "managed NSX group apply needs a later sweep or Kubernetes event", now)
}

func TestGroupReconcileManageNetworkErrorSetsUnknownConditionsAndDoesNotRequeue(t *testing.T) {
	now := time.Date(2026, 5, 19, 2, 45, 0, 0, time.UTC)
	group, kubeClient, recorder := newManageReconcileFixture(t, now)
	recorder.patchGroupErr = &net.DNSError{Err: "timeout", Name: "nsx-a.example.test", IsTimeout: true}

	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return recorder, nil
		},
		Clock: newManualClock(now),
	}

	result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: group.Name},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result != (reconcile.Result{}) {
		t.Fatalf("Reconcile() result = %#v, want empty result", result)
	}

	var updated nsxv1alpha.NSXGroup
	if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: group.Name}, &updated); err != nil {
		t.Fatalf("get updated group: %v", err)
	}
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionApplying, metav1.ConditionUnknown, "ApplyNetworkError", "managed NSX group apply could not confirm because of a network error", now)
	requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown, "ApplyNetworkError", "managed NSX group apply needs a later sweep or Kubernetes event", now)
}

func TestGroupReconcileManageMissingCloudReturnsError(t *testing.T) {
	now := time.Date(2026, 5, 19, 3, 0, 0, 0, time.UTC)
	group := managerGroup("group-a", "missing.example.test", "group-a", nsxv1alpha.NSXGroupModeManage)
	kubeClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(&nsxv1alpha.NSXGroup{}).
		WithObjects(group).
		Build()
	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			t.Fatalf("manager client factory called without matching cloud")
			return nil, nil
		},
		Clock: newManualClock(now),
	}

	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: group.Name},
	})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want missing cloud error")
	}
}

func TestGroupReconcileManageClientFactoryErrorReturnsError(t *testing.T) {
	now := time.Date(2026, 5, 19, 3, 15, 0, 0, time.UTC)
	group, kubeClient, _ := newManageReconcileFixture(t, now)
	reconciler := stateoperator.GroupReconciler{
		Client: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return nil, fmt.Errorf("credentials missing")
		},
		Clock: newManualClock(now),
	}

	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: group.Name},
	})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want manager client factory error")
	}
}

func TestGroupReconcileManageDeleteClassifiedErrorsSetConditionsAndDoNotRequeue(t *testing.T) {
	cases := []struct {
		name            string
		err             error
		deletingStatus  metav1.ConditionStatus
		reason          string
		deletingMessage string
	}{
		{
			name:            "conflict",
			err:             nsxclient.ConflictError{StatusError: nsxclient.StatusError{StatusCode: 409, Method: "DELETE", URL: "/policy/api/v1/infra/domains/default/groups/group-a"}},
			deletingStatus:  metav1.ConditionFalse,
			reason:          "DeleteConflict",
			deletingMessage: "managed NSX group delete was rejected by NSX concurrency control",
		},
		{
			name:            "precondition failed",
			err:             nsxclient.PreconditionFailedError{StatusError: nsxclient.StatusError{StatusCode: 412, Method: "DELETE", URL: "/policy/api/v1/infra/domains/default/groups/group-a"}},
			deletingStatus:  metav1.ConditionFalse,
			reason:          "DeletePreconditionFailed",
			deletingMessage: "managed NSX group delete was rejected by NSX precondition checks",
		},
		{
			name:            "rate limited",
			err:             nsxclient.RateLimitedError{StatusError: nsxclient.StatusError{StatusCode: 429, Method: "DELETE", URL: "/policy/api/v1/infra/domains/default/groups/group-a"}},
			deletingStatus:  metav1.ConditionUnknown,
			reason:          "DeleteRateLimited",
			deletingMessage: "managed NSX group delete was rate limited by NSX",
		},
		{
			name:            "unavailable",
			err:             nsxclient.ServiceUnavailableError{StatusError: nsxclient.StatusError{StatusCode: 503, Method: "DELETE", URL: "/policy/api/v1/infra/domains/default/groups/group-a"}},
			deletingStatus:  metav1.ConditionUnknown,
			reason:          "DeleteUnavailable",
			deletingMessage: "managed NSX group delete could not confirm because NSX is unavailable",
		},
		{
			name:            "network",
			err:             &net.DNSError{Err: "timeout", Name: "nsx-a.example.test", IsTimeout: true},
			deletingStatus:  metav1.ConditionUnknown,
			reason:          "DeleteNetworkError",
			deletingMessage: "managed NSX group delete could not confirm because of a network error",
		},
	}

	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 5, 19, 3, 30+index, 0, 0, time.UTC)
			group, kubeClient, recorder := newManageDeleteReconcileFixture(t, now)
			recorder.deleteGroupErr = tc.err
			reconciler := stateoperator.GroupReconciler{
				Client: kubeClient,
				ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
					return recorder, nil
				},
				Clock: newManualClock(now),
			}

			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: group.Name},
			})
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if result != (reconcile.Result{}) {
				t.Fatalf("Reconcile() result = %#v, want empty result", result)
			}

			var updated nsxv1alpha.NSXGroup
			if err := kubeClient.Get(context.Background(), types.NamespacedName{Name: group.Name}, &updated); err != nil {
				t.Fatalf("get updated group: %v", err)
			}
			requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionDeleting, tc.deletingStatus, tc.reason, tc.deletingMessage, now)
			requireCondition(t, updated.Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown, tc.reason, "managed NSX group delete needs a later sweep or Kubernetes event", now)
		})
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

func newManageReconcileFixture(t *testing.T, now time.Time) (*nsxv1alpha.NSXGroup, client.Client, *operationRecorder) {
	t.Helper()

	cloud := networkCloud("cloud-a", "nsx-a.example.test")
	group := managerGroup("group-a", "nsx-a.example.test", "group-a", nsxv1alpha.NSXGroupModeManage)
	group.Generation = 9
	recorder := &operationRecorder{}

	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nsxv1alpha.NSXGroup{}).
		WithObjects(cloud, group).
		Build()
	_ = now
	return group, kubeClient, recorder
}

func newManageDeleteReconcileFixture(t *testing.T, now time.Time) (*nsxv1alpha.NSXGroup, client.Client, *operationRecorder) {
	t.Helper()

	deletionTime := metav1.NewTime(now)
	cloud := networkCloud("cloud-a", "nsx-a.example.test")
	group := managerGroup("group-a", "nsx-a.example.test", "group-a", nsxv1alpha.NSXGroupModeManage)
	group.Generation = 10
	group.Finalizers = []string{stateoperator.GroupFinalizer}
	group.DeletionTimestamp = &deletionTime
	recorder := &operationRecorder{}

	kubeClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(&nsxv1alpha.NSXGroup{}).
		WithObjects(cloud, group).
		Build()
	return group, kubeClient, recorder
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
	return manualTimer{ch: make(chan time.Time)}
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

func (t manualTimer) C() <-chan time.Time {
	return t.ch
}

func (t manualTimer) Stop() bool {
	return true
}

type fixedSweepIDGenerator struct {
	id string
}

func (g fixedSweepIDGenerator) NewSweepID() string {
	return g.id
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

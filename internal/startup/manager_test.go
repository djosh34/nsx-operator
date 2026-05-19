package startup_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/config"
	"github.com/djosh34/nsx-operator/internal/startup"
	"github.com/djosh34/nsx-operator/internal/stateoperator"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestNewManagerReturnsErrorWhenRestConfigCannotBeLoaded(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing-kubeconfig"))
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	_, err := startup.NewManager(startup.ManagerOptions{
		Config: config.Config{
			Operator: config.OperatorConfig{TickInterval: time.Second},
		},
		Logger: zap.NewNop(),
	})
	if err == nil {
		t.Fatal("NewManager() error = nil, want rest config error")
	}
	if !strings.Contains(err.Error(), "construct controller runtime rest config") {
		t.Fatalf("NewManager() error = %v, want rest config construction error", err)
	}
}

func TestNewManagerReturnsErrorForInvalidTickInterval(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}

	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{repoPath(t, "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer func() {
		if err := testEnvironment.Stop(); err != nil {
			t.Errorf("stop envtest API server: %v", err)
		}
	}()

	_, err = startup.NewManager(startup.ManagerOptions{
		Config:     config.Config{Operator: config.OperatorConfig{TickInterval: 0}},
		RestConfig: restConfig,
		Logger:     zap.NewNop(),
	})
	if err == nil {
		t.Fatal("NewManager() error = nil, want state operator construction error")
	}
	if !strings.Contains(err.Error(), "construct state operator") {
		t.Fatalf("NewManager() error = %v, want state operator construction error", err)
	}
}

func TestNewManagerUsesDefaultLogger(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}

	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{repoPath(t, "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer func() {
		if err := testEnvironment.Stop(); err != nil {
			t.Errorf("stop envtest API server: %v", err)
		}
	}()

	_, err = startup.NewManager(startup.ManagerOptions{
		Config: config.Config{
			Operator: config.OperatorConfig{TickInterval: time.Second},
		},
		RestConfig: restConfig,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
}

func TestNewManagerReturnsErrorForMalformedRestHost(t *testing.T) {
	_, err := startup.NewManager(startup.ManagerOptions{
		Config: config.Config{
			Operator: config.OperatorConfig{TickInterval: time.Second},
		},
		RestConfig: &rest.Config{Host: "http://[::1"},
		Logger:     zap.NewNop(),
	})
	if err == nil {
		t.Fatal("NewManager() error = nil, want manager construction error")
	}
	if !strings.Contains(err.Error(), "construct controller runtime manager") {
		t.Fatalf("NewManager() error = %v, want manager construction error", err)
	}
}

func TestNewManagerRegistersControllersAndPeriodicSweeper(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{repoPath(t, "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer func() {
		if err := testEnvironment.Stop(); err != nil {
			t.Errorf("stop envtest API server: %v", err)
		}
	}()

	core, logs := observer.New(zapcore.DebugLevel)
	sweptClouds := make(chan string, 10)
	manager, err := startup.NewManager(startup.ManagerOptions{
		Config: config.Config{
			Operator: config.OperatorConfig{TickInterval: 50 * time.Millisecond},
		},
		RestConfig: restConfig,
		Logger:     zap.New(core),
		SweepCloud: func(_ context.Context, cloud nsxv1alpha.NSXNetworkCloud, _ stateoperator.SweepContext) error {
			sweptClouds <- cloud.Spec.NetworkCloudFQDN
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	managerCtx, stopManager := context.WithCancel(ctx)
	defer stopManager()
	managerErr := make(chan error, 1)
	go func() {
		managerErr <- manager.Start(managerCtx)
	}()

	apiClient, err := client.New(restConfig, client.Options{Scheme: manager.GetScheme()})
	if err != nil {
		t.Fatalf("create direct client: %v", err)
	}
	if err := apiClient.Create(ctx, &nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "cloud-a"},
		Spec: nsxv1alpha.NSXNetworkCloudSpec{
			NetworkCloudFQDN: "nsx-a.example.test",
			NetworkCloudID:   "cloud-a",
			Name:             "Cloud A",
		},
	}); err != nil {
		t.Fatalf("create NSXNetworkCloud: %v", err)
	}
	if err := apiClient.Create(ctx, &nsxv1alpha.NSXGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "group-a"},
		Spec: nsxv1alpha.NSXGroupSpec{
			NetworkCloudFQDN: "nsx-a.example.test",
			GroupID:          "group-a",
			DisplayName:      "Group A",
			Mode:             nsxv1alpha.NSXGroupModeManage,
			CIDRs:            []string{"10.0.0.0/24"},
		},
	}); err != nil {
		t.Fatalf("create NSXGroup: %v", err)
	}

	requireSweptCloud(t, sweptClouds, "nsx-a.example.test")
	requireObservedLogField(ctx, t, logs, "received reconcile request", "reconcileKey", "cloud-a")
	requireObservedLogField(ctx, t, logs, "received reconcile request", "reconcileKey", "group-a")

	stopManager()
	if err := <-managerErr; err != nil {
		t.Fatalf("manager Start() error = %v", err)
	}
}

func TestNewManagerDefaultSweepUpdatesCloudStatusWithoutCustomSweep(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{repoPath(t, "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer func() {
		if err := testEnvironment.Stop(); err != nil {
			t.Errorf("stop envtest API server: %v", err)
		}
	}()

	manager, err := startup.NewManager(startup.ManagerOptions{
		Config: config.Config{
			Operator: config.OperatorConfig{TickInterval: 50 * time.Millisecond},
		},
		RestConfig: restConfig,
		Logger:     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	managerCtx, stopManager := context.WithCancel(ctx)
	defer stopManager()
	managerErr := make(chan error, 1)
	go func() {
		managerErr <- manager.Start(managerCtx)
	}()

	apiClient, err := client.New(restConfig, client.Options{Scheme: manager.GetScheme()})
	if err != nil {
		t.Fatalf("create direct client: %v", err)
	}
	if err := apiClient.Create(ctx, &nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "cloud-default"},
		Spec: nsxv1alpha.NSXNetworkCloudSpec{
			NetworkCloudFQDN: "nsx-default.example.test",
			NetworkCloudID:   "cloud-default",
			Name:             "Cloud Default",
		},
	}); err != nil {
		t.Fatalf("create NSXNetworkCloud: %v", err)
	}

	requireCloudCondition(ctx, t, apiClient, "cloud-default", nsxv1alpha.ConditionReachable, metav1.ConditionFalse)

	stopManager()
	if err := <-managerErr; err != nil {
		t.Fatalf("manager Start() error = %v", err)
	}
}

func requireSweptCloud(t *testing.T, sweptClouds <-chan string, want string) {
	t.Helper()

	timeout := time.After(10 * time.Second)
	for {
		select {
		case got := <-sweptClouds:
			if got == want {
				return
			}
		case <-timeout:
			t.Fatalf("timed out waiting for swept cloud %q", want)
		}
	}
}

func requireCloudCondition(
	ctx context.Context,
	t *testing.T,
	apiClient client.Client,
	name string,
	conditionType string,
	status metav1.ConditionStatus,
) {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var cloud nsxv1alpha.NSXNetworkCloud
		if err := apiClient.Get(ctx, client.ObjectKey{Name: name}, &cloud); err != nil {
			t.Fatalf("get NSXNetworkCloud %q: %v", name, err)
		}
		for _, condition := range cloud.Status.Conditions {
			if condition.Type == conditionType && condition.Status == status {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for cloud %q condition %s=%s; last conditions: %#v", name, conditionType, status, cloud.Status.Conditions)
		case <-ticker.C:
		}
	}
}

func requireObservedLogField(ctx context.Context, t *testing.T, logs *observer.ObservedLogs, message string, key string, want string) {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, entry := range logs.FilterMessage(message).All() {
			for _, field := range entry.Context {
				if field.Key == key && field.String == want {
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for log %q with %s=%q: %v", message, key, want, logs.All())
		case <-ticker.C:
		}
	}
}

func repoPath(t *testing.T, elements ...string) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current test file path")
	}
	parts := append([]string{filepath.Dir(filename), "..", ".."}, elements...)
	return filepath.Clean(filepath.Join(parts...))
}

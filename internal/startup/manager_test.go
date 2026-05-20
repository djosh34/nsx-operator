package startup_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
		CRDDirectoryPaths:     []string{repoPath(t, "crds")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer stopEnvtest(t, testEnvironment)

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
		CRDDirectoryPaths:     []string{repoPath(t, "crds")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer stopEnvtest(t, testEnvironment)

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
		CRDDirectoryPaths:     []string{repoPath(t, "crds")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer stopEnvtest(t, testEnvironment)

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
	err = apiClient.Create(ctx, &nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "cloud-a"},
		Spec: nsxv1alpha.NSXNetworkCloudSpec{
			NetworkCloudFQDN: "nsx-a.example.test",
			NetworkCloudID:   "cloud-a",
			Name:             "Cloud A",
		},
	})
	if err != nil {
		t.Fatalf("create NSXNetworkCloud: %v", err)
	}
	err = apiClient.Create(ctx, &nsxv1alpha.NSXGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "group-a"},
		Spec: nsxv1alpha.NSXGroupSpec{
			NetworkCloudFQDN: "nsx-a.example.test",
			GroupID:          "group-a",
			DisplayName:      "Group A",
			Mode:             nsxv1alpha.NSXGroupModeObserve,
			CIDRs:            []string{"10.0.0.0/24"},
		},
	})
	if err != nil {
		t.Fatalf("create NSXGroup: %v", err)
	}

	requireSweptCloud(t, sweptClouds, "nsx-a.example.test")
	requireObservedLogField(ctx, t, logs, "reconciled network cloud", "reconcileKey", "cloud-a")
	requireObservedLogField(ctx, t, logs, "reconciling group", "reconcileKey", "group-a")

	stopManager()
	err = <-managerErr
	if err != nil {
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
		CRDDirectoryPaths:     []string{repoPath(t, "crds")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer stopEnvtest(t, testEnvironment)

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
	err = apiClient.Create(ctx, &nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "cloud-default"},
		Spec: nsxv1alpha.NSXNetworkCloudSpec{
			NetworkCloudFQDN: "nsx-default.example.test",
			NetworkCloudID:   "cloud-default",
			Name:             "Cloud Default",
		},
	})
	if err != nil {
		t.Fatalf("create NSXNetworkCloud: %v", err)
	}

	requireCloudCondition(ctx, t, apiClient, "cloud-default", nsxv1alpha.ConditionReachable, metav1.ConditionFalse)

	stopManager()
	err = <-managerErr
	if err != nil {
		t.Fatalf("manager Start() error = %v", err)
	}
}

func TestNewManagerSharesRateLimitedTransportAcrossCloudSweeps(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gate := newNSXListGate(t, "nsx-a.example.net", "nsx-a.example.net:8443")
	server := httptest.NewTLSServer(http.HandlerFunc(gate.handleListGroups))
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse httptest server URL: %v", err)
	}
	previousDefaultTransport := http.DefaultTransport
	http.DefaultTransport = &routingTransport{
		target: serverURL,
		base:   server.Client().Transport,
	}
	t.Cleanup(func() {
		http.DefaultTransport = previousDefaultTransport
		gate.releaseSameHost()
	})

	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{repoPath(t, "crds")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer stopEnvtest(t, testEnvironment)

	manager, err := startup.NewManager(startup.ManagerOptions{
		Config: config.Config{
			NSX: config.NSXConfig{Auth: config.BasicAuth{
				Username: "nsx-admin",
				Password: "nsx-password",
			}},
			Operator: config.OperatorConfig{TickInterval: time.Minute},
			HTTPRateLimiter: config.HTTPRateLimiterConfig{
				MaxRequestsInFlightPerHost:  1,
				MaxRequestsPerSecondPerHost: 100,
			},
		},
		RestConfig: restConfig,
		Logger:     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	apiClient, err := client.New(restConfig, client.Options{Scheme: manager.GetScheme()})
	if err != nil {
		t.Fatalf("create direct client: %v", err)
	}
	createNetworkCloud(ctx, t, apiClient, "cloud-a", "nsx-a.example.net")
	createNetworkCloud(ctx, t, apiClient, "cloud-b", "nsx-a.example.net")
	createNetworkCloud(ctx, t, apiClient, "cloud-c", "nsx-a.example.net:8443")

	managerCtx, stopManager := context.WithCancel(ctx)
	defer stopManager()
	managerErr := make(chan error, 1)
	go func() {
		managerErr <- manager.Start(managerCtx)
	}()

	gate.requireFirstSameHostRequest(ctx)
	gate.requireDifferentPortRequest(ctx)
	gate.requireNoSecondSameHostRequestBeforeRelease(250 * time.Millisecond)
	gate.releaseSameHost()
	gate.requireSecondSameHostRequest(ctx)

	stopManager()
	err = <-managerErr
	if err != nil {
		t.Fatalf("manager Start() error = %v", err)
	}
}

func TestNewManagerUsesConfiguredHTTPRateLimiterLimits(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gate := newNSXListGate(t, "nsx-b.example.net", "")
	server := httptest.NewTLSServer(http.HandlerFunc(gate.handleListGroups))
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse httptest server URL: %v", err)
	}
	previousDefaultTransport := http.DefaultTransport
	http.DefaultTransport = &routingTransport{
		target: serverURL,
		base:   server.Client().Transport,
	}
	t.Cleanup(func() {
		http.DefaultTransport = previousDefaultTransport
		gate.releaseSameHost()
	})

	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{repoPath(t, "crds")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer stopEnvtest(t, testEnvironment)

	manager, err := startup.NewManager(startup.ManagerOptions{
		Config: config.Config{
			NSX: config.NSXConfig{Auth: config.BasicAuth{
				Username: "nsx-admin",
				Password: "nsx-password",
			}},
			Operator: config.OperatorConfig{TickInterval: time.Minute},
			HTTPRateLimiter: config.HTTPRateLimiterConfig{
				MaxRequestsInFlightPerHost:  2,
				MaxRequestsPerSecondPerHost: 100,
			},
		},
		RestConfig: restConfig,
		Logger:     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	apiClient, err := client.New(restConfig, client.Options{Scheme: manager.GetScheme()})
	if err != nil {
		t.Fatalf("create direct client: %v", err)
	}
	createNetworkCloud(ctx, t, apiClient, "cloud-d", "nsx-b.example.net")
	createNetworkCloud(ctx, t, apiClient, "cloud-e", "nsx-b.example.net")

	managerCtx, stopManager := context.WithCancel(ctx)
	defer stopManager()
	managerErr := make(chan error, 1)
	go func() {
		managerErr <- manager.Start(managerCtx)
	}()

	gate.requireFirstSameHostRequest(ctx)
	gate.requireSecondSameHostRequest(ctx)
	gate.releaseSameHost()

	stopManager()
	err = <-managerErr
	if err != nil {
		t.Fatalf("manager Start() error = %v", err)
	}
}

func TestNewManagerUsesConfiguredNSXURLScheme(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	transport := &schemeRecordingTransport{
		requests: make(chan recordedNSXRequest, 1),
	}
	previousDefaultTransport := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = previousDefaultTransport
	})

	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{repoPath(t, "crds")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer stopEnvtest(t, testEnvironment)

	manager, err := startup.NewManager(startup.ManagerOptions{
		Config: config.Config{
			NSX: config.NSXConfig{
				URLScheme: "http",
				Auth: config.BasicAuth{
					Username: "nsx-admin",
					Password: "nsx-password",
				},
			},
			Operator: config.OperatorConfig{TickInterval: time.Minute},
			HTTPRateLimiter: config.HTTPRateLimiterConfig{
				MaxRequestsInFlightPerHost:  2,
				MaxRequestsPerSecondPerHost: 100,
			},
		},
		RestConfig: restConfig,
		Logger:     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	apiClient, err := client.New(restConfig, client.Options{Scheme: manager.GetScheme()})
	if err != nil {
		t.Fatalf("create direct client: %v", err)
	}
	createNetworkCloud(ctx, t, apiClient, "cloud-http", "nsx-t-mockapi:8080")

	managerCtx, stopManager := context.WithCancel(ctx)
	defer stopManager()
	managerErr := make(chan error, 1)
	go func() {
		managerErr <- manager.Start(managerCtx)
	}()

	request := transport.requireRequest(ctx, t)
	if request.Scheme != "http" {
		t.Fatalf("NSX request scheme = %q, want http", request.Scheme)
	}
	if request.Host != "nsx-t-mockapi:8080" {
		t.Fatalf("NSX request host = %q, want nsx-t-mockapi:8080", request.Host)
	}

	stopManager()
	err = <-managerErr
	if err != nil {
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

func createNetworkCloud(ctx context.Context, t *testing.T, apiClient client.Client, name string, fqdn string) {
	t.Helper()

	err := apiClient.Create(ctx, &nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: nsxv1alpha.NSXNetworkCloudSpec{
			NetworkCloudFQDN: fqdn,
			NetworkCloudID:   name,
			Name:             name,
		},
	})
	if err != nil {
		t.Fatalf("create NSXNetworkCloud %q: %v", name, err)
	}
}

func stopEnvtest(t *testing.T, testEnvironment *envtest.Environment) {
	t.Helper()

	err := testEnvironment.Stop()
	if err != nil {
		t.Errorf("stop envtest API server: %v", err)
	}
}

type routingTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (transport *routingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if transport.target == nil {
		return nil, fmt.Errorf("routing transport target URL is required")
	}
	if transport.base == nil {
		return nil, fmt.Errorf("routing transport base RoundTripper is required")
	}

	routed := req.Clone(req.Context())
	routedURL := *req.URL
	routedURL.Scheme = transport.target.Scheme
	routedURL.Host = transport.target.Host
	routed.URL = &routedURL
	routed.Host = req.URL.Host
	return transport.base.RoundTrip(routed)
}

type recordedNSXRequest struct {
	Scheme string
	Host   string
	Path   string
}

type schemeRecordingTransport struct {
	requests chan recordedNSXRequest
}

func (transport *schemeRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	if req.URL == nil {
		return nil, fmt.Errorf("request URL is required")
	}
	if req.URL.Path != "/policy/api/v1/infra/domains/default/groups" {
		return nil, fmt.Errorf("unexpected NSX request path %q", req.URL.Path)
	}
	transport.requests <- recordedNSXRequest{
		Scheme: req.URL.Scheme,
		Host:   req.URL.Host,
		Path:   req.URL.Path,
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"results":[],"result_count":0}`)),
		Request:    req,
	}, nil
}

func (transport *schemeRecordingTransport) requireRequest(ctx context.Context, t *testing.T) recordedNSXRequest {
	t.Helper()

	select {
	case request := <-transport.requests:
		return request
	case <-ctx.Done():
		t.Fatalf("timed out waiting for NSX request: %v", ctx.Err())
		return recordedNSXRequest{}
	}
}

type nsxListGate struct {
	t             *testing.T
	sameHost      string
	differentPort string
	releaseOnce   sync.Once
	release       chan struct{}
	firstSame     chan struct{}
	secondSame    chan struct{}
	different     chan struct{}
	mu            sync.Mutex
	hostCounts    map[string]int
}

func newNSXListGate(t *testing.T, sameHost string, differentPort string) *nsxListGate {
	t.Helper()

	return &nsxListGate{
		t:             t,
		sameHost:      sameHost,
		differentPort: differentPort,
		release:       make(chan struct{}),
		firstSame:     make(chan struct{}, 1),
		secondSame:    make(chan struct{}, 1),
		different:     make(chan struct{}, 1),
		hostCounts:    map[string]int{},
	}
}

func (gate *nsxListGate) handleListGroups(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		return
	}
	if req.URL.Path != "/policy/api/v1/infra/domains/default/groups" {
		http.Error(w, "unexpected path", http.StatusNotFound)
		return
	}

	hostCount := gate.recordHost(req.Host)
	switch req.Host {
	case gate.sameHost:
		if hostCount == 1 {
			gate.signal(gate.firstSame)
			select {
			case <-gate.release:
			case <-req.Context().Done():
				return
			}
		}
		if hostCount == 2 {
			gate.signal(gate.secondSame)
		}
	case gate.differentPort:
		gate.signal(gate.different)
	default:
		http.Error(w, "unexpected host", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write([]byte(`{"results":[],"result_count":0}`))
	if err != nil {
		gate.t.Errorf("write NSX list response: %v", err)
	}
}

func (gate *nsxListGate) recordHost(host string) int {
	gate.mu.Lock()
	defer gate.mu.Unlock()

	gate.hostCounts[host]++
	return gate.hostCounts[host]
}

func (gate *nsxListGate) signal(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (gate *nsxListGate) requireFirstSameHostRequest(ctx context.Context) {
	gate.t.Helper()
	gate.requireSignal(ctx, gate.firstSame, "first same host request")
}

func (gate *nsxListGate) requireDifferentPortRequest(ctx context.Context) {
	gate.t.Helper()
	gate.requireSignal(ctx, gate.different, "different port request")
}

func (gate *nsxListGate) requireNoSecondSameHostRequestBeforeRelease(duration time.Duration) {
	gate.t.Helper()

	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-gate.secondSame:
		gate.t.Fatalf("second same host request reached NSX server before first same host response was released")
	case <-timer.C:
	}
}

func (gate *nsxListGate) requireSecondSameHostRequest(ctx context.Context) {
	gate.t.Helper()
	gate.requireSignal(ctx, gate.secondSame, "second same host request")
}

func (gate *nsxListGate) requireSignal(ctx context.Context, ch <-chan struct{}, description string) {
	gate.t.Helper()

	select {
	case <-ch:
	case <-ctx.Done():
		gate.t.Fatalf("timed out waiting for %s: %v", description, ctx.Err())
	}
}

func (gate *nsxListGate) releaseSameHost() {
	gate.releaseOnce.Do(func() {
		close(gate.release)
	})
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
		err := apiClient.Get(ctx, client.ObjectKey{Name: name}, &cloud)
		if err != nil {
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
		entries := logs.FilterMessage(message).All()
		for entryIndex := range entries {
			entry := entries[entryIndex]
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

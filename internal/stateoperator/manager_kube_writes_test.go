package stateoperator

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestKubeAPIAdapterUsesPriorBatchResourceVersionsWithoutGet(t *testing.T) {
	server := newManagerKubeWriteServer(t)
	kubeClient, err := kubeapi.NewClient(kubeapi.Options{
		Config: &rest.Config{Host: server.URL},
		BatchConfig: kubeapi.BatchConfig{
			NumParallelWorkers:   1,
			MaxRequestsPerSecond: 1000,
			MaxRequestsInFlight:  1,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	createKey := groupBatchKey("create", "", "created-group")
	statusKey := groupBatchKey("updateStatus", "status", "created-group")
	writes := ManagerKubeWritePlan{
		GroupCreates: map[kubeapi.BatchKey]kubeapi.GroupCreateRequest{
			createKey: {
				Object: &nsxv1alpha.NSXGroup{
					ObjectMeta: metav1.ObjectMeta{Name: "created-group"},
					Spec: nsxv1alpha.NSXGroupSpec{
						NetworkCloudFQDN: "nsx-a.example.test",
						GroupID:          "created-group",
						DisplayName:      "Created Group",
						Mode:             nsxv1alpha.NSXGroupModeObserve,
					},
				},
			},
		},
		GroupStatusesAfterGroupWrite: map[kubeapi.BatchKey]GroupStatusAfterGroupWrite{
			createKey: {
				Name: "created-group",
				Status: nsxv1alpha.NSXGroupStatus{
					UnsupportedReason: nsxv1alpha.UnsupportedExpressionReasonSupportedExpression,
				},
			},
		},
		GroupFinalizersAfterStatusWrite: map[kubeapi.BatchKey]GroupFinalizerAfterStatusWrite{
			statusKey: {
				Name:       "created-group",
				Finalizers: []string{"keep.example/finalizer"},
			},
		},
	}

	result, err := (&kubeAPIAdapter{client: kubeClient}).ApplyManagerKubeWrites(context.Background(), writes)
	if err != nil {
		t.Fatalf("ApplyManagerKubeWrites() error = %v", err)
	}
	if result == nil {
		t.Fatal("ApplyManagerKubeWrites() result = nil, want pass-level result")
	}
	if result.GroupCreates[createKey].ResourceVersion != "rv-created" {
		t.Fatalf("created resourceVersion = %q, want rv-created", result.GroupCreates[createKey].ResourceVersion)
	}
	if result.GroupStatusUpdates[statusKey].ResourceVersion != "rv-status" {
		t.Fatalf("status resourceVersion = %q, want rv-status", result.GroupStatusUpdates[statusKey].ResourceVersion)
	}

	records := server.records()
	requireManagerKubeWriteRequestCount(t, records, http.MethodGet, "/apis/nsx.ing.com/v1alpha/nsxgroups/created-group", 0)
	status := requireManagerKubeWriteRequest(t, records, http.MethodPut, "/apis/nsx.ing.com/v1alpha/nsxgroups/created-group/status")
	var statusBody nsxv1alpha.NSXGroup
	err = json.Unmarshal(status.body, &statusBody)
	if err != nil {
		t.Fatalf("decode status body: %v", err)
	}
	if statusBody.ResourceVersion != "rv-created" {
		t.Fatalf("status body resourceVersion = %q, want rv-created from create result", statusBody.ResourceVersion)
	}

	finalizerPatch := requireManagerKubeWriteRequest(t, records, http.MethodPatch, "/apis/nsx.ing.com/v1alpha/nsxgroups/created-group")
	var patch []kubeapi.JSONPatchOperation
	err = json.Unmarshal(finalizerPatch.body, &patch)
	if err != nil {
		t.Fatalf("decode finalizer patch body: %v", err)
	}
	if len(patch) != 2 {
		t.Fatalf("finalizer patch = %#v, want resourceVersion test and finalizer replacement", patch)
	}
	if patch[0].Op != "test" || patch[0].Path != "/metadata/resourceVersion" || patch[0].Value != "rv-status" {
		t.Fatalf("resourceVersion patch op = %#v, want status result resourceVersion", patch[0])
	}
}

func TestKubeAPIAdapterEmptyPlanReturnsEmptyResult(t *testing.T) {
	result, err := (&kubeAPIAdapter{}).ApplyManagerKubeWrites(context.Background(), ManagerKubeWritePlan{})
	if err != nil {
		t.Fatalf("ApplyManagerKubeWrites() error = %v", err)
	}
	if result == nil {
		t.Fatal("ApplyManagerKubeWrites() result = nil, want empty result")
	}
	if len(result.GroupCreates) != 0 || len(result.GroupUpdates) != 0 || len(result.GroupStatusUpdates) != 0 ||
		len(result.GroupFinalizerPatches) != 0 || len(result.GroupDeletes) != 0 || len(result.CloudStatusUpdates) != 0 {
		t.Fatalf("ApplyManagerKubeWrites() result = %#v, want all result buckets empty", result)
	}
}

func TestKubeAPIAdapterRejectsStatusWaitingForMissingGroupWriteResult(t *testing.T) {
	server := newManagerKubeWriteServer(t)
	kubeClient, err := kubeapi.NewClient(kubeapi.Options{
		Config: &rest.Config{Host: server.URL},
		BatchConfig: kubeapi.BatchConfig{
			NumParallelWorkers:   1,
			MaxRequestsPerSecond: 1000,
			MaxRequestsInFlight:  1,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	missingKey := groupBatchKey("update", "", "missing-group")
	_, err = (&kubeAPIAdapter{client: kubeClient}).ApplyManagerKubeWrites(context.Background(), ManagerKubeWritePlan{
		GroupStatusesAfterGroupWrite: map[kubeapi.BatchKey]GroupStatusAfterGroupWrite{
			missingKey: {Name: "missing-group"},
		},
	})
	if err == nil {
		t.Fatal("ApplyManagerKubeWrites() error = nil, want missing group write dependency error")
	}
}

type managerKubeWriteServer struct {
	*httptest.Server
	t        *testing.T
	mu       sync.Mutex
	requests []managerKubeWriteRequest
}

type managerKubeWriteRequest struct {
	method string
	path   string
	body   []byte
}

func newManagerKubeWriteServer(t *testing.T) *managerKubeWriteServer {
	t.Helper()
	server := &managerKubeWriteServer{t: t}
	server.Server = httptest.NewServer(http.HandlerFunc(server.handle))
	t.Cleanup(server.Close)
	return server
}

func (s *managerKubeWriteServer) handle(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		s.t.Errorf("read request body: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, managerKubeWriteRequest{method: req.Method, path: req.URL.Path, body: body})
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	switch {
	case req.Method == http.MethodPost && req.URL.Path == "/apis/nsx.ing.com/v1alpha/nsxgroups":
		var group nsxv1alpha.NSXGroup
		err = json.Unmarshal(body, &group)
		if err != nil {
			s.t.Errorf("decode create group: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		group.ResourceVersion = "rv-created"
		writeManagerKubeWriteJSON(s.t, w, &group)
	case req.Method == http.MethodPut && req.URL.Path == "/apis/nsx.ing.com/v1alpha/nsxgroups/created-group/status":
		var group nsxv1alpha.NSXGroup
		err = json.Unmarshal(body, &group)
		if err != nil {
			s.t.Errorf("decode status group: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		group.ResourceVersion = "rv-status"
		writeManagerKubeWriteJSON(s.t, w, &group)
	case req.Method == http.MethodPatch && req.URL.Path == "/apis/nsx.ing.com/v1alpha/nsxgroups/created-group":
		group := &nsxv1alpha.NSXGroup{ObjectMeta: metav1.ObjectMeta{Name: "created-group", ResourceVersion: "rv-finalized"}}
		writeManagerKubeWriteJSON(s.t, w, group)
	default:
		s.t.Errorf("unexpected request %s %s body=%s", req.Method, req.URL.Path, string(body))
		http.Error(w, "unexpected request", http.StatusNotFound)
	}
}

func (s *managerKubeWriteServer) records() []managerKubeWriteRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]managerKubeWriteRequest(nil), s.requests...)
}

func writeManagerKubeWriteJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	err := json.NewEncoder(w).Encode(value)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func requireManagerKubeWriteRequest(t *testing.T, records []managerKubeWriteRequest, method string, path string) managerKubeWriteRequest {
	t.Helper()
	for _, record := range records {
		if record.method == method && record.path == path {
			return record
		}
	}
	t.Fatalf("missing request %s %s in %s", method, path, formatManagerKubeWriteRecords(records))
	return managerKubeWriteRequest{}
}

func requireManagerKubeWriteRequestCount(t *testing.T, records []managerKubeWriteRequest, method string, path string, want int) {
	t.Helper()
	got := 0
	for _, record := range records {
		if record.method == method && record.path == path {
			got++
		}
	}
	if got != want {
		t.Fatalf("request count %s %s = %d, want %d in %s", method, path, got, want, formatManagerKubeWriteRecords(records))
	}
}

func formatManagerKubeWriteRecords(records []managerKubeWriteRequest) string {
	parts := make([]string, 0, len(records))
	for _, record := range records {
		parts = append(parts, record.method+" "+record.path)
	}
	return strings.Join(parts, ", ")
}

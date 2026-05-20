package operatormetrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"
)

func TestRecorderEmitsTargetedHTTPAndInventoryMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder, err := NewRecorder(registry, zap.NewNop())
	if err != nil {
		t.Fatalf("construct recorder: %v", err)
	}

	recorder.ObserveNSXCall("manager-a.example.test", "list_groups")
	recorder.ObserveNSXHTTP("manager-a.example.test", "list_groups", 12, 44, 15*time.Millisecond)
	recorder.ObserveKubernetesAPI("groups.create", 81, 125, 25*time.Millisecond)
	recorder.SetManagerGroupSnapshot("manager-a.example.test", ManagerGroupSnapshot{
		ListedGroups:         7,
		ObserveGroups:        3,
		ManageGroups:         4,
		ObserveUpdatesNeeded: 2,
		ManageUpdatesNeeded:  1,
		CreatesNeeded:        5,
	})

	expected := `
# HELP nsx_operator_kubernetes_api_bytes_total Total Kubernetes API bytes by typed client function and direction.
# TYPE nsx_operator_kubernetes_api_bytes_total counter
nsx_operator_kubernetes_api_bytes_total{direction="request",function="groups.create"} 81
nsx_operator_kubernetes_api_bytes_total{direction="response",function="groups.create"} 125
# HELP nsx_operator_kubernetes_api_calls_total Total Kubernetes API calls by typed client function.
# TYPE nsx_operator_kubernetes_api_calls_total counter
nsx_operator_kubernetes_api_calls_total{function="groups.create"} 1
# HELP nsx_operator_nsx_client_calls_total Total NSX client calls by manager and function.
# TYPE nsx_operator_nsx_client_calls_total counter
nsx_operator_nsx_client_calls_total{function="list_groups",manager="manager-a.example.test"} 1
# HELP nsx_operator_nsx_group_cr_creates_needed_total Last manager sweep total new group CRs that need to be created.
# TYPE nsx_operator_nsx_group_cr_creates_needed_total gauge
nsx_operator_nsx_group_cr_creates_needed_total{manager="manager-a.example.test"} 5
# HELP nsx_operator_nsx_group_cr_updates_needed_total Last manager sweep total group CR updates needed by mode.
# TYPE nsx_operator_nsx_group_cr_updates_needed_total gauge
nsx_operator_nsx_group_cr_updates_needed_total{manager="manager-a.example.test",mode="manage"} 1
nsx_operator_nsx_group_cr_updates_needed_total{manager="manager-a.example.test",mode="observe"} 2
# HELP nsx_operator_nsx_groups_listed_total Last manager sweep total groups listed from NSX.
# TYPE nsx_operator_nsx_groups_listed_total gauge
nsx_operator_nsx_groups_listed_total{manager="manager-a.example.test"} 7
# HELP nsx_operator_nsx_groups_manage_total Last manager sweep total manage groups considered for this manager.
# TYPE nsx_operator_nsx_groups_manage_total gauge
nsx_operator_nsx_groups_manage_total{manager="manager-a.example.test"} 4
# HELP nsx_operator_nsx_groups_observe_total Last manager sweep total observe groups considered for this manager.
# TYPE nsx_operator_nsx_groups_observe_total gauge
nsx_operator_nsx_groups_observe_total{manager="manager-a.example.test"} 3
# HELP nsx_operator_nsx_http_bytes_total Total NSX HTTP bytes by manager and direction.
# TYPE nsx_operator_nsx_http_bytes_total counter
nsx_operator_nsx_http_bytes_total{direction="request",manager="manager-a.example.test"} 12
nsx_operator_nsx_http_bytes_total{direction="response",manager="manager-a.example.test"} 44
# HELP nsx_operator_nsx_http_requests_total Total NSX HTTP requests by manager.
# TYPE nsx_operator_nsx_http_requests_total counter
nsx_operator_nsx_http_requests_total{manager="manager-a.example.test"} 1
`
	err = testutil.GatherAndCompare(
		registry, strings.NewReader(expected),
		"nsx_operator_kubernetes_api_bytes_total",
		"nsx_operator_kubernetes_api_calls_total",
		"nsx_operator_nsx_client_calls_total",
		"nsx_operator_nsx_group_cr_creates_needed_total",
		"nsx_operator_nsx_group_cr_updates_needed_total",
		"nsx_operator_nsx_groups_listed_total",
		"nsx_operator_nsx_groups_manage_total",
		"nsx_operator_nsx_groups_observe_total",
		"nsx_operator_nsx_http_bytes_total",
		"nsx_operator_nsx_http_requests_total",
	)
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	if count := testutil.CollectAndCount(recorder.nsxHTTPRoundTrip); count != 1 {
		t.Fatalf("nsx round trip histogram series count = %d, want 1", count)
	}
	if count := testutil.CollectAndCount(recorder.nsxHTTPFunctionRoundTrip); count != 1 {
		t.Fatalf("nsx function round trip histogram series count = %d, want 1", count)
	}
	if count := testutil.CollectAndCount(recorder.kubernetesAPIRoundTrip); count != 1 {
		t.Fatalf("kubernetes round trip histogram series count = %d, want 1", count)
	}
}

func TestRecorderConstructorsAndNoopAreSafe(t *testing.T) {
	nop := &NopRecorder{}
	nop.ObserveNSXCall("manager", "function")
	nop.ObserveNSXHTTP("manager", "function", 1, 2, time.Millisecond)
	nop.ObserveKubernetesAPI("function", 1, 2, time.Millisecond)
	nop.SetManagerGroupSnapshot("manager", ManagerGroupSnapshot{})

	_, err := NewRecorder(nil, nil)
	if err == nil {
		t.Fatal("NewRecorder() error = nil, want nil registerer validation")
	}

	registry := prometheus.NewRegistry()
	recorder, err := NewProcessRecorder(registry, nil)
	if err != nil {
		t.Fatalf("NewProcessRecorder() error = %v", err)
	}
	recorder.ObserveNSXHTTP("manager-b.example.test", "get_group", 0, 0, time.Millisecond)
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather process recorder registry: %v", err)
	}
	if len(families) == 0 {
		t.Fatal("process recorder registered no collectors")
	}
}

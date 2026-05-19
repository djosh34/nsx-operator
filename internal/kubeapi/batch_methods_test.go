package kubeapi_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestGroupBatchMethodsUseExpectedKubeAPIRequests(t *testing.T) {
	server := newRecordingKubeAPIServer(t)
	client, err := kubeapi.NewClient(kubeapi.Options{
		Config: &rest.Config{Host: server.URL},
		BatchConfig: kubeapi.BatchConfig{
			NumParallelWorkers:   4,
			MaxRequestsPerSecond: 1000,
			MaxRequestsInFlight:  4,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx := context.Background()

	if _, _, err := client.Groups().CreateBatch(ctx, map[kubeapi.BatchKey]kubeapi.GroupCreateRequest{
		batchKey("create", "nsxgroups", "", "group-create"): {
			Object: groupObject("group-create", ""),
		},
	}); err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}
	if _, _, err := client.Groups().UpdateBatch(ctx, map[kubeapi.BatchKey]kubeapi.GroupUpdateRequest{
		batchKey("update", "nsxgroups", "", "group-update"): {
			Object: groupObject("group-update", "rv-update"),
		},
	}); err != nil {
		t.Fatalf("UpdateBatch() error = %v", err)
	}
	if _, _, err := client.Groups().ApplyBatch(ctx, map[kubeapi.BatchKey]kubeapi.GroupApplyRequest{
		batchKey("apply", "nsxgroups", "", "group-apply"): {
			Object:  groupObject("group-apply", "rv-ignored"),
			Options: kubeapi.ApplyOptions{FieldManager: "batch-test", Force: true},
		},
	}); err != nil {
		t.Fatalf("ApplyBatch() error = %v", err)
	}
	if _, _, err := client.Groups().UpdateStatusBatch(ctx, map[kubeapi.BatchKey]kubeapi.GroupStatusUpdateRequest{
		batchKey("updateStatus", "nsxgroups", "status", "group-status"): {
			Name: "group-status",
			Status: nsxv1alpha.NSXGroupStatus{
				UnsupportedReason: nsxv1alpha.UnsupportedExpressionReasonSupportedExpression,
			},
			Options: kubeapi.StatusUpdateOptions{ResourceVersion: "rv-status"},
		},
	}); err != nil {
		t.Fatalf("UpdateStatusBatch() error = %v", err)
	}
	if _, _, err := client.Groups().PatchFinalizersBatch(ctx, map[kubeapi.BatchKey]kubeapi.GroupFinalizerPatchRequest{
		batchKey("patchFinalizers", "nsxgroups", "finalizers", "group-finalizer"): {
			Name:            "group-finalizer",
			ResourceVersion: "rv-finalizer",
			Finalizers:      []string{"keep.io/finalizer"},
		},
	}); err != nil {
		t.Fatalf("PatchFinalizersBatch() error = %v", err)
	}
	if _, _, err := client.Groups().DeleteBatch(ctx, map[kubeapi.BatchKey]kubeapi.GroupDeleteRequest{
		batchKey("delete", "nsxgroups", "", "group-delete"): {
			Name: "group-delete",
		},
	}); err != nil {
		t.Fatalf("DeleteBatch() error = %v", err)
	}

	records := server.records()
	requireRequestCount(t, records, http.MethodPost, "/apis/nsx.ing.com/v1alpha/nsxgroups", 1)
	requireRequestCount(t, records, http.MethodPut, "/apis/nsx.ing.com/v1alpha/nsxgroups/group-update", 1)
	requireRequestCount(t, records, http.MethodPatch, "/apis/nsx.ing.com/v1alpha/nsxgroups/group-apply", 1)
	requireRequestCount(t, records, http.MethodPut, "/apis/nsx.ing.com/v1alpha/nsxgroups/group-status/status", 1)
	requireRequestCount(t, records, http.MethodPatch, "/apis/nsx.ing.com/v1alpha/nsxgroups/group-finalizer", 1)
	requireRequestCount(t, records, http.MethodDelete, "/apis/nsx.ing.com/v1alpha/nsxgroups/group-delete", 1)
	requireRequestCount(t, records, http.MethodGet, "/apis/nsx.ing.com/v1alpha/nsxgroups/group-update", 0)

	update := requireRequest(t, records, http.MethodPut, "/apis/nsx.ing.com/v1alpha/nsxgroups/group-update")
	var updated nsxv1alpha.NSXGroup
	if err := json.Unmarshal(update.body, &updated); err != nil {
		t.Fatalf("decode update body: %v", err)
	}
	if updated.ResourceVersion != "rv-update" {
		t.Fatalf("update body resourceVersion = %q, want rv-update", updated.ResourceVersion)
	}

	status := requireRequest(t, records, http.MethodPut, "/apis/nsx.ing.com/v1alpha/nsxgroups/group-status/status")
	var statusBody nsxv1alpha.NSXGroup
	if err := json.Unmarshal(status.body, &statusBody); err != nil {
		t.Fatalf("decode status body: %v", err)
	}
	if statusBody.Status.UnsupportedReason != nsxv1alpha.UnsupportedExpressionReasonSupportedExpression {
		t.Fatalf("status unsupportedReason = %q, want SupportedExpression", statusBody.Status.UnsupportedReason)
	}

	finalizerPatch := requireRequest(t, records, http.MethodPatch, "/apis/nsx.ing.com/v1alpha/nsxgroups/group-finalizer")
	if !strings.Contains(finalizerPatch.contentType, "application/json-patch+json") {
		t.Fatalf("finalizer patch content type = %q, want json patch", finalizerPatch.contentType)
	}
	var patch []kubeapi.JSONPatchOperation
	if err := json.Unmarshal(finalizerPatch.body, &patch); err != nil {
		t.Fatalf("decode finalizer patch body: %v", err)
	}
	if len(patch) != 2 {
		t.Fatalf("finalizer patch operation count = %d, want 2: %s", len(patch), string(finalizerPatch.body))
	}
	if patch[0].Op != "test" || patch[0].Path != "/metadata/resourceVersion" || patch[0].Value != "rv-finalizer" {
		t.Fatalf("resourceVersion test patch = %#v, want test rv-finalizer", patch[0])
	}
	finalizers, ok := patch[1].Value.([]interface{})
	if !ok || len(finalizers) != 1 || finalizers[0] != "keep.io/finalizer" {
		t.Fatalf("finalizer patch value = %#v, want full desired finalizers array", patch[1].Value)
	}
}

func TestNetworkCloudBatchMethodsUseExpectedKubeAPIRequests(t *testing.T) {
	server := newRecordingKubeAPIServer(t)
	client, err := kubeapi.NewClient(kubeapi.Options{
		Config: &rest.Config{Host: server.URL},
		BatchConfig: kubeapi.BatchConfig{
			NumParallelWorkers:   4,
			MaxRequestsPerSecond: 1000,
			MaxRequestsInFlight:  4,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx := context.Background()

	if _, _, err := client.NetworkClouds().CreateBatch(ctx, map[kubeapi.BatchKey]kubeapi.NetworkCloudCreateRequest{
		batchKey("create", "nsxnetworkclouds", "", "cloud-create"): {
			Object: networkCloudObject("cloud-create", ""),
		},
	}); err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}
	if _, _, err := client.NetworkClouds().UpdateBatch(ctx, map[kubeapi.BatchKey]kubeapi.NetworkCloudUpdateRequest{
		batchKey("update", "nsxnetworkclouds", "", "cloud-update"): {
			Object: networkCloudObject("cloud-update", "rv-update"),
		},
	}); err != nil {
		t.Fatalf("UpdateBatch() error = %v", err)
	}
	if _, _, err := client.NetworkClouds().ApplyBatch(ctx, map[kubeapi.BatchKey]kubeapi.NetworkCloudApplyRequest{
		batchKey("apply", "nsxnetworkclouds", "", "cloud-apply"): {
			Object:  networkCloudObject("cloud-apply", "rv-ignored"),
			Options: kubeapi.ApplyOptions{FieldManager: "batch-test", Force: true},
		},
	}); err != nil {
		t.Fatalf("ApplyBatch() error = %v", err)
	}
	if _, _, err := client.NetworkClouds().UpdateStatusBatch(ctx, map[kubeapi.BatchKey]kubeapi.NetworkCloudStatusUpdateRequest{
		batchKey("updateStatus", "nsxnetworkclouds", "status", "cloud-status"): {
			Name: "cloud-status",
			Status: nsxv1alpha.NSXNetworkCloudStatus{
				Conditions: []metav1.Condition{{Type: nsxv1alpha.ConditionReachable, Status: metav1.ConditionTrue}},
			},
			Options: kubeapi.StatusUpdateOptions{ResourceVersion: "rv-status"},
		},
	}); err != nil {
		t.Fatalf("UpdateStatusBatch() error = %v", err)
	}
	if _, _, err := client.NetworkClouds().PatchFinalizersBatch(ctx, map[kubeapi.BatchKey]kubeapi.NetworkCloudFinalizerPatchRequest{
		batchKey("patchFinalizers", "nsxnetworkclouds", "finalizers", "cloud-finalizer"): {
			Name:            "cloud-finalizer",
			ResourceVersion: "rv-finalizer",
			Finalizers:      []string{"keep.io/finalizer"},
		},
	}); err != nil {
		t.Fatalf("PatchFinalizersBatch() error = %v", err)
	}
	if _, _, err := client.NetworkClouds().DeleteBatch(ctx, map[kubeapi.BatchKey]kubeapi.NetworkCloudDeleteRequest{
		batchKey("delete", "nsxnetworkclouds", "", "cloud-delete"): {
			Name: "cloud-delete",
		},
	}); err != nil {
		t.Fatalf("DeleteBatch() error = %v", err)
	}

	records := server.records()
	requireRequestCount(t, records, http.MethodPost, "/apis/nsx.ing.com/v1alpha/nsxnetworkclouds", 1)
	requireRequestCount(t, records, http.MethodPut, "/apis/nsx.ing.com/v1alpha/nsxnetworkclouds/cloud-update", 1)
	requireRequestCount(t, records, http.MethodPatch, "/apis/nsx.ing.com/v1alpha/nsxnetworkclouds/cloud-apply", 1)
	requireRequestCount(t, records, http.MethodPut, "/apis/nsx.ing.com/v1alpha/nsxnetworkclouds/cloud-status/status", 1)
	requireRequestCount(t, records, http.MethodPatch, "/apis/nsx.ing.com/v1alpha/nsxnetworkclouds/cloud-finalizer", 1)
	requireRequestCount(t, records, http.MethodDelete, "/apis/nsx.ing.com/v1alpha/nsxnetworkclouds/cloud-delete", 1)
	requireRequestCount(t, records, http.MethodGet, "/apis/nsx.ing.com/v1alpha/nsxnetworkclouds/cloud-update", 0)

	update := requireRequest(t, records, http.MethodPut, "/apis/nsx.ing.com/v1alpha/nsxnetworkclouds/cloud-update")
	var updated nsxv1alpha.NSXNetworkCloud
	if err := json.Unmarshal(update.body, &updated); err != nil {
		t.Fatalf("decode update body: %v", err)
	}
	if updated.ResourceVersion != "rv-update" {
		t.Fatalf("update body resourceVersion = %q, want rv-update", updated.ResourceVersion)
	}

	status := requireRequest(t, records, http.MethodPut, "/apis/nsx.ing.com/v1alpha/nsxnetworkclouds/cloud-status/status")
	var statusBody nsxv1alpha.NSXNetworkCloud
	if err := json.Unmarshal(status.body, &statusBody); err != nil {
		t.Fatalf("decode status body: %v", err)
	}
	if len(statusBody.Status.Conditions) != 1 || statusBody.Status.Conditions[0].Type != nsxv1alpha.ConditionReachable {
		t.Fatalf("status conditions = %#v, want Reachable condition", statusBody.Status.Conditions)
	}

	finalizerPatch := requireRequest(t, records, http.MethodPatch, "/apis/nsx.ing.com/v1alpha/nsxnetworkclouds/cloud-finalizer")
	var patch []kubeapi.JSONPatchOperation
	if err := json.Unmarshal(finalizerPatch.body, &patch); err != nil {
		t.Fatalf("decode finalizer patch body: %v", err)
	}
	if len(patch) != 2 || patch[0].Op != "test" || patch[1].Path != "/metadata/finalizers" {
		t.Fatalf("finalizer patch = %#v, want resourceVersion test and full finalizers add", patch)
	}
}

type recordingKubeAPIServer struct {
	*httptest.Server
	t        *testing.T
	mu       sync.Mutex
	recorded []recordedKubeRequest
}

type recordedKubeRequest struct {
	method      string
	path        string
	contentType string
	body        []byte
}

func newRecordingKubeAPIServer(t *testing.T) *recordingKubeAPIServer {
	t.Helper()
	server := &recordingKubeAPIServer{t: t}
	server.Server = httptest.NewServer(http.HandlerFunc(server.handle))
	t.Cleanup(server.Close)
	return server
}

func (s *recordingKubeAPIServer) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusInternalServerError)
		return
	}
	if err := r.Body.Close(); err != nil {
		http.Error(w, fmt.Sprintf("close body: %v", err), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	s.recorded = append(s.recorded, recordedKubeRequest{
		method:      r.Method,
		path:        r.URL.Path,
		contentType: r.Header.Get("Content-Type"),
		body:        body,
	})
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodDelete {
		if err := json.NewEncoder(w).Encode(&metav1.Status{Status: metav1.StatusSuccess}); err != nil {
			s.t.Errorf("encode delete response: %v", err)
		}
		return
	}
	name := resourceNameFromPath(r.URL.Path)
	if strings.Contains(r.URL.Path, "/nsxgroups") {
		if err := json.NewEncoder(w).Encode(groupObject(name, "rv-response")); err != nil {
			s.t.Errorf("encode group response: %v", err)
		}
		return
	}
	if strings.Contains(r.URL.Path, "/nsxnetworkclouds") {
		if err := json.NewEncoder(w).Encode(networkCloudObject(name, "rv-response")); err != nil {
			s.t.Errorf("encode network cloud response: %v", err)
		}
		return
	}
	http.NotFound(w, r)
}

func (s *recordingKubeAPIServer) records() []recordedKubeRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]recordedKubeRequest, len(s.recorded))
	copy(copied, s.recorded)
	return copied
}

func batchKey(operation string, resource string, subresource string, name string) kubeapi.BatchKey {
	return kubeapi.BatchKey{
		Operation:   operation,
		Resource:    resource,
		Subresource: subresource,
		Name:        name,
	}
}

func groupObject(name string, resourceVersion string) *nsxv1alpha.NSXGroup {
	return &nsxv1alpha.NSXGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			ResourceVersion: resourceVersion,
		},
		Spec: nsxv1alpha.NSXGroupSpec{
			NetworkCloudFQDN: "nsx.example.test",
			GroupID:          name,
			DisplayName:      name,
			Mode:             nsxv1alpha.NSXGroupModeManage,
			CIDRs:            []string{"192.0.2.0/24"},
		},
	}
}

func networkCloudObject(name string, resourceVersion string) *nsxv1alpha.NSXNetworkCloud {
	return &nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			ResourceVersion: resourceVersion,
		},
		Spec: nsxv1alpha.NSXNetworkCloudSpec{
			NetworkCloudFQDN: name + ".example.test",
			NetworkCloudID:   name,
			Name:             name,
		},
	}
}

func resourceNameFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if last == "status" && len(parts) > 1 {
		return parts[len(parts)-2]
	}
	if last == "nsxgroups" || last == "nsxnetworkclouds" {
		return "created"
	}
	return last
}

func requireRequestCount(t *testing.T, records []recordedKubeRequest, method string, path string, want int) {
	t.Helper()
	got := 0
	for _, record := range records {
		if record.method == method && record.path == path {
			got++
		}
	}
	if got != want {
		t.Fatalf("%s %s count = %d, want %d; records = %#v", method, path, got, want, records)
	}
}

func requireRequest(t *testing.T, records []recordedKubeRequest, method string, path string) recordedKubeRequest {
	t.Helper()
	for _, record := range records {
		if record.method == method && record.path == path {
			return record
		}
	}
	t.Fatalf("missing %s %s in records %#v", method, path, records)
	return recordedKubeRequest{}
}

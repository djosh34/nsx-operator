package stateoperator_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/djosh34/nsx-operator/internal/nsxclient"
	"github.com/djosh34/nsx-operator/internal/stateoperator"
	"go.uber.org/zap"
)

func TestManagedWriteUsesSelectedPatchEndpointsAndPreservesUnrelatedMockAPIExpressions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	mock := startStateoperatorMockAPI(t, ctx)
	recorder := &httpRequestRecorder{}
	managerClient := newStateoperatorMockAPIRecordingClient(t, mock.BaseURL(), recorder)
	seedGroup := &nsxclient.Group{
		Resource: nsxclient.Resource{ID: "managed-write-mock", DisplayName: "Remote Managed", ResourceType: "Group"},
		Expression: []json.RawMessage{
			rawExpression(t, map[string]any{
				"resource_type": "IPAddressExpression",
				"ip_addresses":  []string{"10.99.0.1"},
			}),
			rawExpression(t, map[string]any{
				"resource_type":        "ConjunctionOperator",
				"conjunction_operator": "OR",
			}),
			rawExpression(t, map[string]any{
				"resource_type": "PathExpression",
				"paths":         []string{"/infra/domains/default/groups/unrelated-group", "/infra/segments/unrelated"},
			}),
		},
	}
	if _, err := managerClient.PutGroup(ctx, "managed-write-mock", seedGroup); err != nil {
		t.Fatalf("seed managed group: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	if err := managerClient.PatchGroupIPAddressExpression(ctx, "managed-write-mock", "selected-ip", &nsxclient.IPAddressExpressionPatch{
		ID:           "selected-ip",
		ResourceType: "IPAddressExpression",
		IPAddresses:  []string{"10.1.0.0/24"},
	}); err != nil {
		t.Fatalf("seed selected ip expression: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	if err := managerClient.PatchGroupPathExpression(ctx, "managed-write-mock", "selected-path", &nsxclient.PathExpressionPatch{
		ID:           "selected-path",
		ResourceType: "PathExpression",
		Paths:        []string{"/infra/segments/old"},
	}); err != nil {
		t.Fatalf("seed selected path expression: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	recorder.Reset()

	segmentPaths := []string{"/infra/segments/new", "/infra/segments/extra"}
	err := stateoperator.ApplyManagerPlan(ctx, &operationRecorder{}, managerClient, stateoperator.ManagerPlan{
		ManagedWrites: []stateoperator.ManagedGroupWrite{{
			Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "managed-write-mock"},
			DisplayName:           "Managed Write Mock",
			CIDRs:                 []string{"10.42.0.0/24"},
			SegmentPaths:          segmentPaths,
			IPAddressExpressionID: "selected-ip",
			PathExpressionID:      "selected-path",
		}},
	})
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	writeRequests := recorder.Snapshot()

	ipMembers, err := managerClient.ListGroupIPAddressMembers(ctx, "managed-write-mock")
	if err != nil {
		t.Fatalf("list ip members: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	requireStringMembers(t, ipMembers, "10.99.0.1", "10.42.0.0/24")
	groupMembers, err := managerClient.ListGroupIPGroupMembers(ctx, "managed-write-mock")
	if err != nil {
		t.Fatalf("list group members: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	requireMemberPaths(t, groupMembers, "/infra/domains/default/groups/unrelated-group")
	segmentMembers, err := managerClient.ListGroupSegmentMembers(ctx, "managed-write-mock")
	if err != nil {
		t.Fatalf("list segment members: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	requireMemberPaths(t, segmentMembers, "/infra/segments/unrelated", "/infra/segments/new", "/infra/segments/extra")

	wantRequests := []recordedHTTPRequest{
		{
			method: http.MethodPatch,
			path:   "/policy/api/v1/infra/domains/default/groups/managed-write-mock",
			body: map[string]any{
				"id":            "managed-write-mock",
				"display_name":  "Managed Write Mock",
				"resource_type": "Group",
			},
		},
		{
			method: http.MethodPatch,
			path:   "/policy/api/v1/infra/domains/default/groups/managed-write-mock/ip-address-expressions/selected-ip",
			body: map[string]any{
				"id":            "selected-ip",
				"resource_type": "IPAddressExpression",
				"ip_addresses":  []any{"10.42.0.0/24"},
			},
		},
		{
			method: http.MethodPatch,
			path:   "/policy/api/v1/infra/domains/default/groups/managed-write-mock/path-expressions/selected-path",
			body: map[string]any{
				"id":            "selected-path",
				"resource_type": "PathExpression",
				"paths":         []any{"/infra/segments/new", "/infra/segments/extra"},
			},
		},
	}
	requireRecordedHTTPRequests(t, writeRequests, wantRequests)
}

func TestManagedWriteDeletesOnlySelectedIPAddressExpressionWhenCIDRsAreAbsent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	mock := startStateoperatorMockAPI(t, ctx)
	recorder := &httpRequestRecorder{}
	managerClient := newStateoperatorMockAPIRecordingClient(t, mock.BaseURL(), recorder)
	seedGroup := &nsxclient.Group{
		Resource: nsxclient.Resource{ID: "managed-delete-selected-ip", DisplayName: "Remote Managed", ResourceType: "Group"},
		Expression: []json.RawMessage{
			rawExpression(t, map[string]any{
				"resource_type": "IPAddressExpression",
				"ip_addresses":  []string{"10.99.0.2"},
			}),
			rawExpression(t, map[string]any{
				"resource_type":        "ConjunctionOperator",
				"conjunction_operator": "OR",
			}),
			rawExpression(t, map[string]any{
				"resource_type": "PathExpression",
				"paths":         []string{"/infra/segments/unrelated-delete"},
			}),
		},
	}
	if _, err := managerClient.PutGroup(ctx, "managed-delete-selected-ip", seedGroup); err != nil {
		t.Fatalf("seed managed group: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	if err := managerClient.PatchGroupIPAddressExpression(ctx, "managed-delete-selected-ip", "selected-ip", &nsxclient.IPAddressExpressionPatch{
		ID:           "selected-ip",
		ResourceType: "IPAddressExpression",
		IPAddresses:  []string{"10.2.0.0/24"},
	}); err != nil {
		t.Fatalf("seed selected ip expression: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	recorder.Reset()

	err := stateoperator.ApplyManagerPlan(ctx, &operationRecorder{}, managerClient, stateoperator.ManagerPlan{
		ManagedWrites: []stateoperator.ManagedGroupWrite{{
			Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "managed-delete-selected-ip"},
			DisplayName:           "Managed Delete Selected",
			IPAddressExpressionID: "selected-ip",
		}},
	})
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	requireRecordedHTTPRequests(t, recorder.Snapshot(), []recordedHTTPRequest{
		{
			method: http.MethodPatch,
			path:   "/policy/api/v1/infra/domains/default/groups/managed-delete-selected-ip",
			body: map[string]any{
				"id":            "managed-delete-selected-ip",
				"display_name":  "Managed Delete Selected",
				"resource_type": "Group",
			},
		},
		{
			method: http.MethodDelete,
			path:   "/policy/api/v1/infra/domains/default/groups/managed-delete-selected-ip/ip-address-expressions/selected-ip",
		},
	})

	ipMembers, err := managerClient.ListGroupIPAddressMembers(ctx, "managed-delete-selected-ip")
	if err != nil {
		t.Fatalf("list ip members: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	requireStringMembers(t, ipMembers, "10.99.0.2")
	requireStringMembersAbsent(t, ipMembers, "10.2.0.0/24")
	segmentMembers, err := managerClient.ListGroupSegmentMembers(ctx, "managed-delete-selected-ip")
	if err != nil {
		t.Fatalf("list segment members: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	requireMemberPaths(t, segmentMembers, "/infra/segments/unrelated-delete")
}

func TestDisabledNSXWritesDoNotReachMockAPIRecorderWhileReadsStillDo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	mock := startStateoperatorMockAPI(t, ctx)
	recorder := &httpRequestRecorder{}
	managerClient := newStateoperatorMockAPIRecordingClientWithWriteControl(t, mock.BaseURL(), recorder, nsxclient.WriteControl{
		Enabled:          false,
		Reason:           nsxclient.WriteDisabledReasonNetworkCloud,
		NetworkCloudName: "cloud-a",
		NetworkCloudFQDN: "nsx-a.example.test",
	})

	writeCalls := []struct {
		name string
		call func(context.Context) error
	}{
		{
			name: "POST",
			call: func(ctx context.Context) error {
				_, err := managerClient.CreateFirewallSection(ctx, &nsxclient.FirewallSection{
					Resource: nsxclient.Resource{ID: "section-a", DisplayName: "Section A"},
				})
				return err
			},
		},
		{
			name: "PUT",
			call: func(ctx context.Context) error {
				_, err := managerClient.PutGroup(ctx, "disabled-put", &nsxclient.Group{
					Resource: nsxclient.Resource{ID: "disabled-put", DisplayName: "Disabled Put", ResourceType: "Group"},
				})
				return err
			},
		},
		{
			name: "PATCH",
			call: func(ctx context.Context) error {
				return managerClient.PatchGroup(ctx, "disabled-patch", &nsxclient.GroupPatch{
					ID:           "disabled-patch",
					DisplayName:  "Disabled Patch",
					ResourceType: "Group",
				})
			},
		},
		{
			name: "DELETE",
			call: func(ctx context.Context) error {
				return managerClient.DeleteGroup(ctx, "disabled-delete")
			},
		},
	}
	for _, tt := range writeCalls {
		err := tt.call(ctx)
		if err == nil {
			t.Fatalf("%s error = nil, want write disabled error", tt.name)
		}
		var writeDisabled nsxclient.WriteDisabledError
		if !errors.As(err, &writeDisabled) {
			t.Fatalf("%s error = %T %[2]v, want WriteDisabledError", tt.name, err)
		}
	}
	if got := recorder.Snapshot(); len(got) != 0 {
		t.Fatalf("disabled writes reached mock API recorder: %#v\nmockapi logs:\n%s", got, mock.Logs())
	}

	if _, err := managerClient.ListGroups(ctx); err != nil {
		t.Fatalf("ListGroups() error = %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	requireRecordedHTTPRequests(t, recorder.Snapshot(), []recordedHTTPRequest{{
		method: http.MethodGet,
		path:   "/policy/api/v1/infra/domains/default/groups",
	}})
}

func newStateoperatorMockAPIRecordingClient(t *testing.T, baseURL string, recorder *httpRequestRecorder) *nsxclient.Client {
	t.Helper()

	return newStateoperatorMockAPIRecordingClientWithWriteControl(t, baseURL, recorder, nsxclient.WriteControl{})
}

func newStateoperatorMockAPIRecordingClientWithWriteControl(t *testing.T, baseURL string, recorder *httpRequestRecorder, writeControl nsxclient.WriteControl) *nsxclient.Client {
	t.Helper()

	client, err := nsxclient.NewClient(nsxclient.Options{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: recordingTransport{
				base:     http.DefaultTransport,
				recorder: recorder,
			},
		},
		Username:     stateoperatorMockAPIUsername,
		Password:     stateoperatorMockAPIPassword,
		Logger:       zap.NewNop(),
		WriteControl: writeControl,
	})
	if err != nil {
		t.Fatalf("construct recording mockapi nsx client: %v", err)
	}
	return client
}

type httpRequestRecorder struct {
	mu       sync.Mutex
	requests []recordedHTTPRequest
}

type recordedHTTPRequest struct {
	method   string
	path     string
	rawQuery string
	body     map[string]any
}

func (r *httpRequestRecorder) Record(request recordedHTTPRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
}

func (r *httpRequestRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = nil
}

func (r *httpRequestRecorder) Snapshot() []recordedHTTPRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedHTTPRequest(nil), r.requests...)
}

type recordingTransport struct {
	base     http.RoundTripper
	recorder *httpRequestRecorder
}

func (transport recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := readAndRestoreRequestBody(req)
	if err != nil {
		return nil, err
	}
	if transport.recorder != nil {
		transport.recorder.Record(recordedHTTPRequest{
			method:   req.Method,
			path:     req.URL.Path,
			rawQuery: req.URL.RawQuery,
			body:     body,
		})
	}
	base := transport.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func readAndRestoreRequestBody(req *http.Request) (map[string]any, error) {
	if req.Body == nil {
		return nil, nil
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body for recording: %w", err)
	}
	if err := req.Body.Close(); err != nil {
		return nil, fmt.Errorf("close request body for recording: %w", err)
	}
	req.Body = io.NopCloser(bytes.NewReader(raw))
	if len(raw) == 0 {
		return nil, nil
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("decode recorded request body: %w", err)
	}
	return body, nil
}

func requireRecordedHTTPRequests(t *testing.T, got []recordedHTTPRequest, want []recordedHTTPRequest) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("recorded requests = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index].method != want[index].method || got[index].path != want[index].path || got[index].rawQuery != want[index].rawQuery {
			t.Fatalf("recorded request %d = %s %s?%s, want %s %s?%s", index, got[index].method, got[index].path, got[index].rawQuery, want[index].method, want[index].path, want[index].rawQuery)
		}
		if !reflect.DeepEqual(got[index].body, want[index].body) {
			t.Fatalf("recorded request %d body = %#v, want %#v", index, got[index].body, want[index].body)
		}
	}
}

func requireStringMembers(t *testing.T, got []string, want ...string) {
	t.Helper()

	for _, value := range want {
		if !slices.Contains(got, value) {
			t.Fatalf("members = %v, want member %q", got, value)
		}
	}
}

func requireStringMembersAbsent(t *testing.T, got []string, wantAbsent ...string) {
	t.Helper()

	for _, value := range wantAbsent {
		if slices.Contains(got, value) {
			t.Fatalf("members = %v, want member %q absent", got, value)
		}
	}
}

func requireMemberPaths(t *testing.T, got []*nsxclient.GroupMember, want ...string) {
	t.Helper()

	paths := make([]string, 0, len(got))
	for _, member := range got {
		if member != nil {
			paths = append(paths, member.Path)
		}
	}
	for _, value := range want {
		if !slices.Contains(paths, value) {
			t.Fatalf("member paths = %v, want path %q", paths, value)
		}
	}
}

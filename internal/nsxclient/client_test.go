package nsxclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/djosh34/nsx-operator/internal/operatormetrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestClientAddsBasicAuthToReadAndWriteRequests(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	client, err := NewClient(Options{
		BaseURL:  "https://nsx.example.test",
		Username: "nsx_admin",
		Password: "nsx_password",
		Logger:   zap.NewNop(),
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			username, password, ok := req.BasicAuth()
			if !ok {
				t.Errorf("BasicAuth() ok = false")
			}
			if username != "nsx_admin" {
				t.Errorf("BasicAuth username = %q, want nsx_admin", username)
			}
			if password != "nsx_password" {
				t.Errorf("BasicAuth password = %q, want nsx_password", password)
			}
			seen.Add(1)
			switch req.Method {
			case http.MethodGet:
				return jsonResponse(req, http.StatusOK, `{"results":[],"result_count":0}`), nil
			case http.MethodPatch:
				return jsonResponse(req, http.StatusOK, `{}`), nil
			default:
				t.Errorf("unexpected method %s", req.Method)
				return jsonResponse(req, http.StatusTeapot, `{}`), nil
			}
		})},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	err = client.PatchGroup(context.Background(), "app", &GroupPatch{DisplayName: "App"})
	if err != nil {
		t.Fatalf("PatchGroup() error = %v", err)
	}
	if got := seen.Load(); got != 2 {
		t.Fatalf("requests seen = %d, want 2", got)
	}
}

func TestClientRecordsNSXMetricsForPublicCalls(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	recorder, err := operatormetrics.NewRecorder(registry, zap.NewNop())
	if err != nil {
		t.Fatalf("construct recorder: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == defaultDomainPath()+"/groups":
			_, writeErr := io.WriteString(w, `{"results":[{"id":"web","display_name":"Web"}],"result_count":1}`)
			if writeErr != nil {
				t.Errorf("write list response: %v", writeErr)
			}
		case req.Method == http.MethodPatch && req.URL.Path == defaultDomainPath()+"/groups/web":
			_, copyErr := io.Copy(io.Discard, req.Body)
			if copyErr != nil {
				t.Errorf("read patch body: %v", copyErr)
			}
			w.WriteHeader(http.StatusOK)
			_, writeErr := io.WriteString(w, `{}`)
			if writeErr != nil {
				t.Errorf("write patch response: %v", writeErr)
			}
		default:
			t.Errorf("unexpected request %s %s", req.Method, req.URL.RequestURI())
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(Options{
		BaseURL:  server.URL,
		Username: "nsx_admin",
		Password: "nsx_password",
		Logger:   zap.NewNop(),
		WriteControl: WriteControl{
			Enabled:          true,
			NetworkCloudFQDN: "manager-a.example.test",
		},
		Recorder: recorder,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	err = client.PatchGroup(context.Background(), "web", &GroupPatch{DisplayName: "Web"})
	if err != nil {
		t.Fatalf("PatchGroup() error = %v", err)
	}

	expected := `
# HELP nsx_operator_nsx_client_calls_total Total NSX client calls by manager and function.
# TYPE nsx_operator_nsx_client_calls_total counter
nsx_operator_nsx_client_calls_total{function="list_groups",manager="manager-a.example.test"} 1
nsx_operator_nsx_client_calls_total{function="patch_group",manager="manager-a.example.test"} 1
# HELP nsx_operator_nsx_http_requests_total Total NSX HTTP requests by manager.
# TYPE nsx_operator_nsx_http_requests_total counter
nsx_operator_nsx_http_requests_total{manager="manager-a.example.test"} 2
`
	err = testutil.GatherAndCompare(
		registry, strings.NewReader(expected),
		"nsx_operator_nsx_client_calls_total",
		"nsx_operator_nsx_http_requests_total",
	)
	if err != nil {
		t.Fatalf("gather counters: %v", err)
	}
	if got := histogramSampleCount(t, registry, "nsx_operator_nsx_http_round_trip_seconds", map[string]string{"manager": "manager-a.example.test"}); got != 2 {
		t.Fatalf("nsx manager round trip count = %d, want 2", got)
	}
	if got := histogramSampleCount(t, registry, "nsx_operator_nsx_http_function_round_trip_seconds", map[string]string{"manager": "manager-a.example.test", "function": "list_groups"}); got != 1 {
		t.Fatalf("nsx list_groups round trip count = %d, want 1", got)
	}
	if got := histogramSampleCount(t, registry, "nsx_operator_nsx_http_function_round_trip_seconds", map[string]string{"manager": "manager-a.example.test", "function": "patch_group"}); got != 1 {
		t.Fatalf("nsx patch_group round trip count = %d, want 1", got)
	}
}

func TestResponseBodiesCloseForSuccessStatusErrorAndDecodeError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		statusCode int
		body       string
		target     any
	}{
		{name: "success", statusCode: http.StatusOK, body: `{}`, target: &Group{}},
		{name: "status error", statusCode: http.StatusConflict, body: `conflict`, target: &Group{}},
		{name: "decode error", statusCode: http.StatusOK, body: `{`, target: &Group{}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := &closingBody{Reader: strings.NewReader(tt.body)}
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://nsx.example.test/resource", nil)
			if err != nil {
				t.Fatalf("NewRequestWithContext() error = %v", err)
			}
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       body,
				Request:    req,
			}
			client := &Client{log: zap.NewNop()}
			handleErr := client.handleResponse(resp, tt.target)
			if tt.name == "success" && handleErr != nil {
				t.Fatalf("handleResponse() error = %v", handleErr)
			}
			if tt.name != "success" && handleErr == nil {
				t.Fatal("handleResponse() error = nil, want error")
			}
			if !body.closed.Load() {
				t.Fatal("response body was not closed")
			}
		})
	}
}

func TestDecodeListResultsStreamsTypedPointers(t *testing.T) {
	t.Parallel()

	results, cursor, count, err := DecodeListResults[Group](strings.NewReader(`{
		"results":[{"id":"a","display_name":"A"},{"id":"b","display_name":"B"}],
		"cursor":"next",
		"result_count":2
	}`))
	if err != nil {
		t.Fatalf("DecodeListResults() error = %v", err)
	}
	if cursor != "next" {
		t.Fatalf("cursor = %q, want next", cursor)
	}
	if count != 2 {
		t.Fatalf("result_count = %d, want 2", count)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].ID != "a" || results[1].DisplayName != "B" {
		t.Fatalf("decoded results = %#v", results)
	}
}

func TestListMethodsFollowPaginationUntilCursorIsEmpty(t *testing.T) {
	t.Parallel()

	var seenCursors []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/policy/api/v1/infra/domains/default/groups" {
			t.Errorf("path = %q, want default-domain groups path", req.URL.Path)
		}
		seenCursors = append(seenCursors, req.URL.Query().Get("cursor"))
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Query().Get("cursor") == "" {
			w.WriteHeader(http.StatusOK)
			_, writeErr := io.WriteString(w, `{"results":[{"id":"first"}],"cursor":"page-2","result_count":1}`)
			if writeErr != nil {
				t.Errorf("write first page: %v", writeErr)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		_, writeErr := io.WriteString(w, `{"results":[{"id":"second"}],"result_count":1}`)
		if writeErr != nil {
			t.Errorf("write second page: %v", writeErr)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	groups, err := client.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	if got, want := len(groups), 2; got != want {
		t.Fatalf("len(groups) = %d, want %d", got, want)
	}
	if groups[0].ID != "first" || groups[1].ID != "second" {
		t.Fatalf("groups = %#v", groups)
	}
	if len(seenCursors) != 2 || seenCursors[0] != "" || seenCursors[1] != "page-2" {
		t.Fatalf("seen cursors = %#v, want [\"\", \"page-2\"]", seenCursors)
	}
}

func TestClientWriteControlBlocksNonGETAndAllowsReadRequests(t *testing.T) {
	t.Parallel()

	var seen []string
	core, logs := observer.New(zapcore.DebugLevel)
	client, err := NewClient(Options{
		BaseURL:  "https://nsx.example.test",
		Username: "nsx_admin",
		Password: "nsx_password",
		Logger:   zap.New(core),
		WriteControl: WriteControl{
			Enabled:          false,
			Reason:           WriteDisabledReasonNetworkCloud,
			NetworkCloudName: "cloud-a",
			NetworkCloudFQDN: "nsx.example.test",
		},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seen = append(seen, req.Method+" "+req.URL.RequestURI())
			switch req.Method {
			case http.MethodGet:
				if req.URL.Path == defaultDomainPath()+"/groups" {
					return jsonResponse(req, http.StatusOK, `{"results":[],"result_count":0}`), nil
				}
				return jsonResponse(req, http.StatusOK, `{}`), nil
			default:
				t.Errorf("transport saw blocked write %s %s", req.Method, req.URL.RequestURI())
				return jsonResponse(req, http.StatusTeapot, `{}`), nil
			}
		})},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	writeCalls := []struct {
		name string
		call func(context.Context) error
	}{
		{
			name: "POST",
			call: func(ctx context.Context) error {
				_, callErr := client.CreateFirewallSection(ctx, &FirewallSection{Resource: Resource{ID: "section-a", DisplayName: "Section A"}})
				return callErr
			},
		},
		{
			name: "PUT",
			call: func(ctx context.Context) error {
				_, callErr := client.PutGroup(ctx, "app", &Group{Resource: Resource{ID: "app", DisplayName: "App", ResourceType: "Group"}})
				return callErr
			},
		},
		{
			name: "PATCH",
			call: func(ctx context.Context) error {
				return client.PatchGroup(ctx, "app", &GroupPatch{ID: "app", DisplayName: "App", ResourceType: "Group"})
			},
		},
		{
			name: "DELETE",
			call: func(ctx context.Context) error {
				return client.DeleteGroup(ctx, "app")
			},
		},
	}
	for _, tt := range writeCalls {
		t.Run(tt.name, func(t *testing.T) {
			callErr := tt.call(context.Background())
			if callErr == nil {
				t.Fatal("write call error = nil, want write disabled error")
			}
			var writeDisabled WriteDisabledError
			if !errors.As(callErr, &writeDisabled) {
				t.Fatalf("write call error = %T %[1]v, want WriteDisabledError", callErr)
			}
			if writeDisabled.Method != tt.name {
				t.Fatalf("WriteDisabledError.Method = %q, want %q", writeDisabled.Method, tt.name)
			}
			if writeDisabled.Reason != WriteDisabledReasonNetworkCloud {
				t.Fatalf("WriteDisabledError.Reason = %q, want %q", writeDisabled.Reason, WriteDisabledReasonNetworkCloud)
			}
		})
	}

	_, err = client.GetGroup(context.Background(), "app")
	if err != nil {
		t.Fatalf("GetGroup() error = %v", err)
	}
	_, err = client.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	wantSeen := []string{
		"GET /policy/api/v1/infra/domains/default/groups/app",
		"GET /policy/api/v1/infra/domains/default/groups",
	}
	if !reflect.DeepEqual(seen, wantSeen) {
		t.Fatalf("transport saw requests = %v, want only reads %v", seen, wantSeen)
	}

	requireObservedLogField(t, logs, "writeDisabledReason", string(WriteDisabledReasonNetworkCloud))
	requireObservedLogField(t, logs, "networkCloudName", "cloud-a")
	requireObservedLogField(t, logs, "networkCloudFQDN", "nsx.example.test")
	requireObservedLogField(t, logs, "method", http.MethodPatch)
}

func TestGroupPathExpressionRoutesUsePolicyExpressionEndpoints(t *testing.T) {
	t.Parallel()

	var requests []string
	var addPayload PathExpressionPatch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests = append(requests, req.Method+" "+req.URL.RequestURI())
		switch len(requests) {
		case 1:
			if req.Method != http.MethodPost {
				t.Errorf("add method = %s, want POST", req.Method)
			}
			if req.URL.Path != "/policy/api/v1/infra/domains/default/groups/web/path-expressions/segment" {
				t.Errorf("add path = %q, want group path expression endpoint", req.URL.Path)
			}
			if req.URL.Query().Get("action") != "add" {
				t.Errorf("add action query = %q, want add", req.URL.Query().Get("action"))
			}
			decodeErr := json.NewDecoder(req.Body).Decode(&addPayload)
			if decodeErr != nil {
				t.Errorf("decode add payload: %v", decodeErr)
			}
			w.WriteHeader(http.StatusOK)
		case 2:
			if req.Method != http.MethodDelete {
				t.Errorf("delete method = %s, want DELETE", req.Method)
			}
			if req.URL.Path != "/policy/api/v1/infra/domains/default/groups/web/path-expressions/old-segment" {
				t.Errorf("delete path = %q, want group path expression endpoint", req.URL.Path)
			}
			if req.URL.RawQuery != "" {
				t.Errorf("delete query = %q, want empty", req.URL.RawQuery)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request %d: %s %s", len(requests), req.Method, req.URL.RequestURI())
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)
	err := client.AddGroupPathExpression(context.Background(), "web", "segment", &PathExpressionPatch{
		ID:           "segment",
		ResourceType: "PathExpression",
		Paths:        []string{"/infra/segments/web"},
	})
	if err != nil {
		t.Fatalf("AddGroupPathExpression() error = %v", err)
	}
	err = client.DeleteGroupPathExpression(context.Background(), "web", "old-segment")
	if err != nil {
		t.Fatalf("DeleteGroupPathExpression() error = %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("requests = %v, want add and delete", requests)
	}
	if addPayload.ID != "segment" || addPayload.ResourceType != "PathExpression" || !reflect.DeepEqual(addPayload.Paths, []string{"/infra/segments/web"}) {
		t.Fatalf("add payload = %#v, want path expression payload", addPayload)
	}
}

func requireObservedLogField(t *testing.T, logs *observer.ObservedLogs, key string, want string) {
	t.Helper()

	entries := logs.FilterMessage("skipped nsx write request because writes are disabled").All()
	for index := range entries {
		entry := entries[index]
		for _, field := range entry.Context {
			if field.Key == key && field.String == want {
				return
			}
		}
	}
	t.Fatalf("write-disabled log did not contain %s=%q; logs: %v", key, want, logs.All())
}

func TestStatusErrorsMapTypedCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		statusCode int
		assert     func(error) bool
	}{
		{statusCode: http.StatusConflict, assert: func(err error) bool {
			var target ConflictError
			return errors.As(err, &target)
		}},
		{statusCode: http.StatusPreconditionFailed, assert: func(err error) bool {
			var target PreconditionFailedError
			return errors.As(err, &target)
		}},
		{statusCode: http.StatusTooManyRequests, assert: func(err error) bool {
			var target RateLimitedError
			return errors.As(err, &target)
		}},
		{statusCode: http.StatusServiceUnavailable, assert: func(err error) bool {
			var target ServiceUnavailableError
			return errors.As(err, &target)
		}},
	}
	for _, tt := range cases {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, http.StatusText(tt.statusCode), tt.statusCode)
			}))
			t.Cleanup(server.Close)
			client := newTestClient(t, server.URL)
			_, err := client.GetGroup(context.Background(), "missing")
			if err == nil {
				t.Fatal("GetGroup() error = nil, want status error")
			}
			if !tt.assert(err) {
				t.Fatalf("GetGroup() error = %T %[1]v, want typed status error", err)
			}
		})
	}
}

func TestWriteStatusErrorsAreTypedAndNotRetried(t *testing.T) {
	t.Parallel()

	cases := []struct {
		statusCode int
		assert     func(error) bool
	}{
		{statusCode: http.StatusConflict, assert: func(err error) bool {
			var target ConflictError
			return errors.As(err, &target)
		}},
		{statusCode: http.StatusPreconditionFailed, assert: func(err error) bool {
			var target PreconditionFailedError
			return errors.As(err, &target)
		}},
		{statusCode: http.StatusTooManyRequests, assert: func(err error) bool {
			var target RateLimitedError
			return errors.As(err, &target)
		}},
		{statusCode: http.StatusServiceUnavailable, assert: func(err error) bool {
			var target ServiceUnavailableError
			return errors.As(err, &target)
		}},
	}
	for _, tt := range cases {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			t.Parallel()
			var count atomic.Int32
			client, err := NewClient(Options{
				BaseURL:  "https://nsx.example.test",
				Username: "nsx_admin",
				Password: "nsx_password",
				Logger:   zap.NewNop(),
				HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					count.Add(1)
					if req.Method != http.MethodPatch {
						t.Errorf("method = %s, want PATCH", req.Method)
					}
					if req.URL.Path != "/policy/api/v1/infra/domains/default/groups/app" {
						t.Errorf("path = %s, want policy group path", req.URL.Path)
					}
					return jsonResponse(req, tt.statusCode, http.StatusText(tt.statusCode)), nil
				})},
			})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			err = client.PatchGroup(context.Background(), "app", &GroupPatch{ID: "app", DisplayName: "App", ResourceType: "Group"})
			if err == nil {
				t.Fatal("PatchGroup() error = nil, want status error")
			}
			if !tt.assert(err) {
				t.Fatalf("PatchGroup() error = %T %[1]v, want typed status error", err)
			}
			if got := count.Load(); got != 1 {
				t.Fatalf("request count = %d, want exactly 1", got)
			}
		})
	}
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := NewClient(Options{
		BaseURL:  baseURL,
		Username: "nsx_admin",
		Password: "nsx_password",
		Logger:   zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(req *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func histogramSampleCount(t *testing.T, registry *prometheus.Registry, name string, labels map[string]string) uint64 {
	t.Helper()

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather registry: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if !metricHasLabels(metric.GetLabel(), labels) {
				continue
			}
			if metric.GetHistogram() == nil {
				t.Fatalf("metric %s with labels %v is not a histogram", name, labels)
			}
			return metric.GetHistogram().GetSampleCount()
		}
	}
	t.Fatalf("missing histogram %s with labels %v", name, labels)
	return 0
}

func metricHasLabels(pairs []*dto.LabelPair, labels map[string]string) bool {
	for key, want := range labels {
		found := false
		for _, pair := range pairs {
			if pair.GetName() == key && pair.GetValue() == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

type closingBody struct {
	*strings.Reader
	closed atomic.Bool
}

func (body *closingBody) Close() error {
	body.closed.Store(true)
	return nil
}

//go:build largechaos

package nsxclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/djosh34/nsx-operator/internal/httpratelimit"
	"go.uber.org/zap"
)

func TestChaosLowRateSlowAndUnavailableManagerRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	var activeRequests int32
	var maxActiveRequests int32
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		active := atomic.AddInt32(&activeRequests, 1)
		defer atomic.AddInt32(&activeRequests, -1)
		for {
			maxActive := atomic.LoadInt32(&maxActiveRequests)
			if active <= maxActive || atomic.CompareAndSwapInt32(&maxActiveRequests, maxActive, active) {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
		currentRequest := atomic.AddInt32(&requestCount, 1)
		if currentRequest == 1 {
			http.Error(w, "manager temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path != "/policy/api/v1/infra/domains/default/groups" {
			http.Error(w, fmt.Sprintf("unexpected path %q", r.URL.Path), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		response := map[string]any{
			"result_count": 2,
			"results": []Group{
				{Resource: Resource{ID: "app-a", DisplayName: "App A", ResourceType: "Group"}},
				{Resource: Resource{ID: "app-b", DisplayName: "App B", ResourceType: "Group"}},
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encode list response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	httpClient := &http.Client{
		Transport: httpratelimit.NewRoundTripper(http.DefaultTransport, httpratelimit.Config{
			MaxRequestsInFlightPerHost:  1,
			MaxRequestsPerSecondPerHost: 50,
		}, zap.NewNop()),
	}
	client, err := NewClient(Options{
		BaseURL:    server.URL,
		HTTPClient: httpClient,
		Username:   "nsx_admin",
		Password:   "nsx_password",
		Logger:     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	errs := make(chan error, 2)
	var successes atomic.Int32
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	for i := range 2 {
		label := fmt.Sprintf("worker-%d", i)
		go func() {
			defer waitGroup.Done()
			groups, listErr := client.ListGroups(ctx)
			if listErr != nil {
				errs <- fmt.Errorf("%s list groups: %w", label, listErr)
				return
			}
			if len(groups) != 2 {
				errs <- fmt.Errorf("%s group count = %d, want 2", label, len(groups))
				return
			}
			successes.Add(1)
			errs <- nil
		}()
	}
	waitGroup.Wait()
	close(errs)

	var unavailableCount int
	for listErr := range errs {
		if listErr == nil {
			continue
		}
		var unavailable *ServiceUnavailableError
		if !errors.As(listErr, &unavailable) {
			t.Fatalf("list error = %T %[1]v, want ServiceUnavailableError or nil", listErr)
		}
		unavailableCount++
	}
	if unavailableCount != 1 {
		t.Fatalf("unavailable errors = %d, want 1", unavailableCount)
	}
	if successes.Load() != 1 {
		t.Fatalf("successful list calls = %d, want 1", successes.Load())
	}
	if maxActive := atomic.LoadInt32(&maxActiveRequests); maxActive > 1 {
		t.Fatalf("max active requests = %d, want limiter to hold concurrency to 1", maxActive)
	}
	t.Logf("chaos evidence: low per-host limiter serialized %d slow requests; one 503 was classified as ServiceUnavailableError and one request succeeded", requestCount)
}

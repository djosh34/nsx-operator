package kubeapi_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"go.uber.org/zap"
)

func TestExecuteBatchDefaultsToOneWorker(t *testing.T) {
	requests := make(map[kubeapi.BatchKey]int, 8)
	for i := range 8 {
		name := fmt.Sprintf("item-%02d", i)
		requests[kubeapi.BatchKey{Operation: "apply", Resource: "widgets", Name: name}] = i
	}

	var active atomic.Int64
	var maxActive atomic.Int64
	results, itemErrors, err := kubeapi.ExecuteBatch(context.Background(), kubeapi.BatchConfig{}, zap.NewNop(), kubeapi.BatchOperation[int, int]{
		Operation: "apply",
		Resource:  "widgets",
		Execute: func(_ context.Context, request int) (int, error) {
			current := active.Add(1)
			updateMaxActive(&maxActive, current)
			active.Add(-1)
			return request * 2, nil
		},
	}, requests)
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if len(itemErrors) != 0 {
		t.Fatalf("ExecuteBatch() itemErrors = %v, want none", itemErrors)
	}
	if len(results) != len(requests) {
		t.Fatalf("ExecuteBatch() result count = %d, want %d", len(results), len(requests))
	}
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("max concurrent executions = %d, want default single worker", got)
	}
}

func TestExecuteBatchUsesConfiguredWorkerCount(t *testing.T) {
	requests := make(map[kubeapi.BatchKey]int, 20)
	for i := range 20 {
		name := fmt.Sprintf("item-%02d", i)
		requests[kubeapi.BatchKey{Operation: "apply", Resource: "widgets", Name: name}] = i
	}

	release := make(chan struct{})
	var active atomic.Int64
	allWorkersActive := make(chan struct{})
	var signaled atomic.Bool
	var released atomic.Bool
	results, itemErrors, err := kubeapi.ExecuteBatch(context.Background(), kubeapi.BatchConfig{
		NumParallelWorkers:   20,
		MaxRequestsPerSecond: 1000,
		MaxRequestsInFlight:  20,
	}, zap.NewNop(), kubeapi.BatchOperation[int, int]{
		Operation: "apply",
		Resource:  "widgets",
		Execute: func(_ context.Context, request int) (int, error) {
			current := active.Add(1)
			if current == 20 && signaled.CompareAndSwap(false, true) {
				close(allWorkersActive)
				if released.CompareAndSwap(false, true) {
					close(release)
				}
			}
			select {
			case <-allWorkersActive:
			case <-time.After(2 * time.Second):
				t.Errorf("timed out waiting for 20 concurrent executions; active = %d", active.Load())
			}
			<-release
			active.Add(-1)
			return request, nil
		},
	}, requests)
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if len(itemErrors) != 0 {
		t.Fatalf("ExecuteBatch() itemErrors = %v, want none", itemErrors)
	}
	if len(results) != len(requests) {
		t.Fatalf("ExecuteBatch() result count = %d, want %d", len(results), len(requests))
	}
}

func TestExecuteBatchHandles10000Items(t *testing.T) {
	const itemCount = 10_000
	requests := make(map[kubeapi.BatchKey]int, itemCount)
	for i := range itemCount {
		name := fmt.Sprintf("item-%05d", i)
		requests[kubeapi.BatchKey{Operation: "apply", Resource: "widgets", Name: name}] = i
	}

	results, itemErrors, err := kubeapi.ExecuteBatch(context.Background(), kubeapi.BatchConfig{
		NumParallelWorkers:   20,
		MaxRequestsPerSecond: itemCount,
		MaxRequestsInFlight:  itemCount,
	}, zap.NewNop(), kubeapi.BatchOperation[int, int]{
		Operation: "apply",
		Resource:  "widgets",
		Execute: func(_ context.Context, request int) (int, error) {
			return request + 1, nil
		},
	}, requests)
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if len(itemErrors) != 0 {
		t.Fatalf("ExecuteBatch() itemErrors = %v, want none", itemErrors)
	}
	if len(results) != itemCount {
		t.Fatalf("ExecuteBatch() result count = %d, want %d", len(results), itemCount)
	}
	for key, request := range requests {
		if results[key] != request+1 {
			t.Fatalf("result[%+v] = %d, want %d", key, results[key], request+1)
		}
	}
}

func TestExecuteBatchHonorsMaxRequestsInFlight(t *testing.T) {
	requests := make(map[kubeapi.BatchKey]int, 12)
	for i := range 12 {
		name := fmt.Sprintf("item-%02d", i)
		requests[kubeapi.BatchKey{Operation: "apply", Resource: "widgets", Name: name}] = i
	}

	var active atomic.Int64
	var maxActive atomic.Int64
	results, itemErrors, err := kubeapi.ExecuteBatch(context.Background(), kubeapi.BatchConfig{
		NumParallelWorkers:   12,
		MaxRequestsPerSecond: 1000,
		MaxRequestsInFlight:  3,
	}, zap.NewNop(), kubeapi.BatchOperation[int, int]{
		Operation: "apply",
		Resource:  "widgets",
		Execute: func(_ context.Context, request int) (int, error) {
			current := active.Add(1)
			updateMaxActive(&maxActive, current)
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return request, nil
		},
	}, requests)
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if len(itemErrors) != 0 {
		t.Fatalf("ExecuteBatch() itemErrors = %v, want none", itemErrors)
	}
	if len(results) != len(requests) {
		t.Fatalf("ExecuteBatch() result count = %d, want %d", len(results), len(requests))
	}
	if got := maxActive.Load(); got > 3 {
		t.Fatalf("max concurrent executions = %d, want at most 3", got)
	}
}

func TestExecuteBatchHonorsMaxRequestsPerSecond(t *testing.T) {
	requests := make(map[kubeapi.BatchKey]int, 4)
	for i := range 4 {
		name := fmt.Sprintf("item-%02d", i)
		requests[kubeapi.BatchKey{Operation: "apply", Resource: "widgets", Name: name}] = i
	}

	startedAt := make(chan time.Time, len(requests))
	results, itemErrors, err := kubeapi.ExecuteBatch(context.Background(), kubeapi.BatchConfig{
		NumParallelWorkers:   4,
		MaxRequestsPerSecond: 2,
		MaxRequestsInFlight:  4,
	}, zap.NewNop(), kubeapi.BatchOperation[int, int]{
		Operation: "apply",
		Resource:  "widgets",
		Execute: func(_ context.Context, request int) (int, error) {
			startedAt <- time.Now()
			return request, nil
		},
	}, requests)
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if len(itemErrors) != 0 {
		t.Fatalf("ExecuteBatch() itemErrors = %v, want none", itemErrors)
	}
	if len(results) != len(requests) {
		t.Fatalf("ExecuteBatch() result count = %d, want %d", len(results), len(requests))
	}
	close(startedAt)
	timestamps := make([]time.Time, 0, len(requests))
	for value := range startedAt {
		timestamps = append(timestamps, value)
	}
	sort.Slice(timestamps, func(i int, j int) bool {
		return timestamps[i].Before(timestamps[j])
	})
	if len(timestamps) != len(requests) {
		t.Fatalf("execution timestamp count = %d, want %d", len(timestamps), len(requests))
	}
	elapsed := timestamps[len(timestamps)-1].Sub(timestamps[0])
	if elapsed < 900*time.Millisecond {
		t.Fatalf("elapsed between first and last execution = %s, want rate-limited delay near 1s", elapsed)
	}
}

func TestExecuteBatchGathersItemErrorsAndDoesNotRetry(t *testing.T) {
	requests := make(map[kubeapi.BatchKey]int, 6)
	for i := range 6 {
		name := fmt.Sprintf("item-%02d", i)
		requests[kubeapi.BatchKey{Operation: "apply", Resource: "widgets", Name: name}] = i
	}
	failures := map[int]struct{}{
		1: {},
		4: {},
	}
	var attempts atomic.Int64
	results, itemErrors, err := kubeapi.ExecuteBatch(context.Background(), kubeapi.BatchConfig{
		NumParallelWorkers:   3,
		MaxRequestsPerSecond: 1000,
		MaxRequestsInFlight:  3,
	}, zap.NewNop(), kubeapi.BatchOperation[int, int]{
		Operation: "apply",
		Resource:  "widgets",
		Execute: func(_ context.Context, request int) (int, error) {
			attempts.Add(1)
			if _, shouldFail := failures[request]; shouldFail {
				return 0, fmt.Errorf("reject %d", request)
			}
			return request * 10, nil
		},
	}, requests)
	if err == nil {
		t.Fatal("ExecuteBatch() error = nil, want aggregate batch error")
	}
	var batchError *kubeapi.BatchError
	if !errors.As(err, &batchError) {
		t.Fatalf("ExecuteBatch() error type = %T, want BatchError", err)
	}
	if attempts.Load() != int64(len(requests)) {
		t.Fatalf("attempt count = %d, want exactly %d", attempts.Load(), len(requests))
	}
	if len(itemErrors) != len(failures) {
		t.Fatalf("item error count = %d, want %d", len(itemErrors), len(failures))
	}
	if len(batchError.Errors()) != len(itemErrors) {
		t.Fatalf("BatchError.Errors() count = %d, want %d", len(batchError.Errors()), len(itemErrors))
	}
	for key, itemErr := range itemErrors {
		var batchItemError *kubeapi.BatchItemError
		if !errors.As(itemErr, &batchItemError) {
			t.Fatalf("item error for %+v has type %T, want BatchItemError", key, itemErr)
		}
		if batchItemError.Key != key {
			t.Fatalf("BatchItemError.Key = %+v, want %+v", batchItemError.Key, key)
		}
	}
	if len(results) != len(requests)-len(failures) {
		t.Fatalf("result count = %d, want %d", len(results), len(requests)-len(failures))
	}
}

func TestExecuteBatchReportsContextCancellationForUnprocessedItems(t *testing.T) {
	requests := make(map[kubeapi.BatchKey]int, 5)
	for i := range 5 {
		name := fmt.Sprintf("item-%02d", i)
		requests[kubeapi.BatchKey{Operation: "apply", Resource: "widgets", Name: name}] = i
	}
	ctx, cancel := context.WithCancel(context.Background())
	var attempts atomic.Int64
	results, itemErrors, err := kubeapi.ExecuteBatch(ctx, kubeapi.BatchConfig{
		NumParallelWorkers:   1,
		MaxRequestsPerSecond: 1000,
		MaxRequestsInFlight:  1,
	}, zap.NewNop(), kubeapi.BatchOperation[int, int]{
		Operation: "apply",
		Resource:  "widgets",
		Execute: func(_ context.Context, request int) (int, error) {
			if attempts.Add(1) == 1 {
				cancel()
			}
			return request, nil
		},
	}, requests)
	if err == nil {
		t.Fatal("ExecuteBatch() error = nil, want aggregate batch error")
	}
	if len(results) == 0 {
		t.Fatal("ExecuteBatch() returned no successful result, want completed in-flight item preserved")
	}
	if len(itemErrors) == 0 {
		t.Fatal("ExecuteBatch() returned no item errors, want canceled unprocessed items reported")
	}
	for key, itemErr := range itemErrors {
		if !errors.Is(itemErr, context.Canceled) {
			t.Fatalf("item error for %+v = %v, want context.Canceled", key, itemErr)
		}
	}
}

func updateMaxActive(maxActive *atomic.Int64, current int64) {
	for {
		previous := maxActive.Load()
		if current <= previous {
			return
		}
		if maxActive.CompareAndSwap(previous, current) {
			return
		}
	}
}

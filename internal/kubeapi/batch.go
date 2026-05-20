// Package kubeapi provides typed Kubernetes clients for the nsx.ing.com CRDs.
package kubeapi

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// BatchConfig controls concurrency and rate limits for batched Kubernetes calls.
type BatchConfig struct {
	NumParallelWorkers   int
	MaxRequestsPerSecond int
	MaxRequestsInFlight  int
}

// BatchKey identifies one request in a batch operation.
type BatchKey struct {
	Operation   string
	Resource    string
	Subresource string
	Name        string
}

// BatchItemError wraps the error for one failed batch request.
type BatchItemError struct {
	Key BatchKey
	Err error
}

func (e *BatchItemError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("batch item %s/%s %q failed", e.Key.Operation, e.Key.Resource, e.Key.Name)
	}
	return fmt.Sprintf("batch item %s/%s %q failed: %v", e.Key.Operation, e.Key.Resource, e.Key.Name, e.Err)
}

func (e *BatchItemError) Unwrap() error {
	return e.Err
}

// BatchError aggregates per-item errors for a batch operation.
type BatchError struct {
	Operation string
	Resource  string
	Items     map[BatchKey]error
}

func (e BatchError) Error() string {
	return fmt.Sprintf("batch %s/%s failed for %d items", e.Operation, e.Resource, len(e.Items))
}

// Errors returns a defensive copy of the per-item errors.
func (e BatchError) Errors() map[BatchKey]error {
	copied := make(map[BatchKey]error, len(e.Items))
	for key, err := range e.Items {
		copied[key] = err
	}
	return copied
}

// JSONPatchOperation represents one RFC 6902 JSON patch operation.
type JSONPatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	From  string `json:"from,omitempty"`
	Value any    `json:"value,omitempty"`
}

// BatchOperation describes how to execute one request in a typed batch.
type BatchOperation[Request any, Result any] struct {
	Operation string
	Resource  string
	Execute   func(context.Context, Request) (Result, error)
}

// ExecuteBatch executes requests with deterministic scheduling, rate limiting, and partial error reporting.
//
//nolint:contextcheck // ExecuteBatch accepts nil contexts and normalizes them before any child work is scheduled.
func ExecuteBatch[Request any, Result any](
	ctx context.Context,
	cfg BatchConfig,
	log *zap.Logger,
	operation BatchOperation[Request, Result],
	requests map[BatchKey]Request,
) (map[BatchKey]Result, map[BatchKey]error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if log == nil {
		log = zap.NewNop()
	}
	cfg = normalizeBatchConfig(cfg)
	results := make(map[BatchKey]Result, len(requests))
	itemErrors := make(map[BatchKey]error)
	if operation.Execute == nil {
		err := errors.New("batch operation execute function is required")
		return nil, nil, err
	}
	if len(requests) == 0 {
		log.Info(
			"completed kube api batch",
			zap.String("operation", operation.Operation),
			zap.String("resource", operation.Resource),
			zap.Int("itemCount", 0),
			zap.Int("workerCount", cfg.NumParallelWorkers),
			zap.Int("maxRequestsPerSecond", cfg.MaxRequestsPerSecond),
			zap.Int("maxRequestsInFlight", cfg.MaxRequestsInFlight),
			zap.Int("errorCount", 0),
		)
		return results, itemErrors, nil
	}

	log.Info(
		"starting kube api batch",
		zap.String("operation", operation.Operation),
		zap.String("resource", operation.Resource),
		zap.Int("itemCount", len(requests)),
		zap.Int("workerCount", cfg.NumParallelWorkers),
		zap.Int("maxRequestsPerSecond", cfg.MaxRequestsPerSecond),
		zap.Int("maxRequestsInFlight", cfg.MaxRequestsInFlight),
	)

	keys := sortedBatchKeys(requests)
	workCh := make(chan batchWorkItem[Request])
	resultCh := make(chan batchResultItem[Result], len(requests))
	limiter := rate.NewLimiter(rate.Limit(cfg.MaxRequestsPerSecond), cfg.MaxRequestsPerSecond)
	inFlight := make(chan struct{}, cfg.MaxRequestsInFlight)
	for workerID := 0; workerID < cfg.NumParallelWorkers; workerID++ {
		go runBatchWorker(ctx, workerID, log, operation, limiter, inFlight, workCh, resultCh)
	}

	scheduled := make(map[BatchKey]struct{}, len(requests))
	next := 0
	active := 0
	workClosed := false
	for len(results)+len(itemErrors) < len(requests) {
		if next == len(keys) && !workClosed {
			close(workCh)
			workClosed = true
		}

		var sendCh chan<- batchWorkItem[Request]
		var nextWork batchWorkItem[Request]
		if next < len(keys) && ctx.Err() == nil {
			key := keys[next]
			nextWork = batchWorkItem[Request]{key: key, request: requests[key]}
			sendCh = workCh
		}

		select {
		case sendCh <- nextWork:
			key := keys[next]
			scheduled[key] = struct{}{}
			active++
			next++
			log.Debug(
				"scheduled kube api batch item",
				batchKeyFields(key)...,
			)
		case result := <-resultCh:
			active--
			recordBatchResult(result, results, itemErrors)
		case <-ctx.Done():
			if !workClosed {
				close(workCh)
				workClosed = true
			}
			for _, key := range keys {
				if _, ok := scheduled[key]; ok {
					continue
				}
				if _, ok := itemErrors[key]; ok {
					continue
				}
				if _, ok := results[key]; ok {
					continue
				}
				itemErrors[key] = &BatchItemError{Key: key, Err: ctx.Err()}
			}
			for active > 0 {
				result := <-resultCh
				active--
				recordBatchResult(result, results, itemErrors)
			}
		}
	}
	if !workClosed {
		close(workCh)
	}

	log.Info(
		"completed kube api batch",
		zap.String("operation", operation.Operation),
		zap.String("resource", operation.Resource),
		zap.Int("itemCount", len(requests)),
		zap.Int("workerCount", cfg.NumParallelWorkers),
		zap.Int("maxRequestsPerSecond", cfg.MaxRequestsPerSecond),
		zap.Int("maxRequestsInFlight", cfg.MaxRequestsInFlight),
		zap.Int("resultCount", len(results)),
		zap.Int("errorCount", len(itemErrors)),
	)
	if len(itemErrors) > 0 {
		//nolint:nilnil // callers need both partial successes and per-item errors when a batch partially fails.
		return results, itemErrors, BatchError{
			Operation: operation.Operation,
			Resource:  operation.Resource,
			Items:     itemErrors,
		}
	}
	return results, itemErrors, nil
}

func normalizeBatchConfig(cfg BatchConfig) BatchConfig {
	if cfg.NumParallelWorkers <= 0 {
		cfg.NumParallelWorkers = 1
	}
	if cfg.MaxRequestsPerSecond <= 0 {
		cfg.MaxRequestsPerSecond = 100
	}
	if cfg.MaxRequestsInFlight <= 0 {
		cfg.MaxRequestsInFlight = 100
	}
	return cfg
}

type batchWorkItem[Request any] struct {
	key     BatchKey
	request Request
}

type batchResultItem[Result any] struct {
	key    BatchKey
	result Result
	err    error
}

func runBatchWorker[Request any, Result any](
	ctx context.Context,
	workerID int,
	log *zap.Logger,
	operation BatchOperation[Request, Result],
	limiter *rate.Limiter,
	inFlight chan struct{},
	workCh <-chan batchWorkItem[Request],
	resultCh chan<- batchResultItem[Result],
) {
	for item := range workCh {
		result, err := executeBatchItem(ctx, workerID, log, operation, limiter, inFlight, item)
		resultCh <- batchResultItem[Result]{key: item.key, result: result, err: err}
	}
}

func executeBatchItem[Request any, Result any](
	ctx context.Context,
	workerID int,
	log *zap.Logger,
	operation BatchOperation[Request, Result],
	limiter *rate.Limiter,
	inFlight chan struct{},
	item batchWorkItem[Request],
) (Result, error) {
	log.Debug(
		"waiting for kube api batch rate permit",
		append(batchKeyFields(item.key), zap.Int("workerID", workerID))...,
	)
	err := limiter.Wait(ctx)
	if err != nil {
		var zero Result
		log.Debug(
			"kube api batch rate permit wait failed",
			append(batchKeyFields(item.key), zap.Int("workerID", workerID), zap.Error(err))...,
		)
		return zero, &BatchItemError{Key: item.key, Err: err}
	}
	log.Debug(
		"acquired kube api batch rate permit",
		append(batchKeyFields(item.key), zap.Int("workerID", workerID))...,
	)

	log.Debug(
		"waiting for kube api batch in-flight slot",
		append(batchKeyFields(item.key), zap.Int("workerID", workerID))...,
	)
	select {
	case inFlight <- struct{}{}:
	case <-ctx.Done():
		var zero Result
		ctxErr := ctx.Err()
		log.Debug(
			"kube api batch in-flight wait failed",
			append(batchKeyFields(item.key), zap.Int("workerID", workerID), zap.Error(ctxErr))...,
		)
		return zero, &BatchItemError{Key: item.key, Err: ctxErr}
	}
	defer func() {
		<-inFlight
		log.Debug(
			"released kube api batch in-flight slot",
			append(batchKeyFields(item.key), zap.Int("workerID", workerID))...,
		)
	}()

	log.Debug(
		"executing kube api batch item",
		append(batchKeyFields(item.key), zap.Int("workerID", workerID))...,
	)
	result, err := operation.Execute(ctx, item.request)
	if err != nil {
		log.Debug(
			"kube api batch item failed",
			append(batchKeyFields(item.key), zap.Int("workerID", workerID), zap.Error(err))...,
		)
		var zero Result
		return zero, &BatchItemError{Key: item.key, Err: err}
	}
	log.Debug(
		"executed kube api batch item",
		append(batchKeyFields(item.key), zap.Int("workerID", workerID))...,
	)
	return result, nil
}

func recordBatchResult[Result any](result batchResultItem[Result], results map[BatchKey]Result, itemErrors map[BatchKey]error) {
	if result.err != nil {
		itemErrors[result.key] = result.err
		return
	}
	results[result.key] = result.result
}

func sortedBatchKeys[Request any](requests map[BatchKey]Request) []BatchKey {
	keys := make([]BatchKey, 0, len(requests))
	for key := range requests {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i int, j int) bool {
		left := keys[i]
		right := keys[j]
		switch {
		case left.Operation != right.Operation:
			return left.Operation < right.Operation
		case left.Resource != right.Resource:
			return left.Resource < right.Resource
		case left.Subresource != right.Subresource:
			return left.Subresource < right.Subresource
		default:
			return left.Name < right.Name
		}
	})
	return keys
}

func batchKeyFields(key BatchKey) []zap.Field {
	return []zap.Field{
		zap.String("operation", key.Operation),
		zap.String("resource", key.Resource),
		zap.String("subresource", key.Subresource),
		zap.String("name", key.Name),
	}
}

// Package httpratelimit provides shared per-host HTTP client rate limiting.
package httpratelimit

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// Config controls the per-host HTTP rate limits.
type Config struct {
	MaxRequestsInFlightPerHost  int
	MaxRequestsPerSecondPerHost int
}

// NewRoundTripper wraps a base transport with per-host in-flight and rate limits.
func NewRoundTripper(base http.RoundTripper, cfg Config, log *zap.Logger) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if log == nil {
		log = zap.NewNop()
	}
	cfg = normalizeConfig(cfg, log)

	return &roundTripper{
		base: base,
		cfg:  cfg,
		log:  log,
	}
}

type roundTripper struct {
	base http.RoundTripper
	cfg  Config
	log  *zap.Logger
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	key := bucketKey(req)
	log := rt.log.With(zap.String("httpRateLimitBucket", key))
	bkt := globalBuckets.get(key, rt.cfg)

	log.Debug("waiting for HTTP rate limit rate permit")
	err := bkt.waitRate(req.Context())
	if err != nil {
		log.Debug("HTTP rate limit rate wait ended from context", zap.Error(err))
		return nil, err
	}
	log.Debug("acquired HTTP rate limit rate permit")

	log.Debug("waiting for HTTP rate limit in-flight slot")
	err = bkt.acquire(req.Context())
	if err != nil {
		log.Debug("HTTP rate limit in-flight wait ended from context", zap.Error(err))
		return nil, err
	}
	log.Debug("acquired HTTP rate limit in-flight slot")

	log.Debug("starting base HTTP RoundTrip")
	resp, err := rt.base.RoundTrip(req)
	if err != nil {
		bkt.release()
		log.Debug("released HTTP rate limit in-flight slot after RoundTrip error", zap.Error(err))
		return nil, err
	}
	log.Debug("completed base HTTP RoundTrip")
	if resp == nil || resp.Body == nil {
		bkt.release()
		log.Debug("released HTTP rate limit in-flight slot for response without body")
		return resp, nil
	}

	resp.Body = &releaseBody{
		ReadCloser: resp.Body,
		release: func() {
			bkt.release()
			log.Debug("released HTTP rate limit in-flight slot after response body close")
		},
	}
	return resp, nil
}

type bucketRegistry struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

var globalBuckets = bucketRegistry{
	buckets: make(map[string]*bucket),
}

func (registry *bucketRegistry) get(key string, cfg Config) *bucket {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	bkt, ok := registry.buckets[key]
	if ok {
		return bkt
	}

	bkt = &bucket{
		inFlight: make(chan struct{}, cfg.MaxRequestsInFlightPerHost),
		limiter:  rate.NewLimiter(rate.Limit(cfg.MaxRequestsPerSecondPerHost), 1),
	}
	registry.buckets[key] = bkt
	return bkt
}

type bucket struct {
	inFlight chan struct{}
	limiter  *rate.Limiter
}

func (bkt *bucket) waitRate(ctx context.Context) error {
	err := ctx.Err()
	if err != nil {
		return err
	}

	reservation := bkt.limiter.Reserve()
	if !reservation.OK() {
		<-ctx.Done()
		return ctx.Err()
	}

	delay := reservation.Delay()
	if delay == 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		reservation.Cancel()
		return ctx.Err()
	}
}

func (bkt *bucket) acquire(ctx context.Context) error {
	select {
	case bkt.inFlight <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (bkt *bucket) release() {
	<-bkt.inFlight
}

type releaseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (body *releaseBody) Close() error {
	var closeErr error
	body.once.Do(func() {
		closeErr = body.ReadCloser.Close()
		body.release()
	})
	return closeErr
}

func normalizeConfig(cfg Config, log *zap.Logger) Config {
	if cfg.MaxRequestsInFlightPerHost > 0 && cfg.MaxRequestsPerSecondPerHost > 0 {
		return cfg
	}

	normalized := cfg
	if normalized.MaxRequestsInFlightPerHost <= 0 {
		normalized.MaxRequestsInFlightPerHost = 1
	}
	if normalized.MaxRequestsPerSecondPerHost <= 0 {
		normalized.MaxRequestsPerSecondPerHost = 1
	}
	log.Info(
		"normalized non-positive HTTP rate limit configuration",
		zap.Int("maxRequestsInFlightPerHost", cfg.MaxRequestsInFlightPerHost),
		zap.Int("maxRequestsPerSecondPerHost", cfg.MaxRequestsPerSecondPerHost),
		zap.Int("normalizedMaxRequestsInFlightPerHost", normalized.MaxRequestsInFlightPerHost),
		zap.Int("normalizedMaxRequestsPerSecondPerHost", normalized.MaxRequestsPerSecondPerHost),
	)
	return normalized
}

func bucketKey(req *http.Request) string {
	host := strings.ToLower(req.URL.Hostname())
	port := req.URL.Port()
	if port == "" {
		switch req.URL.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return net.JoinHostPort(host, port)
}

package httpratelimit_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/djosh34/nsx-operator/internal/httpratelimit"
	"go.uber.org/zap"
)

func TestRoundTripperSharesInFlightBucketAcrossWrappersForSameEffectiveHostPort(t *testing.T) {
	t.Parallel()

	firstEntered := make(chan struct{}, 1)
	secondEntered := make(chan struct{}, 1)

	cfg := httpratelimit.Config{
		MaxRequestsInFlightPerHost:  1,
		MaxRequestsPerSecondPerHost: 100,
	}
	firstClient := &http.Client{
		Transport: httpratelimit.NewRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
			firstEntered <- struct{}{}
			return okResponse(), nil
		}), cfg, zap.NewNop()),
	}
	secondClient := &http.Client{
		Transport: httpratelimit.NewRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
			secondEntered <- struct{}{}
			return okResponse(), nil
		}), cfg, zap.NewNop()),
	}

	firstRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://EXAMPLE.test/resource", nil)
	if err != nil {
		t.Fatalf("create first request: %v", err)
	}
	firstResponse, err := firstClient.Do(firstRequest)
	if err != nil {
		t.Fatalf("send first request: %v", err)
	}
	t.Cleanup(func() {
		closeErr := firstResponse.Body.Close()
		if closeErr != nil {
			t.Errorf("close first response body cleanup: %v", closeErr)
		}
	})
	receiveSignal(t, firstEntered, "first transport entered")

	secondDone := make(chan error, 1)
	go func() {
		secondRequest, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test:443/other", nil)
		if requestErr != nil {
			secondDone <- requestErr
			return
		}
		secondResponse, requestErr := secondClient.Do(secondRequest)
		if requestErr != nil {
			secondDone <- requestErr
			return
		}
		requestErr = secondResponse.Body.Close()
		if requestErr != nil {
			secondDone <- requestErr
			return
		}
		secondDone <- nil
	}()

	assertNoSignal(t, secondEntered, "second transport entered before first body closed")
	err = firstResponse.Body.Close()
	if err != nil {
		t.Fatalf("close first response body: %v", err)
	}
	receiveSignal(t, secondEntered, "second transport entered after first body closed")
	err = receiveError(t, secondDone, "second request completed")
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
}

func TestRoundTripperIsolatesDifferentPorts(t *testing.T) {
	t.Parallel()

	firstEntered := make(chan struct{}, 1)
	secondEntered := make(chan struct{}, 1)

	cfg := httpratelimit.Config{
		MaxRequestsInFlightPerHost:  1,
		MaxRequestsPerSecondPerHost: 100,
	}
	client := &http.Client{
		Transport: httpratelimit.NewRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Port() {
			case "":
				firstEntered <- struct{}{}
			case "8443":
				secondEntered <- struct{}{}
			default:
				t.Errorf("unexpected request port %q", req.URL.Port())
			}
			return okResponse(), nil
		}), cfg, zap.NewNop()),
	}

	firstRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://ports-isolated.test/resource", nil)
	if err != nil {
		t.Fatalf("create first request: %v", err)
	}
	firstResponse, err := client.Do(firstRequest)
	if err != nil {
		t.Fatalf("send first request: %v", err)
	}
	t.Cleanup(func() {
		closeErr := firstResponse.Body.Close()
		if closeErr != nil {
			t.Errorf("close first response body cleanup: %v", closeErr)
		}
	})
	receiveSignal(t, firstEntered, "first transport entered")

	secondDone := make(chan error, 1)
	go func() {
		secondRequest, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://ports-isolated.test:8443/other", nil)
		if requestErr != nil {
			secondDone <- requestErr
			return
		}
		secondResponse, requestErr := client.Do(secondRequest)
		if requestErr != nil {
			secondDone <- requestErr
			return
		}
		requestErr = secondResponse.Body.Close()
		if requestErr != nil {
			secondDone <- requestErr
			return
		}
		secondDone <- nil
	}()

	receiveSignal(t, secondEntered, "second transport entered for different port")
	err = receiveError(t, secondDone, "second request completed")
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	err = firstResponse.Body.Close()
	if err != nil {
		t.Fatalf("close first response body: %v", err)
	}
}

func TestRoundTripperNormalizesHTTPDefaultPort(t *testing.T) {
	t.Parallel()

	firstEntered := make(chan struct{}, 1)
	secondEntered := make(chan struct{}, 1)

	cfg := httpratelimit.Config{
		MaxRequestsInFlightPerHost:  1,
		MaxRequestsPerSecondPerHost: 100,
	}
	client := &http.Client{
		Transport: httpratelimit.NewRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Host {
			case "HTTP-DEFAULT.test":
				firstEntered <- struct{}{}
			case "http-default.test:80":
				secondEntered <- struct{}{}
			default:
				t.Errorf("unexpected request host %q", req.URL.Host)
			}
			return okResponse(), nil
		}), cfg, zap.NewNop()),
	}

	firstRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://HTTP-DEFAULT.test/resource", nil)
	if err != nil {
		t.Fatalf("create first request: %v", err)
	}
	firstResponse, err := client.Do(firstRequest)
	if err != nil {
		t.Fatalf("send first request: %v", err)
	}
	t.Cleanup(func() {
		closeErr := firstResponse.Body.Close()
		if closeErr != nil {
			t.Errorf("close first response body cleanup: %v", closeErr)
		}
	})
	receiveSignal(t, firstEntered, "first transport entered")

	secondDone := make(chan error, 1)
	go func() {
		secondRequest, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://http-default.test:80/other", nil)
		if requestErr != nil {
			secondDone <- requestErr
			return
		}
		secondResponse, requestErr := client.Do(secondRequest)
		if requestErr != nil {
			secondDone <- requestErr
			return
		}
		requestErr = secondResponse.Body.Close()
		if requestErr != nil {
			secondDone <- requestErr
			return
		}
		secondDone <- nil
	}()

	assertNoSignal(t, secondEntered, "second transport entered before first HTTP default-port body closed")
	err = firstResponse.Body.Close()
	if err != nil {
		t.Fatalf("close first response body: %v", err)
	}
	receiveSignal(t, secondEntered, "second transport entered after first HTTP default-port body closed")
	err = receiveError(t, secondDone, "second request completed")
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
}

func TestRoundTripperReturnsContextErrorWhileWaitingForInFlightSlot(t *testing.T) {
	t.Parallel()

	firstEntered := make(chan struct{}, 1)
	secondEntered := make(chan struct{}, 1)

	cfg := httpratelimit.Config{
		MaxRequestsInFlightPerHost:  1,
		MaxRequestsPerSecondPerHost: 100,
	}
	client := &http.Client{
		Transport: httpratelimit.NewRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/hold":
				firstEntered <- struct{}{}
			case "/wait":
				secondEntered <- struct{}{}
			default:
				t.Errorf("unexpected request path %q", req.URL.Path)
			}
			return okResponse(), nil
		}), cfg, zap.NewNop()),
	}

	firstRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://context-wait.test/hold", nil)
	if err != nil {
		t.Fatalf("create first request: %v", err)
	}
	firstResponse, err := client.Do(firstRequest)
	if err != nil {
		t.Fatalf("send first request: %v", err)
	}
	t.Cleanup(func() {
		closeErr := firstResponse.Body.Close()
		if closeErr != nil {
			t.Errorf("close first response body cleanup: %v", closeErr)
		}
	})
	receiveSignal(t, firstEntered, "first transport entered")

	waitContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	secondRequest, err := http.NewRequestWithContext(waitContext, http.MethodGet, "https://context-wait.test/wait", nil)
	if err != nil {
		t.Fatalf("create second request: %v", err)
	}
	secondResponse, err := client.Do(secondRequest)
	if secondResponse != nil {
		closeErr := secondResponse.Body.Close()
		if closeErr != nil {
			t.Fatalf("close unexpected second response body: %v", closeErr)
		}
		t.Fatalf("second response = %#v, want nil", secondResponse)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second request error = %v, want context deadline exceeded", err)
	}
	assertNoSignal(t, secondEntered, "second transport entered after context deadline")

	err = firstResponse.Body.Close()
	if err != nil {
		t.Fatalf("close first response body: %v", err)
	}
}

func TestRoundTripperReleasesInFlightSlotAfterBaseError(t *testing.T) {
	t.Parallel()

	baseErr := errors.New("base transport failed")
	secondEntered := make(chan struct{}, 1)

	cfg := httpratelimit.Config{
		MaxRequestsInFlightPerHost:  1,
		MaxRequestsPerSecondPerHost: 100,
	}
	client := &http.Client{
		Transport: httpratelimit.NewRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/fail":
				return nil, baseErr
			case "/after":
				secondEntered <- struct{}{}
				return okResponse(), nil
			default:
				t.Errorf("unexpected request path %q", req.URL.Path)
				return okResponse(), nil
			}
		}), cfg, zap.NewNop()),
	}

	firstRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://error-release.test/fail", nil)
	if err != nil {
		t.Fatalf("create first request: %v", err)
	}
	firstResponse, err := client.Do(firstRequest)
	if firstResponse != nil {
		closeErr := firstResponse.Body.Close()
		if closeErr != nil {
			t.Fatalf("close unexpected first response body: %v", closeErr)
		}
		t.Fatalf("first response = %#v, want nil", firstResponse)
	}
	if !errors.Is(err, baseErr) {
		t.Fatalf("first request error = %v, want %v", err, baseErr)
	}

	secondRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://error-release.test/after", nil)
	if err != nil {
		t.Fatalf("create second request: %v", err)
	}
	secondResponse, err := client.Do(secondRequest)
	if err != nil {
		t.Fatalf("send second request: %v", err)
	}
	err = secondResponse.Body.Close()
	if err != nil {
		t.Fatalf("close second response body: %v", err)
	}
	receiveSignal(t, secondEntered, "second transport entered after base error")
}

func TestRoundTripperReleasesInFlightSlotOnceWhenBodyClosedMultipleTimes(t *testing.T) {
	t.Parallel()

	entered := make(chan string, 3)

	cfg := httpratelimit.Config{
		MaxRequestsInFlightPerHost:  1,
		MaxRequestsPerSecondPerHost: 100,
	}
	client := &http.Client{
		Transport: httpratelimit.NewRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			entered <- req.URL.Path
			return okResponse(), nil
		}), cfg, zap.NewNop()),
	}

	firstRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://close-once.test/hold", nil)
	if err != nil {
		t.Fatalf("create first request: %v", err)
	}
	firstResponse, err := client.Do(firstRequest)
	if err != nil {
		t.Fatalf("send first request: %v", err)
	}
	t.Cleanup(func() {
		closeErr := firstResponse.Body.Close()
		if closeErr != nil {
			t.Errorf("close first response body cleanup: %v", closeErr)
		}
	})
	if path := receiveString(t, entered, "first transport entered"); path != "/hold" {
		t.Fatalf("first entered path = %q, want /hold", path)
	}

	results := make(chan requestResult, 2)
	startRequest := func(path string) {
		request, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://close-once.test"+path, nil)
		if requestErr != nil {
			results <- requestResult{path: path, err: requestErr}
			return
		}
		//nolint:bodyclose // responses are closed by the receiving assertions to verify in-flight slot release ordering.
		response, requestErr := client.Do(request)
		results <- requestResult{path: path, response: response, err: requestErr}
	}
	go startRequest("/wait-1")
	go startRequest("/wait-2")

	assertNoStringSignal(t, entered, "waiter entered before first body closed")
	err = firstResponse.Body.Close()
	if err != nil {
		t.Fatalf("close first response body first time: %v", err)
	}
	err = firstResponse.Body.Close()
	if err != nil {
		t.Fatalf("close first response body second time: %v", err)
	}

	firstAdmittedPath := receiveString(t, entered, "first waiter entered")
	firstAdmitted := receiveRequestResult(t, results, "first waiter result")
	if firstAdmitted.err != nil {
		t.Fatalf("first admitted request %s: %v", firstAdmitted.path, firstAdmitted.err)
	}
	if firstAdmitted.response == nil || firstAdmitted.response.Body == nil {
		t.Fatalf("first admitted response for %s has no body", firstAdmitted.path)
	}
	if firstAdmitted.path != firstAdmittedPath {
		t.Fatalf("first admitted result path = %q, entered path = %q", firstAdmitted.path, firstAdmittedPath)
	}
	assertNoStringSignal(t, entered, "second waiter entered after duplicate first body close")

	err = firstAdmitted.response.Body.Close()
	if err != nil {
		t.Fatalf("close first admitted response body: %v", err)
	}
	secondAdmittedPath := receiveString(t, entered, "second waiter entered")
	secondAdmitted := receiveRequestResult(t, results, "second waiter result")
	if secondAdmitted.err != nil {
		t.Fatalf("second admitted request %s: %v", secondAdmitted.path, secondAdmitted.err)
	}
	if secondAdmitted.response == nil || secondAdmitted.response.Body == nil {
		t.Fatalf("second admitted response for %s has no body", secondAdmitted.path)
	}
	if secondAdmitted.path != secondAdmittedPath {
		t.Fatalf("second admitted result path = %q, entered path = %q", secondAdmitted.path, secondAdmittedPath)
	}
	err = secondAdmitted.response.Body.Close()
	if err != nil {
		t.Fatalf("close second admitted response body: %v", err)
	}
}

func TestRoundTripperReturnsContextErrorWhileWaitingForRatePermit(t *testing.T) {
	t.Parallel()

	firstEntered := make(chan struct{}, 1)
	secondEntered := make(chan struct{}, 1)

	cfg := httpratelimit.Config{
		MaxRequestsInFlightPerHost:  10,
		MaxRequestsPerSecondPerHost: 1,
	}
	client := &http.Client{
		Transport: httpratelimit.NewRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/first":
				firstEntered <- struct{}{}
			case "/second":
				secondEntered <- struct{}{}
			default:
				t.Errorf("unexpected request path %q", req.URL.Path)
			}
			return okResponse(), nil
		}), cfg, zap.NewNop()),
	}

	firstRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://rate-wait.test/first", nil)
	if err != nil {
		t.Fatalf("create first request: %v", err)
	}
	firstResponse, err := client.Do(firstRequest)
	if err != nil {
		t.Fatalf("send first request: %v", err)
	}
	err = firstResponse.Body.Close()
	if err != nil {
		t.Fatalf("close first response body: %v", err)
	}
	receiveSignal(t, firstEntered, "first transport entered")

	waitContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	secondRequest, err := http.NewRequestWithContext(waitContext, http.MethodGet, "https://rate-wait.test/second", nil)
	if err != nil {
		t.Fatalf("create second request: %v", err)
	}
	secondResponse, err := client.Do(secondRequest)
	if secondResponse != nil {
		closeErr := secondResponse.Body.Close()
		if closeErr != nil {
			t.Fatalf("close unexpected second response body: %v", closeErr)
		}
		t.Fatalf("second response = %#v, want nil", secondResponse)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second request error = %v, want context deadline exceeded", err)
	}
	assertNoSignal(t, secondEntered, "second transport entered after rate context deadline")
}

func TestRoundTripperNilBaseUsesDefaultTransport(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			t.Errorf("request method = %s, want GET", req.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := httpratelimit.Config{
		MaxRequestsInFlightPerHost:  1,
		MaxRequestsPerSecondPerHost: 100,
	}
	client := &http.Client{
		Transport: httpratelimit.NewRoundTripper(nil, cfg, nil),
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("send request through nil-base limiter: %v", err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("response status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
	err = response.Body.Close()
	if err != nil {
		t.Fatalf("close response body: %v", err)
	}
}

func TestRoundTripperReleasesInFlightSlotForNilResponseBody(t *testing.T) {
	t.Parallel()

	firstEntered := make(chan struct{}, 1)
	secondEntered := make(chan struct{}, 1)

	cfg := httpratelimit.Config{
		MaxRequestsInFlightPerHost:  1,
		MaxRequestsPerSecondPerHost: 1000,
	}
	transport := httpratelimit.NewRoundTripper(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/nil-body":
			firstEntered <- struct{}{}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
			}, nil
		case "/after":
			secondEntered <- struct{}{}
			return okResponse(), nil
		default:
			t.Errorf("unexpected request path %q", req.URL.Path)
			return okResponse(), nil
		}
	}), cfg, zap.NewNop())

	firstRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://nil-body-release.test/nil-body", nil)
	if err != nil {
		t.Fatalf("create first request: %v", err)
	}
	//nolint:bodyclose // this case verifies that responses without a body release their in-flight slot immediately.
	firstResponse, err := transport.RoundTrip(firstRequest)
	if err != nil {
		t.Fatalf("send first request: %v", err)
	}
	if firstResponse == nil {
		t.Fatal("first response is nil")
	}
	if firstResponse.Body != nil {
		t.Fatalf("first response body = %#v, want nil", firstResponse.Body)
	}
	receiveSignal(t, firstEntered, "first transport entered")

	secondRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://nil-body-release.test/after", nil)
	if err != nil {
		t.Fatalf("create second request: %v", err)
	}
	secondResponse, err := transport.RoundTrip(secondRequest)
	if err != nil {
		t.Fatalf("send second request: %v", err)
	}
	if secondResponse == nil || secondResponse.Body == nil {
		t.Fatal("second response has no body")
	}
	err = secondResponse.Body.Close()
	if err != nil {
		t.Fatalf("close second response body: %v", err)
	}
	receiveSignal(t, secondEntered, "second transport entered after nil-body response")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type requestResult struct {
	path     string
	response *http.Response
	err      error
}

func okResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(http.NoBody),
		Header:     make(http.Header),
	}
}

func receiveSignal(t *testing.T, ch <-chan struct{}, description string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func assertNoSignal(t *testing.T, ch <-chan struct{}, failure string) {
	t.Helper()

	select {
	case <-ch:
		t.Fatal(failure)
	case <-time.After(50 * time.Millisecond):
	}
}

func receiveError(t *testing.T, ch <-chan error, description string) error {
	t.Helper()

	select {
	case err := <-ch:
		return err
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
	return nil
}

func receiveString(t *testing.T, ch <-chan string, description string) string {
	t.Helper()

	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
	return ""
}

func assertNoStringSignal(t *testing.T, ch <-chan string, failure string) {
	t.Helper()

	select {
	case value := <-ch:
		t.Fatalf("%s: %s", failure, value)
	case <-time.After(50 * time.Millisecond):
	}
}

func receiveRequestResult(t *testing.T, ch <-chan requestResult, description string) requestResult {
	t.Helper()

	select {
	case result := <-ch:
		return result
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
	return requestResult{}
}

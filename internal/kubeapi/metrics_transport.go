package kubeapi

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/operatormetrics"
	"go.uber.org/zap"
	"k8s.io/client-go/transport"
)

func wrapKubernetesMetricsTransport(
	existing transport.WrapperFunc,
	recorder operatormetrics.Recorder,
	logger *zap.Logger,
) transport.WrapperFunc {
	if recorder == nil {
		recorder = operatormetrics.NopRecorder{}
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(rt http.RoundTripper) http.RoundTripper {
		if existing != nil {
			rt = existing(rt)
		}
		return kubernetesMetricsRoundTripper{
			next:     rt,
			recorder: recorder,
			log:      logger,
		}
	}
}

type kubernetesMetricsRoundTripper struct {
	next     http.RoundTripper
	recorder operatormetrics.Recorder
	log      *zap.Logger
}

func (rt kubernetesMetricsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	function := kubernetesFunction(req)
	requestBody := wrapRequestBody(req)
	start := time.Now()
	resp, err := rt.next.RoundTrip(req)
	if err != nil {
		rt.recorder.ObserveKubernetesAPI(function, requestBody.bytesRead(), 0, time.Since(start))
		rt.log.Debug("recorded failed kubernetes api metric", zap.String("function", function), zap.Error(err))
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		rt.recorder.ObserveKubernetesAPI(function, requestBody.bytesRead(), 0, time.Since(start))
		rt.log.Debug("recorded kubernetes api metric without response body", zap.String("function", function))
		return resp, nil
	}
	resp.Body = &observedResponseBody{
		ReadCloser: resp.Body,
		observe: func(responseBytes int64) {
			rt.recorder.ObserveKubernetesAPI(function, requestBody.bytesRead(), responseBytes, time.Since(start))
			rt.log.Debug(
				"recorded kubernetes api metric",
				zap.String("function", function),
				zap.Int64("requestBytes", requestBody.bytesRead()),
				zap.Int64("responseBytes", responseBytes),
				zap.Duration("duration", time.Since(start)),
			)
		},
	}
	return resp, nil
}

type countedBody struct {
	io.ReadCloser
	count atomic.Int64
}

func wrapRequestBody(req *http.Request) *countedBody {
	if req == nil || req.Body == nil {
		return &countedBody{}
	}
	body := &countedBody{ReadCloser: req.Body}
	req.Body = body
	return body
}

func (body *countedBody) Read(p []byte) (int, error) {
	if body == nil || body.ReadCloser == nil {
		return 0, io.EOF
	}
	n, err := body.ReadCloser.Read(p)
	body.count.Add(int64(n))
	return n, err
}

func (body *countedBody) Close() error {
	if body == nil || body.ReadCloser == nil {
		return nil
	}
	return body.ReadCloser.Close()
}

func (body *countedBody) bytesRead() int64 {
	if body == nil {
		return 0
	}
	return body.count.Load()
}

type observedResponseBody struct {
	io.ReadCloser
	observe func(int64)
	once    sync.Once
	count   atomic.Int64
}

func (body *observedResponseBody) Read(p []byte) (int, error) {
	n, err := body.ReadCloser.Read(p)
	body.count.Add(int64(n))
	return n, err
}

func (body *observedResponseBody) Close() error {
	closeErr := body.ReadCloser.Close()
	body.once.Do(func() {
		body.observe(body.count.Load())
	})
	return closeErr
}

func kubernetesFunction(req *http.Request) string {
	if req == nil || req.URL == nil {
		return "unknown"
	}
	parts := strings.Split(strings.Trim(req.URL.EscapedPath(), "/"), "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] != "apis" || parts[i+1] != nsxv1alpha.GroupName {
			continue
		}
		resourceIndex := i + 3
		if resourceIndex >= len(parts) {
			return "unknown"
		}
		resource := kubernetesResourceLabel(parts[resourceIndex])
		if resource == "unknown" {
			return "unknown"
		}
		return resource + "." + kubernetesOperation(req.Method, parts[resourceIndex+1:])
	}
	return "unknown"
}

func kubernetesResourceLabel(resource string) string {
	switch resource {
	case groupResource:
		return "groups"
	case networkCloudResource:
		return "network_clouds"
	default:
		return "unknown"
	}
}

func kubernetesOperation(method string, tail []string) string {
	subresource := ""
	if len(tail) >= 2 {
		subresource = tail[1]
	}
	switch {
	case method == http.MethodGet && len(tail) == 0:
		return "list"
	case method == http.MethodGet:
		return "get"
	case method == http.MethodPost && len(tail) == 0:
		return "create"
	case method == http.MethodPut && subresource == "status":
		return "update_status"
	case method == http.MethodPut:
		return "update"
	case method == http.MethodPatch:
		return "apply"
	case method == http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

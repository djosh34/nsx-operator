// Package operatormetrics records Prometheus metrics for operator activity.
package operatormetrics

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

const (
	modeObserve       = "observe"
	modeManage        = "manage"
	directionRequest  = "request"
	directionResponse = "response"
)

// Recorder records operator metrics.
type Recorder interface {
	ObserveNSXCall(manager string, function string)
	ObserveNSXHTTP(manager string, function string, requestBytes int64, responseBytes int64, duration time.Duration)
	ObserveKubernetesAPI(function string, requestBytes int64, responseBytes int64, duration time.Duration)
	SetManagerGroupSnapshot(manager string, snapshot ManagerGroupSnapshot)
}

// ManagerGroupSnapshot captures manager group counts from one reconciliation pass.
type ManagerGroupSnapshot struct {
	ListedGroups         int
	ObserveGroups        int
	ManageGroups         int
	ObserveUpdatesNeeded int
	ManageUpdatesNeeded  int
	CreatesNeeded        int
}

// NopRecorder discards all metric observations.
type NopRecorder struct{}

var _ Recorder = (*NopRecorder)(nil)

// ObserveNSXCall discards an NSX client call observation.
func (receiver *NopRecorder) ObserveNSXCall(string, string) {}

// ObserveNSXHTTP discards an NSX HTTP observation.
func (receiver *NopRecorder) ObserveNSXHTTP(string, string, int64, int64, time.Duration) {}

// ObserveKubernetesAPI discards a Kubernetes API observation.
func (receiver *NopRecorder) ObserveKubernetesAPI(string, int64, int64, time.Duration) {}

// SetManagerGroupSnapshot discards a manager group snapshot.
func (receiver *NopRecorder) SetManagerGroupSnapshot(string, ManagerGroupSnapshot) {}

// PrometheusRecorder records operator metrics into Prometheus collectors.
type PrometheusRecorder struct {
	log *zap.Logger

	nsxGroupsListed          *prometheus.GaugeVec
	nsxGroupsObserve         *prometheus.GaugeVec
	nsxGroupsManage          *prometheus.GaugeVec
	nsxGroupCRUpdatesNeeded  *prometheus.GaugeVec
	nsxGroupCRCreatesNeeded  *prometheus.GaugeVec
	nsxClientCalls           *prometheus.CounterVec
	nsxHTTPRequests          *prometheus.CounterVec
	nsxHTTPBytes             *prometheus.CounterVec
	nsxHTTPRoundTrip         *prometheus.HistogramVec
	nsxHTTPFunctionRoundTrip *prometheus.HistogramVec
	kubernetesAPICalls       *prometheus.CounterVec
	kubernetesAPIBytes       *prometheus.CounterVec
	kubernetesAPIRoundTrip   *prometheus.HistogramVec
}

var _ Recorder = (*PrometheusRecorder)(nil)

var processRecorder struct {
	once     sync.Once
	recorder Recorder
	err      error
}

// NewProcessRecorder returns a process-wide Prometheus recorder.
func NewProcessRecorder(registerer prometheus.Registerer, logger *zap.Logger) (Recorder, error) {
	processRecorder.once.Do(func() {
		processRecorder.recorder, processRecorder.err = NewRecorder(registerer, logger)
	})
	if processRecorder.err != nil {
		return nil, processRecorder.err
	}
	return processRecorder.recorder, nil
}

// NewRecorder registers collectors and returns a Prometheus-backed recorder.
func NewRecorder(registerer prometheus.Registerer, logger *zap.Logger) (*PrometheusRecorder, error) {
	if registerer == nil {
		return nil, errors.New("prometheus registerer is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	recorder := &PrometheusRecorder{
		log: logger,
		nsxGroupsListed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nsx_operator_nsx_groups_listed_total",
			Help: "Last manager sweep total groups listed from NSX.",
		}, []string{"manager"}),
		nsxGroupsObserve: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nsx_operator_nsx_groups_observe_total",
			Help: "Last manager sweep total observe groups considered for this manager.",
		}, []string{"manager"}),
		nsxGroupsManage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nsx_operator_nsx_groups_manage_total",
			Help: "Last manager sweep total manage groups considered for this manager.",
		}, []string{"manager"}),
		nsxGroupCRUpdatesNeeded: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nsx_operator_nsx_group_cr_updates_needed_total",
			Help: "Last manager sweep total group CR updates needed by mode.",
		}, []string{"manager", "mode"}),
		nsxGroupCRCreatesNeeded: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nsx_operator_nsx_group_cr_creates_needed_total",
			Help: "Last manager sweep total new group CRs that need to be created.",
		}, []string{"manager"}),
		nsxClientCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nsx_operator_nsx_client_calls_total",
			Help: "Total NSX client calls by manager and function.",
		}, []string{"manager", "function"}),
		nsxHTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nsx_operator_nsx_http_requests_total",
			Help: "Total NSX HTTP requests by manager.",
		}, []string{"manager"}),
		nsxHTTPBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nsx_operator_nsx_http_bytes_total",
			Help: "Total NSX HTTP bytes by manager and direction.",
		}, []string{"manager", "direction"}),
		nsxHTTPRoundTrip: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "nsx_operator_nsx_http_round_trip_seconds",
			Help: "Whole NSX HTTP round trip duration by manager.",
		}, []string{"manager"}),
		nsxHTTPFunctionRoundTrip: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "nsx_operator_nsx_http_function_round_trip_seconds",
			Help: "Whole NSX HTTP round trip duration by manager and function.",
		}, []string{"manager", "function"}),
		kubernetesAPICalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nsx_operator_kubernetes_api_calls_total",
			Help: "Total Kubernetes API calls by typed client function.",
		}, []string{"function"}),
		kubernetesAPIBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nsx_operator_kubernetes_api_bytes_total",
			Help: "Total Kubernetes API bytes by typed client function and direction.",
		}, []string{"function", "direction"}),
		kubernetesAPIRoundTrip: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "nsx_operator_kubernetes_api_round_trip_seconds",
			Help: "Whole Kubernetes API round trip duration by typed client function.",
		}, []string{"function"}),
	}

	collectors := []prometheus.Collector{
		recorder.nsxGroupsListed,
		recorder.nsxGroupsObserve,
		recorder.nsxGroupsManage,
		recorder.nsxGroupCRUpdatesNeeded,
		recorder.nsxGroupCRCreatesNeeded,
		recorder.nsxClientCalls,
		recorder.nsxHTTPRequests,
		recorder.nsxHTTPBytes,
		recorder.nsxHTTPRoundTrip,
		recorder.nsxHTTPFunctionRoundTrip,
		recorder.kubernetesAPICalls,
		recorder.kubernetesAPIBytes,
		recorder.kubernetesAPIRoundTrip,
	}
	for _, collector := range collectors {
		err := registerer.Register(collector)
		if err != nil {
			return nil, fmt.Errorf("register operator metrics collector: %w", err)
		}
	}
	logger.Debug("registered operator prometheus metrics")
	return recorder, nil
}

// ObserveNSXCall records one NSX client call.
func (r *PrometheusRecorder) ObserveNSXCall(manager string, function string) {
	r.nsxClientCalls.WithLabelValues(manager, function).Inc()
	r.log.Debug("recorded nsx client call metric", zap.String("manager", manager), zap.String("function", function))
}

// ObserveNSXHTTP records one NSX HTTP round trip.
func (r *PrometheusRecorder) ObserveNSXHTTP(manager string, function string, requestBytes int64, responseBytes int64, duration time.Duration) {
	r.nsxHTTPRequests.WithLabelValues(manager).Inc()
	addNonNegative(r.nsxHTTPBytes.WithLabelValues(manager, directionRequest), requestBytes)
	addNonNegative(r.nsxHTTPBytes.WithLabelValues(manager, directionResponse), responseBytes)
	r.nsxHTTPRoundTrip.WithLabelValues(manager).Observe(duration.Seconds())
	r.nsxHTTPFunctionRoundTrip.WithLabelValues(manager, function).Observe(duration.Seconds())
	r.log.Debug(
		"recorded nsx http metric",
		zap.String("manager", manager),
		zap.String("function", function),
		zap.Int64("requestBytes", requestBytes),
		zap.Int64("responseBytes", responseBytes),
		zap.Duration("duration", duration),
	)
}

// ObserveKubernetesAPI records one Kubernetes API round trip.
func (r *PrometheusRecorder) ObserveKubernetesAPI(function string, requestBytes int64, responseBytes int64, duration time.Duration) {
	r.kubernetesAPICalls.WithLabelValues(function).Inc()
	addNonNegative(r.kubernetesAPIBytes.WithLabelValues(function, directionRequest), requestBytes)
	addNonNegative(r.kubernetesAPIBytes.WithLabelValues(function, directionResponse), responseBytes)
	r.kubernetesAPIRoundTrip.WithLabelValues(function).Observe(duration.Seconds())
	r.log.Debug(
		"recorded kubernetes api metric",
		zap.String("function", function),
		zap.Int64("requestBytes", requestBytes),
		zap.Int64("responseBytes", responseBytes),
		zap.Duration("duration", duration),
	)
}

// SetManagerGroupSnapshot records manager group counts.
func (r *PrometheusRecorder) SetManagerGroupSnapshot(manager string, snapshot ManagerGroupSnapshot) {
	r.nsxGroupsListed.WithLabelValues(manager).Set(float64(snapshot.ListedGroups))
	r.nsxGroupsObserve.WithLabelValues(manager).Set(float64(snapshot.ObserveGroups))
	r.nsxGroupsManage.WithLabelValues(manager).Set(float64(snapshot.ManageGroups))
	r.nsxGroupCRUpdatesNeeded.WithLabelValues(manager, modeObserve).Set(float64(snapshot.ObserveUpdatesNeeded))
	r.nsxGroupCRUpdatesNeeded.WithLabelValues(manager, modeManage).Set(float64(snapshot.ManageUpdatesNeeded))
	r.nsxGroupCRCreatesNeeded.WithLabelValues(manager).Set(float64(snapshot.CreatesNeeded))
	r.log.Debug(
		"recorded manager group snapshot metrics",
		zap.String("manager", manager),
		zap.Int("listedGroups", snapshot.ListedGroups),
		zap.Int("observeGroups", snapshot.ObserveGroups),
		zap.Int("manageGroups", snapshot.ManageGroups),
		zap.Int("observeUpdatesNeeded", snapshot.ObserveUpdatesNeeded),
		zap.Int("manageUpdatesNeeded", snapshot.ManageUpdatesNeeded),
		zap.Int("createsNeeded", snapshot.CreatesNeeded),
	)
}

func addNonNegative(counter prometheus.Counter, value int64) {
	if value <= 0 {
		return
	}
	counter.Add(float64(value))
}

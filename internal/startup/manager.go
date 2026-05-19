package startup

import (
	"context"
	"fmt"
	"net/http"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/config"
	"github.com/djosh34/nsx-operator/internal/httpratelimit"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/logging"
	"github.com/djosh34/nsx-operator/internal/names"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
	"github.com/djosh34/nsx-operator/internal/operatormetrics"
	"github.com/djosh34/nsx-operator/internal/stateoperator"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type ManagerOptions struct {
	Config      config.Config
	RestConfig  *rest.Config
	Logger      *zap.Logger
	SweepCloud  stateoperator.CloudSweepFunc
	Clock       stateoperator.Clock
	IDGenerator stateoperator.SweepIDGenerator
}

func NewManager(options ManagerOptions) (manager.Manager, error) {
	logger := options.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	ctrl.SetLogger(zapr.NewLogger(logger))
	logger.Info("constructing controller runtime manager", logging.Component("startup"))

	restConfig := options.RestConfig
	if restConfig == nil {
		loadedConfig, err := ctrl.GetConfig()
		if err != nil {
			logger.Info("controller runtime rest config construction failed", logging.Component("startup"), zap.Error(err))
			return nil, fmt.Errorf("construct controller runtime rest config: %w", err)
		}
		restConfig = loadedConfig
	}

	runtimeScheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(runtimeScheme); err != nil {
		logger.Info("register kubernetes scheme failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("register kubernetes scheme: %w", err)
	}
	if err := nsxv1alpha.AddToScheme(runtimeScheme); err != nil {
		logger.Info("register nsx scheme failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("register nsx scheme: %w", err)
	}

	skipNameValidation := true
	metricsBindAddress := options.Config.Operator.MetricsBindAddress
	if metricsBindAddress == "" {
		metricsBindAddress = "0"
	}
	operatorRecorder, err := operatormetrics.NewProcessRecorder(ctrlmetrics.Registry, logger)
	if err != nil {
		logger.Info("operator metrics recorder construction failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("construct operator metrics recorder: %w", err)
	}
	logger.Info("configuring metrics endpoint", logging.Component("startup"), zap.String("metricsBindAddress", metricsBindAddress))
	runtimeManager, err := ctrl.NewManager(restConfig, manager.Options{
		Scheme: runtimeScheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsBindAddress,
		},
		Controller: controllerconfig.Controller{
			SkipNameValidation: &skipNameValidation,
		},
	})
	if err != nil {
		logger.Info("controller runtime manager construction failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("construct controller runtime manager: %w", err)
	}

	typedKubeClient, err := kubeapi.NewClient(kubeapi.Options{
		Config: restConfig,
		Logger: logger,
		BatchConfig: kubeapi.BatchConfig{
			NumParallelWorkers:   options.Config.KubeAPI.NumParallelWorkers,
			MaxRequestsPerSecond: options.Config.KubeAPI.MaxRequestsPerSecond,
			MaxRequestsInFlight:  options.Config.KubeAPI.MaxRequestsInFlight,
		},
		Recorder: operatorRecorder,
	})
	if err != nil {
		logger.Info("typed kubernetes crd client construction failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("construct typed kubernetes crd client: %w", err)
	}
	nsxHTTPClient := newNSXHTTPClient(options.Config.HTTPRateLimiter, logger)
	managerClientFactory := func(_ context.Context, cloud nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
		normalizedFQDN := names.NormalizeNetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN)
		client, err := nsxclient.NewClient(nsxclient.Options{
			BaseURL:    nsxManagerBaseURL(options.Config.NSX, normalizedFQDN),
			HTTPClient: nsxHTTPClient,
			Username:   options.Config.NSX.Auth.Username,
			Password:   options.Config.NSX.Auth.Password,
			Logger:     logger,
			Recorder:   operatorRecorder,
			WriteControl: writeControlForCloud(
				options.Config.NSX,
				cloud,
			),
		})
		if err != nil {
			return nil, err
		}
		return client, nil
	}

	operator, err := stateoperator.New(stateoperator.Options{
		Client:               runtimeManager.GetClient(),
		KubeClient:           typedKubeClient,
		TickInterval:         options.Config.Operator.TickInterval,
		Logger:               logger,
		SweepCloud:           options.SweepCloud,
		ManagerClientFactory: managerClientFactory,
		Clock:                options.Clock,
		IDGenerator:          options.IDGenerator,
		Recorder:             operatorRecorder,
	})
	if err != nil {
		logger.Info("state operator construction failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("construct state operator: %w", err)
	}

	if err := runtimeManager.Add(operator); err != nil {
		logger.Info("state operator runnable registration failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("register state operator runnable: %w", err)
	}
	if err := builder.ControllerManagedBy(runtimeManager).
		Named("nsxnetworkcloud").
		For(&nsxv1alpha.NSXNetworkCloud{}).
		Complete(stateoperator.NetworkCloudReconciler{
			Client: runtimeManager.GetClient(),
			Logger: logger,
		}); err != nil {
		logger.Info("nsx network cloud controller registration failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("register nsx network cloud controller: %w", err)
	}
	if err := builder.ControllerManagedBy(runtimeManager).
		Named("nsxgroup").
		For(&nsxv1alpha.NSXGroup{}).
		Complete(stateoperator.GroupReconciler{
			Client:               runtimeManager.GetClient(),
			ManagerClientFactory: managerClientFactory,
			Logger:               logger,
			Clock:                options.Clock,
		}); err != nil {
		logger.Info("nsx group controller registration failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("register nsx group controller: %w", err)
	}

	logger.Info("constructed controller runtime manager", logging.Component("startup"))
	return runtimeManager, nil
}

func newNSXHTTPClient(cfg config.HTTPRateLimiterConfig, logger *zap.Logger) *http.Client {
	limiterConfig := httpratelimit.Config{
		MaxRequestsInFlightPerHost:  cfg.MaxRequestsInFlightPerHost,
		MaxRequestsPerSecondPerHost: cfg.MaxRequestsPerSecondPerHost,
	}
	logger.Info(
		"constructing shared nsx manager http client",
		logging.Component("startup"),
		zap.Int("http_max_requests_in_flight_per_host", limiterConfig.MaxRequestsInFlightPerHost),
		zap.Int("http_max_requests_per_second_per_host", limiterConfig.MaxRequestsPerSecondPerHost),
	)
	return &http.Client{
		Transport: httpratelimit.NewRoundTripper(http.DefaultTransport, limiterConfig, logger),
	}
}

func nsxManagerBaseURL(cfg config.NSXConfig, normalizedFQDN string) string {
	scheme := cfg.URLScheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + normalizedFQDN
}

func writeControlForCloud(cfg config.NSXConfig, cloud nsxv1alpha.NSXNetworkCloud) nsxclient.WriteControl {
	normalizedFQDN := names.NormalizeNetworkCloudFQDN(cloud.Spec.NetworkCloudFQDN)
	globalWritesEnabled := cfg.WritesEnabled
	if !cfg.WritesEnabledConfigured {
		globalWritesEnabled = true
	}
	control := nsxclient.WriteControl{
		Enabled:          true,
		NetworkCloudName: cloud.Name,
		NetworkCloudFQDN: normalizedFQDN,
	}
	if !globalWritesEnabled {
		control.Enabled = false
		control.Reason = nsxclient.WriteDisabledReasonGlobalConfig
		return control
	}
	if !cloud.Spec.NSXWritesEnabled() {
		control.Enabled = false
		control.Reason = nsxclient.WriteDisabledReasonNetworkCloud
		return control
	}
	return control
}

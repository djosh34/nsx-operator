// Package startup loads runtime configuration and starts the operator manager.
package startup

import (
	"context"
	"fmt"

	"github.com/djosh34/nsx-operator/internal/config"
	"github.com/djosh34/nsx-operator/internal/logging"
	"go.uber.org/zap"
)

// LoggerFactory builds the runtime logger from validated logging config.
type LoggerFactory func(config.LoggingConfig) (*zap.Logger, error)

// RunnableManager is the subset of controller-runtime manager behavior Run needs.
type RunnableManager interface {
	Start(context.Context) error
}

// ManagerFactory builds the runnable controller manager.
type ManagerFactory func(ManagerOptions) (RunnableManager, error)

// RuntimeConstructors are optional dependency construction hooks used by tests.
type RuntimeConstructors struct {
	Kubernetes func(config.Config) error
	NSX        func(config.Config) error
}

// Options configures startup orchestration.
type Options struct {
	Config          config.Options
	Constructors    RuntimeConstructors
	Context         context.Context
	ManagerFactory  ManagerFactory
	BootstrapLogger *zap.Logger
	LoggerFactory   LoggerFactory
}

// Run loads configuration, constructs runtime dependencies, and starts the manager.
//
//nolint:gocritic // public startup API keeps value options so callers can pass literals.
func Run(options Options) error {
	bootstrapLogger := options.BootstrapLogger
	if bootstrapLogger == nil {
		bootstrapLogger = zap.NewNop()
	}

	configOptions := options.Config
	if configOptions.Logger == nil {
		configOptions.Logger = bootstrapLogger
	}

	bootstrapLogger.Info("loading startup config", logging.Component("startup"), zap.String("config_path", configOptions.Path))
	loadedConfig, err := config.Load(configOptions)
	if err != nil {
		bootstrapLogger.Info("startup config validation failed", logging.Component("startup"), zap.Error(err))
		return fmt.Errorf("load startup config: %w", err)
	}

	loggerFactory := options.LoggerFactory
	if loggerFactory == nil {
		loggerFactory = func(loggingConfig config.LoggingConfig) (*zap.Logger, error) {
			return logging.NewStderr(loggingConfig.Level)
		}
	}
	logger, err := loggerFactory(loadedConfig.Logging)
	if err != nil {
		bootstrapLogger.Info("startup logger construction failed", logging.Component("startup"), zap.Error(err))
		return fmt.Errorf("construct startup logger: %w", err)
	}

	logger.Debug(
		"loaded startup config",
		logging.Component("startup"),
		zap.String("logging_level", loadedConfig.Logging.Level),
		zap.String("credential_source", string(loadedConfig.NSX.Auth.Source)),
		zap.Bool("nsx_writes_enabled", loadedConfig.NSX.WritesEnabled),
		zap.Duration("operator_tick_interval", loadedConfig.Operator.TickInterval),
		zap.String("operator_metrics_bind_address", loadedConfig.Operator.MetricsBindAddress),
		zap.Int("http_max_requests_in_flight_per_host", loadedConfig.HTTPRateLimiter.MaxRequestsInFlightPerHost),
		zap.Int("http_max_requests_per_second_per_host", loadedConfig.HTTPRateLimiter.MaxRequestsPerSecondPerHost),
		zap.Int("kubeapi_num_parallel_workers", loadedConfig.KubeAPI.NumParallelWorkers),
		zap.Int("kubeapi_max_requests_per_second", loadedConfig.KubeAPI.MaxRequestsPerSecond),
		zap.Int("kubeapi_max_requests_in_flight", loadedConfig.KubeAPI.MaxRequestsInFlight),
	)

	if options.Constructors.Kubernetes != nil {
		logger.Info("constructing kubernetes clients", logging.Component("startup"))
		constructErr := options.Constructors.Kubernetes(*loadedConfig)
		if constructErr != nil {
			return fmt.Errorf("construct kubernetes clients: %w", constructErr)
		}
		logger.Debug("constructed kubernetes clients", logging.Component("startup"))
	}
	if options.Constructors.NSX != nil {
		logger.Info("constructing nsx clients", logging.Component("startup"))
		constructErr := options.Constructors.NSX(*loadedConfig)
		if constructErr != nil {
			return fmt.Errorf("construct nsx clients: %w", constructErr)
		}
		logger.Debug("constructed nsx clients", logging.Component("startup"))
	}
	if options.ManagerFactory != nil {
		runContext := options.Context
		if runContext == nil {
			runContext = context.Background()
		}
		logger.Info("constructing controller runtime manager", logging.Component("startup"))
		runtimeManager, managerErr := options.ManagerFactory(ManagerOptions{
			Config: *loadedConfig,
			Logger: logger,
		})
		if managerErr != nil {
			return fmt.Errorf("construct controller runtime manager: %w", managerErr)
		}
		logger.Info("starting controller runtime manager", logging.Component("startup"))
		startErr := runtimeManager.Start(runContext)
		if startErr != nil {
			return fmt.Errorf("start controller runtime manager: %w", startErr)
		}
		logger.Info("controller runtime manager stopped", logging.Component("startup"))
	}

	logger.Info("startup completed", logging.Component("startup"))
	return nil
}

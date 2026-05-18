package startup

import (
	"fmt"

	"github.com/djosh34/nsx-operator/internal/config"
	"github.com/djosh34/nsx-operator/internal/logging"
	"go.uber.org/zap"
)

type LoggerFactory func(config.LoggingConfig) (*zap.Logger, error)

type RuntimeConstructors struct {
	Kubernetes func(config.Config) error
	NSX        func(config.Config) error
}

type Options struct {
	Config          config.Options
	Constructors    RuntimeConstructors
	BootstrapLogger *zap.Logger
	LoggerFactory   LoggerFactory
}

func Run(options Options) error {
	bootstrapLogger := options.BootstrapLogger
	if bootstrapLogger == nil {
		bootstrapLogger = zap.NewNop()
	}

	bootstrapLogger.Info("loading startup config", logging.Component("startup"), zap.String("config_path", options.Config.Path))
	loadedConfig, err := config.Load(options.Config)
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
		zap.Duration("operator_tick_interval", loadedConfig.Operator.TickInterval),
		zap.Int("http_max_requests_in_flight_per_host", loadedConfig.HTTPRateLimiter.MaxRequestsInFlightPerHost),
		zap.Int("http_max_requests_per_second_per_host", loadedConfig.HTTPRateLimiter.MaxRequestsPerSecondPerHost),
	)

	if options.Constructors.Kubernetes != nil {
		logger.Info("constructing kubernetes clients", logging.Component("startup"))
		if err := options.Constructors.Kubernetes(loadedConfig); err != nil {
			return fmt.Errorf("construct kubernetes clients: %w", err)
		}
		logger.Debug("constructed kubernetes clients", logging.Component("startup"))
	}
	if options.Constructors.NSX != nil {
		logger.Info("constructing nsx clients", logging.Component("startup"))
		if err := options.Constructors.NSX(loadedConfig); err != nil {
			return fmt.Errorf("construct nsx clients: %w", err)
		}
		logger.Debug("constructed nsx clients", logging.Component("startup"))
	}

	logger.Info("startup completed", logging.Component("startup"))
	return nil
}

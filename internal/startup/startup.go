package startup

import (
	"fmt"

	"github.com/djosh34/nsx-operator/internal/config"
	"go.uber.org/zap"
)

type RuntimeConstructors struct {
	Kubernetes func(config.Config) error
	NSX        func(config.Config) error
}

type Options struct {
	Config       config.Options
	Constructors RuntimeConstructors
	Logger       *zap.Logger
}

func Run(options Options) error {
	logger := options.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	logger.Info("loading startup config", zap.String("config_path", options.Config.Path))
	loadedConfig, err := config.Load(options.Config)
	if err != nil {
		logger.Info("startup config validation failed", zap.Error(err))
		return fmt.Errorf("load startup config: %w", err)
	}

	if options.Constructors.Kubernetes != nil {
		logger.Info("constructing kubernetes clients")
		if err := options.Constructors.Kubernetes(loadedConfig); err != nil {
			return fmt.Errorf("construct kubernetes clients: %w", err)
		}
	}
	if options.Constructors.NSX != nil {
		logger.Info("constructing nsx clients")
		if err := options.Constructors.NSX(loadedConfig); err != nil {
			return fmt.Errorf("construct nsx clients: %w", err)
		}
	}

	logger.Info("startup completed")
	return nil
}

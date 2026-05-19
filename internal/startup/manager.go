package startup

import (
	"fmt"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/config"
	"github.com/djosh34/nsx-operator/internal/logging"
	"github.com/djosh34/nsx-operator/internal/stateoperator"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
	runtimeManager, err := ctrl.NewManager(restConfig, manager.Options{
		Scheme: runtimeScheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		Controller: controllerconfig.Controller{
			SkipNameValidation: &skipNameValidation,
		},
	})
	if err != nil {
		logger.Info("controller runtime manager construction failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("construct controller runtime manager: %w", err)
	}

	operator, err := stateoperator.New(stateoperator.Options{
		Client:       runtimeManager.GetClient(),
		TickInterval: options.Config.Operator.TickInterval,
		Logger:       logger,
		SweepCloud:   options.SweepCloud,
		Clock:        options.Clock,
		IDGenerator:  options.IDGenerator,
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
		Complete(operator); err != nil {
		logger.Info("nsx network cloud controller registration failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("register nsx network cloud controller: %w", err)
	}
	if err := builder.ControllerManagedBy(runtimeManager).
		Named("nsxgroup").
		For(&nsxv1alpha.NSXGroup{}).
		Complete(operator); err != nil {
		logger.Info("nsx group controller registration failed", logging.Component("startup"), zap.Error(err))
		return nil, fmt.Errorf("register nsx group controller: %w", err)
	}

	logger.Info("constructed controller runtime manager", logging.Component("startup"))
	return runtimeManager, nil
}

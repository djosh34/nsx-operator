package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/djosh34/nsx-operator/internal/config"
	"github.com/djosh34/nsx-operator/internal/logging"
	"github.com/djosh34/nsx-operator/internal/startup"
	"go.uber.org/zap"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

var (
	newStderrLogger   = logging.NewStderr
	newRuntimeManager = func(options startup.ManagerOptions) (startup.RunnableManager, error) {
		return startup.NewManager(options)
	}
)

func run(args []string) int {
	bootstrapLogger, err := newStderrLogger("info")
	if err != nil {
		fmt.Fprintf(os.Stderr, "construct bootstrap logger: %v\n", err)
		return 1
	}

	flagSet := flag.NewFlagSet("nsx-operator", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	configPath := flagSet.String("config", "", "path to operator config YAML")
	if err := flagSet.Parse(args); err != nil {
		bootstrapLogger.Info("startup failed", logging.Component("cmd"), zap.Error(fmt.Errorf("parse flags: %w", err)))
		if syncErr := bootstrapLogger.Sync(); syncErr != nil {
			fmt.Fprintf(os.Stderr, "sync logger: %v\n", syncErr)
		}
		return 2
	}

	if *configPath == "" {
		bootstrapLogger.Info("startup failed", logging.Component("cmd"), zap.Error(fmt.Errorf("config path is required")))
		if err := bootstrapLogger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "sync logger: %v\n", err)
		}
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtimeLogger := bootstrapLogger
	err = startup.Run(startup.Options{
		Context: ctx,
		Config: config.Options{
			Path:    *configPath,
			Environ: environMap(os.Environ()),
		},
		BootstrapLogger: bootstrapLogger,
		LoggerFactory: func(loggingConfig config.LoggingConfig) (*zap.Logger, error) {
			logger, err := newStderrLogger(loggingConfig.Level)
			if err != nil {
				return nil, err
			}
			runtimeLogger = logger
			return logger, nil
		},
		ManagerFactory: newRuntimeManager,
	})
	if err != nil {
		bootstrapLogger.Info("startup failed", logging.Component("cmd"), zap.Error(err))
		if runtimeLogger != bootstrapLogger {
			if syncErr := runtimeLogger.Sync(); syncErr != nil {
				fmt.Fprintf(os.Stderr, "sync logger: %v\n", syncErr)
			}
		}
		if syncErr := bootstrapLogger.Sync(); syncErr != nil {
			fmt.Fprintf(os.Stderr, "sync logger: %v\n", syncErr)
		}
		return 1
	}

	runtimeLogger.Info("operator process exiting", logging.Component("cmd"))
	if err := runtimeLogger.Sync(); err != nil {
		fmt.Fprintf(os.Stderr, "sync logger: %v\n", err)
		return 1
	}
	if runtimeLogger != bootstrapLogger {
		if err := bootstrapLogger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "sync logger: %v\n", err)
			return 1
		}
	}
	return 0
}

func environMap(values []string) map[string]string {
	environ := make(map[string]string, len(values))
	for _, value := range values {
		name, environmentValue, found := strings.Cut(value, "=")
		if !found {
			continue
		}
		environ[name] = environmentValue
	}
	return environ
}

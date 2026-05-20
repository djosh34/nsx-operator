// Package main wires the NSX operator process entry point.
package main

import (
	"context"
	"flag"
	"fmt"
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
		writeErr := writeStderrf("construct bootstrap logger: %v\n", err)
		if writeErr != nil {
			return 1
		}
		return 1
	}

	flagSet := flag.NewFlagSet("nsx-operator", flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)
	configPath := flagSet.String("config", "", "path to operator config YAML")
	envScriptPath := flagSet.String("env-script", "", "path to executable script that prints NSX credentials")
	err = flagSet.Parse(args)
	if err != nil {
		return failUsage(bootstrapLogger, flagSet, fmt.Errorf("parse flags: %w", err), false)
	}

	if *configPath == "" {
		return failUsage(bootstrapLogger, flagSet, fmt.Errorf("config path is required"), true)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtimeLogger := bootstrapLogger
	err = startup.Run(startup.Options{
		Context: ctx,
		Config: config.Options{
			Path:          *configPath,
			EnvScriptPath: *envScriptPath,
			Environ:       environMap(os.Environ()),
		},
		BootstrapLogger: bootstrapLogger,
		LoggerFactory: func(loggingConfig config.LoggingConfig) (*zap.Logger, error) {
			logger, loggerErr := newStderrLogger(loggingConfig.Level)
			if loggerErr != nil {
				return nil, loggerErr
			}
			runtimeLogger = logger
			return logger, nil
		},
		ManagerFactory: newRuntimeManager,
	})
	if err != nil {
		bootstrapLogger.Info("startup failed", logging.Component("cmd"), zap.Error(err))
		if runtimeLogger != bootstrapLogger {
			syncErr := runtimeLogger.Sync()
			if syncErr != nil {
				writeSyncError(runtimeLogger, syncErr)
			}
		}
		syncErr := bootstrapLogger.Sync()
		if syncErr != nil {
			writeSyncError(bootstrapLogger, syncErr)
		}
		return 1
	}

	runtimeLogger.Info("operator process exiting", logging.Component("cmd"))
	err = runtimeLogger.Sync()
	if err != nil {
		writeSyncError(runtimeLogger, err)
		return 1
	}
	if runtimeLogger != bootstrapLogger {
		err = bootstrapLogger.Sync()
		if err != nil {
			writeSyncError(bootstrapLogger, err)
			return 1
		}
	}
	return 0
}

func failUsage(bootstrapLogger *zap.Logger, flagSet *flag.FlagSet, err error, printUsage bool) int {
	if printUsage {
		_, writeErr := fmt.Fprintf(flagSet.Output(), "%v\n", err)
		if writeErr != nil {
			bootstrapLogger.Info("usage error write failed", logging.Component("cmd"), zap.Error(writeErr))
		}
		flagSet.Usage()
	}
	bootstrapLogger.Info("startup failed", logging.Component("cmd"), zap.Error(err))
	syncErr := bootstrapLogger.Sync()
	if syncErr != nil {
		writeSyncError(bootstrapLogger, syncErr)
	}
	return 2
}

func writeStderrf(format string, args ...any) error {
	_, err := fmt.Fprintf(os.Stderr, format, args...)
	if err != nil {
		return fmt.Errorf("write stderr: %w", err)
	}
	return nil
}

func writeSyncError(logger *zap.Logger, err error) {
	writeErr := writeStderrf("sync logger: %v\n", err)
	if writeErr != nil {
		logger.Info("sync error write failed", logging.Component("cmd"), zap.Error(writeErr))
	}
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

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/djosh34/nsx-operator/internal/config"
	"github.com/djosh34/nsx-operator/internal/startup"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flagSet := flag.NewFlagSet("nsx-operator", flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)
	configPath := flagSet.String("config", "", "path to operator config YAML")
	if err := flagSet.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		return 2
	}

	logger := newStderrJSONLogger()
	if *configPath == "" {
		logger.Info("startup failed", zap.Error(fmt.Errorf("config path is required")))
		if err := logger.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "sync logger: %v\n", err)
		}
		return 2
	}

	err := startup.Run(startup.Options{
		Config: config.Options{
			Path:    *configPath,
			Environ: environMap(os.Environ()),
		},
		Logger: logger,
	})
	if err != nil {
		logger.Info("startup failed", zap.Error(err))
		if syncErr := logger.Sync(); syncErr != nil {
			fmt.Fprintf(os.Stderr, "sync logger: %v\n", syncErr)
		}
		return 1
	}

	logger.Info("operator process exiting")
	if err := logger.Sync(); err != nil {
		fmt.Fprintf(os.Stderr, "sync logger: %v\n", err)
		return 1
	}
	return 0
}

func newStderrJSONLogger() *zap.Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		stderrWriteSyncer{},
		zap.DebugLevel,
	)
	return zap.New(core)
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

type stderrWriteSyncer struct{}

func (stderrWriteSyncer) Write(p []byte) (int, error) {
	written, err := os.Stderr.Write(p)
	if err != nil {
		return written, fmt.Errorf("write stderr log: %w", err)
	}
	return written, nil
}

func (stderrWriteSyncer) Sync() error {
	return nil
}

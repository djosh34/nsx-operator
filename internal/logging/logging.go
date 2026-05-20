// Package logging builds structured zap loggers and fields.
package logging

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Options configures logger construction.
type Options struct {
	Level string
	Sink  zapcore.WriteSyncer
}

// New constructs a JSON zap logger.
func New(options Options) (*zap.Logger, error) {
	level, err := parseLevel(options.Level)
	if err != nil {
		return nil, err
	}

	sink := options.Sink
	if sink == nil {
		sink = stderrWriteSyncer{}
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		sink,
		level,
	)
	return zap.New(core), nil
}

// NewStderr constructs a JSON zap logger writing to stderr.
func NewStderr(level string) (*zap.Logger, error) {
	return New(Options{
		Level: level,
		Sink:  stderrWriteSyncer{},
	})
}

// Component returns a structured component field.
func Component(value string) zap.Field {
	return zap.String("component", value)
}

// NetworkCloudFQDN returns a structured network cloud FQDN field.
func NetworkCloudFQDN(value string) zap.Field {
	return zap.String("networkCloudFQDN", value)
}

// GroupID returns a structured NSX group ID field.
func GroupID(value string) zap.Field {
	return zap.String("groupID", value)
}

// SweepID returns a structured sweep ID field.
func SweepID(value string) zap.Field {
	return zap.String("sweepID", value)
}

// ReconcileKey returns a structured reconcile key field.
func ReconcileKey(value string) zap.Field {
	return zap.String("reconcileKey", value)
}

func parseLevel(level string) (zapcore.Level, error) {
	switch level {
	case "debug":
		return zap.DebugLevel, nil
	case "info":
		return zap.InfoLevel, nil
	case "warn":
		return zap.WarnLevel, nil
	case "error":
		return zap.ErrorLevel, nil
	default:
		return zap.InfoLevel, fmt.Errorf("unsupported logging level %q", level)
	}
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

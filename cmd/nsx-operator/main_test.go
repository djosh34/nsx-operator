package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djosh34/nsx-operator/internal/startup"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestRunReturnsUsageErrorWhenConfigFlagIsMissing(t *testing.T) {
	exitCode := run([]string{})
	if exitCode != 2 {
		t.Fatalf("run() exit code = %d, want 2", exitCode)
	}
}

func TestRunReturnsUsageErrorForInvalidFlag(t *testing.T) {
	exitCode := run([]string{"--not-a-real-flag"})
	if exitCode != 2 {
		t.Fatalf("run() exit code = %d, want 2", exitCode)
	}
}

func TestRunWritesValidJSONLToStderrForInvalidFlag(t *testing.T) {
	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{"--not-a-real-flag"})
	})
	if exitCode != 2 {
		t.Fatalf("run() exit code = %d, want 2; stderr: %q", exitCode, stderr)
	}

	entries := parseCommandLogs(t, stderr)
	if !commandLogContains(entries, "info", "startup failed") {
		t.Fatalf("stderr did not include startup failed log: %q", stderr)
	}
}

func TestRunReturnsStartupErrorForInvalidConfig(t *testing.T) {
	configPath := writeCommandConfig(t, `
operator:
  tickInterval: 0s
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: 20
nsx:
  auth:
    username: config-user
    password: config-pass
logging:
  level: info
`)

	exitCode := run([]string{"--config", configPath})
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
}

func TestRunReturnsSuccessForValidConfig(t *testing.T) {
	replaceNewRuntimeManager(t, successfulRuntimeManager)
	configPath := writeCommandConfig(t, `
operator:
  tickInterval: 30s
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: 20
nsx:
  auth:
    username: config-user
    password: config-pass
logging:
  level: info
`)

	exitCode := run([]string{"--config", configPath})
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
}

func TestRunWritesValidJSONLToStderrForValidConfig(t *testing.T) {
	replaceNewRuntimeManager(t, successfulRuntimeManager)
	configPath := writeCommandConfig(t, `
operator:
  tickInterval: 30s
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: 20
nsx:
  auth:
    username: config-user
    password: config-pass
logging:
  level: info
`)

	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{"--config", configPath})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %q", exitCode, stderr)
	}

	entries := parseCommandLogs(t, stderr)
	if len(entries) == 0 {
		t.Fatalf("stderr had no JSONL logs")
	}
	if !commandLogContains(entries, "info", "startup completed") {
		t.Fatalf("stderr did not include startup completion: %q", stderr)
	}
	if !commandLogContains(entries, "info", "operator process exiting") {
		t.Fatalf("stderr did not include process exit log: %q", stderr)
	}
}

func TestRunDoesNotWriteCredentialsToStderr(t *testing.T) {
	replaceNewRuntimeManager(t, successfulRuntimeManager)
	configPath := writeCommandConfig(t, `
operator:
  tickInterval: 30s
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: 20
nsx:
  auth:
    username: command-sentinel-user
    password: command-sentinel-password
logging:
  level: debug
`)

	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{"--config", configPath})
	})
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0; stderr: %q", exitCode, stderr)
	}

	parseCommandLogs(t, stderr)
	if strings.Contains(stderr, "command-sentinel-user") || strings.Contains(stderr, "command-sentinel-password") {
		t.Fatalf("stderr leaked credentials: %q", stderr)
	}
}

func TestRunReturnsErrorWhenBootstrapLoggerConstructionFails(t *testing.T) {
	replaceNewStderrLogger(t, func(string) (*zap.Logger, error) {
		return nil, errors.New("construct boom")
	})

	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{})
	})
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "construct bootstrap logger: construct boom") {
		t.Fatalf("stderr = %q, want bootstrap construction error", stderr)
	}
}

func TestRunReturnsStartupErrorWhenRuntimeLoggerConstructionFails(t *testing.T) {
	configPath := writeCommandConfig(t, `
operator:
  tickInterval: 30s
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: 20
nsx:
  auth:
    username: config-user
    password: config-pass
logging:
  level: info
`)

	callCount := 0
	replaceNewStderrLogger(t, func(string) (*zap.Logger, error) {
		callCount++
		if callCount == 1 {
			return zap.NewNop(), nil
		}
		return nil, errors.New("runtime construct boom")
	})

	exitCode := run([]string{"--config", configPath})
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
}

func TestRunReportsLoggerSyncError(t *testing.T) {
	replaceNewStderrLogger(t, func(string) (*zap.Logger, error) {
		return newSyncErrorLogger(), nil
	})

	var exitCode int
	stderr := captureStderr(t, func() {
		exitCode = run([]string{})
	})
	if exitCode != 2 {
		t.Fatalf("run() exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr, "sync logger: sync boom") {
		t.Fatalf("stderr = %q, want sync error", stderr)
	}
}

func TestEnvironMapUsesLastValueForDuplicateNames(t *testing.T) {
	environ := environMap([]string{
		"NSX_USERNAME=first",
		"MALFORMED",
		"NSX_USERNAME=second",
		"NSX_PASSWORD=password",
	})

	if environ["NSX_USERNAME"] != "second" {
		t.Fatalf("NSX_USERNAME = %q, want second", environ["NSX_USERNAME"])
	}
	if environ["NSX_PASSWORD"] != "password" {
		t.Fatalf("NSX_PASSWORD = %q, want password", environ["NSX_PASSWORD"])
	}
	if _, ok := environ["MALFORMED"]; ok {
		t.Fatal("MALFORMED env entry was included")
	}
}

func replaceNewStderrLogger(t *testing.T, replacement func(string) (*zap.Logger, error)) {
	t.Helper()

	original := newStderrLogger
	newStderrLogger = replacement
	t.Cleanup(func() {
		newStderrLogger = original
	})
}

func replaceNewRuntimeManager(t *testing.T, replacement func(startup.ManagerOptions) (startup.RunnableManager, error)) {
	t.Helper()

	original := newRuntimeManager
	newRuntimeManager = replacement
	t.Cleanup(func() {
		newRuntimeManager = original
	})
}

func successfulRuntimeManager(startup.ManagerOptions) (startup.RunnableManager, error) {
	return commandFakeManager{}, nil
}

type commandFakeManager struct{}

func (commandFakeManager) Start(context.Context) error {
	return nil
}

func newSyncErrorLogger() *zap.Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		syncErrorWriteSyncer{},
		zap.DebugLevel,
	)
	return zap.New(core)
}

type syncErrorWriteSyncer struct{}

func (syncErrorWriteSyncer) Write(p []byte) (int, error) {
	return len(p), nil
}

func (syncErrorWriteSyncer) Sync() error {
	return errors.New("sync boom")
}

func captureStderr(t *testing.T, run func()) string {
	t.Helper()

	originalStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = writer
	defer func() {
		os.Stderr = originalStderr
	}()

	run()

	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return string(output)
}

func parseCommandLogs(t *testing.T, output string) []map[string]any {
	t.Helper()

	var entries []map[string]any
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("stderr line is not valid JSON: %v; line: %q; output: %q", err, line, output)
		}
		entries = append(entries, entry)
	}
	return entries
}

func commandLogContains(entries []map[string]any, level string, message string) bool {
	for _, entry := range entries {
		if entry["level"] == level && entry["msg"] == message {
			return true
		}
	}
	return false
}

func writeCommandConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write command config: %v", err)
	}
	return path
}

package startup_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/djosh34/nsx-operator/internal/config"
	"github.com/djosh34/nsx-operator/internal/logging"
	"github.com/djosh34/nsx-operator/internal/startup"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestRunInvalidConfigReturnsBeforeClientConstruction(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configYAML := []byte(`
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
	if err := os.WriteFile(configPath, configYAML, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	kubernetesCalled := false
	nsxCalled := false
	err := startup.Run(startup.Options{
		Config: config.Options{
			Path:    configPath,
			Environ: map[string]string{},
		},
		Constructors: startup.RuntimeConstructors{
			Kubernetes: func(config.Config) error {
				kubernetesCalled = true
				return nil
			},
			NSX: func(config.Config) error {
				nsxCalled = true
				return nil
			},
		},
		BootstrapLogger: zap.NewNop(),
	})
	if err == nil {
		t.Fatal("Run() error = nil, want invalid config error")
	}
	if kubernetesCalled {
		t.Fatal("Kubernetes constructor was called for invalid config")
	}
	if nsxCalled {
		t.Fatal("NSX constructor was called for invalid config")
	}
}

func TestRunInvalidConfigLogsStructuredValidationFailureThroughBootstrapLogger(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configYAML := []byte(`
operator:
  tickInterval: 0s
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: 20
nsx:
  auth:
    username: sentinel-user
    password: sentinel-password
logging:
  level: info
`)
	if err := os.WriteFile(configPath, configYAML, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var bootstrapLogs bytes.Buffer
	bootstrapLogger, err := logging.New(logging.Options{
		Level: "debug",
		Sink:  zapcore.AddSync(&bootstrapLogs),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	kubernetesCalled := false
	nsxCalled := false
	err = startup.Run(startup.Options{
		Config: config.Options{
			Path:    configPath,
			Environ: map[string]string{},
		},
		Constructors: startup.RuntimeConstructors{
			Kubernetes: func(config.Config) error {
				kubernetesCalled = true
				return nil
			},
			NSX: func(config.Config) error {
				nsxCalled = true
				return nil
			},
		},
		BootstrapLogger: bootstrapLogger,
	})
	if err == nil {
		t.Fatal("Run() error = nil, want validation error")
	}
	if kubernetesCalled {
		t.Fatal("Kubernetes constructor was called for invalid config")
	}
	if nsxCalled {
		t.Fatal("NSX constructor was called for invalid config")
	}

	entries := parseStartupLogs(t, bootstrapLogs.String())
	if !hasLogEntry(entries, "info", "startup config validation failed") {
		t.Fatalf("bootstrap logs did not include validation failure: %q", bootstrapLogs.String())
	}
	if strings.Contains(bootstrapLogs.String(), "sentinel-user") || strings.Contains(bootstrapLogs.String(), "sentinel-password") {
		t.Fatalf("bootstrap logs leaked credentials: %q", bootstrapLogs.String())
	}
}

func TestRunValidConfigConstructsClientsInOrderWithLoadedConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configYAML := []byte(`
operator:
  tickInterval: 45s
httpRateLimiter:
  maxRequestsInFlightPerHost: 9
  maxRequestsPerSecondPerHost: 21
nsx:
  auth:
    username: config-user
    password: config-pass
logging:
  level: debug
`)
	if err := os.WriteFile(configPath, configYAML, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var calls []string
	err := startup.Run(startup.Options{
		Config: config.Options{
			Path:    configPath,
			Environ: map[string]string{},
		},
		Constructors: startup.RuntimeConstructors{
			Kubernetes: func(loaded config.Config) error {
				calls = append(calls, "kubernetes")
				if loaded.Logging.Level != "debug" {
					t.Fatalf("Kubernetes constructor got logging level %q, want debug", loaded.Logging.Level)
				}
				return nil
			},
			NSX: func(loaded config.Config) error {
				calls = append(calls, "nsx")
				if loaded.NSX.Auth.Source != config.CredentialSourceConfigValues {
					t.Fatalf("NSX constructor got credential source %q, want %q", loaded.NSX.Auth.Source, config.CredentialSourceConfigValues)
				}
				return nil
			},
		},
		BootstrapLogger: zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantCalls := []string{"kubernetes", "nsx"}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("constructor calls = %v, want %v", calls, wantCalls)
	}
}

func TestRunUsesConfiguredRuntimeLoggerForDebugStartupDetails(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configYAML := []byte(`
operator:
  tickInterval: 45s
httpRateLimiter:
  maxRequestsInFlightPerHost: 9
  maxRequestsPerSecondPerHost: 21
nsx:
  auth:
    username: config-user
    password: config-pass
logging:
  level: debug
`)
	if err := os.WriteFile(configPath, configYAML, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var runtimeLogs bytes.Buffer
	err := startup.Run(startup.Options{
		Config: config.Options{
			Path:    configPath,
			Environ: map[string]string{},
		},
		LoggerFactory: func(loggingConfig config.LoggingConfig) (*zap.Logger, error) {
			return logging.New(logging.Options{
				Level: loggingConfig.Level,
				Sink:  zapcore.AddSync(&runtimeLogs),
			})
		},
		BootstrapLogger: zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	entries := parseStartupLogs(t, runtimeLogs.String())
	if !hasLogEntry(entries, "debug", "loaded startup config") {
		t.Fatalf("runtime logs did not include debug startup details: %q", runtimeLogs.String())
	}
	if !hasLogEntry(entries, "info", "startup completed") {
		t.Fatalf("runtime logs did not include startup completion: %q", runtimeLogs.String())
	}
}

func TestRunLogsCredentialSourceWithoutCredentialMaterial(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configYAML := []byte(`
operator:
  tickInterval: 45s
httpRateLimiter:
  maxRequestsInFlightPerHost: 9
  maxRequestsPerSecondPerHost: 21
nsx:
  auth:
    username: runtime-sentinel-user
    password: runtime-sentinel-password
logging:
  level: debug
`)
	if err := os.WriteFile(configPath, configYAML, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var runtimeLogs bytes.Buffer
	err := startup.Run(startup.Options{
		Config: config.Options{
			Path:    configPath,
			Environ: map[string]string{},
		},
		LoggerFactory: func(loggingConfig config.LoggingConfig) (*zap.Logger, error) {
			return logging.New(logging.Options{
				Level: loggingConfig.Level,
				Sink:  zapcore.AddSync(&runtimeLogs),
			})
		},
		BootstrapLogger: zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	entries := parseStartupLogs(t, runtimeLogs.String())
	var foundCredentialSource bool
	for _, entry := range entries {
		if entry["credential_source"] == string(config.CredentialSourceConfigValues) {
			foundCredentialSource = true
		}
	}
	if !foundCredentialSource {
		t.Fatalf("runtime logs did not include credential source: %q", runtimeLogs.String())
	}
	if strings.Contains(runtimeLogs.String(), "runtime-sentinel-user") || strings.Contains(runtimeLogs.String(), "runtime-sentinel-password") {
		t.Fatalf("runtime logs leaked credentials: %q", runtimeLogs.String())
	}
}

func parseStartupLogs(t *testing.T, output string) []map[string]any {
	t.Helper()

	var entries []map[string]any
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not valid JSON: %v; line: %q", err, line)
		}
		entries = append(entries, entry)
	}
	return entries
}

func hasLogEntry(entries []map[string]any, level string, message string) bool {
	for _, entry := range entries {
		if entry["level"] == level && entry["msg"] == message {
			return true
		}
	}
	return false
}

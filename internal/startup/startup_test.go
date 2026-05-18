package startup_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/djosh34/nsx-operator/internal/config"
	"github.com/djosh34/nsx-operator/internal/startup"
	"go.uber.org/zap"
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
		Logger: zap.NewNop(),
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
		Logger: zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantCalls := []string{"kubernetes", "nsx"}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("constructor calls = %v, want %v", calls, wantCalls)
	}
}

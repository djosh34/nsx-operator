package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/djosh34/nsx-operator/internal/config"
)

func TestLoadValidConfigCredentials(t *testing.T) {
	dir := t.TempDir()
	caBundlePath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caBundlePath, []byte("certificate"), 0o600); err != nil {
		t.Fatalf("write CA bundle: %v", err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	configYAML := []byte(`
operator:
  tickInterval: 30s
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: 20
nsx:
  auth:
    username: config-user
    password: config-pass
  tls:
    caBundleFile: ` + caBundlePath + `
logging:
  level: info
`)
	if err := os.WriteFile(configPath, configYAML, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loaded, err := config.Load(config.Options{
		Path:    configPath,
		Environ: map[string]string{},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Operator.TickInterval != 30*time.Second {
		t.Fatalf("TickInterval = %s, want 30s", loaded.Operator.TickInterval)
	}
	if loaded.HTTPRateLimiter.MaxRequestsInFlightPerHost != 8 {
		t.Fatalf("MaxRequestsInFlightPerHost = %d, want 8", loaded.HTTPRateLimiter.MaxRequestsInFlightPerHost)
	}
	if loaded.HTTPRateLimiter.MaxRequestsPerSecondPerHost != 20 {
		t.Fatalf("MaxRequestsPerSecondPerHost = %d, want 20", loaded.HTTPRateLimiter.MaxRequestsPerSecondPerHost)
	}
	if loaded.NSX.TLS.CABundleFile != caBundlePath {
		t.Fatalf("CABundleFile = %q, want %q", loaded.NSX.TLS.CABundleFile, caBundlePath)
	}
	if loaded.Logging.Level != "info" {
		t.Fatalf("Level = %q, want info", loaded.Logging.Level)
	}
	if loaded.NSX.Auth.Username != "config-user" {
		t.Fatalf("Username = %q, want config-user", loaded.NSX.Auth.Username)
	}
	if loaded.NSX.Auth.Password != "config-pass" {
		t.Fatalf("Password = %q, want config-pass", loaded.NSX.Auth.Password)
	}
	if loaded.NSX.Auth.Source != config.CredentialSourceConfigValues {
		t.Fatalf("Source = %q, want %q", loaded.NSX.Auth.Source, config.CredentialSourceConfigValues)
	}
}

func TestLoadNSXURLSchemeDefaultsToHTTPSAndAllowsHTTP(t *testing.T) {
	t.Run("default https", func(t *testing.T) {
		configPath := writeValidConfig(t, t.TempDir(), `
nsx:
  auth:
    username: config-user
    password: config-pass
`)

		loaded, err := config.Load(config.Options{
			Path:    configPath,
			Environ: map[string]string{},
		})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if loaded.NSX.URLScheme != "https" {
			t.Fatalf("URLScheme = %q, want https", loaded.NSX.URLScheme)
		}
	})

	t.Run("explicit http", func(t *testing.T) {
		configPath := writeValidConfig(t, t.TempDir(), `
nsx:
  urlScheme: http
  auth:
    username: config-user
    password: config-pass
`)

		loaded, err := config.Load(config.Options{
			Path:    configPath,
			Environ: map[string]string{},
		})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if loaded.NSX.URLScheme != "http" {
			t.Fatalf("URLScheme = %q, want http", loaded.NSX.URLScheme)
		}
	})
}

func TestLoadNSXWritesEnabledDefaultsTrueAndAllowsFalse(t *testing.T) {
	t.Run("default true", func(t *testing.T) {
		configPath := writeValidConfig(t, t.TempDir(), `
nsx:
  auth:
    username: config-user
    password: config-pass
`)

		loaded, err := config.Load(config.Options{
			Path:    configPath,
			Environ: map[string]string{},
		})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if !loaded.NSX.WritesEnabled {
			t.Fatal("WritesEnabled = false, want default true")
		}
	})

	t.Run("explicit false", func(t *testing.T) {
		configPath := writeValidConfig(t, t.TempDir(), `
nsx:
  writesEnabled: false
  auth:
    username: config-user
    password: config-pass
`)

		loaded, err := config.Load(config.Options{
			Path:    configPath,
			Environ: map[string]string{},
		})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if loaded.NSX.WritesEnabled {
			t.Fatal("WritesEnabled = true, want false")
		}
	})
}

func TestLoadEnvCredentialsOverrideConfigCredentials(t *testing.T) {
	configPath := writeValidConfig(t, t.TempDir(), `
nsx:
  auth:
    username: config-user
    password: config-pass
`)

	loaded, err := config.Load(config.Options{
		Path: configPath,
		Environ: map[string]string{
			"NSX_USERNAME": "env-user",
			"NSX_PASSWORD": "env-pass",
		},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.NSX.Auth.Username != "env-user" {
		t.Fatalf("Username = %q, want env-user", loaded.NSX.Auth.Username)
	}
	if loaded.NSX.Auth.Password != "env-pass" {
		t.Fatalf("Password = %q, want env-pass", loaded.NSX.Auth.Password)
	}
	if loaded.NSX.Auth.Source != config.CredentialSourceEnv {
		t.Fatalf("Source = %q, want %q", loaded.NSX.Auth.Source, config.CredentialSourceEnv)
	}
}

func TestLoadEnvCredentialFilesOverrideConfigCredentialsAndTrimOneNewline(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "username.txt"), []byte("env-file-user\n\n"), 0o600); err != nil {
		t.Fatalf("write username file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "password.txt"), []byte(" env-file-pass \r\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    username: config-user
    password: config-pass
`)

	loaded, err := config.Load(config.Options{
		Path: configPath,
		Environ: map[string]string{
			"NSX_USERNAME_FILE": "username.txt",
			"NSX_PASSWORD_FILE": "password.txt",
		},
		FS: os.DirFS(dir),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.NSX.Auth.Username != "env-file-user\n" {
		t.Fatalf("Username = %q, want one newline preserved", loaded.NSX.Auth.Username)
	}
	if loaded.NSX.Auth.Password != " env-file-pass " {
		t.Fatalf("Password = %q, want spaces preserved and CRLF trimmed", loaded.NSX.Auth.Password)
	}
	if loaded.NSX.Auth.Source != config.CredentialSourceEnvFiles {
		t.Fatalf("Source = %q, want %q", loaded.NSX.Auth.Source, config.CredentialSourceEnvFiles)
	}
}

func TestLoadConfigCredentialFilesAsFinalFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config-username.txt"), []byte("config-file-user\n"), 0o600); err != nil {
		t.Fatalf("write username file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config-password.txt"), []byte("config-file-pass\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    usernameFile: config-username.txt
    passwordFile: config-password.txt
`)

	loaded, err := config.Load(config.Options{
		Path:    configPath,
		Environ: map[string]string{},
		FS:      os.DirFS(dir),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.NSX.Auth.Username != "config-file-user" {
		t.Fatalf("Username = %q, want config-file-user", loaded.NSX.Auth.Username)
	}
	if loaded.NSX.Auth.Password != "config-file-pass" {
		t.Fatalf("Password = %q, want config-file-pass", loaded.NSX.Auth.Password)
	}
	if loaded.NSX.Auth.Source != config.CredentialSourceConfigFiles {
		t.Fatalf("Source = %q, want %q", loaded.NSX.Auth.Source, config.CredentialSourceConfigFiles)
	}
}

func TestLoadRejectsPartialHigherPriorityCredentialSource(t *testing.T) {
	configPath := writeValidConfig(t, t.TempDir(), `
nsx:
  auth:
    username: config-user
    password: config-pass
`)

	_, err := config.Load(config.Options{
		Path: configPath,
		Environ: map[string]string{
			"NSX_USERNAME": "env-user",
		},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want partial credential source error")
	}
	if containsAny(err.Error(), "env-user", "config-pass") {
		t.Fatalf("Load() error leaked credential material: %v", err)
	}
}

func TestLoadRejectsMissingAndEmptyCredentialFilesWithoutLeakingSecrets(t *testing.T) {
	t.Run("missing username file stops before fallback without leaking config password", func(t *testing.T) {
		dir := t.TempDir()
		configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    username: config-user
    password: lower-priority-secret
`)

		_, err := config.Load(config.Options{
			Path: configPath,
			Environ: map[string]string{
				"NSX_USERNAME_FILE": "missing-username.txt",
				"NSX_PASSWORD_FILE": "missing-password.txt",
			},
			FS: os.DirFS(dir),
		})
		if err == nil {
			t.Fatal("Load() error = nil, want missing credential file error")
		}
		if containsAny(err.Error(), "lower-priority-secret") {
			t.Fatalf("Load() error leaked credential material: %v", err)
		}
	})

	t.Run("empty selected password file is rejected without leaking username contents", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "username.txt"), []byte("username-secret\n"), 0o600); err != nil {
			t.Fatalf("write username file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "password.txt"), []byte("\r\n"), 0o600); err != nil {
			t.Fatalf("write password file: %v", err)
		}
		configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    usernameFile: username.txt
    passwordFile: password.txt
`)

		_, err := config.Load(config.Options{
			Path:    configPath,
			Environ: map[string]string{},
			FS:      os.DirFS(dir),
		})
		if err == nil {
			t.Fatal("Load() error = nil, want empty credential file error")
		}
		if containsAny(err.Error(), "username-secret") {
			t.Fatalf("Load() error leaked credential material: %v", err)
		}
	})
}

func TestLoadRejectsNoResolvedCredentialSource(t *testing.T) {
	configPath := writeValidConfig(t, t.TempDir(), `
nsx:
  auth: {}
`)

	_, err := config.Load(config.Options{
		Path:    configPath,
		Environ: map[string]string{},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want no credential source error")
	}
}

func TestLoadRejectsInvalidNumericValues(t *testing.T) {
	tests := map[string]string{
		"missing tick interval": `
operator: {}
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: 20
nsx:
  auth:
    username: config-user
    password: config-pass
logging:
  level: info
`,
		"zero tick interval": `
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
`,
		"zero max requests in flight": `
operator:
  tickInterval: 30s
httpRateLimiter:
  maxRequestsInFlightPerHost: 0
  maxRequestsPerSecondPerHost: 20
nsx:
  auth:
    username: config-user
    password: config-pass
logging:
  level: info
`,
		"negative max requests per second": `
operator:
  tickInterval: 30s
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: -1
nsx:
  auth:
    username: config-user
    password: config-pass
logging:
  level: info
`,
	}

	for name, yamlContent := range tests {
		t.Run(name, func(t *testing.T) {
			configPath := writeConfig(t, t.TempDir(), yamlContent)

			_, err := config.Load(config.Options{
				Path:    configPath,
				Environ: map[string]string{},
			})
			if err == nil {
				t.Fatal("Load() error = nil, want validation error")
			}
		})
	}
}

func TestLoadRejectsMissingCABundleFile(t *testing.T) {
	missingCAPath := filepath.Join(t.TempDir(), "missing-ca.pem")
	configPath := writeValidConfig(t, t.TempDir(), `
nsx:
  auth:
    username: config-user
    password: config-pass
  tls:
    caBundleFile: `+missingCAPath+`
`)

	_, err := config.Load(config.Options{
		Path:    configPath,
		Environ: map[string]string{},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want missing CA bundle error")
	}
}

func writeValidConfig(t *testing.T, dir string, overrides string) string {
	t.Helper()

	configYAML := []byte(`
operator:
  tickInterval: 30s
httpRateLimiter:
  maxRequestsInFlightPerHost: 8
  maxRequestsPerSecondPerHost: 20
` + overrides + `
logging:
  level: info
`)
	return writeConfig(t, dir, string(configYAML))
}

func writeConfig(t *testing.T, dir string, content string) string {
	t.Helper()

	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

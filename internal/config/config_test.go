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
	if loaded.Operator.MetricsBindAddress != ":8080" {
		t.Fatalf("MetricsBindAddress = %q, want :8080", loaded.Operator.MetricsBindAddress)
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

func TestLoadOperatorMetricsBindAddressAllowsOverride(t *testing.T) {
	configPath := writeConfig(t, t.TempDir(), `
operator:
  tickInterval: 30s
  metricsBindAddress: "127.0.0.1:9090"
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

	loaded, err := config.Load(config.Options{
		Path:    configPath,
		Environ: map[string]string{},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Operator.MetricsBindAddress != "127.0.0.1:9090" {
		t.Fatalf("MetricsBindAddress = %q, want 127.0.0.1:9090", loaded.Operator.MetricsBindAddress)
	}
}

func TestLoadKubeAPIConfigDefaultsWhenOmitted(t *testing.T) {
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

	if loaded.KubeAPI.NumParallelWorkers != 1 {
		t.Fatalf("KubeAPI.NumParallelWorkers = %d, want 1", loaded.KubeAPI.NumParallelWorkers)
	}
	if loaded.KubeAPI.MaxRequestsPerSecond != 100 {
		t.Fatalf("KubeAPI.MaxRequestsPerSecond = %d, want 100", loaded.KubeAPI.MaxRequestsPerSecond)
	}
	if loaded.KubeAPI.MaxRequestsInFlight != 100 {
		t.Fatalf("KubeAPI.MaxRequestsInFlight = %d, want 100", loaded.KubeAPI.MaxRequestsInFlight)
	}
}

func TestLoadKubeAPIConfigAcceptsExplicitValues(t *testing.T) {
	configPath := writeValidConfig(t, t.TempDir(), `
kubeAPI:
  numParallelWorkers: 20
  maxRequestsPerSecond: 100
  maxRequestsInFlight: 100
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

	if loaded.KubeAPI.NumParallelWorkers != 20 {
		t.Fatalf("KubeAPI.NumParallelWorkers = %d, want 20", loaded.KubeAPI.NumParallelWorkers)
	}
	if loaded.KubeAPI.MaxRequestsPerSecond != 100 {
		t.Fatalf("KubeAPI.MaxRequestsPerSecond = %d, want 100", loaded.KubeAPI.MaxRequestsPerSecond)
	}
	if loaded.KubeAPI.MaxRequestsInFlight != 100 {
		t.Fatalf("KubeAPI.MaxRequestsInFlight = %d, want 100", loaded.KubeAPI.MaxRequestsInFlight)
	}
}

func TestLoadKubeAPIConfigRejectsNegativeValues(t *testing.T) {
	tests := map[string]struct {
		configYAML string
		wantError  string
	}{
		"negative workers": {
			configYAML: `
kubeAPI:
  numParallelWorkers: -1
nsx:
  auth:
    username: config-user
    password: config-pass
`,
			wantError: "kubeAPI.numParallelWorkers",
		},
		"negative requests per second": {
			configYAML: `
kubeAPI:
  maxRequestsPerSecond: -1
nsx:
  auth:
    username: config-user
    password: config-pass
`,
			wantError: "kubeAPI.maxRequestsPerSecond",
		},
		"negative in flight": {
			configYAML: `
kubeAPI:
  maxRequestsInFlight: -1
nsx:
  auth:
    username: config-user
    password: config-pass
`,
			wantError: "kubeAPI.maxRequestsInFlight",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			configPath := writeValidConfig(t, t.TempDir(), test.configYAML)

			_, err := config.Load(config.Options{
				Path:    configPath,
				Environ: map[string]string{},
			})
			if err == nil {
				t.Fatal("Load() error = nil, want kubeAPI validation error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Load() error = %v, want field %q", err, test.wantError)
			}
		})
	}
}

func TestLoadComposeConfigSetsKubeAPIBatchDefaultsForNormalRuntime(t *testing.T) {
	loaded, err := config.Load(config.Options{
		Path:    filepath.Join("..", "..", "config", "compose", "nsx-operator-config.yaml"),
		Environ: map[string]string{},
	})
	if err != nil {
		t.Fatalf("Load() compose config error = %v", err)
	}

	if loaded.KubeAPI.NumParallelWorkers != 20 {
		t.Fatalf("KubeAPI.NumParallelWorkers = %d, want 20", loaded.KubeAPI.NumParallelWorkers)
	}
	if loaded.KubeAPI.MaxRequestsPerSecond != 100 {
		t.Fatalf("KubeAPI.MaxRequestsPerSecond = %d, want 100", loaded.KubeAPI.MaxRequestsPerSecond)
	}
	if loaded.KubeAPI.MaxRequestsInFlight != 100 {
		t.Fatalf("KubeAPI.MaxRequestsInFlight = %d, want 100", loaded.KubeAPI.MaxRequestsInFlight)
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

func TestLoadRejectsEnvScriptMissingUsernameWithoutFallback(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "credentials.sh")
	script := []byte(`#!/bin/sh
printf '%s\n' 'NSX_PASSWORD=script-pass'
`)
	if err := os.WriteFile(scriptPath, script, 0o700); err != nil {
		t.Fatalf("write env script: %v", err)
	}
	configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    username: config-user
    password: config-pass
`)

	_, err := config.Load(config.Options{
		Path:          configPath,
		EnvScriptPath: scriptPath,
		Environ: map[string]string{
			"NSX_USERNAME": "env-user",
			"NSX_PASSWORD": "env-pass",
		},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want missing env script username error")
	}
	if !strings.Contains(err.Error(), "NSX_USERNAME") {
		t.Fatalf("Load() error = %v, want NSX_USERNAME error", err)
	}
	if containsAny(err.Error(), "script-pass", "env-user", "env-pass", "config-user", "config-pass") {
		t.Fatalf("Load() error leaked credential material: %v", err)
	}
}

func TestLoadRejectsEnvScriptMissingPasswordWithoutLeakingUsername(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "credentials.sh")
	script := []byte(`#!/bin/sh
printf '%s\n' 'NSX_USERNAME=script-user'
`)
	if err := os.WriteFile(scriptPath, script, 0o700); err != nil {
		t.Fatalf("write env script: %v", err)
	}
	configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    username: config-user
    password: config-pass
`)

	_, err := config.Load(config.Options{
		Path:          configPath,
		EnvScriptPath: scriptPath,
		Environ:       map[string]string{},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want missing env script password error")
	}
	if !strings.Contains(err.Error(), "NSX_PASSWORD") {
		t.Fatalf("Load() error = %v, want NSX_PASSWORD error", err)
	}
	if containsAny(err.Error(), "script-user", "config-user", "config-pass") {
		t.Fatalf("Load() error leaked credential material: %v", err)
	}
}

func TestLoadRejectsEnvScriptExecutionFailureWithoutLeakingOutput(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "credentials.sh")
	script := []byte(`#!/bin/sh
printf '%s\n' 'NSX_USERNAME=stdout-user'
printf '%s\n' 'NSX_PASSWORD=stdout-pass'
printf '%s\n' 'stderr-secret' >&2
exit 7
`)
	if err := os.WriteFile(scriptPath, script, 0o700); err != nil {
		t.Fatalf("write env script: %v", err)
	}
	configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    username: config-user
    password: config-pass
`)

	_, err := config.Load(config.Options{
		Path:          configPath,
		EnvScriptPath: scriptPath,
		Environ:       map[string]string{},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want env script execution error")
	}
	if !strings.Contains(err.Error(), "execute env script") {
		t.Fatalf("Load() error = %v, want execution context", err)
	}
	if containsAny(err.Error(), "stdout-user", "stdout-pass", "stderr-secret", "config-user", "config-pass") {
		t.Fatalf("Load() error leaked script output or fallback credentials: %v", err)
	}
}

func TestLoadRejectsEnvScriptMissingOrEmptyShebang(t *testing.T) {
	tests := map[string]struct {
		script    string
		wantError string
	}{
		"missing shebang": {
			script:    "NSX_USERNAME=script-user\nNSX_PASSWORD=script-pass\n",
			wantError: "must start with a shebang",
		},
		"empty shebang": {
			script:    "#!   \nNSX_USERNAME=script-user\nNSX_PASSWORD=script-pass\n",
			wantError: "shebang must include an interpreter",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			scriptPath := filepath.Join(dir, "credentials.sh")
			if err := os.WriteFile(scriptPath, []byte(test.script), 0o700); err != nil {
				t.Fatalf("write env script: %v", err)
			}
			configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    username: config-user
    password: config-pass
`)

			_, err := config.Load(config.Options{
				Path:          configPath,
				EnvScriptPath: scriptPath,
				Environ:       map[string]string{},
			})
			if err == nil {
				t.Fatal("Load() error = nil, want env script shebang error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Load() error = %v, want %q", err, test.wantError)
			}
			if containsAny(err.Error(), "script-user", "script-pass", "config-user", "config-pass") {
				t.Fatalf("Load() error leaked credential material: %v", err)
			}
		})
	}
}

func TestLoadEnvScriptUsesShebangInterpreterAndArguments(t *testing.T) {
	dir := t.TempDir()
	argvPath := filepath.Join(dir, "argv.txt")
	interpreterPath := filepath.Join(dir, "fake-interpreter.sh")
	interpreter := []byte(`#!/bin/sh
printf '%s\n' "$@" > "` + argvPath + `"
printf '%s\n' 'NSX_USERNAME=script-user'
printf '%s\n' 'NSX_PASSWORD=script-pass'
`)
	if err := os.WriteFile(interpreterPath, interpreter, 0o700); err != nil {
		t.Fatalf("write fake interpreter: %v", err)
	}
	scriptPath := filepath.Join(dir, "credentials.custom")
	script := []byte("#!" + interpreterPath + " --format env\nignored by fake interpreter\n")
	if err := os.WriteFile(scriptPath, script, 0o600); err != nil {
		t.Fatalf("write env script: %v", err)
	}
	configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    username: config-user
    password: config-pass
`)

	loaded, err := config.Load(config.Options{
		Path:          configPath,
		EnvScriptPath: scriptPath,
		Environ:       map[string]string{},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.NSX.Auth.Username != "script-user" {
		t.Fatalf("Username = %q, want script-user", loaded.NSX.Auth.Username)
	}
	if loaded.NSX.Auth.Password != "script-pass" {
		t.Fatalf("Password = %q, want script-pass", loaded.NSX.Auth.Password)
	}
	if loaded.NSX.Auth.Source != config.CredentialSourceEnvScript {
		t.Fatalf("Source = %q, want %q", loaded.NSX.Auth.Source, config.CredentialSourceEnvScript)
	}

	argvContent, err := os.ReadFile(argvPath)
	if err != nil {
		t.Fatalf("read fake interpreter argv: %v", err)
	}
	argvLines := strings.Split(strings.TrimSuffix(string(argvContent), "\n"), "\n")
	wantArgv := []string{"--format", "env", scriptPath}
	if len(argvLines) != len(wantArgv) {
		t.Fatalf("fake interpreter argv = %#v, want %#v", argvLines, wantArgv)
	}
	for i, want := range wantArgv {
		if argvLines[i] != want {
			t.Fatalf("fake interpreter argv[%d] = %q, want %q; all argv = %#v", i, argvLines[i], want, argvLines)
		}
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

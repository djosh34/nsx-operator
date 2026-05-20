// Package config loads and validates nsx-operator configuration.
package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/djosh34/nsx-operator/internal/logging"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	Operator        OperatorConfig
	HTTPRateLimiter HTTPRateLimiterConfig
	KubeAPI         KubeAPIConfig
	NSX             NSXConfig
	Logging         LoggingConfig
}

// OperatorConfig controls operator process behavior.
type OperatorConfig struct {
	TickInterval       time.Duration
	MetricsBindAddress string
}

// HTTPRateLimiterConfig controls NSX HTTP client rate limiting.
type HTTPRateLimiterConfig struct {
	MaxRequestsInFlightPerHost  int
	MaxRequestsPerSecondPerHost int
}

// KubeAPIConfig controls Kubernetes API batch behavior.
type KubeAPIConfig struct {
	NumParallelWorkers   int
	MaxRequestsPerSecond int
	MaxRequestsInFlight  int
}

// NSXConfig contains NSX endpoint, auth, write, and TLS settings.
type NSXConfig struct {
	URLScheme               string
	WritesEnabled           bool
	WritesEnabledConfigured bool
	Auth                    BasicAuth
	TLS                     TLSConfig
}

// BasicAuth contains resolved NSX basic auth credentials.
type BasicAuth struct {
	Username string
	Password string
	Source   CredentialSource
}

// CredentialSource identifies where NSX credentials were resolved from.
type CredentialSource string

const (
	// CredentialSourceEnvScript means credentials came from an environment script.
	CredentialSourceEnvScript CredentialSource = "env_script"
	// CredentialSourceEnv means credentials came from environment variables.
	CredentialSourceEnv CredentialSource = "env"
	// CredentialSourceEnvFiles means credentials came from environment-named files.
	CredentialSourceEnvFiles CredentialSource = "env_files"
	// CredentialSourceConfigValues means credentials came from config values.
	CredentialSourceConfigValues CredentialSource = "config_values"
	// CredentialSourceConfigFiles means credentials came from files named in config.
	CredentialSourceConfigFiles CredentialSource = "config_files"
)

// TLSConfig controls NSX TLS trust configuration.
type TLSConfig struct {
	CABundleFile string
}

// LoggingConfig controls zap logger configuration.
type LoggingConfig struct {
	Level string
}

// Options controls how configuration is loaded.
type Options struct {
	Path          string
	EnvScriptPath string
	Environ       map[string]string
	FS            fs.FS
	Logger        *zap.Logger
}

type rawConfig struct {
	Operator        rawOperatorConfig        `yaml:"operator"`
	HTTPRateLimiter rawHTTPRateLimiterConfig `yaml:"httpRateLimiter"`
	KubeAPI         rawKubeAPIConfig         `yaml:"kubeAPI"`
	NSX             rawNSXConfig             `yaml:"nsx"`
	Logging         rawLoggingConfig         `yaml:"logging"`
}

type rawOperatorConfig struct {
	TickInterval       string `yaml:"tickInterval"`
	MetricsBindAddress string `yaml:"metricsBindAddress"`
}

type rawHTTPRateLimiterConfig struct {
	MaxRequestsInFlightPerHost  int `yaml:"maxRequestsInFlightPerHost"`
	MaxRequestsPerSecondPerHost int `yaml:"maxRequestsPerSecondPerHost"`
}

type rawKubeAPIConfig struct {
	NumParallelWorkers   int `yaml:"numParallelWorkers"`
	MaxRequestsPerSecond int `yaml:"maxRequestsPerSecond"`
	MaxRequestsInFlight  int `yaml:"maxRequestsInFlight"`
}

type rawNSXConfig struct {
	URLScheme     string        `yaml:"urlScheme"`
	WritesEnabled *bool         `yaml:"writesEnabled"`
	Auth          rawAuthConfig `yaml:"auth"`
	TLS           rawTLSConfig  `yaml:"tls"`
}

type rawAuthConfig struct {
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	UsernameFile string `yaml:"usernameFile"`
	PasswordFile string `yaml:"passwordFile"`
}

type rawTLSConfig struct {
	CABundleFile string `yaml:"caBundleFile"`
}

type rawLoggingConfig struct {
	Level string `yaml:"level"`
}

// Load reads, validates, and resolves the operator configuration.
func Load(options Options) (*Config, error) {
	if options.Path == "" {
		return nil, errors.New("config path is required")
	}

	content, err := os.ReadFile(options.Path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var raw rawConfig
	err = yaml.Unmarshal(content, &raw)
	if err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}

	tickInterval, err := parsePositiveDuration("operator.tickInterval", raw.Operator.TickInterval)
	if err != nil {
		return nil, err
	}
	if raw.HTTPRateLimiter.MaxRequestsInFlightPerHost <= 0 {
		return nil, fmt.Errorf("httpRateLimiter.maxRequestsInFlightPerHost must be positive")
	}
	if raw.HTTPRateLimiter.MaxRequestsPerSecondPerHost <= 0 {
		return nil, fmt.Errorf("httpRateLimiter.maxRequestsPerSecondPerHost must be positive")
	}
	kubeAPI, err := parseKubeAPIConfig(raw.KubeAPI)
	if err != nil {
		return nil, err
	}
	if !isSupportedLogLevel(raw.Logging.Level) {
		return nil, fmt.Errorf("logging.level must be one of debug, info, warn, or error")
	}
	nsxURLScheme, err := parseNSXURLScheme(raw.NSX.URLScheme)
	if err != nil {
		return nil, err
	}
	if raw.NSX.TLS.CABundleFile != "" {
		_, err = os.Stat(raw.NSX.TLS.CABundleFile)
		if err != nil {
			return nil, fmt.Errorf("validate nsx.tls.caBundleFile %q: %w", raw.NSX.TLS.CABundleFile, err)
		}
	}

	logger := options.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	auth, err := resolveBasicAuth(raw.NSX.Auth, options.EnvScriptPath, options.Environ, options.FS, logger)
	if err != nil {
		return nil, err
	}

	writesEnabled := true
	if raw.NSX.WritesEnabled != nil {
		writesEnabled = *raw.NSX.WritesEnabled
	}

	return &Config{
		Operator: OperatorConfig{
			TickInterval:       tickInterval,
			MetricsBindAddress: parseMetricsBindAddress(raw.Operator.MetricsBindAddress),
		},
		HTTPRateLimiter: HTTPRateLimiterConfig{
			MaxRequestsInFlightPerHost:  raw.HTTPRateLimiter.MaxRequestsInFlightPerHost,
			MaxRequestsPerSecondPerHost: raw.HTTPRateLimiter.MaxRequestsPerSecondPerHost,
		},
		KubeAPI: *kubeAPI,
		NSX: NSXConfig{
			URLScheme:               nsxURLScheme,
			WritesEnabled:           writesEnabled,
			WritesEnabledConfigured: true,
			Auth:                    *auth,
			TLS: TLSConfig{
				CABundleFile: raw.NSX.TLS.CABundleFile,
			},
		},
		Logging: LoggingConfig{
			Level: raw.Logging.Level,
		},
	}, nil
}

func parseKubeAPIConfig(raw rawKubeAPIConfig) (*KubeAPIConfig, error) {
	if raw.NumParallelWorkers < 0 {
		return nil, fmt.Errorf("kubeAPI.numParallelWorkers must not be negative")
	}
	if raw.MaxRequestsPerSecond < 0 {
		return nil, fmt.Errorf("kubeAPI.maxRequestsPerSecond must not be negative")
	}
	if raw.MaxRequestsInFlight < 0 {
		return nil, fmt.Errorf("kubeAPI.maxRequestsInFlight must not be negative")
	}
	cfg := KubeAPIConfig(raw)
	if cfg.NumParallelWorkers == 0 {
		cfg.NumParallelWorkers = 1
	}
	if cfg.MaxRequestsPerSecond == 0 {
		cfg.MaxRequestsPerSecond = 100
	}
	if cfg.MaxRequestsInFlight == 0 {
		cfg.MaxRequestsInFlight = 100
	}
	return &cfg, nil
}

func parseMetricsBindAddress(value string) string {
	if value == "" {
		return ":8080"
	}
	return value
}

func parsePositiveDuration(field string, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", field, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be positive", field)
	}
	return duration, nil
}

func isSupportedLogLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

func parseNSXURLScheme(value string) (string, error) {
	if value == "" {
		return "https", nil
	}
	switch value {
	case "http", "https":
		return value, nil
	default:
		return "", fmt.Errorf("nsx.urlScheme must be http or https")
	}
}

func resolveBasicAuth(raw rawAuthConfig, envScriptPath string, environ map[string]string, filesystem fs.FS, logger *zap.Logger) (*BasicAuth, error) {
	if envScriptPath != "" {
		auth, err := resolveEnvScriptBasicAuth(envScriptPath, logger)
		if err != nil {
			return nil, err
		}
		return auth, nil
	}
	if hasPartialCredentialPair(environ["NSX_USERNAME"], environ["NSX_PASSWORD"]) {
		return nil, fmt.Errorf("NSX_USERNAME and NSX_PASSWORD must both be set or both be unset")
	}
	if hasCompleteCredentialPair(environ["NSX_USERNAME"], environ["NSX_PASSWORD"]) {
		return &BasicAuth{
			Username: environ["NSX_USERNAME"],
			Password: environ["NSX_PASSWORD"],
			Source:   CredentialSourceEnv,
		}, nil
	}
	if hasPartialCredentialPair(environ["NSX_USERNAME_FILE"], environ["NSX_PASSWORD_FILE"]) {
		return nil, fmt.Errorf("NSX_USERNAME_FILE and NSX_PASSWORD_FILE must both be set or both be unset")
	}
	if hasCompleteCredentialPair(environ["NSX_USERNAME_FILE"], environ["NSX_PASSWORD_FILE"]) {
		username, err := readCredentialFile(filesystem, environ["NSX_USERNAME_FILE"])
		if err != nil {
			return nil, fmt.Errorf("read NSX_USERNAME_FILE: %w", err)
		}
		password, err := readCredentialFile(filesystem, environ["NSX_PASSWORD_FILE"])
		if err != nil {
			return nil, fmt.Errorf("read NSX_PASSWORD_FILE: %w", err)
		}
		return &BasicAuth{
			Username: username,
			Password: password,
			Source:   CredentialSourceEnvFiles,
		}, nil
	}
	if hasPartialCredentialPair(raw.Username, raw.Password) {
		return nil, fmt.Errorf("nsx.auth.username and nsx.auth.password must both be set or both be unset")
	}
	if hasCompleteCredentialPair(raw.Username, raw.Password) {
		return &BasicAuth{
			Username: raw.Username,
			Password: raw.Password,
			Source:   CredentialSourceConfigValues,
		}, nil
	}
	if hasPartialCredentialPair(raw.UsernameFile, raw.PasswordFile) {
		return nil, fmt.Errorf("nsx.auth.usernameFile and nsx.auth.passwordFile must both be set or both be unset")
	}
	if hasCompleteCredentialPair(raw.UsernameFile, raw.PasswordFile) {
		username, err := readCredentialFile(filesystem, raw.UsernameFile)
		if err != nil {
			return nil, fmt.Errorf("read nsx.auth.usernameFile: %w", err)
		}
		password, err := readCredentialFile(filesystem, raw.PasswordFile)
		if err != nil {
			return nil, fmt.Errorf("read nsx.auth.passwordFile: %w", err)
		}
		return &BasicAuth{
			Username: username,
			Password: password,
			Source:   CredentialSourceConfigFiles,
		}, nil
	}
	return nil, fmt.Errorf("nsx.auth must resolve one complete basic auth credential source")
}

func resolveEnvScriptBasicAuth(path string, logger *zap.Logger) (*BasicAuth, error) {
	interpreter, args, err := readEnvScriptShebang(path)
	if err != nil {
		return nil, err
	}
	logger.Info(
		"loading nsx credentials from env script",
		logging.Component("config"),
		zap.String("env_script_path", path),
		zap.String("interpreter", interpreter),
	)
	logger.Debug(
		"parsed env script shebang",
		logging.Component("config"),
		zap.String("env_script_path", path),
		zap.String("interpreter", interpreter),
		zap.Int("interpreter_arg_count", len(args)),
	)

	args = append(args, path)
	commandCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(commandCtx, interpreter, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if err != nil {
		var exitError *exec.ExitError
		exitCode := -1
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
		logger.Info(
			"env script credential command failed",
			logging.Component("config"),
			zap.String("env_script_path", path),
			zap.String("interpreter", interpreter),
			zap.Int("exit_code", exitCode),
			zap.Error(err),
		)
		return nil, fmt.Errorf("execute env script %q with interpreter %q: %w", path, interpreter, err)
	}

	values, err := parseEnvScriptOutput(stdout.String())
	if err != nil {
		return nil, err
	}
	username := values["NSX_USERNAME"]
	password := values["NSX_PASSWORD"]
	logger.Debug(
		"parsed env script credential output",
		logging.Component("config"),
		zap.String("env_script_path", path),
		zap.Bool("has_nsx_username", username != ""),
		zap.Bool("has_nsx_password", password != ""),
	)
	if username == "" {
		return nil, fmt.Errorf("env script %q did not provide NSX_USERNAME", path)
	}
	if password == "" {
		return nil, fmt.Errorf("env script %q did not provide NSX_PASSWORD", path)
	}
	return &BasicAuth{
		Username: username,
		Password: password,
		Source:   CredentialSourceEnvScript,
	}, nil
}

func readEnvScriptShebang(path string) (string, []string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read env script %q: %w", path, err)
	}

	firstLine, _, _ := strings.Cut(string(content), "\n")
	firstLine = strings.TrimSuffix(firstLine, "\r")
	if !strings.HasPrefix(firstLine, "#!") {
		return "", nil, fmt.Errorf("env script %q must start with a shebang", path)
	}
	fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(firstLine, "#!")))
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("env script %q shebang must include an interpreter", path)
	}
	return fields[0], fields[1:], nil
}

func parseEnvScriptOutput(output string) (map[string]string, error) {
	values := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("env script output contains malformed line")
		}
		switch key {
		case "NSX_USERNAME", "NSX_PASSWORD":
			values[key] = value
		}
	}
	return values, nil
}

func hasCompleteCredentialPair(first string, second string) bool {
	return first != "" && second != ""
}

func hasPartialCredentialPair(first string, second string) bool {
	return (first == "") != (second == "")
}

func readCredentialFile(filesystem fs.FS, path string) (string, error) {
	content, err := readFile(filesystem, path)
	if err != nil {
		return "", err
	}

	value := string(content)
	value = strings.TrimSuffix(value, "\r\n")
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return "", fmt.Errorf("credential file %q is empty", path)
	}
	return value, nil
}

func readFile(filesystem fs.FS, path string) ([]byte, error) {
	if filesystem != nil && !filepath.IsAbs(path) {
		content, err := fs.ReadFile(filesystem, path)
		if err != nil {
			return nil, err
		}
		return content, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return content, nil
}

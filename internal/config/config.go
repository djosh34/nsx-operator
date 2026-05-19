package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Operator        OperatorConfig
	HTTPRateLimiter HTTPRateLimiterConfig
	NSX             NSXConfig
	Logging         LoggingConfig
}

type OperatorConfig struct {
	TickInterval       time.Duration
	MetricsBindAddress string
}

type HTTPRateLimiterConfig struct {
	MaxRequestsInFlightPerHost  int
	MaxRequestsPerSecondPerHost int
}

type NSXConfig struct {
	URLScheme               string
	WritesEnabled           bool
	WritesEnabledConfigured bool
	Auth                    BasicAuth
	TLS                     TLSConfig
}

type BasicAuth struct {
	Username string
	Password string
	Source   CredentialSource
}

type CredentialSource string

const (
	CredentialSourceEnv          CredentialSource = "env"
	CredentialSourceEnvFiles     CredentialSource = "env_files"
	CredentialSourceConfigValues CredentialSource = "config_values"
	CredentialSourceConfigFiles  CredentialSource = "config_files"
)

type TLSConfig struct {
	CABundleFile string
}

type LoggingConfig struct {
	Level string
}

type Options struct {
	Path    string
	Environ map[string]string
	FS      fs.FS
}

type rawConfig struct {
	Operator        rawOperatorConfig        `yaml:"operator"`
	HTTPRateLimiter rawHTTPRateLimiterConfig `yaml:"httpRateLimiter"`
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

func Load(options Options) (Config, error) {
	if options.Path == "" {
		return Config{}, errors.New("config path is required")
	}

	content, err := os.ReadFile(options.Path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return Config{}, fmt.Errorf("parse config yaml: %w", err)
	}

	tickInterval, err := parsePositiveDuration("operator.tickInterval", raw.Operator.TickInterval)
	if err != nil {
		return Config{}, err
	}
	if raw.HTTPRateLimiter.MaxRequestsInFlightPerHost <= 0 {
		return Config{}, fmt.Errorf("httpRateLimiter.maxRequestsInFlightPerHost must be positive")
	}
	if raw.HTTPRateLimiter.MaxRequestsPerSecondPerHost <= 0 {
		return Config{}, fmt.Errorf("httpRateLimiter.maxRequestsPerSecondPerHost must be positive")
	}
	if !isSupportedLogLevel(raw.Logging.Level) {
		return Config{}, fmt.Errorf("logging.level must be one of debug, info, warn, or error")
	}
	nsxURLScheme, err := parseNSXURLScheme(raw.NSX.URLScheme)
	if err != nil {
		return Config{}, err
	}
	if raw.NSX.TLS.CABundleFile != "" {
		if _, err := os.Stat(raw.NSX.TLS.CABundleFile); err != nil {
			return Config{}, fmt.Errorf("validate nsx.tls.caBundleFile %q: %w", raw.NSX.TLS.CABundleFile, err)
		}
	}

	auth, err := resolveBasicAuth(raw.NSX.Auth, options.Environ, options.FS)
	if err != nil {
		return Config{}, err
	}

	writesEnabled := true
	if raw.NSX.WritesEnabled != nil {
		writesEnabled = *raw.NSX.WritesEnabled
	}

	return Config{
		Operator: OperatorConfig{
			TickInterval:       tickInterval,
			MetricsBindAddress: parseMetricsBindAddress(raw.Operator.MetricsBindAddress),
		},
		HTTPRateLimiter: HTTPRateLimiterConfig{
			MaxRequestsInFlightPerHost:  raw.HTTPRateLimiter.MaxRequestsInFlightPerHost,
			MaxRequestsPerSecondPerHost: raw.HTTPRateLimiter.MaxRequestsPerSecondPerHost,
		},
		NSX: NSXConfig{
			URLScheme:               nsxURLScheme,
			WritesEnabled:           writesEnabled,
			WritesEnabledConfigured: true,
			Auth:                    auth,
			TLS: TLSConfig{
				CABundleFile: raw.NSX.TLS.CABundleFile,
			},
		},
		Logging: LoggingConfig{
			Level: raw.Logging.Level,
		},
	}, nil
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

func resolveBasicAuth(raw rawAuthConfig, environ map[string]string, filesystem fs.FS) (BasicAuth, error) {
	if hasPartialCredentialPair(environ["NSX_USERNAME"], environ["NSX_PASSWORD"]) {
		return BasicAuth{}, fmt.Errorf("NSX_USERNAME and NSX_PASSWORD must both be set or both be unset")
	}
	if hasCompleteCredentialPair(environ["NSX_USERNAME"], environ["NSX_PASSWORD"]) {
		return BasicAuth{
			Username: environ["NSX_USERNAME"],
			Password: environ["NSX_PASSWORD"],
			Source:   CredentialSourceEnv,
		}, nil
	}
	if hasPartialCredentialPair(environ["NSX_USERNAME_FILE"], environ["NSX_PASSWORD_FILE"]) {
		return BasicAuth{}, fmt.Errorf("NSX_USERNAME_FILE and NSX_PASSWORD_FILE must both be set or both be unset")
	}
	if hasCompleteCredentialPair(environ["NSX_USERNAME_FILE"], environ["NSX_PASSWORD_FILE"]) {
		username, err := readCredentialFile(filesystem, environ["NSX_USERNAME_FILE"])
		if err != nil {
			return BasicAuth{}, fmt.Errorf("read NSX_USERNAME_FILE: %w", err)
		}
		password, err := readCredentialFile(filesystem, environ["NSX_PASSWORD_FILE"])
		if err != nil {
			return BasicAuth{}, fmt.Errorf("read NSX_PASSWORD_FILE: %w", err)
		}
		return BasicAuth{
			Username: username,
			Password: password,
			Source:   CredentialSourceEnvFiles,
		}, nil
	}
	if hasPartialCredentialPair(raw.Username, raw.Password) {
		return BasicAuth{}, fmt.Errorf("nsx.auth.username and nsx.auth.password must both be set or both be unset")
	}
	if hasCompleteCredentialPair(raw.Username, raw.Password) {
		return BasicAuth{
			Username: raw.Username,
			Password: raw.Password,
			Source:   CredentialSourceConfigValues,
		}, nil
	}
	if hasPartialCredentialPair(raw.UsernameFile, raw.PasswordFile) {
		return BasicAuth{}, fmt.Errorf("nsx.auth.usernameFile and nsx.auth.passwordFile must both be set or both be unset")
	}
	if hasCompleteCredentialPair(raw.UsernameFile, raw.PasswordFile) {
		username, err := readCredentialFile(filesystem, raw.UsernameFile)
		if err != nil {
			return BasicAuth{}, fmt.Errorf("read nsx.auth.usernameFile: %w", err)
		}
		password, err := readCredentialFile(filesystem, raw.PasswordFile)
		if err != nil {
			return BasicAuth{}, fmt.Errorf("read nsx.auth.passwordFile: %w", err)
		}
		return BasicAuth{
			Username: username,
			Password: password,
			Source:   CredentialSourceConfigFiles,
		}, nil
	}
	return BasicAuth{}, fmt.Errorf("nsx.auth must resolve one complete basic auth credential source")
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

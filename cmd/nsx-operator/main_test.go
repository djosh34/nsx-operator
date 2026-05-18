package main

import (
	"os"
	"path/filepath"
	"testing"
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

func writeCommandConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write command config: %v", err)
	}
	return path
}

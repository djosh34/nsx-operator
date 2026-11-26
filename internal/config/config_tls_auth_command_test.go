package config_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/djosh34/nsx-operator/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoadNSXTLSInsecureSkipVerifyDefaultsFalseAndAllowsTrue(t *testing.T) {
	tests := map[string]struct {
		tlsYAML string
		want    bool
	}{
		"omitted": {tlsYAML: "", want: false},
		"false":   {tlsYAML: "  tls:\n    insecureSkipVerify: false\n", want: false},
		"true":    {tlsYAML: "  tls:\n    insecureSkipVerify: true\n", want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			configPath := writeValidConfig(t, t.TempDir(), `
nsx:
  auth:
    username: config-user
    password: config-pass
`+tt.tlsYAML)

			loaded, err := config.Load(config.Options{
				Path:    configPath,
				Environ: map[string]string{},
			})
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if loaded.NSX.TLS.InsecureSkipVerify != tt.want {
				t.Fatalf("InsecureSkipVerify = %t, want %t", loaded.NSX.TLS.InsecureSkipVerify, tt.want)
			}
		})
	}
}

func TestLoadRejectsCABundleFileWithoutValidCertificates(t *testing.T) {
	dir := t.TempDir()
	caBundlePath := filepath.Join(dir, "not-a-ca.pem")
	err := os.WriteFile(caBundlePath, []byte("not a certificate\n"), 0o600)
	if err != nil {
		t.Fatalf("write CA bundle: %v", err)
	}
	configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    username: config-user
    password: config-pass
  tls:
    caBundleFile: `+caBundlePath+`
`)

	_, err = config.Load(config.Options{
		Path:    configPath,
		Environ: map[string]string{},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want invalid CA bundle error")
	}
	if !strings.Contains(err.Error(), "no valid PEM certificates") || !strings.Contains(err.Error(), caBundlePath) {
		t.Fatalf("Load() error = %v, want clear invalid CA bundle error naming the file", err)
	}
}

func TestTLSConfigRootCAsIncludesCABundle(t *testing.T) {
	dir := t.TempDir()
	caBundlePath := filepath.Join(dir, "ca.pem")
	certificatePEM := selfSignedCertificatePEM(t)
	err := os.WriteFile(caBundlePath, certificatePEM, 0o600)
	if err != nil {
		t.Fatalf("write CA bundle: %v", err)
	}
	block, _ := pem.Decode(certificatePEM)
	if block == nil {
		t.Fatal("decode generated certificate PEM returned nil block")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse generated certificate: %v", err)
	}

	pool, err := (&config.TLSConfig{CABundleFile: caBundlePath}).RootCAs()
	if err != nil {
		t.Fatalf("RootCAs() error = %v", err)
	}
	_, err = certificate.Verify(x509.VerifyOptions{Roots: pool})
	if err != nil {
		t.Fatalf("verify generated certificate with RootCAs() pool: %v", err)
	}

	systemPool, err := (&config.TLSConfig{}).RootCAs()
	if err != nil {
		t.Fatalf("RootCAs() without bundle error = %v", err)
	}
	_, err = certificate.Verify(x509.VerifyOptions{Roots: systemPool})
	if err == nil {
		t.Fatal("verify generated certificate with system-only pool succeeded, want unknown authority")
	}

	_, err = (&config.TLSConfig{CABundleFile: filepath.Join(dir, "missing.pem")}).RootCAs()
	if err == nil {
		t.Fatal("RootCAs() with missing bundle error = nil, want read error")
	}
}

func TestLoadAuthCommandResolvesJSONCredentials(t *testing.T) {
	dir := t.TempDir()
	commandPath := writeAuthCommand(t, dir, `printf '%s' '{"username":"cmd-user","password":"cmd-pass"}'
echo "diagnostic noise" >&2`)
	configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    command: `+commandPath+`
`)
	core, logs := observer.New(zapcore.DebugLevel)

	loaded, err := config.Load(config.Options{
		Path:    configPath,
		Environ: map[string]string{},
		Logger:  zap.New(core),
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.NSX.Auth.Username != "cmd-user" {
		t.Fatalf("Username = %q, want cmd-user", loaded.NSX.Auth.Username)
	}
	if loaded.NSX.Auth.Password != "cmd-pass" {
		t.Fatalf("Password = %q, want cmd-pass", loaded.NSX.Auth.Password)
	}
	if loaded.NSX.Auth.Source != config.CredentialSourceConfigCommand {
		t.Fatalf("Source = %q, want %q", loaded.NSX.Auth.Source, config.CredentialSourceConfigCommand)
	}
	if logs.FilterMessageSnippet("loading nsx credentials from auth command").Len() != 1 {
		t.Fatalf("auth command info log count = %d, want 1", logs.FilterMessageSnippet("loading nsx credentials from auth command").Len())
	}
	requireLogsExclude(t, logs, "cmd-pass")
}

func TestLoadEnvCredentialsOverrideAuthCommandWithoutRunningIt(t *testing.T) {
	dir := t.TempDir()
	markerPath := filepath.Join(dir, "ran")
	commandPath := writeAuthCommand(t, dir, `touch "`+markerPath+`"
exit 1`)
	configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    command: `+commandPath+`
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
	if loaded.NSX.Auth.Source != config.CredentialSourceEnv {
		t.Fatalf("Source = %q, want %q", loaded.NSX.Auth.Source, config.CredentialSourceEnv)
	}
	_, err = os.Stat(markerPath)
	if !os.IsNotExist(err) {
		t.Fatalf("auth command marker stat error = %v, want not exist (command must not run)", err)
	}
}

func TestLoadRejectsFailingAuthCommand(t *testing.T) {
	tests := map[string]struct {
		body        string
		wantInError []string
	}{
		"non-zero exit": {
			body:        `echo "vault login expired" >&2` + "\n" + `printf '%s' '{"username":"cmd-user","password":"leaked-secret"}'` + "\nexit 3",
			wantInError: []string{"exit status 3", "vault login expired"},
		},
		"invalid json": {
			body:        `echo "rendering credentials" >&2` + "\n" + `printf '%s' 'username=cmd-user password=leaked-secret'`,
			wantInError: []string{"must be a JSON object", "rendering credentials"},
		},
		"empty stdout": {
			body:        `echo "nothing to print" >&2`,
			wantInError: []string{"must be a JSON object", "nothing to print"},
		},
		"empty username": {
			body:        `printf '%s' '{"username":"","password":"leaked-secret"}'`,
			wantInError: []string{"non-empty username"},
		},
		"missing password": {
			body:        `printf '%s' '{"username":"cmd-user"}'`,
			wantInError: []string{"non-empty password"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			commandPath := writeAuthCommand(t, dir, tt.body)
			configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    command: `+commandPath+`
`)

			core, logs := observer.New(zapcore.DebugLevel)

			_, err := config.Load(config.Options{
				Path:    configPath,
				Environ: map[string]string{},
				Logger:  zap.New(core),
			})
			if err == nil {
				t.Fatal("Load() error = nil, want auth command error")
			}
			requireLogsExclude(t, logs, "leaked-secret")
			if !strings.Contains(err.Error(), commandPath) {
				t.Fatalf("Load() error = %v, want command path %q", err, commandPath)
			}
			for _, want := range tt.wantInError {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("Load() error = %v, want it to contain %q", err, want)
				}
			}
			if strings.Contains(err.Error(), "leaked-secret") {
				t.Fatalf("Load() error leaked password: %v", err)
			}
		})
	}
}

func TestLoadRejectsMissingAuthCommand(t *testing.T) {
	dir := t.TempDir()
	commandPath := filepath.Join(dir, "missing-auth.sh")
	configPath := writeValidConfig(t, dir, `
nsx:
  auth:
    command: `+commandPath+`
`)

	_, err := config.Load(config.Options{
		Path:    configPath,
		Environ: map[string]string{},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want missing auth command error")
	}
	if !strings.Contains(err.Error(), commandPath) {
		t.Fatalf("Load() error = %v, want command path %q", err, commandPath)
	}
}

func TestLoadRejectsMultipleConfigCredentialSources(t *testing.T) {
	tests := map[string]string{
		"command and username/password": `
    command: /bin/true
    username: config-user
    password: config-pass
`,
		"command and credential files": `
    command: /bin/true
    usernameFile: user.txt
    passwordFile: pass.txt
`,
		"username/password and credential files": `
    username: config-user
    password: config-pass
    usernameFile: user.txt
    passwordFile: pass.txt
`,
	}
	for name, authYAML := range tests {
		t.Run(name, func(t *testing.T) {
			configPath := writeValidConfig(t, t.TempDir(), `
nsx:
  auth:`+authYAML)

			_, err := config.Load(config.Options{
				Path:    configPath,
				Environ: map[string]string{},
			})
			if err == nil {
				t.Fatal("Load() error = nil, want multiple credential source error")
			}
			if !strings.Contains(err.Error(), "only one credential source") {
				t.Fatalf("Load() error = %v, want multiple credential source error", err)
			}
			if strings.Contains(err.Error(), "config-pass") {
				t.Fatalf("Load() error leaked password: %v", err)
			}
		})
	}
}

func requireLogsExclude(t *testing.T, logs *observer.ObservedLogs, secret string) {
	t.Helper()

	entries := logs.All()
	if len(entries) == 0 {
		t.Fatal("no log entries observed, want auth command logging")
	}
	for index := range entries {
		rendered := entries[index].Message + fmt.Sprint(entries[index].ContextMap())
		if strings.Contains(rendered, secret) {
			t.Fatalf("log entry leaked password: %s", rendered)
		}
	}
}

func writeAuthCommand(t *testing.T, dir string, body string) string {
	t.Helper()

	commandPath := filepath.Join(dir, "nsx-auth.sh")
	err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"+body+"\n"), 0o700)
	if err != nil {
		t.Fatalf("write auth command: %v", err)
	}
	return commandPath
}

func selfSignedCertificatePEM(t *testing.T) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "nsx-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

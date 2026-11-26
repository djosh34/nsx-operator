package startup

import (
	"context"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djosh34/nsx-operator/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewNSXHTTPClientTLSTrust(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := io.WriteString(w, "ok")
		if err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	caBundlePath := filepath.Join(t.TempDir(), "nsx-ca.pem")
	err := os.WriteFile(caBundlePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600)
	if err != nil {
		t.Fatalf("write CA bundle: %v", err)
	}

	tests := []struct {
		name        string
		tlsConfig   config.TLSConfig
		wantErr     string
		wantWarning bool
	}{
		{
			name:      "system roots reject self-signed nsx certificate",
			tlsConfig: config.TLSConfig{},
			wantErr:   "certificate",
		},
		{
			name:      "ca bundle trusts self-signed nsx certificate",
			tlsConfig: config.TLSConfig{CABundleFile: caBundlePath},
		},
		{
			name:        "insecure skip verify accepts self-signed nsx certificate",
			tlsConfig:   config.TLSConfig{InsecureSkipVerify: true},
			wantWarning: true,
		},
	}
	for testIndex := range tests {
		tt := tests[testIndex]
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			client, clientErr := newNSXHTTPClient(rateLimiterConfigForTLSTest(), tt.tlsConfig, zap.New(core))
			if clientErr != nil {
				t.Fatalf("newNSXHTTPClient() error = %v", clientErr)
			}

			getErr := getNSXTestServer(t, client, server.URL)
			if tt.wantErr == "" && getErr != nil {
				t.Fatalf("GET self-signed NSX server error = %v, want success", getErr)
			}
			if tt.wantErr != "" && (getErr == nil || !strings.Contains(getErr.Error(), tt.wantErr)) {
				t.Fatalf("GET self-signed NSX server error = %v, want error containing %q", getErr, tt.wantErr)
			}

			warnings := logs.FilterLevelExact(zapcore.WarnLevel).FilterMessageSnippet("verification is disabled").Len()
			if tt.wantWarning && warnings != 1 {
				t.Fatalf("insecure TLS warning count = %d, want 1", warnings)
			}
			if !tt.wantWarning && warnings != 0 {
				t.Fatalf("insecure TLS warning count = %d, want 0", warnings)
			}
		})
	}
}

func TestNewNSXHTTPClientRejectsCABundleWithoutCertificates(t *testing.T) {
	caBundlePath := filepath.Join(t.TempDir(), "empty-ca.pem")
	err := os.WriteFile(caBundlePath, []byte("no certificates here\n"), 0o600)
	if err != nil {
		t.Fatalf("write CA bundle: %v", err)
	}

	_, err = newNSXHTTPClient(rateLimiterConfigForTLSTest(), config.TLSConfig{CABundleFile: caBundlePath}, zap.NewNop())
	if err == nil {
		t.Fatal("newNSXHTTPClient() error = nil, want invalid CA bundle error")
	}
	if !strings.Contains(err.Error(), "no valid PEM certificates") {
		t.Fatalf("newNSXHTTPClient() error = %v, want no valid PEM certificates", err)
	}
}

func TestNewNSXHTTPClientRejectsNonStandardDefaultTransportWhenTLSConfigured(t *testing.T) {
	previousDefaultTransport := http.DefaultTransport
	http.DefaultTransport = &failingRoundTripper{}
	t.Cleanup(func() {
		http.DefaultTransport = previousDefaultTransport
	})

	_, err := newNSXHTTPClient(rateLimiterConfigForTLSTest(), config.TLSConfig{InsecureSkipVerify: true}, zap.NewNop())
	if err == nil {
		t.Fatal("newNSXHTTPClient() error = nil, want default transport type error")
	}
}

type failingRoundTripper struct{}

func (*failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.EOF
}

func rateLimiterConfigForTLSTest() config.HTTPRateLimiterConfig {
	return config.HTTPRateLimiterConfig{
		MaxRequestsInFlightPerHost:  1,
		MaxRequestsPerSecondPerHost: 100,
	}
}

func getNSXTestServer(t *testing.T, client *http.Client, url string) error {
	t.Helper()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := response.Body.Close()
		if closeErr != nil {
			t.Errorf("close response body: %v", closeErr)
		}
	}()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("response body = %q, want ok", body)
	}
	return nil
}

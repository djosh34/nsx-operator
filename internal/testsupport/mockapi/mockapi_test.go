package mockapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestStartRunsPublicMockAPIImage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)

	process := Start(t, ctx)
	if process.BaseURL() == "" {
		t.Fatal("BaseURL() = empty, want public mockapi endpoint")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, process.BaseURL()+"/policy/api/v1/eula/acceptance", nil)
	if err != nil {
		t.Fatalf("create readiness request: %v", err)
	}
	req.SetBasicAuth(Username, Password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET eula acceptance: %v\nlogs:\n%s", err, process.Logs())
	}
	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			t.Fatalf("close readiness response body: %v", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET eula acceptance status = %d, want %d\nlogs:\n%s", resp.StatusCode, http.StatusOK, process.Logs())
	}
	_ = process.Logs()
}

func TestDockerOutputReportsCommandFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	_, output, err := dockerOutput(ctx, "image", "inspect", "ghcr.io/djosh34/nsx-t-mockapi:definitely-missing-for-test")
	if err == nil {
		t.Fatal("dockerOutput() error = nil, want docker failure")
	}
	if !strings.Contains(output, "definitely-missing-for-test") {
		t.Fatalf("dockerOutput() output = %q, want missing image tag", output)
	}
}

func TestDockerOutputReturnsStdoutOnSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	stdout, output, err := dockerOutput(ctx, "version", "--format", "{{.Server.Os}}")
	if err != nil {
		t.Fatalf("dockerOutput() error = %v\n%s", err, output)
	}
	if strings.TrimSpace(stdout) != "linux" {
		t.Fatalf("dockerOutput() stdout = %q, want linux", stdout)
	}
}

func TestDockerOutputReportsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := dockerOutput(ctx, "version")
	if err == nil {
		t.Fatal("dockerOutput() error = nil, want context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dockerOutput() error = %v, want %v", err, context.Canceled)
	}
}

func TestEmptyProcessLogsAreEmpty(t *testing.T) {
	if logs := (Process{}).Logs(); logs != "" {
		t.Fatalf("empty process Logs() = %q, want empty", logs)
	}
}

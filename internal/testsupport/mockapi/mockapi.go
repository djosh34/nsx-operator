// Package mockapi starts the public NSX-T mockapi container for integration tests.
package mockapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	// Image is the public mockapi container image used by integration tests.
	Image = "ghcr.io/djosh34/nsx-t-mockapi:latest"
	// Username is the default mockapi username.
	Username = "nsx_admin"
	// Password is the default mockapi password.
	Password = "nsx_password"
)

// Process represents a running mockapi container.
type Process struct {
	baseURL     string
	containerID string
}

// Start launches the public mockapi container and waits until it is ready.
//
//nolint:revive // test helpers conventionally accept *testing.T first.
func Start(t *testing.T, ctx context.Context) Process {
	t.Helper()

	port := freeTCPPort(t, ctx)
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	config := `server:
  listen_addr: "0.0.0.0:8080"
database:
  path: "/tmp/nsx-t-mockapi.db"
realization:
  default_delay_ms: 0
  create_delay_ms: 0
  update_delay_ms: 0
  delete_delay_ms: 0
  kind_delay_ms: {}
search:
  default_page_size: 1000
  max_page_size: 1000
`
	err := os.WriteFile(configPath, []byte(config), 0o600)
	if err != nil {
		t.Fatalf("write mockapi config: %v", err)
	}

	containerID, runOutput, err := dockerOutput(
		ctx,
		"run",
		"--detach",
		"--rm",
		"--platform", "linux/amd64",
		"--publish", fmt.Sprintf("127.0.0.1:%d:8080", port),
		"--volume", configPath+":/config/config.yaml:ro",
		Image,
		"serve",
		"-config",
		"/config/config.yaml",
	)
	if err != nil {
		t.Fatalf("start public mockapi image %s: %v\n%s", Image, err, runOutput)
	}
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		t.Fatalf("start public mockapi image %s returned empty container id\n%s", Image, runOutput)
	}

	process := Process{
		baseURL:     fmt.Sprintf("http://127.0.0.1:%d", port),
		containerID: containerID,
	}
	//nolint:contextcheck // cleanup needs a fresh timeout even if the test context was canceled.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, removeOutput, removeErr := dockerOutput(cleanupCtx, "rm", "--force", containerID)
		if removeErr != nil && !strings.Contains(removeOutput, "No such container") {
			t.Errorf("remove mockapi container %s: %v\n%s", containerID, removeErr, removeOutput)
		}
	})

	waitReady(t, ctx, process)
	return process
}

// BaseURL returns the HTTP base URL for the running mockapi process.
func (process *Process) BaseURL() string {
	return process.baseURL
}

// Logs returns recent mockapi container logs for diagnostics.
func (process *Process) Logs() string {
	logsCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return process.LogsContext(logsCtx)
}

// LogsContext returns recent mockapi container logs using the supplied context.
func (process *Process) LogsContext(ctx context.Context) string {
	if process.containerID == "" {
		return ""
	}
	stdout, output, err := dockerOutput(ctx, "logs", process.containerID)
	if err != nil {
		return strings.TrimSpace(output)
	}
	return stdout
}

//nolint:revive // test helpers conventionally accept *testing.T first.
func waitReady(t *testing.T, ctx context.Context, process Process) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, process.baseURL+"/policy/api/v1/eula/acceptance", nil)
		if err != nil {
			t.Fatalf("create mockapi readiness request: %v", err)
		}
		req.SetBasicAuth(Username, Password)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			closeErr := resp.Body.Close()
			if closeErr != nil {
				t.Fatalf("close mockapi readiness body: %v", closeErr)
			}
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("mockapi readiness context ended: %v\nlogs:\n%s", ctx.Err(), process.LogsContext(ctx))
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("mockapi did not become ready; logs:\n%s", process.LogsContext(ctx))
}

//nolint:revive // test helpers conventionally accept *testing.T first.
func freeTCPPort(t *testing.T, ctx context.Context) int {
	t.Helper()

	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on free tcp port: %v", err)
	}
	defer func() {
		closeErr := listener.Close()
		if closeErr != nil {
			t.Fatalf("close free tcp listener: %v", closeErr)
		}
	}()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr = %T, want *net.TCPAddr", listener.Addr())
	}
	return addr.Port
}

func dockerOutput(ctx context.Context, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	combined := stdout.String() + stderr.String()
	if err == nil {
		return stdout.String(), combined, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
		return stdout.String(), combined, ctx.Err()
	}
	return stdout.String(), combined, err
}

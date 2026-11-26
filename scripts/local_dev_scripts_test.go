package scripts

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type authCommandOutput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func TestNSXAuthMockPrintsMockAPICredentialsAsJSON(t *testing.T) {
	t.Parallel()

	output := runScript(t, "./nsx-auth-mock.sh", nil)

	var credentials authCommandOutput
	err := json.Unmarshal(output, &credentials)
	if err != nil {
		t.Fatalf("parse nsx-auth-mock.sh stdout as JSON: %v\n%s", err, output)
	}
	if credentials.Username != "nsx_admin" || credentials.Password != "nsx_password" {
		t.Fatalf("credentials = %+v, want nsx_admin/nsx_password", credentials)
	}
}

func TestNSXAuthExampleEscapesSecretsIntoValidJSON(t *testing.T) {
	t.Parallel()

	secretsDir := t.TempDir()
	wantUsername := `corp\admin@example.com`
	wantPassword := "p\\a\"s\ts$x\r"
	err := os.WriteFile(filepath.Join(secretsDir, "username"), []byte(wantUsername+"\n"), 0o600)
	if err != nil {
		t.Fatalf("write username secret: %v", err)
	}
	err = os.WriteFile(filepath.Join(secretsDir, "password"), []byte(wantPassword), 0o600)
	if err != nil {
		t.Fatalf("write password secret: %v", err)
	}

	output := runScript(t, "./nsx-auth.example.sh", []string{"NSX_SECRETS_DIR=" + secretsDir})

	var credentials authCommandOutput
	err = json.Unmarshal(output, &credentials)
	if err != nil {
		t.Fatalf("parse nsx-auth.example.sh stdout as JSON: %v\n%s", err, output)
	}
	if credentials.Username != wantUsername {
		t.Fatalf("username = %q, want %q", credentials.Username, wantUsername)
	}
	if credentials.Password != wantPassword {
		t.Fatalf("password = %q, want %q", credentials.Password, wantPassword)
	}
}

func TestApplyCRDsAppliesCRDsWaitsAndOptionallySamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantCalls []string
	}{
		{
			name: "crds only",
			wantCalls: []string{
				"apply -f <repo>/crds",
				"wait --for=condition=Established --timeout=60s crd/nsxnetworkclouds.nsx.ing.com crd/nsxgroups.nsx.ing.com",
			},
		},
		{
			name: "with samples",
			args: []string{"--samples"},
			wantCalls: []string{
				"apply -f <repo>/crds",
				"wait --for=condition=Established --timeout=60s crd/nsxnetworkclouds.nsx.ing.com crd/nsxgroups.nsx.ing.com",
				"apply -f <repo>/config/samples/mock.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			binDir := filepath.Join(tempDir, "bin")
			err := os.Mkdir(binDir, 0o755)
			if err != nil {
				t.Fatalf("create fake bin dir: %v", err)
			}
			callsPath := filepath.Join(tempDir, "kubectl-calls.txt")
			writeExecutable(t, filepath.Join(binDir, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${KUBECTL_CALLS_PATH}"
`)

			runScript(t, "./apply-crds.sh", []string{"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"), "KUBECTL_CALLS_PATH=" + callsPath}, tt.args...)

			rawCalls, err := os.ReadFile(callsPath)
			if err != nil {
				t.Fatalf("read kubectl calls: %v", err)
			}
			repoRoot := filepath.Dir(filepath.Dir(scriptTestFilePath(t)))
			gotCalls := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(rawCalls), repoRoot, "<repo>")), "\n")
			if strings.Join(gotCalls, "\n") != strings.Join(tt.wantCalls, "\n") {
				t.Fatalf("kubectl calls =\n%s\nwant\n%s", strings.Join(gotCalls, "\n"), strings.Join(tt.wantCalls, "\n"))
			}
		})
	}
}

func runScript(t *testing.T, script string, env []string, args ...string) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, script, args...)
	cmd.Dir = filepath.Dir(scriptTestFilePath(t))
	cmd.Env = append(os.Environ(), env...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("run %s: %v\nstdout:\n%s\nstderr:\n%s", script, err, output, stderr.String())
	}
	return output
}

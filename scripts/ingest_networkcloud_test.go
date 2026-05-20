package scripts

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

type networkCloudManifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		NetworkCloudFQDN string `yaml:"networkCloudFQDN"`
		NetworkCloudID   string `yaml:"networkCloudId"`
		Name             string `yaml:"name"`
	} `yaml:"spec"`
}

func TestIngestNetworkCloudAppliesManifestFromStdin(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	err := os.Mkdir(binDir, 0o755)
	if err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}

	capturedManifestPath := filepath.Join(tempDir, "applied.yaml")
	writeExecutable(t, filepath.Join(binDir, "yq"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$#" -ne 1 || "$1" != "-p=json" ]]; then
  printf 'unexpected yq args: %s\n' "$*" >&2
  exit 64
fi
cat
`)
	writeExecutable(t, filepath.Join(binDir, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$#" -ne 3 || "$1" != "apply" || "$2" != "-f" || "$3" != "-" ]]; then
  printf 'unexpected kubectl args: %s\n' "$*" >&2
  exit 64
fi
cat >"${CAPTURED_MANIFEST_PATH}"
`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "./ingest-networkcloud.sh")
	cmd.Dir = filepath.Dir(scriptTestFilePath(t))
	cmd.Env = append(
		os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CAPTURED_MANIFEST_PATH="+capturedManifestPath,
	)
	cmd.Stdin = bytes.NewBufferString(`{"name":"cloud-a","fqdn":"nsx-a.example.net","id":"cloud-a-id"}`)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run ingest script: %v\n%s", err, output)
	}

	rawManifest, err := os.ReadFile(capturedManifestPath)
	if err != nil {
		t.Fatalf("read captured manifest: %v", err)
	}

	var manifest networkCloudManifest
	err = yaml.Unmarshal(rawManifest, &manifest)
	if err != nil {
		t.Fatalf("parse captured manifest as yaml: %v\n%s", err, rawManifest)
	}

	if manifest.APIVersion != "nsx.ing.com/v1alpha" {
		t.Fatalf("apiVersion = %q, want %q", manifest.APIVersion, "nsx.ing.com/v1alpha")
	}
	if manifest.Kind != "NSXNetworkCloud" {
		t.Fatalf("kind = %q, want %q", manifest.Kind, "NSXNetworkCloud")
	}
	if manifest.Metadata.Name != "cloud-a" {
		t.Fatalf("metadata.name = %q, want %q", manifest.Metadata.Name, "cloud-a")
	}
	if manifest.Spec.Name != "cloud-a" {
		t.Fatalf("spec.name = %q, want %q", manifest.Spec.Name, "cloud-a")
	}
	if manifest.Spec.NetworkCloudFQDN != "nsx-a.example.net" {
		t.Fatalf("spec.networkCloudFQDN = %q, want %q", manifest.Spec.NetworkCloudFQDN, "nsx-a.example.net")
	}
	if manifest.Spec.NetworkCloudID != "cloud-a-id" {
		t.Fatalf("spec.networkCloudId = %q, want %q", manifest.Spec.NetworkCloudID, "cloud-a-id")
	}
}

func TestIngestNetworkCloudReportsMissingRequiredTool(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	err := os.Mkdir(binDir, 0o755)
	if err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}

	realBash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("find bash: %v", err)
	}
	err = os.Symlink(realBash, filepath.Join(binDir, "bash"))
	if err != nil {
		t.Fatalf("link bash into fake path: %v", err)
	}
	writeExecutable(t, filepath.Join(binDir, "jq"), `#!/usr/bin/env bash
set -euo pipefail
cat
`)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "./ingest-networkcloud.sh")
	cmd.Dir = filepath.Dir(scriptTestFilePath(t))
	cmd.Env = append(os.Environ(), "PATH="+binDir)
	cmd.Stdin = bytes.NewBufferString(`{"name":"cloud-a","fqdn":"nsx-a.example.net","id":"cloud-a-id"}`)

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("run ingest script unexpectedly succeeded\n%s", output)
	}
	if !strings.Contains(string(output), "missing required command: yq") {
		t.Fatalf("missing-tool output = %q, want message for missing yq", output)
	}
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()

	err := os.WriteFile(path, []byte(content), 0o755)
	if err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func scriptTestFilePath(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	return file
}

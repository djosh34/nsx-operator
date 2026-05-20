package scripts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeleteAllCRsSerialFallbackClearsFinalizersAndDeletesAcrossNamespaces(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	err := os.Mkdir(binDir, 0o755)
	if err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}

	linkBashIntoPath(t, binDir)

	callsPath := filepath.Join(tempDir, "kubectl-calls.txt")
	writeDeleteAllCRsKubectl(t, filepath.Join(binDir, "kubectl"), callsPath)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "./delete-all-crs.sh")
	cmd.Dir = filepath.Dir(scriptTestFilePath(t))
	cmd.Env = append(os.Environ(), "PATH="+binDir, "KUBECTL_CALLS_PATH="+callsPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run delete-all-crs script: %v\n%s", err, output)
	}

	assertDeleteAllCRsCalls(t, callsPath)
}

func TestDeleteAllCRsUsesGNUParallelWithMaxTwentyJobsWhenAvailable(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	err := os.Mkdir(binDir, 0o755)
	if err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}

	linkBashIntoPath(t, binDir)

	callsPath := filepath.Join(tempDir, "kubectl-calls.txt")
	parallelCallsPath := filepath.Join(tempDir, "parallel-calls.txt")
	writeDeleteAllCRsKubectl(t, filepath.Join(binDir, "kubectl"), callsPath)
	writeDeleteAllCRsParallel(t, filepath.Join(binDir, "parallel"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "./delete-all-crs.sh")
	cmd.Dir = filepath.Dir(scriptTestFilePath(t))
	cmd.Env = append(
		os.Environ(),
		"PATH="+binDir,
		"KUBECTL_CALLS_PATH="+callsPath,
		"PARALLEL_CALLS_PATH="+parallelCallsPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run delete-all-crs script: %v\n%s", err, output)
	}

	assertDeleteAllCRsCalls(t, callsPath)
	rawParallelCalls, err := os.ReadFile(parallelCallsPath)
	if err != nil {
		t.Fatalf("read parallel call log: %v", err)
	}
	if strings.TrimSpace(string(rawParallelCalls)) != "parallel -j 20" {
		t.Fatalf("parallel calls = %q, want max concurrency 20", rawParallelCalls)
	}
}

func writeDeleteAllCRsKubectl(t *testing.T, path string, callsPath string) {
	t.Helper()

	writeExecutable(t, path, `#!/usr/bin/env bash
set -euo pipefail

record_call() {
  printf '%s\n' "$1" >>"${KUBECTL_CALLS_PATH}"
}

case "${1:-}" in
  get)
    if [[ "$#" -ne 5 || "$2" != "nsxgroups.nsx.ing.com" || "$3" != "-A" || "$4" != "-o" ]]; then
      printf 'unexpected kubectl get args: %s\n' "$*" >&2
      exit 64
    fi
    case "$5" in
      jsonpath=*) ;;
      *)
        printf 'unexpected kubectl get output format: %s\n' "$5" >&2
        exit 64
        ;;
    esac
    record_call "get"
    printf 'ns-a\tgroup-a\nns-b\tgroup-b\n'
    ;;
  patch)
    if [[ "$#" -ne 8 || "$2" != "nsxgroups.nsx.ing.com" || "$4" != "--namespace" || "$6" != "--type=merge" || "$7" != "-p" ]]; then
      printf 'unexpected kubectl patch args: %s\n' "$*" >&2
      exit 64
    fi
    if [[ "$8" != '{"metadata":{"finalizers":[]}}' ]]; then
      printf 'unexpected kubectl patch payload: %s\n' "$8" >&2
      exit 64
    fi
    case "$5/$3" in
      ns-a/group-a|ns-b/group-b) ;;
      *)
        printf 'unexpected kubectl patch target namespace/name: %s/%s\n' "$5" "$3" >&2
        exit 64
        ;;
    esac
    record_call "patch $5/$3"
    ;;
  delete)
    if [[ "$#" -ne 4 || "$2" != "nsxgroups.nsx.ing.com" || "$3" != "-A" || "$4" != "--all" ]]; then
      printf 'unexpected kubectl delete args: %s\n' "$*" >&2
      exit 64
    fi
    record_call "delete"
    ;;
  *)
    printf 'unexpected kubectl command: %s\n' "$*" >&2
    exit 64
    ;;
esac
`)

	if callsPath == "" {
		t.Fatal("calls path must not be empty")
	}
}

func writeDeleteAllCRsParallel(t *testing.T, path string) {
	t.Helper()

	writeExecutable(t, path, `#!/usr/bin/env bash
set -euo pipefail

if [[ "$#" -ne 5 || "$1" != "-j" || "$2" != "20" || "$3" != "--colsep" ]]; then
  printf 'unexpected parallel args: %s\n' "$*" >&2
  exit 64
fi

recorded_colsep="$4"
if [[ "${recorded_colsep}" != $'\t' && "${recorded_colsep}" != "\\t" ]]; then
  printf 'unexpected parallel colsep: %q\n' "${recorded_colsep}" >&2
  exit 64
fi

printf 'parallel -j 20\n' >"${PARALLEL_CALLS_PATH}"
template="$5"
while IFS=$'\t' read -r namespace name; do
  if [[ -z "${namespace}" || -z "${name}" ]]; then
    continue
  fi
  command="${template//\{1\}/${namespace}}"
  command="${command//\{2\}/${name}}"
  bash -c "${command}"
done
`)
}

func assertDeleteAllCRsCalls(t *testing.T, callsPath string) {
	t.Helper()

	rawCalls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read kubectl call log: %v", err)
	}

	calls := strings.Split(strings.TrimSpace(string(rawCalls)), "\n")
	wantCalls := []string{
		"get",
		"patch ns-a/group-a",
		"patch ns-b/group-b",
		"delete",
	}
	if strings.Join(calls, "\n") != strings.Join(wantCalls, "\n") {
		t.Fatalf("kubectl calls = %#v, want %#v", calls, wantCalls)
	}
}

func linkBashIntoPath(t *testing.T, binDir string) {
	t.Helper()

	realBash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("find bash: %v", err)
	}
	err = os.Symlink(realBash, filepath.Join(binDir, "bash"))
	if err != nil {
		t.Fatalf("link bash into fake path: %v", err)
	}
}

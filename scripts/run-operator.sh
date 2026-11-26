#!/usr/bin/env bash
# Run nsx-operator on the host against the cluster of the current KUBECONFIG.
# Usage: scripts/run-operator.sh [config file]   (default: config/mock.yaml)
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
config_file="${1:-config/mock.yaml}"

if [[ -z "${KUBECONFIG:-}" ]]; then
  printf 'KUBECONFIG is not set; run scripts/k8s-up.sh and export the line it prints\n' >&2
  exit 1
fi

cd "${repo_root}"
printf 'run-operator: config=%s KUBECONFIG=%s\n' "${config_file}" "${KUBECONFIG}" >&2
exec go run ./cmd/nsx-operator -config "${config_file}"

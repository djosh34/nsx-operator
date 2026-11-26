#!/usr/bin/env bash
# Install the nsx-operator CRDs into the cluster of the current KUBECONFIG.
# With --samples, also apply config/samples/mock.yaml (NSXNetworkCloud for the local mock API).
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

apply_samples=false
case "${1:-}" in
  "") ;;
  --samples) apply_samples=true ;;
  *)
    printf 'usage: %s [--samples]\n' "$0" >&2
    exit 64
    ;;
esac

if ! command -v kubectl >/dev/null 2>&1; then
  printf 'missing required command: kubectl\n' >&2
  exit 1
fi

kubectl apply -f "${repo_root}/crds"
kubectl wait --for=condition=Established --timeout=60s \
  crd/nsxnetworkclouds.nsx.ing.com crd/nsxgroups.nsx.ing.com

if [[ "${apply_samples}" == true ]]; then
  kubectl apply -f "${repo_root}/config/samples/mock.yaml"
fi

#!/usr/bin/env bash
# Stop the local demo environment and delete its containers, volumes and kubeconfig.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

docker compose -f "${repo_root}/scripts/dev-env/compose.yaml" down -v --remove-orphans
rm -f "${repo_root}/.local/kubeconfig.yaml"
printf 'k8s-down: removed containers, volumes and .local/kubeconfig.yaml\n' >&2

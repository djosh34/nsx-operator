#!/usr/bin/env bash
# Start the local demo environment: single-node k3s and the NSX-T mock API.
# Writes a host kubeconfig to .local/kubeconfig.yaml.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
compose_file="${repo_root}/scripts/dev-env/compose.yaml"
kubeconfig="${repo_root}/.local/kubeconfig.yaml"
api_port="${K8S_API_PORT:-6443}"

log() { printf 'k8s-up: %s\n' "$*" >&2; }

for name in docker curl; do
  if ! command -v "${name}" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "${name}" >&2
    exit 1
  fi
done

log "building mock API and starting k3s (API on 127.0.0.1:${api_port}, mock API on 127.0.0.1:18080)"
K8S_API_PORT="${api_port}" docker compose -f "${compose_file}" up -d --build --wait

log "writing ${kubeconfig}"
mkdir -p "$(dirname "${kubeconfig}")"
docker compose -f "${compose_file}" exec -T k3s cat /etc/rancher/k3s/k3s.yaml \
  | sed "s#https://127.0.0.1:6443#https://127.0.0.1:${api_port}#" >"${kubeconfig}.tmp"
chmod 600 "${kubeconfig}.tmp"
mv "${kubeconfig}.tmp" "${kubeconfig}"

log "waiting for the mock API"
for _ in $(seq 1 60); do
  if curl -fsS -o /dev/null -u nsx_admin:nsx_password \
    http://127.0.0.1:18080/policy/api/v1/infra/domains/default/groups; then
    log "ready"
    printf 'export KUBECONFIG=%s\n' "${kubeconfig}"
    exit 0
  fi
  sleep 1
done
log "mock API did not answer on http://127.0.0.1:18080"
docker compose -f "${compose_file}" logs --no-color nsx-t-mockapi >&2
exit 1

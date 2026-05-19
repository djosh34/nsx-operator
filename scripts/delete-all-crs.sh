#!/usr/bin/env bash
set -euo pipefail

RESOURCE="nsxgroups.nsx.ing.com"
PATCH_PAYLOAD='{"metadata":{"finalizers":[]}}'

require_command() {
  local name="$1"
  if ! command -v "${name}" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "${name}" >&2
    exit 1
  fi
}

patch_resource() {
  local namespace="$1"
  local name="$2"

  printf 'Patching %s/%s\n' "${namespace}" "${name}"
  kubectl patch "${RESOURCE}" "${name}" \
    --namespace "${namespace}" \
    --type=merge \
    -p "${PATCH_PAYLOAD}"
}

list_resources() {
  kubectl get "${RESOURCE}" -A \
    -o jsonpath='{range .items[*]}{.metadata.namespace}{"\t"}{.metadata.name}{"\n"}{end}'
}

require_command kubectl

if command -v parallel >/dev/null 2>&1; then
  export RESOURCE PATCH_PAYLOAD
  list_resources |
    parallel -j 20 --colsep $'\t' \
      'printf "Patching %s/%s\n" "{1}" "{2}"; kubectl patch "${RESOURCE}" "{2}" --namespace "{1}" --type=merge -p "${PATCH_PAYLOAD}"'
else
  list_resources |
    while IFS=$'\t' read -r namespace name; do
      if [[ -z "${namespace}" || -z "${name}" ]]; then
        continue
      fi
      patch_resource "${namespace}" "${name}"
    done
fi

kubectl delete "${RESOURCE}" -A --all

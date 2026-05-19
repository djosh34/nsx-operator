#!/usr/bin/env bash
set -euo pipefail

require_command() {
  local name="$1"
  if ! command -v "${name}" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "${name}" >&2
    exit 1
  fi
}

require_command jq
require_command yq
require_command kubectl

jq '
{
  apiVersion: "nsx.ing.com/v1alpha",
  kind: "NSXNetworkCloud",
  metadata: { name: .name },
  spec: {
    networkCloudFQDN: .fqdn,
    networkCloudId: .id,
    name: .name
  }
}
' | yq -p=json | kubectl apply -f -

#!/usr/bin/env bash
# Create N Manage-mode NSXGroups (demo-group-1 .. demo-group-N), each with one /32 CIDR.
# Usage: scripts/make-nsxgroups.sh N
# Env:   NSX_FQDN (default 127.0.0.1:18080, the mock API from scripts/k8s-up.sh)
set -euo pipefail

if [[ $# -ne 1 || ! "$1" =~ ^[1-9][0-9]*$ ]]; then
  printf 'usage: %s N   (N >= 1)\n' "$0" >&2
  exit 1
fi
count="$1"
if (( count > 16777216 )); then
  printf 'N must be at most 16777216 (one 10.x.y.z/32 per group)\n' >&2
  exit 1
fi
fqdn="${NSX_FQDN:-127.0.0.1:18080}"

printf 'make-nsxgroups: creating %d NSXGroups on %s\n' "${count}" "${fqdn}" >&2
for ((i = 1; i <= count; i++)); do
  n=$((i - 1))
  cat <<YAML
---
apiVersion: nsx.ing.com/v1alpha
kind: NSXGroup
metadata:
  name: demo-group-${i}
spec:
  networkCloudFQDN: ${fqdn}
  groupID: demo-group-${i}
  display_name: Demo Group ${i}
  mode: Manage
  cidrs:
    - 10.$((n / 65536)).$((n / 256 % 256)).$((n % 256))/32
YAML
done | kubectl apply --server-side -f -

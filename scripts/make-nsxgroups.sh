#!/usr/bin/env bash
# Create N Manage-mode NSXGroups (demo-group-1 .. demo-group-N), each with one /32 CIDR.
# Usage: scripts/make-nsxgroups.sh N
# Env:   NSX_FQDN   (default 127.0.0.1:18080, the mock API from scripts/k8s-up.sh)
#        BATCH_SIZE (default 500, groups per kubectl apply; progress prints after each batch)
set -euo pipefail

if [[ $# -ne 1 || ! "$1" =~ ^[1-9][0-9]*$ ]]; then
  printf 'usage: %s N   (N >= 1)\n' "$0" >&2
  exit 1
fi
count="$1"
if ((count > 16777216)); then
  printf 'N must be at most 16777216 (one 10.x.y.z/32 per group)\n' >&2
  exit 1
fi
if [[ -z "${KUBECONFIG:-}" ]]; then
  printf 'KUBECONFIG is not set; run scripts/k8s-up.sh and export the line it prints\n' >&2
  exit 1
fi
fqdn="${NSX_FQDN:-127.0.0.1:18080}"
batch_size="${BATCH_SIZE:-500}"

server="$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')"
printf 'make-nsxgroups: creating %d NSXGroups on %s via %s\n' "${count}" "${fqdn}" "${server}" >&2
if ! kubectl --request-timeout=5s get --raw=/readyz >/dev/null; then
  printf 'make-nsxgroups: Kubernetes API %s is not reachable\n' "${server}" >&2
  exit 1
fi

# printf (a bash builtin) instead of a heredoc per group: no process per group.
manifest() {
  local first="$1" last="$2" i n
  for ((i = first; i <= last; i++)); do
    n=$((i - 1))
    printf -- '---\napiVersion: nsx.ing.com/v1alpha\nkind: NSXGroup\nmetadata:\n  name: demo-group-%d\nspec:\n  networkCloudFQDN: %s\n  groupID: demo-group-%d\n  display_name: Demo Group %d\n  mode: Manage\n  cidrs:\n    - 10.%d.%d.%d/32\n' \
      "${i}" "${fqdn}" "${i}" "${i}" $((n / 65536)) $((n / 256 % 256)) $((n % 256))
  done
}

for ((first = 1; first <= count; first += batch_size)); do
  last=$((first + batch_size - 1))
  if ((last > count)); then
    last="${count}"
  fi
  manifest "${first}" "${last}" | kubectl apply --server-side --request-timeout=60s -f - >/dev/null
  printf 'make-nsxgroups: applied %d/%d\n' "${last}" "${count}" >&2
done

#!/usr/bin/env bash
# Example nsx.auth.command for a real NSX-T manager.
#
#   cp scripts/nsx-auth.example.sh scripts/nsx-auth.sh   # scripts/nsx-auth.sh is gitignored
#
# The operator runs this once at startup and reads one JSON object from stdout:
#   {"username":"...","password":"..."}
# Replace the two lookups below with your secret store (pass, vault, 1password CLI, ...).
# Write diagnostics to stderr only.
set -euo pipefail

secrets_dir="${NSX_SECRETS_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.local/nsx}"
username="$(cat "${secrets_dir}/username")"
password="$(cat "${secrets_dir}/password")"

json_escape() {
  # Characters held in variables so bash 3.2 (macOS) and bash 5 escape the same way.
  # shellcheck disable=SC1003 # bs is a literal backslash
  local value="$1" bs='\' dq='"' tab=$'\t' cr=$'\r' nl=$'\n'
  value="${value//"${bs}"/${bs}${bs}}"
  value="${value//"${dq}"/${bs}${dq}}"
  value="${value//"${tab}"/${bs}t}"
  value="${value//"${cr}"/${bs}r}"
  value="${value//"${nl}"/${bs}n}"
  printf '%s' "${value}"
}

printf '{"username":"%s","password":"%s"}\n' "$(json_escape "${username}")" "$(json_escape "${password}")"

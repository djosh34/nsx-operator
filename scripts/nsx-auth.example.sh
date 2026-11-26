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

# shellcheck disable=SC1003 # single-quoted backslash is a literal backslash
json_escape() {
  # One character at a time: the same result on bash 3.2 (macOS) through 5.2+.
  local value="$1" out='' ch i
  for ((i = 0; i < ${#value}; i++)); do
    ch="${value:i:1}"
    case "${ch}" in
      '\') out+='\\' ;;
      '"') out+='\"' ;;
      $'\t') out+='\t' ;;
      $'\r') out+='\r' ;;
      $'\n') out+='\n' ;;
      *) out+="${ch}" ;;
    esac
  done
  printf '%s' "${out}"
}

printf '{"username":"%s","password":"%s"}\n' "$(json_escape "${username}")" "$(json_escape "${password}")"

#!/usr/bin/env bash
# nsx.auth.command for the local NSX-T mock API. Prints its fixed basic auth credentials.
set -euo pipefail

printf '{"username":"nsx_admin","password":"nsx_password"}\n'

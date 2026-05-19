#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
generated_dir="${repo_root}/hack/compose/generated"
cert_dir="${generated_dir}/certs"

require_command() {
  local name="$1"
  if ! command -v "${name}" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "${name}" >&2
    exit 1
  fi
}

write_kubeconfig() {
  local path="$1"
  local server="$2"
  local ca_data="$3"
  local client_cert_data="$4"
  local client_key_data="$5"

  cat >"${path}" <<EOF_KUBECONFIG
apiVersion: v1
kind: Config
clusters:
  - name: compose
    cluster:
      certificate-authority-data: ${ca_data}
      server: ${server}
contexts:
  - name: compose
    context:
      cluster: compose
      user: admin
current-context: compose
users:
  - name: admin
    user:
      client-certificate-data: ${client_cert_data}
      client-key-data: ${client_key_data}
EOF_KUBECONFIG
}

require_command openssl
require_command base64

mkdir -p "${cert_dir}"

openssl genrsa -out "${cert_dir}/ca.key" 2048
openssl req -x509 -new -nodes \
  -key "${cert_dir}/ca.key" \
  -sha256 \
  -days 3650 \
  -subj "/CN=nsx-operator-compose-ca" \
  -out "${cert_dir}/ca.crt"

openssl genrsa -out "${cert_dir}/apiserver.key" 2048
openssl req -new \
  -key "${cert_dir}/apiserver.key" \
  -subj "/CN=kube-apiserver" \
  -out "${cert_dir}/apiserver.csr"
cat >"${cert_dir}/apiserver.ext" <<'EOF_APISERVER_EXT'
subjectAltName=DNS:kube-apiserver,DNS:localhost,IP:127.0.0.1
extendedKeyUsage=serverAuth
EOF_APISERVER_EXT
openssl x509 -req \
  -in "${cert_dir}/apiserver.csr" \
  -CA "${cert_dir}/ca.crt" \
  -CAkey "${cert_dir}/ca.key" \
  -CAcreateserial \
  -out "${cert_dir}/apiserver.crt" \
  -days 3650 \
  -sha256 \
  -extfile "${cert_dir}/apiserver.ext"

openssl genrsa -out "${cert_dir}/admin.key" 2048
openssl req -new \
  -key "${cert_dir}/admin.key" \
  -subj "/CN=compose-admin/O=system:masters" \
  -out "${cert_dir}/admin.csr"
cat >"${cert_dir}/admin.ext" <<'EOF_ADMIN_EXT'
extendedKeyUsage=clientAuth
EOF_ADMIN_EXT
openssl x509 -req \
  -in "${cert_dir}/admin.csr" \
  -CA "${cert_dir}/ca.crt" \
  -CAkey "${cert_dir}/ca.key" \
  -CAcreateserial \
  -out "${cert_dir}/admin.crt" \
  -days 3650 \
  -sha256 \
  -extfile "${cert_dir}/admin.ext"

openssl genrsa -out "${cert_dir}/sa.key" 2048
openssl rsa -in "${cert_dir}/sa.key" -pubout -out "${cert_dir}/sa.pub"

ca_data="$(base64 -w 0 <"${cert_dir}/ca.crt")"
client_cert_data="$(base64 -w 0 <"${cert_dir}/admin.crt")"
client_key_data="$(base64 -w 0 <"${cert_dir}/admin.key")"

write_kubeconfig "${generated_dir}/kubeconfig-host.yaml" "https://127.0.0.1:16443" "${ca_data}" "${client_cert_data}" "${client_key_data}"
write_kubeconfig "${generated_dir}/kubeconfig-operator.yaml" "https://kube-apiserver:6443" "${ca_data}" "${client_cert_data}" "${client_key_data}"

chmod 0600 "${cert_dir}"/*.key "${generated_dir}"/kubeconfig-*.yaml
printf 'wrote generated Kubernetes assets under %s\n' "${generated_dir}"

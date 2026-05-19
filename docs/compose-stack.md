# Scratch Docker Compose Stack

This stack runs `nsx-operator` from `Dockerfile.scratch` with real process
boundaries. `kine` stores Kubernetes data in memory, `kube-apiserver` talks to
`kine`, `crd-init` installs this repository's CRDs and sample resources, the
operator talks to `kube-apiserver` through a generated kubeconfig, and NSX calls
go to the sibling `../nsx-t-mockapi` service by Compose service name.

The Compose project name is `nsx-operator-scratch`, from `compose.yaml`.

## Prerequisites

Run commands from the repository root. The local flow does not use your host
Kubernetes context.

- Docker Engine with the Docker Compose plugin available as `docker compose`.
- `kubectl` for host-side API checks.
- `openssl` and GNU `base64` for `hack/compose/generate-kube-assets.sh`.
- A sibling `../nsx-t-mockapi` checkout containing `Dockerfile` and
  `config/config.yaml`.

## Services, Ports, And Files

`compose.yaml` defines these services:

- `kine`: in-memory Kubernetes storage, no host port.
- `kube-apiserver`: Kubernetes API on container port `6443`, published to host
  port `${NSX_OPERATOR_KUBE_APISERVER_PORT:-16443}`.
- `nsx-t-mockapi`: sibling NSX-T mock API on container port `8080`, published to
  host port `${NSX_T_MOCKAPI_PORT:-18080}` and reachable in Compose as
  `nsx-t-mockapi`, `nsx-t-1`, and `nsx-t-2`.
- `crd-init`: waits for Kubernetes readiness, applies `config/crd/bases`, waits
  for `nsxnetworkclouds.nsx.ing.com` and `nsxgroups.nsx.ing.com`, then applies
  `hack/compose/manifests/sample.yaml`.
- `operator`: runs the scratch operator image with
  `hack/compose/nsx-operator-config.yaml`.

`hack/compose/generate-kube-assets.sh` writes ignored local files under
`hack/compose/generated/`, including:

- `hack/compose/generated/certs/`
- `hack/compose/generated/kubeconfig-host.yaml`
- `hack/compose/generated/kubeconfig-operator.yaml`

## Build And Start

Start from a clean stack when validating the workflow:

```bash
docker compose down --volumes --remove-orphans
rm -rf hack/compose/generated
```

Generate local Kubernetes TLS and kubeconfig artifacts:

```bash
./hack/compose/generate-kube-assets.sh
```

Check the resolved Compose configuration before building:

```bash
docker compose config
```

Build the scratch operator image and sibling mock API image:

```bash
docker compose build
```

Start the stack:

```bash
docker compose up -d
```

Inspect service state:

```bash
docker compose ps
docker compose ps -a crd-init
```

Expected state after startup:

- `kine`, `kube-apiserver`, `nsx-t-mockapi`, and `operator` are running.
- `crd-init` has exited successfully after applying CRDs and sample manifests.

## Verify Kubernetes API

Verify `kube-apiserver` from the host through the generated host kubeconfig:

```bash
kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get --raw=/readyz
```

The response should be `ok`.

Verify the sample `NSXNetworkCloud` and `NSXGroup` objects that `crd-init`
applied:

```bash
kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get nsxnetworkclouds.nsx.ing.com
kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get nsxgroups.nsx.ing.com
```

Expected resources include `mockapi` and `compose-managed-web`.

## Verify NSX-T Mock API

Verify the mock API by Compose service name from inside the Compose network:

```bash
docker compose run --rm --entrypoint curl crd-init \
  -fsS -u nsx_admin:nsx_password \
  http://nsx-t-mockapi:8080/policy/api/v1/infra/domains/default/groups
```

Verify the same endpoint from the host through the published default port:

```bash
curl -fsS -u nsx_admin:nsx_password \
  http://127.0.0.1:18080/policy/api/v1/infra/domains/default/groups
```

If you override `NSX_T_MOCKAPI_PORT`, use that port instead of `18080`.
The host response should be successful JSON; it may list a different resource
set than the in-network `nsx-t-mockapi` call because the mock API keeps manager
state by requested host.

## Inspect Operator Logs

Inspect operator logs:

```bash
docker compose logs --no-color operator
```

The operator uses zap structured JSONL logging on stderr. Docker merges stdout
and stderr in `docker compose logs`, so each operator log entry should appear as
a JSON object after Docker's service prefix. Useful successful log messages
include `loaded startup config`, `constructed nsx manager client`, and
`completed default manager sweep`.

Use `-f` while debugging:

```bash
docker compose logs --no-color -f operator
```

## Cleanup

Stop and remove containers, networks, and volumes:

```bash
docker compose down --volumes --remove-orphans
```

Remove generated local Kubernetes assets:

```bash
rm -rf hack/compose/generated
```

`hack/compose/generated/` is ignored by Git and can be regenerated at any time.

## Troubleshooting

Missing sibling mock API checkout:

```text
unable to prepare context: path "../nsx-t-mockapi" not found
```

Clone or restore the sibling checkout so `../nsx-t-mockapi/Dockerfile` and
`../nsx-t-mockapi/config/config.yaml` exist.

Port conflicts:

```text
Bind for 0.0.0.0:16443 failed: port is already allocated
Bind for 0.0.0.0:18080 failed: port is already allocated
```

Use alternate host ports:

```bash
NSX_OPERATOR_KUBE_APISERVER_PORT=26443 NSX_T_MOCKAPI_PORT=28080 docker compose up -d
```

If you change `NSX_OPERATOR_KUBE_APISERVER_PORT`, the generated host kubeconfig
still points at `https://127.0.0.1:16443`. Either edit the `server:` value in
`hack/compose/generated/kubeconfig-host.yaml` or override it per command:

```bash
kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml \
  --server=https://127.0.0.1:26443 get --raw=/readyz
```

Unhealthy or exited services:

```bash
docker compose ps
docker compose logs --no-color crd-init
docker compose logs --no-color kube-apiserver
docker compose logs --no-color operator
```

`crd-init` should exit with code `0`. If it fails, the operator will not start
because it depends on `crd-init` completing successfully.

Stale generated config or certificates:

```bash
docker compose down --volumes --remove-orphans
rm -rf hack/compose/generated
./hack/compose/generate-kube-assets.sh
docker compose up -d
```

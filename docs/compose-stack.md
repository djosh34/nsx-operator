# Scratch Docker Compose Stack

This stack runs `nsx-operator` from `Dockerfile.scratch` with real process boundaries:
kine stores Kubernetes data in memory, kube-apiserver talks to kine, the operator talks
to kube-apiserver through a generated kubeconfig, and NSX calls go to the sibling
`../nsx-t-mockapi` service by Compose service name.

## Build and Start

Generate local Kubernetes TLS and kubeconfig artifacts:

```bash
./hack/compose/generate-kube-assets.sh
```

Build the scratch operator image and the sibling mock API image:

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
```

## Verification

Verify the kube-apiserver from the host without using a host Kubernetes context:

```bash
kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get --raw=/readyz
kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get nsxnetworkclouds.nsx.ing.com
kubectl --kubeconfig hack/compose/generated/kubeconfig-host.yaml get nsxgroups.nsx.ing.com
```

Verify the NSX-T mock API by Compose service name from inside the Compose network:

```bash
docker compose run --rm --entrypoint curl crd-init \
  -fsS -u nsx_admin:nsx_password \
  http://nsx-t-mockapi:8080/policy/api/v1/infra/domains/default/groups
```

Verify operator zap JSONL logs on stderr through Docker logs:

```bash
docker compose logs --no-color operator
```

Useful successful log lines include `loaded startup config`, `constructed nsx manager client`,
and `default manager gather completed`. Docker merges stdout and stderr in `logs`; the operator
itself writes zap JSON logs to stderr.

## Cleanup

Stop and remove containers and volumes:

```bash
docker compose down --volumes --remove-orphans
```

Generated files under `hack/compose/generated/` are ignored and can be removed at any time:

```bash
rm -rf hack/compose/generated
```

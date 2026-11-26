# nsx-operator

A Kubernetes operator that keeps NSX-T Policy Groups (domain `default`) in sync
with Kubernetes custom resources, across one or more NSX-T managers.

Two CRDs (in `crds/`):

- `NSXNetworkCloud`: one NSX-T manager, by FQDN (`host` or `host:port`).
- `NSXGroup`: one NSX Policy Group on that manager: `groupID`, `display_name`,
  `cidrs` and optional `segment_paths`.

Each `NSXGroup` has a mode:

- **Observe**: Kubernetes mirrors NSX. The operator never writes to NSX. Every
  NSX group it finds shows up as an `Observe` NSXGroup; when the group goes
  away in NSX, the CR goes too.
- **Manage**: the CR is the truth. The operator creates or patches the NSX
  group to match it, and repairs drift or deletion in NSX.

The operator sweeps every manager each `tickInterval` and reports progress in
the CR status conditions. The full design is in [docs/spec.md](docs/spec.md).

## Run it

Needs Docker (Linux, or Docker Desktop on Mac), `kubectl`, `curl` and Go.

```bash
# 1. single-node k3s (embedded etcd) on 127.0.0.1:6443 + NSX-T mock API on 127.0.0.1:18080
scripts/k8s-up.sh
export KUBECONFIG=$PWD/.local/kubeconfig.yaml   # k8s-up.sh prints this line

# 2. CRDs, plus an NSXNetworkCloud for the mock API
scripts/apply-crds.sh --samples

# 3. operator on the host, against $KUBECONFIG (leave it running in its own terminal)
scripts/run-operator.sh config/mock.yaml
```

The first `k8s-up.sh` builds the mock API image from
<https://github.com/djosh34/nsx-t-mockapi> for your machine's architecture,
which takes a minute. Set `K8S_API_PORT` if 6443 is taken.

Any other cluster works too: still run step 1 for the mock API, then point
`KUBECONFIG` at your cluster instead of the k3s one.

## Demo

Run these in a second terminal from the repo root, with the same `KUBECONFIG`.

The mock API cloud is there and reachable:

```bash
kubectl get nsxnetworkclouds
```

Create a managed group with two CIDRs:

```bash
kubectl apply -f - <<'EOF'
apiVersion: nsx.ing.com/v1alpha
kind: NSXGroup
metadata:
  name: demo-web
spec:
  networkCloudFQDN: 127.0.0.1:18080
  groupID: demo-web
  display_name: Demo Web
  mode: Manage
  cidrs:
    - 10.10.0.0/24
    - 10.20.0.5
EOF
kubectl wait --for=condition=Synced nsxgroup/demo-web --timeout=60s
kubectl get nsxgroups
```

The group is now in NSX:

```bash
curl -s -u nsx_admin:nsx_password \
  http://127.0.0.1:18080/policy/api/v1/infra/domains/default/groups/demo-web
```

Change it in NSX behind the operator's back, then watch it change back:

```bash
curl -s -X PATCH -u nsx_admin:nsx_password -H 'Content-Type: application/json' \
  -d '{"display_name":"Changed In NSX"}' \
  http://127.0.0.1:18080/policy/api/v1/infra/domains/default/groups/demo-web
sleep 10
curl -s -u nsx_admin:nsx_password \
  http://127.0.0.1:18080/policy/api/v1/infra/domains/default/groups/demo-web
```

Observe mode: a group created directly in NSX shows up in Kubernetes on its own:

```bash
curl -s -X PATCH -u nsx_admin:nsx_password -H 'Content-Type: application/json' \
  -d '{"display_name":"Made In NSX"}' \
  http://127.0.0.1:18080/policy/api/v1/infra/domains/default/groups/made-in-nsx
sleep 10
kubectl get nsxgroups
```

Clean up: stop the operator with Ctrl-C, then remove k3s, the mock API, their
volumes and the kubeconfig:

```bash
scripts/k8s-down.sh
```

## Real NSX-T

The operator talks HTTPS with basic auth to every `NSXNetworkCloud` FQDN.
Credentials come from `nsx.auth.command`: an executable the operator runs once
at startup, which prints `{"username":"...","password":"..."}` on stdout.

```bash
cp config/real-nsx.example.yaml config/real-nsx.yaml   # gitignored
cp scripts/nsx-auth.example.sh scripts/nsx-auth.sh     # gitignored
```

1. Edit `scripts/nsx-auth.sh` to fetch the credentials from your secret store.
   As shipped, it reads `.local/nsx/username` and `.local/nsx/password`
   (gitignored).
2. If the manager's certificate comes from a private CA, set
   `nsx.tls.caBundleFile` to the PEM bundle. For a lab with a self-signed
   certificate you can set `nsx.tls.insecureSkipVerify: true` instead.
3. Put your manager's FQDN in `config/samples/real-nsx.example.yaml` and apply it.
4. Run the operator with the real config:

```bash
scripts/apply-crds.sh
kubectl apply -f config/samples/real-nsx.example.yaml
scripts/run-operator.sh config/real-nsx.yaml
kubectl get nsxnetworkclouds,nsxgroups
```

The example config starts read-only: `writesEnabled: false` globally and on the
`NSXNetworkCloud`. Groups found in NSX show up as `Observe` NSXGroups, and no
call other than GET reaches NSX. Turn writes on (both places) only once you
want `Manage` groups to change NSX.

## Development

```bash
make check   # fmt, vet, lint, tests, race, contract, e2e, chaos, coverage
```

Contract tests expect the mock API checked out at `../nsx-t-mockapi`.

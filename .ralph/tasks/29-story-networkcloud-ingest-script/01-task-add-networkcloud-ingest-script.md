## Task: Add NetworkCloud Ingest Script <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add a simple stdin-to-kubectl helper script for ingesting a NetworkCloud JSON object into an `NSXNetworkCloud` custom resource.

The requested script behavior is:

```bash
#!/bin/bash

set -euo pipefail

cat | jq '

{
  apiVersion: "nsx.ing.com/v1alpha",
  kind: "NSXNetworkCloud",
  metadata: { name: .name },
  spec: {
    networkCloudFQDN: .fqdn,
    networkCloudId: .id,
    name: .name,
  }
}

' | yq -p=json | kubectl apply -f -
```

Keep this script simple. It should read a single JSON object from stdin with `name`, `fqdn`, and `id`, transform it into the project-supported `NSXNetworkCloud` resource, convert through `yq`, and apply it with `kubectl apply -f -`. Use the correct API version, kind, and field names from the repo if they differ from the requested snippet, and record any differences in verification evidence.

In scope: add an executable script under the repo scripts directory; validate required tool assumptions only as much as needed for helpful errors; preserve simple pipe behavior; add a smoke test or fixture test that proves the generated manifest is correct.

Out of scope: bulk import formats; interactive prompts; replacing `jq`, `yq`, or `kubectl`; adding a full NetworkCloud management CLI.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] An executable NetworkCloud ingest script exists under the repo's scripts directory.
- [ ] The script reads JSON from stdin.
- [ ] The script maps `.name` to `metadata.name` and `spec.name`.
- [ ] The script maps `.fqdn` to `spec.networkCloudFQDN`.
- [ ] The script maps `.id` to `spec.networkCloudId`.
- [ ] The script emits or applies a valid `NSXNetworkCloud` manifest using the repo's actual API version and kind.
- [ ] The script pipes the manifest to `kubectl apply -f -`.
- [ ] Verification proves the generated manifest shape and an apply path against a test or live cluster.
</acceptance_criteria>

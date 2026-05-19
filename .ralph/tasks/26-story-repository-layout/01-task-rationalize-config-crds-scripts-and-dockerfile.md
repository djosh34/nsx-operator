## Task: Rationalize Config CRDs Scripts And Dockerfile Layout <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Make repository directories and file names more sensible. Move configuration material to `./config`, move CRD manifests that currently live under paths such as `./config/crds/base/...` to `./crds`, move scripts to `./scripts`, and rename `dockerfile.scratch` so the Dockerfile is named `Dockerfile`.

The task must update all references that are affected by the moves, including Makefile targets, documentation, compose files, CI workflows, tests, scripts, manifests, and any embedded paths. The resulting layout must be easy to understand from the repo root: operator config under `config`, CRDs under `crds`, executable helper scripts under `scripts`, and the scratch Dockerfile available as `Dockerfile`.

In scope: move files/directories; update references; preserve executable bits for scripts; update tests and docs that mention old paths; verify build/test commands still work after the move.

Out of scope: changing CRD schema content; changing runtime behavior unrelated to path references; broad repository cleanup beyond the requested path moves.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Config files are located under `./config` in the final layout.
- [ ] CRD manifests previously under `./config/crds/base/...` or equivalent nested config CRD paths are located under `./crds`.
- [ ] Repository helper scripts are located under `./scripts` and remain executable where applicable.
- [ ] `dockerfile.scratch` is renamed or replaced so the intended Dockerfile path is `Dockerfile`.
- [ ] All Makefile, CI, compose, docs, tests, and script references are updated to the new paths.
- [ ] There are no stale references to the old paths except in intentional historical notes or task evidence.
- [ ] Build, test, CRD install/apply, and Docker build verification commands pass with the new layout.
</acceptance_criteria>

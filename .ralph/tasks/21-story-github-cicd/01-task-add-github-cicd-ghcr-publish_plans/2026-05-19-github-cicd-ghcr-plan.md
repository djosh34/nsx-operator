# Plan: GitHub CI/CD GHCR Publish

## Task

Add a GitHub Actions workflow that runs the Docker image build and Go tests as independent jobs, then publishes the operator image to GitHub Container Registry only after both jobs succeed. Record concrete manual GitHub Actions and GHCR evidence before marking the task complete.

## Current Context

- Task file: `.ralph/tasks/21-story-github-cicd/01-task-add-github-cicd-ghcr-publish.md`.
- Repository remote: `https://github.com/djosh34/nsx-operator.git`.
- Current branch: `master`.
- No `.github/workflows/` workflow exists yet.
- Image build entry point: `Dockerfile.scratch`.
- Existing test entry point: `make test`, which uses `setup-envtest` and runs `go test ./...`.
- Required final local checks remain `make check`, `make test`, and `make test-coverage`.
- This is workflow/infrastructure work, so the TDD exception applies: do not add brittle tests that assert YAML text. Verify behavior by executing the commands the workflow will run and then by triggering the workflow in GitHub.

## Public CI Interface

- Add `.github/workflows/ci-cd.yml`.
- Trigger on:
  - `push` to `master`.
  - `pull_request` to `master`, for build/test verification without package publication.
  - `workflow_dispatch`, for manual verification.
- Use workflow-level permissions with `contents: read`, publish-job permissions with `packages: write`, and an `if` guard so package writes are only attempted for non-PR runs.
- Publish image name `ghcr.io/${{ github.repository_owner }}/nsx-operator`, matching this repository and keeping the package path predictable.
- Use traceable tags:
  - `sha-${{ github.sha }}` for every publish run.
  - `latest` only for pushes or manual runs on `refs/heads/master`.
  - `ref`/semver tag metadata if Git tags are later used, without blocking this task on a release convention that does not exist yet.

## Boundary Plan

Use `$improve-code-boundaries` to keep the workflow clear rather than muddy:

- Keep workflow behavior in one CI/CD workflow file instead of scattering helper scripts unless a command becomes genuinely complex.
- Keep job responsibilities deep and narrow:
  - `docker-build` proves the image builds and uploads the built image as a temporary artifact.
  - `go-test` proves the repository's Go tests pass through `make test`.
  - `publish-ghcr` only logs in, loads the already-built image artifact, tags it, and pushes it.
- Avoid duplicating Dockerfile/image/tag computation in multiple jobs by using workflow `env` and `docker/metadata-action` outputs where practical.
- Do not change application code, Dockerfile behavior, local Compose workflows, or Makefile targets unless execution proves an existing entry point is broken.
- Keep debug output useful through explicit step names and shell tracing for short CI shell blocks; handle shell errors with `set -euo pipefail`.

## TDD / Verification Plan

Use `$tdd` as vertical executable slices, but apply the non-code exception:

1. RED equivalent: run the existing image build command before adding the workflow:
   - `docker build -f Dockerfile.scratch -t nsx-operator:ci-local .`
   - This proves the public build command currently succeeds or exposes the first blocker.
2. GREEN equivalent: add the `docker-build` job using the same Dockerfile and verify locally with the same Docker command.
3. RED equivalent: run the existing Go test command:
   - `make test`
   - This proves the public test command currently succeeds or exposes the first blocker.
4. GREEN equivalent: add the `go-test` job using `make test`.
5. Add the `publish-ghcr` job with `needs: [docker-build, go-test]`, guarded so it cannot run unless both dependencies succeeded and the event is publish-eligible.
6. Verify locally again with the final workflow-equivalent commands:
   - `docker build -f Dockerfile.scratch -t nsx-operator:ci-local .`
   - `make test`
   - `make check`
   - `make test-coverage`
7. Push and manually trigger or observe the GitHub Actions run.
8. Record concrete evidence in the task file:
   - workflow run URL;
   - job statuses for `docker-build`, `go-test`, and `publish-ghcr`;
   - proof that `publish-ghcr` declares and observed both dependencies;
   - GHCR package/version evidence showing the `sha-...` tag was published.

## Implementation Steps

- Add `.github/workflows/ci-cd.yml` with three jobs:
  - `docker-build`:
    - checkout repository;
    - set up Docker Buildx;
    - build `Dockerfile.scratch`;
    - export the image to a local tar artifact for the publish job;
    - upload the artifact with a short retention period.
  - `go-test`:
    - checkout repository;
    - checkout the sibling `../nsx-t-mockapi` repository required by the existing contract tests;
    - set up Go from `go.mod`;
    - cache Go build/module data through the setup action;
    - run `make test`.
  - `publish-ghcr`:
    - `needs: [docker-build, go-test]`;
    - `if` excludes pull requests;
    - set up Docker Buildx if needed for image tooling;
    - download the image artifact;
    - load the image;
    - log in to `ghcr.io` with `GITHUB_TOKEN`;
    - apply metadata tags;
    - push all generated tags.
- Keep shell blocks short and explicit:
  - use `set -euo pipefail`;
  - print meaningful job context;
  - do not ignore command errors.
- If GitHub Actions cannot publish packages because repository/package settings block `GITHUB_TOKEN`, do not mark complete; record the blocker and fix repository permissions or workflow settings before rerunning.

## Manual Verification Evidence To Collect

- Local:
  - successful `docker build -f Dockerfile.scratch -t nsx-operator:ci-local .`;
  - successful `make test`;
  - successful `make check`;
  - successful `make test-coverage` with total coverage at or above 80%.
- GitHub:
  - successful Actions run URL after the workflow is present on `master` or manually dispatched;
  - `docker-build` job success;
  - `go-test` job success;
  - `publish-ghcr` job success and visible dependency on both upstream jobs;
  - GHCR package/version page or API/CLI output proving `ghcr.io/djosh34/nsx-operator:sha-<commit>` exists.

## Required Final Checks

- [x] Local Docker image build command succeeds.
- [x] `make test` succeeds.
- [x] `make check` succeeds.
- [x] `make test-coverage` succeeds and reports at least 80% coverage.
- [x] GitHub Actions workflow succeeds end to end.
- [x] GHCR image publication is proven with concrete evidence.
- [x] Final `$improve-code-boundaries` review confirms the workflow has clear job boundaries, no duplicated command soup, and no unrelated application/local workflow changes.
- [x] Update task acceptance criteria and evidence.
- [x] Set `<passes>true</passes>`.
- [x] Run `/bin/bash .ralph/task_switch.sh`.
- [ ] Commit all files with `task finished 01-task-add-github-cicd-ghcr-publish: add github ci cd ghcr publish workflow`.
- [ ] Push.

EXECUTED

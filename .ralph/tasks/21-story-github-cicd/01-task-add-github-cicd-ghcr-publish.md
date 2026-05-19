## Task: Add GitHub CI/CD Workflow for Parallel Build, Test, and GHCR Publish <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add a GitHub Actions CI/CD workflow for this repository that runs the Docker image build and Go test suite in parallel, waits for both jobs to complete successfully, and only then publishes the image to GitHub Container Registry.

The higher order goal is to make every change verifiable through automated GitHub CI while keeping image publication gated on both core checks: the container image must build successfully and the Go tests must pass before anything is pushed to GHCR.

In scope: create or update the GitHub Actions workflow under `.github/workflows/`; configure one job that builds the Docker image; configure a separate job that runs Go tests; make those jobs run in parallel where GitHub Actions scheduling allows; configure a publish job that depends on both the image build job and the Go test job; publish the image to GHCR only after both dependencies succeed; use appropriate GitHub Actions permissions for `contents` and `packages`; tag the GHCR image in a practical, traceable way such as commit SHA and any existing release/tag convention if present; handle errors explicitly in shell scripts or workflow commands; and keep workflow logging useful enough to diagnose failures.

The workflow must respect this repo's existing build and test commands rather than inventing unrelated entry points. If the repository already has a Dockerfile, Makefile targets, or test scripts, use those existing commands unless there is a documented reason to add or adjust them. Go tests must not be skipped. Docker build failures must fail the workflow. GHCR publish must not run if either the Docker build or Go tests fail.

Manual verification is mandatory: after implementation, trigger the workflow in GitHub and record evidence that the workflow ran successfully end to end, including proof that the Docker build job succeeded, the Go test job succeeded, the publish job waited for both jobs, and the image was published to GHCR. Evidence can include GitHub Actions run URLs, job logs, screenshots, GHCR package/version page details, or equivalent concrete proof.

Out of scope: changing application behavior, weakening or removing existing tests, bypassing failing checks, publishing to registries other than GHCR unless separately requested, and replacing local development workflows unrelated to GitHub CI/CD.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the GitHub CI/CD workflow works and succeeds.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] A GitHub Actions workflow exists under `.github/workflows/` for CI/CD.
- [ ] The workflow has a Docker image build job that fails the workflow if the image cannot be built.
- [ ] The workflow has a Go test job that runs the repository's Go tests and fails the workflow if tests fail.
- [ ] The Docker image build job and Go test job are configured as independent jobs so they can run in parallel.
- [ ] The workflow has a GHCR publish job that declares dependencies on both the Docker image build job and the Go test job.
- [ ] The GHCR publish job runs only after both the Docker build and Go tests have succeeded.
- [ ] The GHCR publish job pushes the image to GitHub Container Registry with traceable tags such as commit SHA and any existing release/tag convention if applicable.
- [ ] GitHub Actions permissions are configured narrowly enough to publish packages while allowing checkout and normal workflow execution.
- [ ] Manual verification evidence includes a successful GitHub Actions workflow run where the Docker build job, Go test job, and GHCR publish job all completed successfully.
- [ ] Manual verification evidence includes proof that the GHCR image/package was actually published.
</acceptance_criteria>

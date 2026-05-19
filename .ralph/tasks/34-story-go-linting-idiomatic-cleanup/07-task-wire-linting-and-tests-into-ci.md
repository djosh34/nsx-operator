## Task: Wire Strict Linting And Tests Into CI <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.


**Goal:** Make CI fail on the full strict linting and testing contract for this story. CI must run `golangci-lint run ./...`, the custom pointer-receiver analyzer, the custom struct-return analyzer, `go test ./...`, and `go test -race ./...`. The repository should have local make targets or documented commands that match CI so developers can reproduce failures before pushing.

Update the existing CI workflow, make targets, scripts, or task runner configuration in the style already used by the repository. If the existing project has a `Makefile`, add or update targets for linting, custom linting, full tests, race tests, and an aggregate verification target. If GitHub Actions or another CI workflow exists, wire these same commands into the pipeline. Commands must fail fast enough to be useful but still record enough output to diagnose failures.

CI enforcement must include the two custom project rules: no value receivers anywhere, and no function returning `(Struct, error)` where `(*Struct, error)` is required. It must also enforce disciplined `nolint` comments, no unchecked errors, no `err` shadowing, no inline `err` declarations, nil/error correctness, pointer/copy safety, interface hygiene, and the enabled formatters. The final PR checklist must confirm: all methods use pointer receivers, no value or mixed receivers remain, all interface assertions use `(*Type)(nil)`, no `err` shadowing remains, no inline `err` declarations remain, failing constructors/factories return `(*T, error)`, error paths return `nil, err` where applicable, no ambiguous `nil, nil` remains, no copied locks remain, no large avoidable struct copies remain, all `nolint` comments are specific and explained, `golangci-lint run ./...` passes, `go test ./...` passes, and `go test -race ./...` passes.

Manual verification must include running the same commands locally and recording their outputs. If `go test -race ./...` is too slow or exposes an unrelated existing flake, the blocker must be recorded with exact package names, error output, and a follow-up task; do not mark this task passing without a concrete resolution or explicit documented exception.


</description>


<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] CI runs `golangci-lint run ./...`.
- [ ] CI runs the custom no-value-receiver analyzer.
- [ ] CI runs the custom no-`(Struct, error)` analyzer.
- [ ] CI runs `go test ./...`.
- [ ] CI runs `go test -race ./...`.
- [ ] Local make targets or documented commands reproduce the same lint and test checks as CI.
- [ ] The final checklist from the story description is recorded with concrete command output.
- [ ] CI configuration changes are tested locally where possible and the workflow command or validation output is recorded.
</acceptance_criteria>

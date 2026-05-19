## Task: Add Env Script Credentials Flag <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Add a simple `env-script` flag that lets the operator load NSX credentials from an executable script. The flag must be passed through normal CLI/config loading, inspect the target script's shebang (`#!`) to determine which script executor/interpreter to invoke, execute it with that interpreter, then extract only the needed environment variables for `NSX_USERNAME` and `NSX_PASSWORD`.

Keep the implementation stupid simple. Do not introduce a broad secret provider framework, templating system, shell-specific parser, or unrelated credential refactor. The implementation must read the `#!` line at the top of the file and use that shebang to determine what script executor to invoke. The operator must capture the script output in the simplest reliable format already used by the project or a minimal `KEY=VALUE` environment-style output, and must fail clearly if either `NSX_USERNAME` or `NSX_PASSWORD` is missing.

In scope: add the `env-script` flag; pass it through to config/credential loading; parse the script's shebang and execute the script with the indicated interpreter; extract `NSX_USERNAME` and `NSX_PASSWORD`; log larger actions at info and detailed behavior at debug using zap structured JSONL to stderr; add tests for success, missing variables, non-zero script exit, missing/invalid shebang, and shebang interpreter selection.

Out of scope: managing Kubernetes Secrets; adding encryption; changing credential precedence except as needed to make the new flag deterministic and documented in tests; supporting arbitrary shell snippets instead of an executable script path.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] A new `env-script` flag can be passed to the operator command.
- [ ] The flag value is passed into config or credential loading without bypassing existing validation.
- [ ] The configured script's `#!` shebang is read and used to choose the script executor/interpreter.
- [ ] The implementation does not hardcode `bash script` or assume every env script is Bash.
- [ ] `NSX_USERNAME` and `NSX_PASSWORD` are extracted from the script output or exported environment data.
- [ ] Missing `NSX_USERNAME` or `NSX_PASSWORD` produces a clear error and does not continue with empty credentials.
- [ ] Non-zero script exit and execution failures return errors that are logged and surfaced.
- [ ] Tests cover the happy path, missing username, missing password, script execution failure, missing/invalid shebang, and shebang interpreter selection.
- [ ] Relevant existing test gates pass and no errors are intentionally ignored.
</acceptance_criteria>

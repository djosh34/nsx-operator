## Task: Print Help For Bad Or Missing Flags <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Make CLI startup errors more helpful when users pass wrong flags or omit required flags. On bad or missing flag errors, the command must say how to use it by always printing the command help output.

The desired behavior is intentionally simple: when flag parsing or required-flag validation fails, surface the actual error and print the same help text a user would get from the `help` command. Do not replace the existing CLI library or create a custom documentation system. Keep stderr/stdout behavior consistent with the CLI framework already in use.

In scope: detect wrong flag values, unknown flags, and missing required flags; print command help on those failures; preserve non-zero exit behavior; keep useful structured zap logging for startup/debug details where the app logger is available; add tests or command-level verification for each failure mode.

Out of scope: redesigning command names; adding interactive prompts; changing successful command output; suppressing the underlying parse/validation error.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Unknown flags return a non-zero error and print command help.
- [ ] Wrongly formatted or invalid flag values return a non-zero error and print command help.
- [ ] Missing required flags return a non-zero error and print command help.
- [ ] The original error message remains visible alongside the help text.
- [ ] Successful invocations do not print help unexpectedly.
- [ ] Tests or scripted verification capture stderr/stdout and exit status for the relevant failure cases.
- [ ] Relevant existing test gates pass and no errors are intentionally ignored.
</acceptance_criteria>

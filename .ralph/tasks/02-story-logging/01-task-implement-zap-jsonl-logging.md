## Task: Implement Zap JSONL Logging <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/logging` and wire the operator to use zap structured logging in JSON lines to stderr. Debug logging should be available for detailed actions, while larger lifecycle actions should log at info. Logs must use structured fields instead of formatted plaintext context.

In scope: create a logger constructor honoring validated `logging.level`; emit JSONL to stderr; provide helpers or conventions for required fields `component`, `networkCloudFQDN`, `groupID`, `sweepID`, and `reconcileKey`; scrub or avoid plaintext credentials and credential file contents; migrate operator/client logs to zap where touched. Out of scope: external log aggregation.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Tests or captured process output prove logs are valid JSONL on stderr.
- [ ] Verification proves credentials and credential file contents are not present in logs.
</acceptance_criteria>

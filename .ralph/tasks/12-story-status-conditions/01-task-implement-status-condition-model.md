## Task: Implement Status Condition Model <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement status updates using conditions only for both CRDs. There must be no top-level synced field, remote object field, revision field, fingerprint field, or last sweep timestamp on `NSXGroup`.

In scope: condition helpers for `Reachable`, `Swept`, `RemotePresent`, `SpecMatchesRemote`, `UnsupportedExpression`, `Realized`, `Synced`, `Applying`, and `Deleting`; correct True/False/Unknown behavior from the design; observedGeneration handling; lastTransitionTime update only on transitions; descriptive reason/message; synced derivation based on RemotePresent=True, SpecMatchesRemote=True, UnsupportedExpression=False, and Realized=True. Out of scope: business logic encoded in reason/message strings.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Tests prove each condition status rule, including Unknown cases.
- [ ] CRD/object verification proves statuses contain conditions only.
</acceptance_criteria>

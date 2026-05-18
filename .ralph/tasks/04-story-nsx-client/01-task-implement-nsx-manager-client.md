## Task: Implement Typed NSX Manager API Client <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Implement `internal/nsxclient` as a concurrent-safe typed client for the NSX-T 4.2.3 APIs used by this operator and the parent-dir `nsx-t-mockapi`. The group APIs must hide the domain ID and always use hard-coded NSX domain `default`.

In scope: `NewClient` options for base URL, shared HTTP client, Basic Auth, and zap logger; JSON request/response behavior; Basic Auth on every request; closed response bodies; typed group methods for list/read/patch/delete group, IP address expression patch/delete, and path expression patch/delete; generic `DecodeListResults[T any]` using `json.Decoder` directly on streams to return `[]*T`, `cursor`, and `resultCount`; pagination of all list calls until cursor is empty; typed status errors for 409, 412, 429, and 503; route inventory from parent `nsx-t-mockapi`; typed methods and contract tests for every mockapi route family listed in the design. Out of scope: automatic retry loops, automatic refetch/reapply after 409/412, automatic retry after 429/503, and caller-owned reconciliation logic.

Method families to cover from mockapi/OpenAPI: firewall sections/rules/stats, ip-sets, policy groups, group IP-address expressions, group path expressions, group members, security policies/rules/stats, segments, segment state/statistics, tier-0 list, tier-1 list/read/state/segments, and EULA acceptance.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Contract tests exercise every typed client method against `nsx-t-mockapi`.
- [ ] CI or a test fails when route inventory and typed client coverage diverge.
- [ ] Tests prove list pagination, stream decoding, Basic Auth, response body closure, and typed error mapping.
</acceptance_criteria>

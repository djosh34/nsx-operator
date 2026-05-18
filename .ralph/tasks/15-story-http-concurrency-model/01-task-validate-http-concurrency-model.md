## Task: Validate HTTP Concurrency Model Across Manager Goroutines <status>not_started</status> <passes>false</passes>

<description>
Must be manually verified with concrete evidence that it works.

**Goal:** Wire and verify HTTP concurrency so manager goroutines are independent while sharing the same global per host+port limiter registry through the shared rate-limited transport.

In scope: all NSX clients share the same rate-limited transport; two clients targeting `https://nsx-a.example.net` share the `nsx-a.example.net:443` bucket; `https://nsx-a.example.net` and `https://nsx-a.example.net:8443` use different buckets; different NSX managers do not block each other unless they target the same effective host+port; no request queues, worker pools, or manager queues are introduced. Out of scope: route-specific throttling.

</description>

<acceptance_criteria>
- [ ] Manual verification was performed with concrete calls, commands, logs, screenshots, external service status, or other evidence proving the feature/functionality/task works.
- [ ] The verification evidence is recorded in the task or linked artifact.
- [ ] Completion is not based only on a shallow checkbox, assumption, or code inspection.
- [ ] Race tests prove shared limiter behavior across concurrent manager goroutines.
- [ ] E2E or integration logs prove same-host sharing and different-host/different-port isolation.
</acceptance_criteria>

package nsxclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/djosh34/nsx-operator/internal/httpratelimit"
	"github.com/djosh34/nsx-operator/internal/testsupport/mockapi"
	"go.uber.org/zap"
)

const (
	mockAPIUsername = mockapi.Username
	mockAPIPassword = mockapi.Password
)

type routeCoverage struct {
	name   string
	method string
}

func TestMockAPIRouteInventoryIsSupportedAndContracted(t *testing.T) {
	t.Parallel()

	inventory := readMockAPIRouteInventory(t)
	supported := supportedRouteCoverage()
	contracted := contractedRouteNames()

	supportedNames := map[string]string{}
	for _, route := range supported {
		if previousMethod, exists := supportedNames[route.name]; exists {
			t.Fatalf("duplicate supported route %q for methods %s and %s", route.name, previousMethod, route.method)
		}
		supportedNames[route.name] = route.method
	}

	for _, routeName := range inventory {
		if _, ok := supportedNames[routeName]; !ok {
			t.Fatalf("mockapi route %q is not in nsxclient supported route coverage", routeName)
		}
		if !contracted[routeName] {
			t.Fatalf("mockapi route %q has no nsxclient contract test case", routeName)
		}
	}
	for routeName := range supportedNames {
		if !inventoryContains(inventory, routeName) {
			t.Fatalf("nsxclient supports route %q but mockapi inventory does not contain it", routeName)
		}
	}
}

func TestTypedClientContractsAgainstMockAPI(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	mock := mockapi.Start(t, ctx)
	client, err := NewClient(Options{
		BaseURL:    mock.BaseURL(),
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		Username:   mockAPIUsername,
		Password:   mockAPIPassword,
		Logger:     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	covered := map[string]bool{}
	cover := func(routeName string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s contract call failed: %v\nmockapi logs:\n%s", routeName, err, mock.Logs())
		}
		covered[routeName] = true
	}
	coverNotFound := func(routeName string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s contract call unexpectedly succeeded", routeName)
		}
		var statusErr *StatusError
		if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusNotFound {
			t.Fatalf("%s error = %T %[2]v, want 404 StatusError", routeName, err)
		}
		covered[routeName] = true
	}

	_, err = client.GetEULAAcceptance(ctx)
	cover("policy.eula.acceptance", err)

	group := &Group{Resource: Resource{ID: "web", DisplayName: "Web", ResourceType: "Group"}}
	_, err = client.PutGroup(ctx, "web", group)
	cover("policy.groups.put", err)
	cover("policy.groups.patch", client.PatchGroup(ctx, "web", &GroupPatch{DisplayName: "Web Patched", ResourceType: "Group"}))
	_, err = client.GetGroup(ctx, "web")
	cover("policy.groups.get", err)
	_, err = client.ListGroups(ctx)
	cover("policy.groups.list", err)
	_, err = client.GetGlobalConsolidatedEffectiveIPAddresses(ctx, "web")
	cover("policy.global.groups.consolidated_effective_ip_addresses", err)

	ipExpression := &IPAddressExpressionPatch{
		ID:           "ips",
		ResourceType: "IPAddressExpression",
		IPAddresses:  []string{"10.0.0.1", "10.0.0.2"},
	}
	cover("policy.groups.ip_address_expressions.patch", client.PatchGroupIPAddressExpression(ctx, "web", "ips", ipExpression))
	cover("policy.groups.ip_address_expressions.add", client.AddGroupIPAddressExpression(ctx, "web", "ips", &IPAddressExpressionPatch{IPAddresses: []string{"10.0.0.3"}}))
	cover("policy.groups.ip_address_expressions.remove", client.RemoveGroupIPAddressExpression(ctx, "web", "ips", &IPAddressExpressionPatch{IPAddresses: []string{"10.0.0.2"}}))
	cover("policy.groups.path_expressions.patch", client.PatchGroupPathExpression(ctx, "web", "paths", &PathExpressionPatch{
		ID:           "paths",
		ResourceType: "PathExpression",
		Paths:        []string{"/infra/tier-1s/app-t1/segments/app-seg"},
	}))
	_, err = client.ListGroupIPAddressMembers(ctx, "web")
	cover("policy.groups.members.ip_addresses", err)
	_, err = client.ListGroupIPGroupMembers(ctx, "web")
	cover("policy.groups.members.ip_groups", err)
	_, err = client.ListGroupSegmentMembers(ctx, "web")
	cover("policy.groups.members.segments", err)
	cover("policy.groups.ip_address_expressions.delete", client.DeleteGroupIPAddressExpression(ctx, "web", "ips"))

	section := &FirewallSection{
		Resource:    Resource{ID: "sec-a", DisplayName: "Section A", ResourceType: "FirewallSection"},
		SectionType: "LAYER3",
		Stateful:    true,
	}
	createdSection, err := client.CreateFirewallSection(ctx, section)
	cover("manager.firewall.sections.create", err)
	_, err = client.GetFirewallSection(ctx, createdSection.ID)
	cover("manager.firewall.sections.get", err)
	_, err = client.ListFirewallSections(ctx)
	cover("manager.firewall.sections.list", err)
	createdSection.DisplayName = "Section A Updated"
	updatedSection, err := client.UpdateFirewallSection(ctx, createdSection.ID, createdSection)
	cover("manager.firewall.sections.update", err)
	updatedSection.DisplayName = "Section A Revised"
	revisedSection, err := client.ReviseFirewallSection(ctx, updatedSection.ID, updatedSection)
	cover("manager.firewall.sections.revise", err)

	rule := &FirewallRule{Resource: Resource{ID: "rule-a", DisplayName: "Rule A", ResourceType: "FirewallRule"}, Action: "ALLOW"}
	createdRule, err := client.CreateFirewallRule(ctx, revisedSection.ID, rule)
	cover("manager.firewall.rules.create", err)
	_, err = client.GetFirewallRule(ctx, revisedSection.ID, createdRule.ID)
	cover("manager.firewall.rules.get", err)
	_, err = client.ListFirewallRules(ctx, revisedSection.ID)
	cover("manager.firewall.rules.list", err)
	_, err = client.ListFirewallRuleStats(ctx, revisedSection.ID)
	cover("manager.firewall.rules.stats", err)
	createdRule.Action = "DROP"
	updatedRule, err := client.UpdateFirewallRule(ctx, revisedSection.ID, createdRule.ID, createdRule)
	cover("manager.firewall.rules.update", err)
	updatedRule.Action = "ALLOW"
	_, err = client.ReviseFirewallRule(ctx, revisedSection.ID, updatedRule.ID, updatedRule)
	cover("manager.firewall.rules.revise", err)
	_, err = client.CreateMultipleFirewallRules(ctx, revisedSection.ID, []*FirewallRule{
		{Resource: Resource{ID: "rule-b", DisplayName: "Rule B", ResourceType: "FirewallRule"}, Action: "DETECT"},
	})
	cover("manager.firewall.rules.create_multiple", err)
	_, err = client.ListFirewallSectionWithRules(ctx, revisedSection.ID)
	cover("manager.firewall.sections.list_with_rules", err)
	revisedSection.Rules = []*FirewallRule{{Resource: Resource{ID: "rule-c", DisplayName: "Rule C", ResourceType: "FirewallRule"}, Action: "ALLOW"}}
	sectionWithRules, err := client.UpdateFirewallSectionWithRules(ctx, revisedSection.ID, revisedSection)
	cover("manager.firewall.sections.update_with_rules", err)
	sectionWithRules.Rules = []*FirewallRule{{Resource: Resource{ID: "rule-d", DisplayName: "Rule D", ResourceType: "FirewallRule"}, Action: "DROP"}}
	_, err = client.ReviseFirewallSectionWithRules(ctx, sectionWithRules.ID, sectionWithRules)
	cover("manager.firewall.sections.revise_with_rules", err)
	cover("manager.firewall.rules.delete", client.DeleteFirewallRule(ctx, revisedSection.ID, "rule-d"))
	cover("manager.firewall.sections.delete", client.DeleteFirewallSection(ctx, revisedSection.ID, true))

	sectionBundle := &FirewallSection{
		Resource:    Resource{ID: "sec-b", DisplayName: "Section B", ResourceType: "FirewallSection"},
		SectionType: "LAYER3",
		Stateful:    true,
		Rules:       []*FirewallRule{{Resource: Resource{ID: "bundle-rule", DisplayName: "Bundle Rule", ResourceType: "FirewallRule"}, Action: "ALLOW"}},
	}
	_, err = client.CreateFirewallSectionWithRules(ctx, sectionBundle)
	cover("manager.firewall.sections.create_with_rules", err)

	ipSet := &IPSet{Resource: Resource{ID: "ips-a", DisplayName: "IPs A", ResourceType: "IPSet"}, IPAddresses: []string{"192.0.2.1"}}
	createdIPSet, err := client.CreateIPSet(ctx, ipSet)
	cover("manager.ip_sets.create", err)
	_, err = client.GetIPSet(ctx, createdIPSet.ID)
	cover("manager.ip_sets.get", err)
	_, err = client.ListIPSets(ctx)
	cover("manager.ip_sets.list", err)
	createdIPSet.IPAddresses = append(createdIPSet.IPAddresses, "192.0.2.2")
	updatedIPSet, err := client.UpdateIPSet(ctx, createdIPSet.ID, createdIPSet)
	cover("manager.ip_sets.update", err)
	_, err = client.AddIPSetMember(ctx, updatedIPSet.ID, IPElement{IPAddress: "192.0.2.3"})
	cover("manager.ip_sets.add_ip", err)
	_, err = client.RemoveIPSetMember(ctx, updatedIPSet.ID, IPElement{IPAddress: "192.0.2.2"})
	cover("manager.ip_sets.remove_ip", err)
	_, err = client.ListIPSetMembers(ctx, updatedIPSet.ID)
	cover("manager.ip_sets.members", err)

	_, err = client.SearchManagerQuery(ctx, SearchQueryOptions{Query: "resource_type:IPSet"})
	cover("manager.search.query", err)
	_, err = client.SearchManagerDSL(ctx, SearchQueryOptions{Query: "IPSet"})
	cover("manager.search.dsl", err)
	cover("manager.ip_sets.delete", client.DeleteIPSet(ctx, updatedIPSet.ID))

	policy := &SecurityPolicy{Resource: Resource{ID: "policy-a", DisplayName: "Policy A", ResourceType: "SecurityPolicy"}, Category: "Application"}
	createdPolicy, err := client.PutSecurityPolicy(ctx, "policy-a", policy)
	cover("policy.security_policies.put", err)
	_, err = client.GetSecurityPolicy(ctx, createdPolicy.ID)
	cover("policy.security_policies.get", err)
	_, err = client.ListSecurityPolicies(ctx)
	cover("policy.security_policies.list", err)
	cover("policy.security_policies.patch", client.PatchSecurityPolicy(ctx, createdPolicy.ID, &SecurityPolicy{Resource: Resource{DisplayName: "Policy A Patch", ResourceType: "SecurityPolicy"}}))
	revisedPolicy, err := client.ReviseSecurityPolicy(ctx, createdPolicy.ID, &SecurityPolicy{Resource: Resource{DisplayName: "Policy A Revised", ResourceType: "SecurityPolicy"}})
	cover("policy.security_policies.revise", err)
	_, err = client.ListSecurityPolicyStats(ctx, revisedPolicy.ID)
	cover("policy.security_policies.statistics", err)

	securityRule := &SecurityRule{Resource: Resource{ID: "allow-web", DisplayName: "Allow Web", ResourceType: "Rule"}, Action: "ALLOW"}
	createdSecurityRule, err := client.PutSecurityRule(ctx, revisedPolicy.ID, "allow-web", securityRule)
	cover("policy.security_rules.put", err)
	_, err = client.GetSecurityRule(ctx, revisedPolicy.ID, createdSecurityRule.ID)
	cover("policy.security_rules.get", err)
	_, err = client.ListSecurityRules(ctx, revisedPolicy.ID)
	cover("policy.security_rules.list", err)
	cover("policy.security_rules.patch", client.PatchSecurityRule(ctx, revisedPolicy.ID, createdSecurityRule.ID, &SecurityRule{Resource: Resource{DisplayName: "Allow Web Patch", ResourceType: "Rule"}, Action: "DROP"}))
	revisedSecurityRule, err := client.ReviseSecurityRule(ctx, revisedPolicy.ID, createdSecurityRule.ID, &SecurityRule{Resource: Resource{DisplayName: "Allow Web Revised", ResourceType: "Rule"}, Action: "ALLOW"})
	cover("policy.security_rules.revise", err)
	_, err = client.ListSecurityRuleStats(ctx, revisedPolicy.ID, revisedSecurityRule.ID)
	cover("policy.security_rules.statistics", err)
	cover("policy.security_rules.delete", client.DeleteSecurityRule(ctx, revisedPolicy.ID, revisedSecurityRule.ID))
	cover("policy.security_policies.delete", client.DeleteSecurityPolicy(ctx, revisedPolicy.ID))

	segment := &Segment{Resource: Resource{ID: "seg-a", DisplayName: "Segment A", ResourceType: "Segment"}, AdminState: "UP"}
	createdSegment, err := client.PutInfraSegment(ctx, "seg-a", segment)
	cover("policy.infra.segments.put", err)
	_, err = client.GetInfraSegment(ctx, createdSegment.ID)
	cover("policy.infra.segments.get", err)
	_, err = client.ListInfraSegments(ctx)
	cover("policy.infra.segments.list", err)
	cover("policy.infra.segments.patch", client.PatchInfraSegment(ctx, createdSegment.ID, &Segment{Resource: Resource{DisplayName: "Segment A Patch", ResourceType: "Segment"}}))
	_, err = client.GetInfraSegmentState(ctx, createdSegment.ID)
	cover("policy.infra.segments.state", err)
	_, err = client.GetInfraSegmentStatistics(ctx, createdSegment.ID)
	cover("policy.infra.segments.statistics", err)
	_, err = client.ListInfraSegmentStates(ctx)
	cover("policy.infra.segments.state.list", err)
	cover("policy.infra.segments.delete", client.DeleteInfraSegment(ctx, createdSegment.ID))

	_, err = client.ListTier0s(ctx)
	cover("policy.tier0s.list", err)
	_, err = client.ListTier1s(ctx)
	cover("policy.tier1s.list", err)
	_, err = client.GetTier1(ctx, "missing-t1")
	coverNotFound("policy.tier1s.get", err)
	_, err = client.GetTier1State(ctx, "missing-t1")
	coverNotFound("policy.tier1s.state", err)

	tier1Segment := &Segment{
		Resource:         Resource{ID: "app-seg", DisplayName: "App Segment", ResourceType: "Segment"},
		ConnectivityPath: "/infra/tier-1s/app-t1",
	}
	createdTier1Segment, err := client.PutTier1Segment(ctx, "app-t1", "app-seg", tier1Segment)
	cover("policy.tier1.segments.put", err)
	_, err = client.GetTier1Segment(ctx, "app-t1", createdTier1Segment.ID)
	cover("policy.tier1.segments.get", err)
	_, err = client.ListTier1Segments(ctx, "app-t1")
	cover("policy.tier1.segments.list", err)
	cover("policy.tier1.segments.patch", client.PatchTier1Segment(ctx, "app-t1", createdTier1Segment.ID, &Segment{Resource: Resource{DisplayName: "Tier1 Segment Patch", ResourceType: "Segment"}}))
	_, err = client.GetTier1SegmentState(ctx, "app-t1", createdTier1Segment.ID)
	cover("policy.tier1.segments.state", err)
	_, err = client.GetTier1SegmentStatistics(ctx, "app-t1", createdTier1Segment.ID)
	cover("policy.tier1.segments.statistics", err)
	_, err = client.ListTier1SegmentStates(ctx, "app-t1")
	cover("policy.tier1.segments.state.list", err)
	_, err = client.GetGlobalTier1SegmentState(ctx, "app-t1", createdTier1Segment.ID)
	cover("policy.global.tier1.segments.state", err)
	_, err = client.GetGlobalTier1SegmentStatistics(ctx, "app-t1", createdTier1Segment.ID)
	cover("policy.global.tier1.segments.statistics", err)
	cover("policy.tier1.segments.delete", client.DeleteTier1Segment(ctx, "app-t1", createdTier1Segment.ID))

	_, err = client.SearchPolicyQuery(ctx, SearchQueryOptions{Query: "resource_type:Group"})
	cover("policy.search.query", err)
	_, err = client.SearchPolicyDSL(ctx, SearchQueryOptions{Query: "Group"})
	cover("policy.search.dsl", err)

	cover("policy.groups.delete", client.DeleteGroup(ctx, "web"))

	for routeName := range contractedRouteNames() {
		if !covered[routeName] {
			t.Fatalf("contract route %q was not covered by test execution", routeName)
		}
	}
}

func TestSharedRateLimitedClientConcurrencyAgainstMockAPI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	mock := mockapi.Start(t, ctx)
	mockURL, err := url.Parse(mock.BaseURL())
	if err != nil {
		t.Fatalf("parse mockapi base URL: %v", err)
	}
	gate := newMockAPIRateLimitGate(t, "nsx-mock-a.example.net", "nsx-mock-a.example.net:8080")
	sharedHTTPClient := &http.Client{
		Transport: httpratelimit.NewRoundTripper(&mockAPIRoutingTransport{
			target: mockURL,
			base:   http.DefaultTransport,
			gate:   gate,
		}, httpratelimit.Config{
			MaxRequestsInFlightPerHost:  1,
			MaxRequestsPerSecondPerHost: 100,
		}, zap.NewNop()),
	}

	firstSame := newMockAPINSXClient(t, "http://nsx-mock-a.example.net", sharedHTTPClient)
	secondSame := newMockAPINSXClient(t, "http://nsx-mock-a.example.net", sharedHTTPClient)
	differentPort := newMockAPINSXClient(t, "http://nsx-mock-a.example.net:8080", sharedHTTPClient)
	errs := make(chan error, 3)

	go listGroupsAsync(ctx, firstSame, "first same host", errs)
	gate.requireFirstSameHostRequest(ctx)

	go listGroupsAsync(ctx, secondSame, "second same host", errs)
	go listGroupsAsync(ctx, differentPort, "different port", errs)

	gate.requireDifferentPortRequest(ctx)
	gate.requireNoSecondSameHostRequestBeforeRelease(250 * time.Millisecond)
	gate.releaseSameHost()
	gate.requireSecondSameHostRequest(ctx)

	for range 3 {
		receivedErr := <-errs
		if receivedErr != nil {
			t.Fatalf("%v\nmockapi logs:\n%s", receivedErr, mock.Logs())
		}
	}
	t.Logf("mockapi concurrency evidence: same logical host nsx-mock-a.example.net:80 shared one in-flight slot; nsx-mock-a.example.net:8080 reached mockapi while :80 was blocked")
}

func newMockAPINSXClient(t *testing.T, baseURL string, httpClient *http.Client) *Client {
	t.Helper()

	client, err := NewClient(Options{
		BaseURL:    baseURL,
		HTTPClient: httpClient,
		Username:   mockAPIUsername,
		Password:   mockAPIPassword,
		Logger:     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("NewClient(%q) error = %v", baseURL, err)
	}
	return client
}

func listGroupsAsync(ctx context.Context, client *Client, label string, errs chan<- error) {
	_, err := client.ListGroups(ctx)
	if err != nil {
		errs <- fmt.Errorf("%s ListGroups: %w", label, err)
		return
	}
	errs <- nil
}

type mockAPIRoutingTransport struct {
	target *url.URL
	base   http.RoundTripper
	gate   *mockAPIRateLimitGate
}

func (transport *mockAPIRoutingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if transport.target == nil {
		return nil, fmt.Errorf("mockapi routing transport target URL is required")
	}
	if transport.base == nil {
		return nil, fmt.Errorf("mockapi routing transport base RoundTripper is required")
	}
	if transport.gate != nil {
		gateErr := transport.gate.beforeRoundTrip(req)
		if gateErr != nil {
			return nil, gateErr
		}
	}

	routed := req.Clone(req.Context())
	routedURL := *req.URL
	routedURL.Scheme = transport.target.Scheme
	routedURL.Host = transport.target.Host
	routed.URL = &routedURL
	routed.Host = req.URL.Host
	return transport.base.RoundTrip(routed)
}

type mockAPIRateLimitGate struct {
	t             *testing.T
	sameHost      string
	differentPort string
	releaseOnce   sync.Once
	release       chan struct{}
	firstSame     chan struct{}
	secondSame    chan struct{}
	different     chan struct{}
	mu            sync.Mutex
	hostCounts    map[string]int
}

func newMockAPIRateLimitGate(t *testing.T, sameHost string, differentPort string) *mockAPIRateLimitGate {
	t.Helper()

	return &mockAPIRateLimitGate{
		t:             t,
		sameHost:      sameHost,
		differentPort: differentPort,
		release:       make(chan struct{}),
		firstSame:     make(chan struct{}, 1),
		secondSame:    make(chan struct{}, 1),
		different:     make(chan struct{}, 1),
		hostCounts:    map[string]int{},
	}
}

func (gate *mockAPIRateLimitGate) beforeRoundTrip(req *http.Request) error {
	hostCount := gate.recordHost(req.URL.Host)
	switch req.URL.Host {
	case gate.sameHost:
		if hostCount == 1 {
			gate.signal(gate.firstSame)
			select {
			case <-gate.release:
			case <-req.Context().Done():
				return req.Context().Err()
			}
		}
		if hostCount == 2 {
			gate.signal(gate.secondSame)
		}
	case gate.differentPort:
		gate.signal(gate.different)
	default:
		return fmt.Errorf("unexpected mockapi logical host %q", req.URL.Host)
	}
	return nil
}

func (gate *mockAPIRateLimitGate) recordHost(host string) int {
	gate.mu.Lock()
	defer gate.mu.Unlock()

	gate.hostCounts[host]++
	return gate.hostCounts[host]
}

func (gate *mockAPIRateLimitGate) signal(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (gate *mockAPIRateLimitGate) requireFirstSameHostRequest(ctx context.Context) {
	gate.t.Helper()
	gate.requireSignal(ctx, gate.firstSame, "first same host request")
}

func (gate *mockAPIRateLimitGate) requireDifferentPortRequest(ctx context.Context) {
	gate.t.Helper()
	gate.requireSignal(ctx, gate.different, "different port request")
}

func (gate *mockAPIRateLimitGate) requireNoSecondSameHostRequestBeforeRelease(duration time.Duration) {
	gate.t.Helper()

	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-gate.secondSame:
		gate.t.Fatalf("second same host request reached mockapi transport before first same host response was released")
	case <-timer.C:
	}
}

func (gate *mockAPIRateLimitGate) requireSecondSameHostRequest(ctx context.Context) {
	gate.t.Helper()
	gate.requireSignal(ctx, gate.secondSame, "second same host request")
}

func (gate *mockAPIRateLimitGate) requireSignal(ctx context.Context, ch <-chan struct{}, description string) {
	gate.t.Helper()

	select {
	case <-ch:
	case <-ctx.Done():
		gate.t.Fatalf("timed out waiting for %s: %v", description, ctx.Err())
	}
}

func (gate *mockAPIRateLimitGate) releaseSameHost() {
	gate.releaseOnce.Do(func() {
		close(gate.release)
	})
}

func readMockAPIRouteInventory(t *testing.T) []string {
	t.Helper()

	mockAPIBinary := readMockAPIBinaryFromPublicImage(t)
	names := make([]string, 0, len(supportedRouteCoverage()))
	for _, route := range supportedRouteCoverage() {
		if bytes.Contains(mockAPIBinary, []byte(route.name)) {
			names = append(names, route.name)
		}
	}
	if len(names) == 0 {
		t.Fatalf("public mockapi image %s binary did not expose any supported route names", mockapi.Image)
	}
	sort.Strings(names)
	return names
}

func readMockAPIBinaryFromPublicImage(t *testing.T) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	create := exec.CommandContext(ctx, "docker", "create", "--platform", "linux/amd64", mockapi.Image)
	createOutput, err := create.CombinedOutput()
	if err != nil {
		t.Fatalf("create public mockapi image container for route inventory: %v\n%s", err, string(createOutput))
	}
	containerID := strings.TrimSpace(string(createOutput))
	if containerID == "" {
		t.Fatalf("create public mockapi image container returned empty id")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		remove := exec.CommandContext(cleanupCtx, "docker", "rm", "--force", containerID)
		removeOutput, removeErr := remove.CombinedOutput()
		if removeErr != nil && !strings.Contains(string(removeOutput), "No such container") {
			t.Errorf("remove route inventory container %s: %v\n%s", containerID, removeErr, string(removeOutput))
		}
	})

	binaryPath := filepath.Join(t.TempDir(), "nsx-t-mockapi")
	copyCommand := exec.CommandContext(ctx, "docker", "cp", containerID+":/nsx-t-mockapi", binaryPath)
	copyOutput, err := copyCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("copy mockapi binary from public image %s: %v\n%s", mockapi.Image, err, string(copyOutput))
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read copied mockapi binary: %v", err)
	}
	return binary
}

func inventoryContains(inventory []string, routeName string) bool {
	for _, candidate := range inventory {
		if candidate == routeName {
			return true
		}
	}
	return false
}

func contractedRouteNames() map[string]bool {
	contracted := map[string]bool{}
	for _, route := range supportedRouteCoverage() {
		contracted[route.name] = true
	}
	return contracted
}

func supportedRouteCoverage() []routeCoverage {
	return []routeCoverage{
		{name: "manager.search.query", method: "SearchManagerQuery"},
		{name: "manager.search.dsl", method: "SearchManagerDSL"},
		{name: "manager.firewall.sections.list", method: "ListFirewallSections"},
		{name: "manager.firewall.sections.create", method: "CreateFirewallSection"},
		{name: "manager.firewall.sections.create_with_rules", method: "CreateFirewallSectionWithRules"},
		{name: "manager.firewall.sections.get", method: "GetFirewallSection"},
		{name: "manager.firewall.sections.update", method: "UpdateFirewallSection"},
		{name: "manager.firewall.sections.delete", method: "DeleteFirewallSection"},
		{name: "manager.firewall.sections.revise", method: "ReviseFirewallSection"},
		{name: "manager.firewall.sections.list_with_rules", method: "ListFirewallSectionWithRules"},
		{name: "manager.firewall.sections.update_with_rules", method: "UpdateFirewallSectionWithRules"},
		{name: "manager.firewall.sections.revise_with_rules", method: "ReviseFirewallSectionWithRules"},
		{name: "manager.firewall.rules.list", method: "ListFirewallRules"},
		{name: "manager.firewall.rules.create", method: "CreateFirewallRule"},
		{name: "manager.firewall.rules.create_multiple", method: "CreateMultipleFirewallRules"},
		{name: "manager.firewall.rules.stats", method: "ListFirewallRuleStats"},
		{name: "manager.firewall.rules.get", method: "GetFirewallRule"},
		{name: "manager.firewall.rules.update", method: "UpdateFirewallRule"},
		{name: "manager.firewall.rules.delete", method: "DeleteFirewallRule"},
		{name: "manager.firewall.rules.revise", method: "ReviseFirewallRule"},
		{name: "manager.ip_sets.list", method: "ListIPSets"},
		{name: "manager.ip_sets.create", method: "CreateIPSet"},
		{name: "manager.ip_sets.get", method: "GetIPSet"},
		{name: "manager.ip_sets.update", method: "UpdateIPSet"},
		{name: "manager.ip_sets.delete", method: "DeleteIPSet"},
		{name: "manager.ip_sets.add_ip", method: "AddIPSetMember"},
		{name: "manager.ip_sets.remove_ip", method: "RemoveIPSetMember"},
		{name: "manager.ip_sets.members", method: "ListIPSetMembers"},
		{name: "policy.search.query", method: "SearchPolicyQuery"},
		{name: "policy.search.dsl", method: "SearchPolicyDSL"},
		{name: "policy.eula.acceptance", method: "GetEULAAcceptance"},
		{name: "policy.global.groups.consolidated_effective_ip_addresses", method: "GetGlobalConsolidatedEffectiveIPAddresses"},
		{name: "policy.global.tier1.segments.state", method: "GetGlobalTier1SegmentState"},
		{name: "policy.global.tier1.segments.statistics", method: "GetGlobalTier1SegmentStatistics"},
		{name: "policy.groups.list", method: "ListGroups"},
		{name: "policy.groups.patch", method: "PatchGroup"},
		{name: "policy.groups.put", method: "PutGroup"},
		{name: "policy.groups.get", method: "GetGroup"},
		{name: "policy.groups.delete", method: "DeleteGroup"},
		{name: "policy.groups.ip_address_expressions.patch", method: "PatchGroupIPAddressExpression"},
		{name: "policy.groups.ip_address_expressions.add", method: "AddGroupIPAddressExpression"},
		{name: "policy.groups.ip_address_expressions.remove", method: "RemoveGroupIPAddressExpression"},
		{name: "policy.groups.ip_address_expressions.delete", method: "DeleteGroupIPAddressExpression"},
		{name: "policy.groups.path_expressions.patch", method: "PatchGroupPathExpression"},
		{name: "policy.groups.members.ip_addresses", method: "ListGroupIPAddressMembers"},
		{name: "policy.groups.members.ip_groups", method: "ListGroupIPGroupMembers"},
		{name: "policy.groups.members.segments", method: "ListGroupSegmentMembers"},
		{name: "policy.security_policies.list", method: "ListSecurityPolicies"},
		{name: "policy.security_policies.put", method: "PutSecurityPolicy"},
		{name: "policy.security_policies.delete", method: "DeleteSecurityPolicy"},
		{name: "policy.security_policies.get", method: "GetSecurityPolicy"},
		{name: "policy.security_policies.patch", method: "PatchSecurityPolicy"},
		{name: "policy.security_policies.revise", method: "ReviseSecurityPolicy"},
		{name: "policy.security_policies.statistics", method: "ListSecurityPolicyStats"},
		{name: "policy.security_rules.list", method: "ListSecurityRules"},
		{name: "policy.security_rules.patch", method: "PatchSecurityRule"},
		{name: "policy.security_rules.delete", method: "DeleteSecurityRule"},
		{name: "policy.security_rules.put", method: "PutSecurityRule"},
		{name: "policy.security_rules.get", method: "GetSecurityRule"},
		{name: "policy.security_rules.statistics", method: "ListSecurityRuleStats"},
		{name: "policy.security_rules.revise", method: "ReviseSecurityRule"},
		{name: "policy.infra.segments.list", method: "ListInfraSegments"},
		{name: "policy.infra.segments.state.list", method: "ListInfraSegmentStates"},
		{name: "policy.infra.segments.put", method: "PutInfraSegment"},
		{name: "policy.infra.segments.delete", method: "DeleteInfraSegment"},
		{name: "policy.infra.segments.patch", method: "PatchInfraSegment"},
		{name: "policy.infra.segments.get", method: "GetInfraSegment"},
		{name: "policy.infra.segments.state", method: "GetInfraSegmentState"},
		{name: "policy.infra.segments.statistics", method: "GetInfraSegmentStatistics"},
		{name: "policy.tier0s.list", method: "ListTier0s"},
		{name: "policy.tier1s.list", method: "ListTier1s"},
		{name: "policy.tier1s.get", method: "GetTier1"},
		{name: "policy.tier1s.state", method: "GetTier1State"},
		{name: "policy.tier1.segments.list", method: "ListTier1Segments"},
		{name: "policy.tier1.segments.state.list", method: "ListTier1SegmentStates"},
		{name: "policy.tier1.segments.delete", method: "DeleteTier1Segment"},
		{name: "policy.tier1.segments.patch", method: "PatchTier1Segment"},
		{name: "policy.tier1.segments.put", method: "PutTier1Segment"},
		{name: "policy.tier1.segments.get", method: "GetTier1Segment"},
		{name: "policy.tier1.segments.state", method: "GetTier1SegmentState"},
		{name: "policy.tier1.segments.statistics", method: "GetTier1SegmentStatistics"},
	}
}

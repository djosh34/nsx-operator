//nolint:revive // route method names directly mirror NSX API operations; per-method comments would duplicate the names.
package nsxclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

//nolint:gocritic // public search methods keep value options so callers can pass literals.
func (c *Client) SearchManagerQuery(ctx context.Context, options SearchQueryOptions) ([]*SearchResult, error) {
	return c.search(ctx, "/api/v1/search/query", &options)
}

//nolint:gocritic // public search methods keep value options so callers can pass literals.
func (c *Client) SearchManagerDSL(ctx context.Context, options SearchQueryOptions) ([]*SearchResult, error) {
	return c.search(ctx, "/api/v1/search/dsl", &options)
}

//nolint:gocritic // public search methods keep value options so callers can pass literals.
func (c *Client) SearchPolicyQuery(ctx context.Context, options SearchQueryOptions) ([]*SearchResult, error) {
	return c.search(ctx, "/policy/api/v1/search/query", &options)
}

//nolint:gocritic // public search methods keep value options so callers can pass literals.
func (c *Client) SearchPolicyDSL(ctx context.Context, options SearchQueryOptions) ([]*SearchResult, error) {
	return c.search(ctx, "/policy/api/v1/search/dsl", &options)
}

func (c *Client) search(ctx context.Context, path string, options *SearchQueryOptions) ([]*SearchResult, error) {
	query := url.Values{}
	if options.Query != "" {
		query.Set("query", options.Query)
	}
	if options.PageSize > 0 {
		query.Set("page_size", strconv.Itoa(options.PageSize))
	}
	if options.Cursor != "" {
		query.Set("cursor", options.Cursor)
	}
	if options.SortBy != "" {
		query.Set("sort_by", options.SortBy)
	}
	if options.Ascending != nil {
		query.Set("sort_ascending", strconv.FormatBool(*options.Ascending))
	}
	if len(options.Fields) > 0 {
		query.Set("fields", strings.Join(options.Fields, ","))
	}
	return listAllTyped[SearchResult](ctx, c, path, query)
}

func doTyped[T any](ctx context.Context, c *Client, method string, path string, query url.Values, payload any) (*T, error) {
	var result T
	err := c.do(ctx, method, path, query, payload, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListFirewallSections(ctx context.Context) ([]*FirewallSection, error) {
	return listAllTyped[FirewallSection](ctx, c, "/api/v1/firewall/sections", nil)
}

func (c *Client) CreateFirewallSection(ctx context.Context, section *FirewallSection) (*FirewallSection, error) {
	var result FirewallSection
	err := c.do(ctx, http.MethodPost, "/api/v1/firewall/sections", nil, section, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateFirewallSectionWithRules(ctx context.Context, section *FirewallSection) (*FirewallSection, error) {
	var result FirewallSection
	err := c.do(ctx, http.MethodPost, "/api/v1/firewall/sections", actionQuery("create_with_rules"), section, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetFirewallSection(ctx context.Context, sectionID string) (*FirewallSection, error) {
	var result FirewallSection
	err := c.do(ctx, http.MethodGet, "/api/v1/firewall/sections/"+pathEscape(sectionID), nil, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateFirewallSection(ctx context.Context, sectionID string, section *FirewallSection) (*FirewallSection, error) {
	var result FirewallSection
	err := c.do(ctx, http.MethodPut, "/api/v1/firewall/sections/"+pathEscape(sectionID), nil, section, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteFirewallSection(ctx context.Context, sectionID string, cascade bool) error {
	query := url.Values{}
	if cascade {
		query.Set("cascade", "true")
	}
	return c.do(ctx, http.MethodDelete, "/api/v1/firewall/sections/"+pathEscape(sectionID), query, nil, nil)
}

func (c *Client) ReviseFirewallSection(ctx context.Context, sectionID string, section *FirewallSection) (*FirewallSection, error) {
	var result FirewallSection
	err := c.do(ctx, http.MethodPost, "/api/v1/firewall/sections/"+pathEscape(sectionID), actionQuery("revise"), section, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) ListFirewallSectionWithRules(ctx context.Context, sectionID string) (*FirewallSection, error) {
	var result FirewallSection
	err := c.do(ctx, http.MethodPost, "/api/v1/firewall/sections/"+pathEscape(sectionID), actionQuery("list_with_rules"), struct{}{}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) UpdateFirewallSectionWithRules(ctx context.Context, sectionID string, section *FirewallSection) (*FirewallSection, error) {
	return doTyped[FirewallSection](ctx, c, http.MethodPost, "/api/v1/firewall/sections/"+pathEscape(sectionID), actionQuery("update_with_rules"), section)
}

func (c *Client) ReviseFirewallSectionWithRules(ctx context.Context, sectionID string, section *FirewallSection) (*FirewallSection, error) {
	return doTyped[FirewallSection](ctx, c, http.MethodPost, "/api/v1/firewall/sections/"+pathEscape(sectionID), actionQuery("revise_with_rules"), section)
}

func (c *Client) ListFirewallRules(ctx context.Context, sectionID string) ([]*FirewallRule, error) {
	return listAllTyped[FirewallRule](ctx, c, "/api/v1/firewall/sections/"+pathEscape(sectionID)+"/rules", nil)
}

func (c *Client) CreateFirewallRule(ctx context.Context, sectionID string, rule *FirewallRule) (*FirewallRule, error) {
	return doTyped[FirewallRule](ctx, c, http.MethodPost, "/api/v1/firewall/sections/"+pathEscape(sectionID)+"/rules", nil, rule)
}

func (c *Client) CreateMultipleFirewallRules(ctx context.Context, sectionID string, rules []*FirewallRule) ([]*FirewallRule, error) {
	var result struct {
		Rules []*FirewallRule `json:"rules"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/firewall/sections/"+pathEscape(sectionID)+"/rules", actionQuery("create_multiple"), map[string]any{"rules": rules}, &result)
	if err != nil {
		return nil, err
	}
	return result.Rules, nil
}

func (c *Client) ListFirewallRuleStats(ctx context.Context, sectionID string) ([]*FirewallRuleStats, error) {
	return listAllTyped[FirewallRuleStats](ctx, c, "/api/v1/firewall/sections/"+pathEscape(sectionID)+"/rules/stats", nil)
}

func (c *Client) GetFirewallRule(ctx context.Context, sectionID string, ruleID string) (*FirewallRule, error) {
	path := "/api/v1/firewall/sections/" + pathEscape(sectionID) + "/rules/" + pathEscape(ruleID)
	return doTyped[FirewallRule](ctx, c, http.MethodGet, path, nil, nil)
}

func (c *Client) UpdateFirewallRule(ctx context.Context, sectionID string, ruleID string, rule *FirewallRule) (*FirewallRule, error) {
	path := "/api/v1/firewall/sections/" + pathEscape(sectionID) + "/rules/" + pathEscape(ruleID)
	return doTyped[FirewallRule](ctx, c, http.MethodPut, path, nil, rule)
}

func (c *Client) DeleteFirewallRule(ctx context.Context, sectionID string, ruleID string) error {
	path := "/api/v1/firewall/sections/" + pathEscape(sectionID) + "/rules/" + pathEscape(ruleID)
	return c.do(ctx, http.MethodDelete, path, nil, nil, nil)
}

func (c *Client) ReviseFirewallRule(ctx context.Context, sectionID string, ruleID string, rule *FirewallRule) (*FirewallRule, error) {
	path := "/api/v1/firewall/sections/" + pathEscape(sectionID) + "/rules/" + pathEscape(ruleID)
	return doTyped[FirewallRule](ctx, c, http.MethodPost, path, actionQuery("revise"), rule)
}

func (c *Client) ListIPSets(ctx context.Context) ([]*IPSet, error) {
	return listAllTyped[IPSet](ctx, c, "/api/v1/ip-sets", nil)
}

func (c *Client) CreateIPSet(ctx context.Context, ipSet *IPSet) (*IPSet, error) {
	return doTyped[IPSet](ctx, c, http.MethodPost, "/api/v1/ip-sets", nil, ipSet)
}

func (c *Client) GetIPSet(ctx context.Context, ipSetID string) (*IPSet, error) {
	return doTyped[IPSet](ctx, c, http.MethodGet, "/api/v1/ip-sets/"+pathEscape(ipSetID), nil, nil)
}

func (c *Client) UpdateIPSet(ctx context.Context, ipSetID string, ipSet *IPSet) (*IPSet, error) {
	return doTyped[IPSet](ctx, c, http.MethodPut, "/api/v1/ip-sets/"+pathEscape(ipSetID), nil, ipSet)
}

func (c *Client) DeleteIPSet(ctx context.Context, ipSetID string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/ip-sets/"+pathEscape(ipSetID), nil, nil, nil)
}

func (c *Client) AddIPSetMember(ctx context.Context, ipSetID string, element IPElement) (*IPElement, error) {
	return c.ipSetMemberAction(ctx, ipSetID, "add_ip", element)
}

func (c *Client) RemoveIPSetMember(ctx context.Context, ipSetID string, element IPElement) (*IPElement, error) {
	return c.ipSetMemberAction(ctx, ipSetID, "remove_ip", element)
}

func (c *Client) ipSetMemberAction(ctx context.Context, ipSetID string, action string, element IPElement) (*IPElement, error) {
	return doTyped[IPElement](ctx, c, http.MethodPost, "/api/v1/ip-sets/"+pathEscape(ipSetID), actionQuery(action), element)
}

func (c *Client) ListIPSetMembers(ctx context.Context, ipSetID string) ([]*IPElement, error) {
	return listAllTyped[IPElement](ctx, c, "/api/v1/ip-sets/"+pathEscape(ipSetID)+"/members", nil)
}

func (c *Client) ListGroups(ctx context.Context) ([]*Group, error) {
	return listAllTyped[Group](ctx, c, defaultDomainPath()+"/groups", nil)
}

func (c *Client) PatchGroup(ctx context.Context, groupID string, group *GroupPatch) error {
	return c.do(ctx, http.MethodPatch, groupPath(groupID), nil, group, nil)
}

func (c *Client) PutGroup(ctx context.Context, groupID string, group *Group) (*Group, error) {
	return doTyped[Group](ctx, c, http.MethodPut, groupPath(groupID), nil, group)
}

func (c *Client) GetGroup(ctx context.Context, groupID string) (*Group, error) {
	return doTyped[Group](ctx, c, http.MethodGet, groupPath(groupID), nil, nil)
}

func (c *Client) DeleteGroup(ctx context.Context, groupID string) error {
	return c.do(ctx, http.MethodDelete, groupPath(groupID), nil, nil, nil)
}

func (c *Client) PatchGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string, expression *IPAddressExpressionPatch) error {
	return c.do(ctx, http.MethodPatch, groupExpressionPath(groupID, "ip-address-expressions", expressionID), nil, expression, nil)
}

func (c *Client) AddGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string, expression *IPAddressExpressionPatch) error {
	return c.do(ctx, http.MethodPost, groupExpressionPath(groupID, "ip-address-expressions", expressionID), actionQuery("add"), expression, nil)
}

func (c *Client) RemoveGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string, expression *IPAddressExpressionPatch) error {
	return c.do(ctx, http.MethodPost, groupExpressionPath(groupID, "ip-address-expressions", expressionID), actionQuery("remove"), expression, nil)
}

func (c *Client) DeleteGroupIPAddressExpression(ctx context.Context, groupID string, expressionID string) error {
	return c.do(ctx, http.MethodDelete, groupExpressionPath(groupID, "ip-address-expressions", expressionID), nil, nil, nil)
}

func (c *Client) PatchGroupPathExpression(ctx context.Context, groupID string, expressionID string, expression *PathExpressionPatch) error {
	return c.do(ctx, http.MethodPatch, groupExpressionPath(groupID, "path-expressions", expressionID), nil, expression, nil)
}

func (c *Client) AddGroupPathExpression(ctx context.Context, groupID string, expressionID string, expression *PathExpressionPatch) error {
	return c.do(ctx, http.MethodPost, groupExpressionPath(groupID, "path-expressions", expressionID), actionQuery("add"), expression, nil)
}

func (c *Client) DeleteGroupPathExpression(ctx context.Context, groupID string, expressionID string) error {
	return c.do(ctx, http.MethodDelete, groupExpressionPath(groupID, "path-expressions", expressionID), nil, nil, nil)
}

func (c *Client) ListGroupIPAddressMembers(ctx context.Context, groupID string) ([]string, error) {
	items, err := listAllTyped[string](ctx, c, groupPath(groupID)+"/members/ip-addresses", nil)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil {
			values = append(values, *item)
		}
	}
	return values, nil
}

func (c *Client) ListGroupIPGroupMembers(ctx context.Context, groupID string) ([]*GroupMember, error) {
	return listAllTyped[GroupMember](ctx, c, groupPath(groupID)+"/members/ip-groups", nil)
}

func (c *Client) ListGroupSegmentMembers(ctx context.Context, groupID string) ([]*GroupMember, error) {
	return listAllTyped[GroupMember](ctx, c, groupPath(groupID)+"/members/segments", nil)
}

func (c *Client) GetGlobalConsolidatedEffectiveIPAddresses(ctx context.Context, groupID string) (*ConsolidatedEffectiveIPAddresses, error) {
	path := "/policy/api/v1/global-infra/domains/" + defaultDomainID + "/groups/" + pathEscape(groupID) + "/members/consolidated-effective-ip-addresses"
	return doTyped[ConsolidatedEffectiveIPAddresses](ctx, c, http.MethodGet, path, nil, nil)
}

func (c *Client) GetEULAAcceptance(ctx context.Context) (*EULAAcceptance, error) {
	return doTyped[EULAAcceptance](ctx, c, http.MethodGet, "/policy/api/v1/eula/acceptance", nil, nil)
}

func (c *Client) ListSecurityPolicies(ctx context.Context) ([]*SecurityPolicy, error) {
	return listAllTyped[SecurityPolicy](ctx, c, defaultDomainPath()+"/security-policies", nil)
}

func (c *Client) PutSecurityPolicy(ctx context.Context, policyID string, policy *SecurityPolicy) (*SecurityPolicy, error) {
	return doTyped[SecurityPolicy](ctx, c, http.MethodPut, securityPolicyPath(policyID), nil, policy)
}

func (c *Client) DeleteSecurityPolicy(ctx context.Context, policyID string) error {
	return c.do(ctx, http.MethodDelete, securityPolicyPath(policyID), nil, nil, nil)
}

func (c *Client) GetSecurityPolicy(ctx context.Context, policyID string) (*SecurityPolicy, error) {
	return doTyped[SecurityPolicy](ctx, c, http.MethodGet, securityPolicyPath(policyID), nil, nil)
}

func (c *Client) PatchSecurityPolicy(ctx context.Context, policyID string, policy *SecurityPolicy) error {
	return c.do(ctx, http.MethodPatch, securityPolicyPath(policyID), nil, policy, nil)
}

func (c *Client) ReviseSecurityPolicy(ctx context.Context, policyID string, policy *SecurityPolicy) (*SecurityPolicy, error) {
	return doTyped[SecurityPolicy](ctx, c, http.MethodPost, securityPolicyPath(policyID), actionQuery("revise"), policy)
}

func (c *Client) ListSecurityPolicyStats(ctx context.Context, policyID string) ([]*SecurityPolicyStats, error) {
	return listAllTyped[SecurityPolicyStats](ctx, c, securityPolicyPath(policyID)+"/statistics", nil)
}

func (c *Client) ListSecurityRules(ctx context.Context, policyID string) ([]*SecurityRule, error) {
	return listAllTyped[SecurityRule](ctx, c, securityPolicyPath(policyID)+"/rules", nil)
}

func (c *Client) PatchSecurityRule(ctx context.Context, policyID string, ruleID string, rule *SecurityRule) error {
	return c.do(ctx, http.MethodPatch, securityRulePath(policyID, ruleID), nil, rule, nil)
}

func (c *Client) DeleteSecurityRule(ctx context.Context, policyID string, ruleID string) error {
	return c.do(ctx, http.MethodDelete, securityRulePath(policyID, ruleID), nil, nil, nil)
}

func (c *Client) PutSecurityRule(ctx context.Context, policyID string, ruleID string, rule *SecurityRule) (*SecurityRule, error) {
	return doTyped[SecurityRule](ctx, c, http.MethodPut, securityRulePath(policyID, ruleID), nil, rule)
}

func (c *Client) GetSecurityRule(ctx context.Context, policyID string, ruleID string) (*SecurityRule, error) {
	return doTyped[SecurityRule](ctx, c, http.MethodGet, securityRulePath(policyID, ruleID), nil, nil)
}

func (c *Client) ListSecurityRuleStats(ctx context.Context, policyID string, ruleID string) ([]*SecurityRuleStats, error) {
	return listAllTyped[SecurityRuleStats](ctx, c, securityRulePath(policyID, ruleID)+"/statistics", nil)
}

func (c *Client) ReviseSecurityRule(ctx context.Context, policyID string, ruleID string, rule *SecurityRule) (*SecurityRule, error) {
	return doTyped[SecurityRule](ctx, c, http.MethodPost, securityRulePath(policyID, ruleID), actionQuery("revise"), rule)
}

func (c *Client) ListInfraSegments(ctx context.Context) ([]*Segment, error) {
	return listAllTyped[Segment](ctx, c, "/policy/api/v1/infra/segments", nil)
}

func (c *Client) ListInfraSegmentStates(ctx context.Context) ([]*SegmentState, error) {
	return listAllTyped[SegmentState](ctx, c, "/policy/api/v1/infra/segments/state", nil)
}

func (c *Client) PutInfraSegment(ctx context.Context, segmentID string, segment *Segment) (*Segment, error) {
	return c.putSegment(ctx, infraSegmentPath(segmentID), segment)
}

func (c *Client) DeleteInfraSegment(ctx context.Context, segmentID string) error {
	return c.do(ctx, http.MethodDelete, infraSegmentPath(segmentID), nil, nil, nil)
}

func (c *Client) PatchInfraSegment(ctx context.Context, segmentID string, segment *Segment) error {
	return c.do(ctx, http.MethodPatch, infraSegmentPath(segmentID), nil, segment, nil)
}

func (c *Client) GetInfraSegment(ctx context.Context, segmentID string) (*Segment, error) {
	return c.getSegment(ctx, infraSegmentPath(segmentID))
}

func (c *Client) GetInfraSegmentState(ctx context.Context, segmentID string) (*SegmentState, error) {
	return doTyped[SegmentState](ctx, c, http.MethodGet, infraSegmentPath(segmentID)+"/state", nil, nil)
}

func (c *Client) GetInfraSegmentStatistics(ctx context.Context, segmentID string) (*SegmentStatistics, error) {
	return doTyped[SegmentStatistics](ctx, c, http.MethodGet, infraSegmentPath(segmentID)+"/statistics", nil, nil)
}

func (c *Client) ListTier0s(ctx context.Context) ([]*Tier0, error) {
	return listAllTyped[Tier0](ctx, c, "/policy/api/v1/infra/tier-0s", nil)
}

func (c *Client) ListTier1s(ctx context.Context) ([]*Tier1, error) {
	return listAllTyped[Tier1](ctx, c, "/policy/api/v1/infra/tier-1s", nil)
}

func (c *Client) GetTier1(ctx context.Context, tier1ID string) (*Tier1, error) {
	return doTyped[Tier1](ctx, c, http.MethodGet, tier1Path(tier1ID), nil, nil)
}

func (c *Client) GetTier1State(ctx context.Context, tier1ID string) (*Tier1State, error) {
	return doTyped[Tier1State](ctx, c, http.MethodGet, tier1Path(tier1ID)+"/state", nil, nil)
}

func (c *Client) ListTier1Segments(ctx context.Context, tier1ID string) ([]*Segment, error) {
	return listAllTyped[Segment](ctx, c, tier1Path(tier1ID)+"/segments", nil)
}

func (c *Client) ListTier1SegmentStates(ctx context.Context, tier1ID string) ([]*SegmentState, error) {
	return listAllTyped[SegmentState](ctx, c, tier1Path(tier1ID)+"/segments/state", nil)
}

func (c *Client) DeleteTier1Segment(ctx context.Context, tier1ID string, segmentID string) error {
	return c.do(ctx, http.MethodDelete, tier1SegmentPath(tier1ID, segmentID), nil, nil, nil)
}

func (c *Client) PatchTier1Segment(ctx context.Context, tier1ID string, segmentID string, segment *Segment) error {
	return c.do(ctx, http.MethodPatch, tier1SegmentPath(tier1ID, segmentID), nil, segment, nil)
}

func (c *Client) PutTier1Segment(ctx context.Context, tier1ID string, segmentID string, segment *Segment) (*Segment, error) {
	return c.putSegment(ctx, tier1SegmentPath(tier1ID, segmentID), segment)
}

func (c *Client) GetTier1Segment(ctx context.Context, tier1ID string, segmentID string) (*Segment, error) {
	return c.getSegment(ctx, tier1SegmentPath(tier1ID, segmentID))
}

func (c *Client) GetTier1SegmentState(ctx context.Context, tier1ID string, segmentID string) (*SegmentState, error) {
	return doTyped[SegmentState](ctx, c, http.MethodGet, tier1SegmentPath(tier1ID, segmentID)+"/state", nil, nil)
}

func (c *Client) GetTier1SegmentStatistics(ctx context.Context, tier1ID string, segmentID string) (*SegmentStatistics, error) {
	return doTyped[SegmentStatistics](ctx, c, http.MethodGet, tier1SegmentPath(tier1ID, segmentID)+"/statistics", nil, nil)
}

func (c *Client) GetGlobalTier1SegmentState(ctx context.Context, tier1ID string, segmentID string) (*SegmentState, error) {
	path := "/policy/api/v1/global-infra/tier-1s/" + pathEscape(tier1ID) + "/segments/" + pathEscape(segmentID) + "/state"
	return doTyped[SegmentState](ctx, c, http.MethodGet, path, nil, nil)
}

func (c *Client) GetGlobalTier1SegmentStatistics(ctx context.Context, tier1ID string, segmentID string) (*SegmentStatistics, error) {
	path := "/policy/api/v1/global-infra/tier-1s/" + pathEscape(tier1ID) + "/segments/" + pathEscape(segmentID) + "/statistics"
	return doTyped[SegmentStatistics](ctx, c, http.MethodGet, path, nil, nil)
}

func (c *Client) putSegment(ctx context.Context, path string, segment *Segment) (*Segment, error) {
	return doTyped[Segment](ctx, c, http.MethodPut, path, nil, segment)
}

func (c *Client) getSegment(ctx context.Context, path string) (*Segment, error) {
	return doTyped[Segment](ctx, c, http.MethodGet, path, nil, nil)
}

func defaultDomainPath() string {
	return "/policy/api/v1/infra/domains/" + defaultDomainID
}

func groupPath(groupID string) string {
	return defaultDomainPath() + "/groups/" + pathEscape(groupID)
}

func groupExpressionPath(groupID string, segment string, expressionID string) string {
	return groupPath(groupID) + "/" + segment + "/" + pathEscape(expressionID)
}

func securityPolicyPath(policyID string) string {
	return defaultDomainPath() + "/security-policies/" + pathEscape(policyID)
}

func securityRulePath(policyID string, ruleID string) string {
	return securityPolicyPath(policyID) + "/rules/" + pathEscape(ruleID)
}

func infraSegmentPath(segmentID string) string {
	return "/policy/api/v1/infra/segments/" + pathEscape(segmentID)
}

func tier1Path(tier1ID string) string {
	return "/policy/api/v1/infra/tier-1s/" + pathEscape(tier1ID)
}

func tier1SegmentPath(tier1ID string, segmentID string) string {
	return tier1Path(tier1ID) + "/segments/" + pathEscape(segmentID)
}

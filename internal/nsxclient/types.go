// Package nsxclient provides a typed client for the NSX-T Manager and Policy
// API routes used by this operator.
//
//nolint:revive,tagliatelle // DTO names and JSON fields intentionally mirror NSX API resources and wire fields.
package nsxclient

import "encoding/json"

type Resource struct {
	ID               string `json:"id,omitempty"`
	DisplayName      string `json:"display_name,omitempty"`
	Description      string `json:"description,omitempty"`
	ResourceType     string `json:"resource_type,omitempty"`
	Path             string `json:"path,omitempty"`
	ParentPath       string `json:"parent_path,omitempty"`
	RelativePath     string `json:"relative_path,omitempty"`
	Revision         int64  `json:"_revision"`
	CreateUser       string `json:"_create_user,omitempty"`
	LastModifiedUser string `json:"_last_modified_user,omitempty"`
	CreateTime       int64  `json:"_create_time,omitempty"`
	LastModifiedTime int64  `json:"_last_modified_time,omitempty"`
}

type GroupPatch struct {
	ID           string `json:"id,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
}

type Group struct {
	Resource
	GroupType          []string          `json:"group_type,omitempty"`
	Expression         []json.RawMessage `json:"expression,omitempty"`
	ExtendedExpression []json.RawMessage `json:"extended_expression,omitempty"`
	State              string            `json:"state,omitempty"`
}

type IPAddressExpression struct {
	Resource
	IPAddresses []string `json:"ip_addresses,omitempty"`
}

type IPAddressExpressionPatch struct {
	ID           string   `json:"id,omitempty"`
	ResourceType string   `json:"resource_type,omitempty"`
	IPAddresses  []string `json:"ip_addresses,omitempty"`
}

type PathExpression struct {
	Resource
	Paths []string `json:"paths,omitempty"`
}

type PathExpressionPatch struct {
	ID           string   `json:"id,omitempty"`
	ResourceType string   `json:"resource_type,omitempty"`
	Paths        []string `json:"paths,omitempty"`
}

type GroupMember struct {
	DisplayName string `json:"display_name,omitempty"`
	ID          string `json:"id,omitempty"`
	Path        string `json:"path,omitempty"`
}

type ConsolidatedEffectiveIPAddresses struct {
	Results     []string `json:"results,omitempty"`
	ResultCount int      `json:"result_count,omitempty"`
}

type FirewallSection struct {
	Resource
	SectionType string          `json:"section_type,omitempty"`
	Stateful    bool            `json:"stateful"`
	AppliedTos  []string        `json:"applied_tos,omitempty"`
	Rules       []*FirewallRule `json:"rules,omitempty"`
	RuleCount   int             `json:"rule_count,omitempty"`
}

type FirewallRule struct {
	Resource
	Action          string   `json:"action,omitempty"`
	Direction       string   `json:"direction,omitempty"`
	IPProtocol      string   `json:"ip_protocol,omitempty"`
	SequenceNumber  int      `json:"sequence_number,omitempty"`
	Sources         []string `json:"sources,omitempty"`
	Destinations    []string `json:"destinations,omitempty"`
	Services        []string `json:"services,omitempty"`
	AppliedTos      []string `json:"applied_tos,omitempty"`
	ContextProfiles []string `json:"context_profiles,omitempty"`
	ExtendedSources []string `json:"extended_sources,omitempty"`
	Notes           string   `json:"notes,omitempty"`
	RuleTag         string   `json:"rule_tag,omitempty"`
}

type FirewallRuleStats struct {
	SectionID  string          `json:"section_id,omitempty"`
	Statistics json.RawMessage `json:"statistics,omitempty"`
}

type IPSet struct {
	Resource
	IPAddresses []string `json:"ip_addresses,omitempty"`
}

type IPElement struct {
	IPAddress string `json:"ip_address,omitempty"`
}

type SecurityPolicy struct {
	Resource
	Category       string          `json:"category,omitempty"`
	SequenceNumber int             `json:"sequence_number,omitempty"`
	Scope          []string        `json:"scope,omitempty"`
	Rules          []*SecurityRule `json:"rules,omitempty"`
	RuleCount      int             `json:"rule_count,omitempty"`
}

type SecurityRule struct {
	Resource
	Action            string            `json:"action,omitempty"`
	Direction         string            `json:"direction,omitempty"`
	IPProtocol        string            `json:"ip_protocol,omitempty"`
	SequenceNumber    int               `json:"sequence_number,omitempty"`
	SourceGroups      []string          `json:"source_groups,omitempty"`
	DestinationGroups []string          `json:"destination_groups,omitempty"`
	Services          []string          `json:"services,omitempty"`
	Profiles          []string          `json:"profiles,omitempty"`
	Scope             []string          `json:"scope,omitempty"`
	ServiceEntries    []json.RawMessage `json:"service_entries,omitempty"`
	Notes             string            `json:"notes,omitempty"`
}

type SecurityPolicyStats struct {
	EnforcementPoint string          `json:"enforcement_point,omitempty"`
	Statistics       json.RawMessage `json:"statistics,omitempty"`
}

type SecurityRuleStats struct {
	Resource
}

type Segment struct {
	Resource
	ReplicationMode   string            `json:"replication_mode,omitempty"`
	AdminState        string            `json:"admin_state,omitempty"`
	Subnets           []json.RawMessage `json:"subnets,omitempty"`
	TransportZonePath string            `json:"transport_zone_path,omitempty"`
	ConnectivityPath  string            `json:"connectivity_path,omitempty"`
	DHCPConfigPath    string            `json:"dhcp_config_path,omitempty"`
	State             string            `json:"state,omitempty"`
}

type SegmentState struct {
	ID                  string `json:"id,omitempty"`
	Path                string `json:"path,omitempty"`
	RealizationStatus   string `json:"realization_status,omitempty"`
	ConsolidatedStatus  string `json:"consolidated_status,omitempty"`
	PublishStatus       string `json:"publish_status,omitempty"`
	LastUpdateTimestamp int64  `json:"last_update_timestamp,omitempty"`
}

type SegmentStatistics struct {
	LogicalSwitchID     string `json:"logical_switch_id,omitempty"`
	LastUpdateTimestamp int64  `json:"last_update_timestamp,omitempty"`
}

type Tier0 struct {
	Resource
}

type Tier1 struct {
	Resource
}

type Tier1State struct {
	ID                string `json:"id,omitempty"`
	Path              string `json:"path,omitempty"`
	RealizationStatus string `json:"realization_status,omitempty"`
}

type EULAAcceptance struct {
	Accepted   bool `json:"accepted,omitempty"`
	Acceptance bool `json:"acceptance,omitempty"`
}

type SearchResult struct {
	ResourceType string          `json:"resource_type,omitempty"`
	ID           string          `json:"id,omitempty"`
	DisplayName  string          `json:"display_name,omitempty"`
	Path         string          `json:"path,omitempty"`
	Raw          json.RawMessage `json:"-"`
}

func (result *SearchResult) UnmarshalJSON(data []byte) error {
	type searchResult SearchResult
	var decoded searchResult
	err := json.Unmarshal(data, &decoded)
	if err != nil {
		return err
	}
	decoded.Raw = append(decoded.Raw[:0], data...)
	*result = SearchResult(decoded)
	return nil
}

type SearchQueryOptions struct {
	Query     string
	PageSize  int
	Cursor    string
	SortBy    string
	Ascending *bool
	Fields    []string
}

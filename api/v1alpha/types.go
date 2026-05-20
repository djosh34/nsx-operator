// Package v1alpha defines the nsx.ing.com Kubernetes API.
package v1alpha

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// API constants identify the Kubernetes group and version.
const (
	// GroupName is the Kubernetes API group for NSX resources.
	GroupName = "nsx.ing.com"
	// Version is the Kubernetes API version for NSX resources.
	Version = "v1alpha"
)

// SchemeGroupVersion identifies the nsx.ing.com/v1alpha API.
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}

// NSXNetworkCloud describes one NSX manager endpoint.
type NSXNetworkCloud struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NSXNetworkCloudSpec   `json:"spec,omitempty"`
	Status            NSXNetworkCloudStatus `json:"status,omitempty"`
}

// NSXNetworkCloudList contains NSXNetworkCloud resources.
type NSXNetworkCloudList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NSXNetworkCloud `json:"items"`
}

// NSXNetworkCloudSpec is the desired state for an NSXNetworkCloud.
//
//nolint:recvcheck // Kubernetes deepcopy generation requires pointer receivers while NSXWritesEnabled is useful on literals.
type NSXNetworkCloudSpec struct {
	NetworkCloudFQDN string `json:"networkCloudFQDN"`
	NetworkCloudID   string `json:"networkCloudId"`
	Name             string `json:"name"`
	WritesEnabled    *bool  `json:"writesEnabled,omitempty"`
}

// NSXWritesEnabled reports whether writes are enabled for the network cloud.
func (in NSXNetworkCloudSpec) NSXWritesEnabled() bool {
	if in.WritesEnabled == nil {
		return true
	}
	return *in.WritesEnabled
}

// NSXNetworkCloudStatus is the observed state for an NSXNetworkCloud.
type NSXNetworkCloudStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NSXGroup describes one NSX policy group managed or observed by the operator.
type NSXGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NSXGroupSpec   `json:"spec,omitempty"`
	Status            NSXGroupStatus `json:"status,omitempty"`
}

// NSXGroupList contains NSXGroup resources.
type NSXGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NSXGroup `json:"items"`
}

// NSXGroupMode controls whether the operator observes or manages an NSX group.
type NSXGroupMode string

// NSXGroupMode constants identify supported group operating modes.
// Condition constants are Kubernetes condition type names used by NSX resources.
const (
	// NSXGroupModeObserve keeps the group read-only from the operator perspective.
	NSXGroupModeObserve NSXGroupMode = "Observe"
	// NSXGroupModeManage lets the operator write the NSX group.
	NSXGroupModeManage NSXGroupMode = "Manage"
)

// NSXGroupSpec is the desired state for an NSXGroup.
type NSXGroupSpec struct {
	NetworkCloudFQDN string       `json:"networkCloudFQDN"`
	GroupID          string       `json:"groupID"`
	DisplayName      string       `json:"display_name"`
	Mode             NSXGroupMode `json:"mode"`
	CIDRs            []string     `json:"cidrs"`
	SegmentPaths     []string     `json:"segment_paths,omitempty"`
}

// NSXGroupStatus is the observed state for an NSXGroup.
type NSXGroupStatus struct {
	UnsupportedReason UnsupportedExpressionReason `json:"unsupportedReason,omitempty"`
	Conditions        []metav1.Condition          `json:"conditions,omitempty"`
}

// UnsupportedExpressionReason constants describe unsupported expression states.
const (
	ConditionReachable             = "Reachable"
	ConditionSwept                 = "Swept"
	ConditionRemotePresent         = "RemotePresent"
	ConditionSpecMatchesRemote     = "SpecMatchesRemote"
	ConditionUnsupportedExpression = "UnsupportedExpression"
	ConditionRealized              = "Realized"
	ConditionSynced                = "Synced"
	ConditionApplying              = "Applying"
	ConditionDeleting              = "Deleting"
)

// UnsupportedExpressionReason explains why an NSX expression is unsupported.
type UnsupportedExpressionReason string

const (
	// UnsupportedExpressionReasonSupportedExpression means the expression is supported.
	UnsupportedExpressionReasonSupportedExpression UnsupportedExpressionReason = "SupportedExpression"
	// UnsupportedExpressionReasonUnsupportedExpressionType means the expression type is unsupported.
	UnsupportedExpressionReasonUnsupportedExpressionType UnsupportedExpressionReason = "UnsupportedExpressionType"
	// UnsupportedExpressionReasonMultipleIPAddressExpressions means multiple IP expressions were found.
	UnsupportedExpressionReasonMultipleIPAddressExpressions UnsupportedExpressionReason = "MultipleIPAddressExpressions"
	// UnsupportedExpressionReasonMultiplePathExpressions means multiple path expressions were found.
	UnsupportedExpressionReasonMultiplePathExpressions UnsupportedExpressionReason = "MultiplePathExpressions"
	// UnsupportedExpressionReasonInvalidIPAddressExpression means an IP expression was malformed.
	UnsupportedExpressionReasonInvalidIPAddressExpression UnsupportedExpressionReason = "InvalidIPAddressExpression"
	// UnsupportedExpressionReasonInvalidPathExpression means a path expression was malformed.
	UnsupportedExpressionReasonInvalidPathExpression UnsupportedExpressionReason = "InvalidPathExpression"
	// UnsupportedExpressionReasonUnsupportedIPAddressExpressionFields means unsupported IP expression fields were present.
	UnsupportedExpressionReasonUnsupportedIPAddressExpressionFields UnsupportedExpressionReason = "UnsupportedIPAddressExpressionFields"
	// UnsupportedExpressionReasonUnsupportedPathExpressionFields means unsupported path expression fields were present.
	UnsupportedExpressionReasonUnsupportedPathExpressionFields UnsupportedExpressionReason = "UnsupportedPathExpressionFields"
	// UnsupportedExpressionReasonUnsupportedNestedExpression means nested expressions are unsupported.
	UnsupportedExpressionReasonUnsupportedNestedExpression UnsupportedExpressionReason = "UnsupportedNestedExpression"
)

// SchemeBuilder registers v1alpha API types.
var SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

// AddToScheme registers v1alpha API types with a runtime scheme.
func AddToScheme(scheme *runtime.Scheme) error {
	return SchemeBuilder.AddToScheme(scheme)
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		SchemeGroupVersion,
		&NSXNetworkCloud{},
		&NSXNetworkCloudList{},
		&NSXGroup{},
		&NSXGroupList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

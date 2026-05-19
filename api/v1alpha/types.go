package v1alpha

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	GroupName = "nsx.ing.com"
	Version   = "v1alpha"
)

var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}

type NSXNetworkCloud struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NSXNetworkCloudSpec   `json:"spec,omitempty"`
	Status            NSXNetworkCloudStatus `json:"status,omitempty"`
}

type NSXNetworkCloudList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NSXNetworkCloud `json:"items"`
}

type NSXNetworkCloudSpec struct {
	NetworkCloudFQDN string `json:"networkCloudFQDN"`
	NetworkCloudID   string `json:"networkCloudId"`
	Name             string `json:"name"`
	WritesEnabled    *bool  `json:"writesEnabled,omitempty"`
}

func (in NSXNetworkCloudSpec) NSXWritesEnabled() bool {
	if in.WritesEnabled == nil {
		return true
	}
	return *in.WritesEnabled
}

type NSXNetworkCloudStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type NSXGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NSXGroupSpec   `json:"spec,omitempty"`
	Status            NSXGroupStatus `json:"status,omitempty"`
}

type NSXGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NSXGroup `json:"items"`
}

type NSXGroupMode string

const (
	NSXGroupModeObserve NSXGroupMode = "Observe"
	NSXGroupModeManage  NSXGroupMode = "Manage"
)

type NSXGroupSpec struct {
	NetworkCloudFQDN string       `json:"networkCloudFQDN"`
	GroupID          string       `json:"groupID"`
	DisplayName      string       `json:"display_name"`
	Mode             NSXGroupMode `json:"mode"`
	CIDRs            []string     `json:"cidrs"`
	SegmentPath      *string      `json:"segment_path,omitempty"`
}

type NSXGroupStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

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

var SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

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

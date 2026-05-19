package v1alpha

import (
	"encoding/json"
	"slices"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestAddToSchemeRegistersNetworkCloudAndGroupTypes(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme returned error: %v", err)
	}

	for _, tc := range []struct {
		name string
		gvk  schema.GroupVersionKind
	}{
		{
			name: "network cloud",
			gvk:  SchemeGroupVersion.WithKind("NSXNetworkCloud"),
		},
		{
			name: "network cloud list",
			gvk:  SchemeGroupVersion.WithKind("NSXNetworkCloudList"),
		},
		{
			name: "group",
			gvk:  SchemeGroupVersion.WithKind("NSXGroup"),
		},
		{
			name: "group list",
			gvk:  SchemeGroupVersion.WithKind("NSXGroupList"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			object, err := scheme.New(tc.gvk)
			if err != nil {
				t.Fatalf("scheme.New(%s) returned error: %v", tc.gvk.String(), err)
			}
			knownKinds, _, err := scheme.ObjectKinds(object)
			if err != nil {
				t.Fatalf("scheme.ObjectKinds(%T) returned error: %v", object, err)
			}
			if !slices.Contains(knownKinds, tc.gvk) {
				t.Fatalf("registered object kinds = %v, want one to be %s", knownKinds, tc.gvk.String())
			}
		})
	}
}

func TestDeepCopyObjectKeepsNetworkCloudAndGroupIndependent(t *testing.T) {
	writesEnabled := false
	group := &NSXGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "NSXGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   "nsx-a--app",
			Labels: map[string]string{"app": "demo"},
		},
		Spec: NSXGroupSpec{
			NetworkCloudFQDN: "nsx-a.example.net",
			GroupID:          "app",
			DisplayName:      "App",
			Mode:             NSXGroupModeManage,
			CIDRs:            []string{"10.0.0.0/24"},
			SegmentPaths:     []string{"/infra/segments/app", "/infra/segments/db"},
		},
		Status: NSXGroupStatus{
			Conditions: []metav1.Condition{{
				Type:    ConditionSynced,
				Status:  metav1.ConditionTrue,
				Reason:  "Matched",
				Message: "remote matches spec",
			}},
		},
	}

	groupCopyObject := group.DeepCopyObject()
	groupCopy, ok := groupCopyObject.(*NSXGroup)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *NSXGroup", groupCopyObject)
	}
	groupCopy.Labels["app"] = "changed"
	groupCopy.Spec.CIDRs[0] = "10.1.0.0/24"
	groupCopy.Spec.SegmentPaths[0] = "/infra/segments/other"
	groupCopy.Status.Conditions[0].Message = "changed"

	if group.Labels["app"] != "demo" {
		t.Fatalf("original group label mutated to %q", group.Labels["app"])
	}
	if group.Spec.CIDRs[0] != "10.0.0.0/24" {
		t.Fatalf("original group CIDR mutated to %q", group.Spec.CIDRs[0])
	}
	if !slices.Equal(group.Spec.SegmentPaths, []string{"/infra/segments/app", "/infra/segments/db"}) {
		t.Fatalf("original group segment paths mutated to %v", group.Spec.SegmentPaths)
	}
	if group.Status.Conditions[0].Message != "remote matches spec" {
		t.Fatalf("original group condition message mutated to %q", group.Status.Conditions[0].Message)
	}

	networkCloud := &NSXNetworkCloud{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "NSXNetworkCloud",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   "nsx-a",
			Labels: map[string]string{"cloud": "a"},
		},
		Spec: NSXNetworkCloudSpec{
			NetworkCloudFQDN: "nsx-a.example.net",
			NetworkCloudID:   "cloud-a",
			Name:             "Cloud A",
			WritesEnabled:    &writesEnabled,
		},
		Status: NSXNetworkCloudStatus{
			Conditions: []metav1.Condition{{
				Type:    ConditionReachable,
				Status:  metav1.ConditionTrue,
				Reason:  "Connected",
				Message: "manager is reachable",
			}},
		},
	}

	networkCloudCopyObject := networkCloud.DeepCopyObject()
	networkCloudCopy, ok := networkCloudCopyObject.(*NSXNetworkCloud)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *NSXNetworkCloud", networkCloudCopyObject)
	}
	networkCloudCopy.Labels["cloud"] = "changed"
	*networkCloudCopy.Spec.WritesEnabled = true
	networkCloudCopy.Status.Conditions[0].Reason = "Changed"

	if networkCloud.Labels["cloud"] != "a" {
		t.Fatalf("original network cloud label mutated to %q", networkCloud.Labels["cloud"])
	}
	if networkCloud.Spec.WritesEnabled == nil || *networkCloud.Spec.WritesEnabled {
		t.Fatalf("original network cloud writesEnabled mutated to %v", networkCloud.Spec.WritesEnabled)
	}
	if networkCloud.Status.Conditions[0].Reason != "Connected" {
		t.Fatalf("original network cloud condition reason mutated to %q", networkCloud.Status.Conditions[0].Reason)
	}
}

func TestNetworkCloudSpecWritesEnabledDefaultsTrueAndAllowsFalse(t *testing.T) {
	if !(NSXNetworkCloudSpec{}).NSXWritesEnabled() {
		t.Fatal("NSXWritesEnabled() = false for omitted field, want true")
	}

	disabled := false
	if (NSXNetworkCloudSpec{WritesEnabled: &disabled}).NSXWritesEnabled() {
		t.Fatal("NSXWritesEnabled() = true for explicit false, want false")
	}
}

func TestDeepCopyObjectKeepsNetworkCloudAndGroupListsIndependent(t *testing.T) {
	clouds := &NSXNetworkCloudList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "NSXNetworkCloudList",
		},
		Items: []NSXNetworkCloud{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "cloud-a",
				Labels: map[string]string{"cloud": "a"},
			},
			Status: NSXNetworkCloudStatus{
				Conditions: []metav1.Condition{{Type: ConditionReachable, Status: metav1.ConditionTrue}},
			},
		}},
	}
	cloudsCopyObject := clouds.DeepCopyObject()
	cloudsCopy, ok := cloudsCopyObject.(*NSXNetworkCloudList)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *NSXNetworkCloudList", cloudsCopyObject)
	}
	cloudsCopy.Items[0].Labels["cloud"] = "changed"
	cloudsCopy.Items[0].Status.Conditions[0].Status = metav1.ConditionFalse
	if clouds.Items[0].Labels["cloud"] != "a" {
		t.Fatalf("original network cloud list item label mutated to %q", clouds.Items[0].Labels["cloud"])
	}
	if clouds.Items[0].Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Fatalf("original network cloud list condition status mutated to %q", clouds.Items[0].Status.Conditions[0].Status)
	}

	groups := &NSXGroupList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "NSXGroupList",
		},
		Items: []NSXGroup{{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "group-a",
				Labels: map[string]string{"group": "a"},
			},
			Spec: NSXGroupSpec{
				CIDRs:        []string{"10.0.0.0/24"},
				SegmentPaths: []string{"/infra/segments/app", "/infra/segments/db"},
			},
		}},
	}
	groupsCopyObject := groups.DeepCopyObject()
	groupsCopy, ok := groupsCopyObject.(*NSXGroupList)
	if !ok {
		t.Fatalf("DeepCopyObject returned %T, want *NSXGroupList", groupsCopyObject)
	}
	groupsCopy.Items[0].Labels["group"] = "changed"
	groupsCopy.Items[0].Spec.CIDRs[0] = "10.1.0.0/24"
	groupsCopy.Items[0].Spec.SegmentPaths[0] = "/infra/segments/other"
	if groups.Items[0].Labels["group"] != "a" {
		t.Fatalf("original group list item label mutated to %q", groups.Items[0].Labels["group"])
	}
	if groups.Items[0].Spec.CIDRs[0] != "10.0.0.0/24" {
		t.Fatalf("original group list CIDR mutated to %q", groups.Items[0].Spec.CIDRs[0])
	}
	if !slices.Equal(groups.Items[0].Spec.SegmentPaths, []string{"/infra/segments/app", "/infra/segments/db"}) {
		t.Fatalf("original group list segment paths mutated to %v", groups.Items[0].Spec.SegmentPaths)
	}
}

func TestNilDeepCopyReturnsNil(t *testing.T) {
	var networkCloud *NSXNetworkCloud
	if networkCloud.DeepCopy() != nil {
		t.Fatalf("nil NSXNetworkCloud DeepCopy returned non-nil")
	}
	var networkCloudList *NSXNetworkCloudList
	if networkCloudList.DeepCopy() != nil {
		t.Fatalf("nil NSXNetworkCloudList DeepCopy returned non-nil")
	}
	var group *NSXGroup
	if group.DeepCopy() != nil {
		t.Fatalf("nil NSXGroup DeepCopy returned non-nil")
	}
	var groupList *NSXGroupList
	if groupList.DeepCopy() != nil {
		t.Fatalf("nil NSXGroupList DeepCopy returned non-nil")
	}
}

func TestJSONShapeUsesPublicAPIFieldNames(t *testing.T) {
	group := NSXGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "NSXGroup",
		},
		Spec: NSXGroupSpec{
			NetworkCloudFQDN: "nsx-a.example.net",
			GroupID:          "app",
			DisplayName:      "App",
			Mode:             NSXGroupModeObserve,
			CIDRs:            []string{"10.0.0.0/24"},
			SegmentPaths:     []string{"/infra/segments/app", "/infra/segments/db"},
		},
		Status: NSXGroupStatus{
			UnsupportedReason: UnsupportedExpressionReasonInvalidIPAddressExpression,
			Conditions: []metav1.Condition{{
				Type:    ConditionRemotePresent,
				Status:  metav1.ConditionTrue,
				Reason:  "Found",
				Message: "remote group exists",
			}},
		},
	}

	groupJSON, err := json.Marshal(group)
	if err != nil {
		t.Fatalf("marshal group: %v", err)
	}
	var groupMap map[string]any
	if err := json.Unmarshal(groupJSON, &groupMap); err != nil {
		t.Fatalf("unmarshal group as map: %v", err)
	}
	groupSpec, ok := groupMap["spec"].(map[string]any)
	if !ok {
		t.Fatalf("group spec decoded as %T, want object", groupMap["spec"])
	}
	for _, field := range []string{"networkCloudFQDN", "groupID", "display_name", "mode", "cidrs", "segment_paths"} {
		if _, ok := groupSpec[field]; !ok {
			t.Fatalf("group spec missing JSON field %q in %s", field, string(groupJSON))
		}
	}
	legacySegmentField := "segment" + "_path"
	if _, ok := groupSpec[legacySegmentField]; ok {
		t.Fatalf("group spec unexpectedly exposes %s in %s", legacySegmentField, string(groupJSON))
	}
	if _, ok := groupSpec["domainId"]; ok {
		t.Fatalf("group spec unexpectedly exposes domainId in %s", string(groupJSON))
	}
	groupStatus, ok := groupMap["status"].(map[string]any)
	if !ok {
		t.Fatalf("group status decoded as %T, want object", groupMap["status"])
	}
	if got, ok := groupStatus["unsupportedReason"].(string); !ok || got != string(UnsupportedExpressionReasonInvalidIPAddressExpression) {
		t.Fatalf("group status unsupportedReason = %#v, want %q in %s", groupStatus["unsupportedReason"], UnsupportedExpressionReasonInvalidIPAddressExpression, string(groupJSON))
	}

	var decodedGroup NSXGroup
	if err := json.Unmarshal(groupJSON, &decodedGroup); err != nil {
		t.Fatalf("unmarshal group into API type: %v", err)
	}
	if decodedGroup.Spec.DisplayName != group.Spec.DisplayName {
		t.Fatalf("decoded display_name = %q, want %q", decodedGroup.Spec.DisplayName, group.Spec.DisplayName)
	}
	if !slices.Equal(decodedGroup.Spec.SegmentPaths, group.Spec.SegmentPaths) {
		t.Fatalf("decoded segment_paths = %v, want %v", decodedGroup.Spec.SegmentPaths, group.Spec.SegmentPaths)
	}
	if len(decodedGroup.Status.Conditions) != 1 || decodedGroup.Status.Conditions[0].Type != ConditionRemotePresent {
		t.Fatalf("decoded conditions = %#v, want RemotePresent condition", decodedGroup.Status.Conditions)
	}
	if decodedGroup.Status.UnsupportedReason != UnsupportedExpressionReasonInvalidIPAddressExpression {
		t.Fatalf("decoded unsupportedReason = %q, want %q", decodedGroup.Status.UnsupportedReason, UnsupportedExpressionReasonInvalidIPAddressExpression)
	}

	networkCloud := NSXNetworkCloud{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "NSXNetworkCloud",
		},
		Spec: NSXNetworkCloudSpec{
			NetworkCloudFQDN: "nsx-a.example.net",
			NetworkCloudID:   "cloud-a",
			Name:             "Cloud A",
		},
	}
	networkCloudJSON, err := json.Marshal(networkCloud)
	if err != nil {
		t.Fatalf("marshal network cloud: %v", err)
	}
	var networkCloudMap map[string]any
	if err := json.Unmarshal(networkCloudJSON, &networkCloudMap); err != nil {
		t.Fatalf("unmarshal network cloud as map: %v", err)
	}
	networkCloudSpec, ok := networkCloudMap["spec"].(map[string]any)
	if !ok {
		t.Fatalf("network cloud spec decoded as %T, want object", networkCloudMap["spec"])
	}
	for _, field := range []string{"networkCloudFQDN", "networkCloudId", "name"} {
		if _, ok := networkCloudSpec[field]; !ok {
			t.Fatalf("network cloud spec missing JSON field %q in %s", field, string(networkCloudJSON))
		}
	}
	if _, ok := networkCloudSpec["domainId"]; ok {
		t.Fatalf("network cloud spec unexpectedly exposes domainId in %s", string(networkCloudJSON))
	}

	groupWithoutSegment := NSXGroup{Spec: NSXGroupSpec{SegmentPaths: nil}}
	groupWithoutSegmentJSON, err := json.Marshal(groupWithoutSegment)
	if err != nil {
		t.Fatalf("marshal group without segment paths: %v", err)
	}
	var groupWithoutSegmentMap map[string]any
	if err := json.Unmarshal(groupWithoutSegmentJSON, &groupWithoutSegmentMap); err != nil {
		t.Fatalf("unmarshal group without segment path as map: %v", err)
	}
	groupWithoutSegmentSpec, ok := groupWithoutSegmentMap["spec"].(map[string]any)
	if !ok {
		t.Fatalf("group without segment paths spec decoded as %T, want object", groupWithoutSegmentMap["spec"])
	}
	if _, ok := groupWithoutSegmentSpec["segment_paths"]; ok {
		t.Fatalf("nil segment_paths should be absent from JSON, got %s", string(groupWithoutSegmentJSON))
	}

	groupWithEmptySegments := NSXGroup{Spec: NSXGroupSpec{SegmentPaths: []string{}}}
	groupWithEmptySegmentsJSON, err := json.Marshal(groupWithEmptySegments)
	if err != nil {
		t.Fatalf("marshal group with empty segment paths: %v", err)
	}
	var groupWithEmptySegmentsMap map[string]any
	if err := json.Unmarshal(groupWithEmptySegmentsJSON, &groupWithEmptySegmentsMap); err != nil {
		t.Fatalf("unmarshal group with empty segment paths as map: %v", err)
	}
	groupWithEmptySegmentsSpec, ok := groupWithEmptySegmentsMap["spec"].(map[string]any)
	if !ok {
		t.Fatalf("group with empty segment paths spec decoded as %T, want object", groupWithEmptySegmentsMap["spec"])
	}
	if _, ok := groupWithEmptySegmentsSpec["segment_paths"]; ok {
		t.Fatalf("empty segment_paths should be absent from JSON, got %s", string(groupWithEmptySegmentsJSON))
	}

	groupWithoutUnsupportedReason := NSXGroup{Status: NSXGroupStatus{}}
	groupWithoutUnsupportedReasonJSON, err := json.Marshal(groupWithoutUnsupportedReason)
	if err != nil {
		t.Fatalf("marshal group without unsupported reason: %v", err)
	}
	var groupWithoutUnsupportedReasonMap map[string]any
	if err := json.Unmarshal(groupWithoutUnsupportedReasonJSON, &groupWithoutUnsupportedReasonMap); err != nil {
		t.Fatalf("unmarshal group without unsupported reason as map: %v", err)
	}
	groupWithoutUnsupportedReasonStatus, ok := groupWithoutUnsupportedReasonMap["status"].(map[string]any)
	if !ok {
		t.Fatalf("group without unsupported reason status decoded as %T, want object", groupWithoutUnsupportedReasonMap["status"])
	}
	if _, ok := groupWithoutUnsupportedReasonStatus["unsupportedReason"]; ok {
		t.Fatalf("empty unsupportedReason should be absent from JSON, got %s", string(groupWithoutUnsupportedReasonJSON))
	}
}

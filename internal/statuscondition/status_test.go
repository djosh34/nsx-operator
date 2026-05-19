package statuscondition_test

import (
	"reflect"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/statuscondition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildGroupStatusPreservesTransitionTimeWhenStatusDoesNotChange(t *testing.T) {
	oldTime := time.Date(2026, 5, 19, 8, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 5, 19, 9, 0, 0, 0, time.UTC)
	previous := nsxv1alpha.NSXGroupStatus{Conditions: []metav1.Condition{{
		Type:               nsxv1alpha.ConditionRemotePresent,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 7,
		LastTransitionTime: metav1.NewTime(oldTime),
		Reason:             "OldReason",
		Message:            "old message",
	}}}

	status, err := statuscondition.BuildGroupStatus(
		previous,
		9,
		newTime,
		statuscondition.RemotePresent(metav1.ConditionTrue, "RemoteFound", "remote NSX group is present"),
	)
	if err != nil {
		t.Fatalf("BuildGroupStatus() error = %v", err)
	}

	if len(status.Conditions) != 1 {
		t.Fatalf("conditions = %#v, want only RemotePresent", status.Conditions)
	}
	condition := status.Conditions[0]
	if condition.Type != nsxv1alpha.ConditionRemotePresent {
		t.Fatalf("condition type = %s, want RemotePresent", condition.Type)
	}
	if condition.Status != metav1.ConditionTrue {
		t.Fatalf("RemotePresent status = %s, want True", condition.Status)
	}
	if condition.ObservedGeneration != 9 {
		t.Fatalf("ObservedGeneration = %d, want 9", condition.ObservedGeneration)
	}
	if !condition.LastTransitionTime.Time.Equal(oldTime) {
		t.Fatalf("LastTransitionTime = %s, want preserved %s", condition.LastTransitionTime.Time, oldTime)
	}
	if condition.Reason != "RemoteFound" || condition.Message != "remote NSX group is present" {
		t.Fatalf("condition reason/message = %s/%q, want updated reason/message", condition.Reason, condition.Message)
	}
}

func TestBuildGroupStatusUpdatesTransitionTimeWhenStatusChanges(t *testing.T) {
	oldTime := time.Date(2026, 5, 19, 8, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 5, 19, 9, 0, 0, 0, time.UTC)
	previous := nsxv1alpha.NSXGroupStatus{Conditions: []metav1.Condition{{
		Type:               nsxv1alpha.ConditionRemotePresent,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 7,
		LastTransitionTime: metav1.NewTime(oldTime),
		Reason:             "RemoteFound",
		Message:            "remote NSX group is present",
	}}}

	status, err := statuscondition.BuildGroupStatus(
		previous,
		10,
		newTime,
		statuscondition.RemotePresent(metav1.ConditionFalse, "RemoteMissing", "remote NSX group is missing"),
	)
	if err != nil {
		t.Fatalf("BuildGroupStatus() error = %v", err)
	}

	if len(status.Conditions) != 1 {
		t.Fatalf("conditions = %#v, want only RemotePresent", status.Conditions)
	}
	condition := status.Conditions[0]
	if condition.Status != metav1.ConditionFalse {
		t.Fatalf("RemotePresent status = %s, want False", condition.Status)
	}
	if condition.ObservedGeneration != 10 {
		t.Fatalf("ObservedGeneration = %d, want 10", condition.ObservedGeneration)
	}
	if !condition.LastTransitionTime.Time.Equal(newTime) {
		t.Fatalf("LastTransitionTime = %s, want new transition %s", condition.LastTransitionTime.Time, newTime)
	}
	if condition.Reason != "RemoteMissing" || condition.Message != "remote NSX group is missing" {
		t.Fatalf("condition reason/message = %s/%q, want updated missing remote reason/message", condition.Reason, condition.Message)
	}
}

func TestBuildGroupStatusCarriesUnsupportedReasonWhenConditionIsTrue(t *testing.T) {
	now := time.Date(2026, 5, 19, 9, 30, 0, 0, time.UTC)
	status, err := statuscondition.BuildGroupStatus(
		nsxv1alpha.NSXGroupStatus{},
		11,
		now,
		statuscondition.UnsupportedExpression(
			metav1.ConditionTrue,
			string(nsxv1alpha.UnsupportedExpressionReasonInvalidPathExpression),
			"remote NSX group expression is not fully representable: InvalidPathExpression",
		),
	)
	if err != nil {
		t.Fatalf("BuildGroupStatus() error = %v", err)
	}
	if status.UnsupportedReason != nsxv1alpha.UnsupportedExpressionReasonInvalidPathExpression {
		t.Fatalf("UnsupportedReason = %q, want %q", status.UnsupportedReason, nsxv1alpha.UnsupportedExpressionReasonInvalidPathExpression)
	}

	cleared, err := statuscondition.BuildGroupStatus(
		status,
		12,
		now.Add(time.Minute),
		statuscondition.UnsupportedExpression(
			metav1.ConditionFalse,
			string(nsxv1alpha.UnsupportedExpressionReasonSupportedExpression),
			"remote NSX group expression is representable",
		),
	)
	if err != nil {
		t.Fatalf("BuildGroupStatus() clearing error = %v", err)
	}
	if cleared.UnsupportedReason != "" {
		t.Fatalf("UnsupportedReason = %q after supported condition, want empty", cleared.UnsupportedReason)
	}
}

func TestSyncedDerivesStatusFromRequiredConditionStatuses(t *testing.T) {
	tests := []struct {
		name                  string
		remotePresent         metav1.ConditionStatus
		specMatchesRemote     metav1.ConditionStatus
		unsupportedExpression metav1.ConditionStatus
		realized              metav1.ConditionStatus
		want                  metav1.ConditionStatus
	}{
		{
			name:                  "all required inputs synced",
			remotePresent:         metav1.ConditionTrue,
			specMatchesRemote:     metav1.ConditionTrue,
			unsupportedExpression: metav1.ConditionFalse,
			realized:              metav1.ConditionTrue,
			want:                  metav1.ConditionTrue,
		},
		{
			name:                  "remote missing is known unsynced",
			remotePresent:         metav1.ConditionFalse,
			specMatchesRemote:     metav1.ConditionTrue,
			unsupportedExpression: metav1.ConditionFalse,
			realized:              metav1.ConditionTrue,
			want:                  metav1.ConditionFalse,
		},
		{
			name:                  "spec mismatch is known unsynced",
			remotePresent:         metav1.ConditionTrue,
			specMatchesRemote:     metav1.ConditionFalse,
			unsupportedExpression: metav1.ConditionFalse,
			realized:              metav1.ConditionTrue,
			want:                  metav1.ConditionFalse,
		},
		{
			name:                  "unsupported expression is known unsynced",
			remotePresent:         metav1.ConditionTrue,
			specMatchesRemote:     metav1.ConditionTrue,
			unsupportedExpression: metav1.ConditionTrue,
			realized:              metav1.ConditionTrue,
			want:                  metav1.ConditionFalse,
		},
		{
			name:                  "realization failure is known unsynced",
			remotePresent:         metav1.ConditionTrue,
			specMatchesRemote:     metav1.ConditionTrue,
			unsupportedExpression: metav1.ConditionFalse,
			realized:              metav1.ConditionFalse,
			want:                  metav1.ConditionFalse,
		},
		{
			name:                  "unknown realization keeps synced unknown",
			remotePresent:         metav1.ConditionTrue,
			specMatchesRemote:     metav1.ConditionTrue,
			unsupportedExpression: metav1.ConditionFalse,
			realized:              metav1.ConditionUnknown,
			want:                  metav1.ConditionUnknown,
		},
		{
			name:                  "unknown remote with no known false input keeps synced unknown",
			remotePresent:         metav1.ConditionUnknown,
			specMatchesRemote:     metav1.ConditionTrue,
			unsupportedExpression: metav1.ConditionFalse,
			realized:              metav1.ConditionTrue,
			want:                  metav1.ConditionUnknown,
		},
		{
			name:                  "known unsupported expression overrides unknown realization",
			remotePresent:         metav1.ConditionTrue,
			specMatchesRemote:     metav1.ConditionTrue,
			unsupportedExpression: metav1.ConditionTrue,
			realized:              metav1.ConditionUnknown,
			want:                  metav1.ConditionFalse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update := statuscondition.Synced(
				tt.remotePresent,
				tt.specMatchesRemote,
				tt.unsupportedExpression,
				tt.realized,
				"Derived",
				"synced status is derived from condition inputs",
			)
			if update.Type != nsxv1alpha.ConditionSynced {
				t.Fatalf("Synced() type = %s, want Synced", update.Type)
			}
			if update.Status != tt.want {
				t.Fatalf("Synced() status = %s, want %s", update.Status, tt.want)
			}
			if update.Reason != "Derived" || update.Message != "synced status is derived from condition inputs" {
				t.Fatalf("Synced() reason/message = %s/%q, want caller-provided text", update.Reason, update.Message)
			}
		})
	}
}

func TestBuildGroupStatusOrdersAllSupportedConditionsAndPreservesUnknown(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)

	status, err := statuscondition.BuildGroupStatus(
		nsxv1alpha.NSXGroupStatus{},
		12,
		now,
		statuscondition.Deleting(metav1.ConditionFalse, "NotDeleting", "group is not being deleted"),
		statuscondition.Synced(
			metav1.ConditionTrue,
			metav1.ConditionTrue,
			metav1.ConditionFalse,
			metav1.ConditionUnknown,
			"RealizationPending",
			"remote realization is still pending",
		),
		statuscondition.RemotePresent(metav1.ConditionTrue, "RemoteFound", "remote NSX group is present"),
		statuscondition.Applying(metav1.ConditionFalse, "NotApplying", "no NSX write is planned"),
		statuscondition.Realized(metav1.ConditionUnknown, "RealizationPending", "remote realization is still pending"),
		statuscondition.SpecMatchesRemote(metav1.ConditionTrue, "SpecMatches", "local group spec matches remote NSX group"),
		statuscondition.UnsupportedExpression(metav1.ConditionFalse, "SupportedExpression", "remote NSX group expression is representable"),
	)
	if err != nil {
		t.Fatalf("BuildGroupStatus() error = %v", err)
	}

	gotTypes := conditionTypes(status.Conditions)
	wantTypes := []string{
		nsxv1alpha.ConditionRemotePresent,
		nsxv1alpha.ConditionSpecMatchesRemote,
		nsxv1alpha.ConditionUnsupportedExpression,
		nsxv1alpha.ConditionRealized,
		nsxv1alpha.ConditionSynced,
		nsxv1alpha.ConditionApplying,
		nsxv1alpha.ConditionDeleting,
	}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("condition order = %v, want %v", gotTypes, wantTypes)
	}
	for _, condition := range status.Conditions {
		if condition.ObservedGeneration != 12 {
			t.Fatalf("%s ObservedGeneration = %d, want 12", condition.Type, condition.ObservedGeneration)
		}
		if !condition.LastTransitionTime.Time.Equal(now) {
			t.Fatalf("%s LastTransitionTime = %s, want %s", condition.Type, condition.LastTransitionTime.Time, now)
		}
	}
	requireStatusCondition(t, status.Conditions, nsxv1alpha.ConditionRealized, metav1.ConditionUnknown)
	requireStatusCondition(t, status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown)
}

func TestBuildNetworkCloudStatusOrdersCloudConditions(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 15, 0, 0, time.UTC)

	status, err := statuscondition.BuildNetworkCloudStatus(
		nsxv1alpha.NSXNetworkCloudStatus{},
		5,
		now,
		statuscondition.Swept(metav1.ConditionTrue, "SweepComplete", "manager sweep completed"),
		statuscondition.Reachable(metav1.ConditionTrue, "Reachable", "NSX manager is reachable"),
	)
	if err != nil {
		t.Fatalf("BuildNetworkCloudStatus() error = %v", err)
	}

	gotTypes := conditionTypes(status.Conditions)
	wantTypes := []string{nsxv1alpha.ConditionReachable, nsxv1alpha.ConditionSwept}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("condition order = %v, want %v", gotTypes, wantTypes)
	}
	for _, condition := range status.Conditions {
		if condition.ObservedGeneration != 5 {
			t.Fatalf("%s ObservedGeneration = %d, want 5", condition.Type, condition.ObservedGeneration)
		}
		if !condition.LastTransitionTime.Time.Equal(now) {
			t.Fatalf("%s LastTransitionTime = %s, want %s", condition.Type, condition.LastTransitionTime.Time, now)
		}
	}
}

func conditionTypes(conditions []metav1.Condition) []string {
	types := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		types = append(types, condition.Type)
	}
	return types
}

func requireStatusCondition(t *testing.T, conditions []metav1.Condition, conditionType string, status metav1.ConditionStatus) {
	t.Helper()

	for _, condition := range conditions {
		if condition.Type == conditionType {
			if condition.Status != status {
				t.Fatalf("%s status = %s, want %s", conditionType, condition.Status, status)
			}
			return
		}
	}
	t.Fatalf("%s condition not found in %#v", conditionType, conditions)
}

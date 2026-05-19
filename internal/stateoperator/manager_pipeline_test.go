package stateoperator_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/names"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
	"github.com/djosh34/nsx-operator/internal/operatormetrics"
	"github.com/djosh34/nsx-operator/internal/stateoperator"
	"github.com/djosh34/nsx-operator/internal/statuscondition"
	"github.com/djosh34/nsx-operator/internal/testsupport/mockapi"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestProcessManagerSnapshotGatherFailureOnlyPlansCloudStatus(t *testing.T) {
	oldTime := time.Date(2026, 5, 19, 11, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	cloud := networkCloud("cloud-a", "nsx-a.example.test")
	cloud.Generation = 4
	cloud.Status = nsxv1alpha.NSXNetworkCloudStatus{Conditions: []metav1.Condition{
		{
			Type:               nsxv1alpha.ConditionReachable,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: 3,
			LastTransitionTime: metav1.NewTime(oldTime),
			Reason:             "PreviousFailure",
			Message:            "previous failure",
		},
		{
			Type:               nsxv1alpha.ConditionSwept,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: 3,
			LastTransitionTime: metav1.NewTime(oldTime),
			Reason:             "PreviousSuccess",
			Message:            "previous success",
		},
	}}
	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *cloud,
		NetworkCloudFQDN: "nsx-a.example.test",
		LocalGroups: []nsxv1alpha.NSXGroup{
			*managerGroup("managed-a", "nsx-a.example.test", "managed-a", nsxv1alpha.NSXGroupModeManage),
			*managerGroup("observe-a", "nsx-a.example.test", "observe-a", nsxv1alpha.NSXGroupModeObserve),
		},
		GatherError: errors.New("nsx unavailable"),
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.ObserveUpserts) != 0 {
		t.Fatalf("ObserveUpserts = %#v, want empty on gather failure", plan.ObserveUpserts)
	}
	if len(plan.ManagedWrites) != 0 {
		t.Fatalf("ManagedWrites = %#v, want empty on gather failure", plan.ManagedWrites)
	}
	if len(plan.ManagedDeletes) != 0 {
		t.Fatalf("ManagedDeletes = %#v, want empty on gather failure", plan.ManagedDeletes)
	}
	if len(plan.GroupStatuses) != 0 {
		t.Fatalf("GroupStatuses = %#v, want empty on gather failure", plan.GroupStatuses)
	}
	if len(plan.ObserveDeletes) != 0 {
		t.Fatalf("ObserveDeletes = %#v, want empty on gather failure", plan.ObserveDeletes)
	}
	if plan.CloudStatus == nil {
		t.Fatal("CloudStatus = nil, want failed cloud status")
	}
	gotConditions := plan.CloudStatus.Status.Conditions
	if len(gotConditions) != 2 {
		t.Fatalf("cloud status conditions = %#v, want Reachable and Swept", gotConditions)
	}
	requireCondition(t, gotConditions, nsxv1alpha.ConditionReachable, metav1.ConditionFalse, "GatherFailed", "nsx unavailable", oldTime)
	requireCondition(t, gotConditions, nsxv1alpha.ConditionSwept, metav1.ConditionFalse, "GatherFailed", "nsx unavailable", now)
	requireObservedGeneration(t, gotConditions, nsxv1alpha.ConditionReachable, 4)
	requireObservedGeneration(t, gotConditions, nsxv1alpha.ConditionSwept, 4)
}

func TestBuildBindingsSortsDeterministicallyAndRejectsDuplicates(t *testing.T) {
	snapshot := stateoperator.ManagerSnapshot{
		NetworkCloudFQDN: "nsx-a.example.test",
		LocalGroups: []nsxv1alpha.NSXGroup{
			*managerGroup("z-local", "nsx-a.example.test", "group-z", nsxv1alpha.NSXGroupModeManage),
			*managerGroup("a-local", "nsx-a.example.test", "group-a", nsxv1alpha.NSXGroupModeObserve),
		},
		RemoteGroups: []stateoperator.RemoteGroup{
			{Key: stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "group-z"}, DisplayName: "Remote Z"},
			{Key: stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "group-a"}, DisplayName: "Remote A"},
		},
	}
	bindings, err := stateoperator.BuildBindings(snapshot)
	if err != nil {
		t.Fatalf("BuildBindings() error = %v", err)
	}
	gotLocalNames := []string{bindings.Local[0].Group.Name, bindings.Local[1].Group.Name}
	if want := []string{"a-local", "z-local"}; !reflect.DeepEqual(gotLocalNames, want) {
		t.Fatalf("local binding order = %v, want %v", gotLocalNames, want)
	}
	gotRemoteIDs := []string{bindings.Remote[0].Remote.Key.GroupID, bindings.Remote[1].Remote.Key.GroupID}
	if want := []string{"group-a", "group-z"}; !reflect.DeepEqual(gotRemoteIDs, want) {
		t.Fatalf("remote binding order = %v, want %v", gotRemoteIDs, want)
	}

	duplicateLocal := snapshot
	duplicateLocal.LocalGroups = append(duplicateLocal.LocalGroups, *managerGroup("dupe-local", "nsx-a.example.test", "group-a", nsxv1alpha.NSXGroupModeManage))
	_, err = stateoperator.BuildBindings(duplicateLocal)
	if err == nil || !strings.Contains(err.Error(), "duplicate local binding") {
		t.Fatalf("BuildBindings() duplicate local error = %v, want duplicate local binding", err)
	}

	duplicateRemote := snapshot
	duplicateRemote.RemoteGroups = append(duplicateRemote.RemoteGroups, stateoperator.RemoteGroup{
		Key:         stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "group-a"},
		DisplayName: "Remote A Duplicate",
	})
	_, err = stateoperator.BuildBindings(duplicateRemote)
	if err == nil || !strings.Contains(err.Error(), "duplicate remote binding") {
		t.Fatalf("BuildBindings() duplicate remote error = %v, want duplicate remote binding", err)
	}
}

func TestProcessManagerSnapshotSuccessfulGatherPlansCloudStatus(t *testing.T) {
	oldTime := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 19, 12, 15, 0, 0, time.UTC)
	cloud := networkCloud("cloud-a", "nsx-a.example.test")
	cloud.Generation = 6
	cloud.Status = nsxv1alpha.NSXNetworkCloudStatus{Conditions: []metav1.Condition{{
		Type:               nsxv1alpha.ConditionReachable,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 5,
		LastTransitionTime: metav1.NewTime(oldTime),
		Reason:             "PreviousReachable",
		Message:            "previous success",
	}}}

	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *cloud,
		NetworkCloudFQDN: "nsx-a.example.test",
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if plan.CloudStatus == nil {
		t.Fatal("CloudStatus = nil, want successful cloud status")
	}
	gotConditions := plan.CloudStatus.Status.Conditions
	if len(gotConditions) != 2 {
		t.Fatalf("cloud status conditions = %#v, want Reachable and Swept", gotConditions)
	}
	requireCondition(t, gotConditions, nsxv1alpha.ConditionReachable, metav1.ConditionTrue, "GatherSucceeded", "NSX manager gather completed", oldTime)
	requireCondition(t, gotConditions, nsxv1alpha.ConditionSwept, metav1.ConditionTrue, "SweepPlanned", "manager snapshot was processed", now)
	requireObservedGeneration(t, gotConditions, nsxv1alpha.ConditionReachable, 6)
	requireObservedGeneration(t, gotConditions, nsxv1alpha.ConditionSwept, 6)
}

func TestProcessManagerSnapshotSkipsAlreadyCorrectCloudStatus(t *testing.T) {
	oldTime := time.Date(2026, 5, 19, 12, 20, 0, 0, time.UTC)
	now := time.Date(2026, 5, 19, 12, 25, 0, 0, time.UTC)
	cloud := networkCloud("cloud-a", "nsx-a.example.test")
	cloud.Generation = 6
	cloud.Status = alreadySweptCloudStatus(t, cloud.Generation, oldTime)

	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *cloud,
		NetworkCloudFQDN: "nsx-a.example.test",
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if plan.CloudStatus != nil {
		t.Fatalf("CloudStatus = %#v, want nil for already-correct cloud status", plan.CloudStatus)
	}
}

func TestProcessManagerSnapshotImportsRemoteOnlyGroupsAsObserveUpserts(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 30, 0, 0, time.UTC)
	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "NSX-A.Example.Test:8443"),
		NetworkCloudFQDN: "nsx-a.example.test:8443",
		RemoteGroups: []stateoperator.RemoteGroup{{
			Key:          stateoperator.BindingKey{NetworkCloudFQDN: "NSX-A.Example.Test:8443", GroupID: "App/Web_GROUP"},
			DisplayName:  "App Web",
			CIDRs:        []string{"10.20.0.0/24", "10.20.1.0/24"},
			SegmentPaths: []string{"/infra/segments/web", "/infra/segments/db"},
		}},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.ObserveUpserts) != 1 {
		t.Fatalf("ObserveUpserts = %#v, want one remote-only import", plan.ObserveUpserts)
	}
	upsert := plan.ObserveUpserts[0]
	createKey := kubeapi.BatchKey{Operation: "create", Resource: "nsxgroups", Name: "nsx-a.example.test-8443-app-web-group"}
	createRequest, found := plan.KubeWrites.GroupCreates[createKey]
	if !found {
		t.Fatalf("GroupCreates = %#v, want typed create request for remote-only import", plan.KubeWrites.GroupCreates)
	}
	if createRequest.Object == nil || createRequest.Object.Name != "nsx-a.example.test-8443-app-web-group" {
		t.Fatalf("GroupCreateRequest = %#v, want import object", createRequest)
	}
	if upsert.Name != "nsx-a.example.test-8443-app-web-group" {
		t.Fatalf("Observe upsert name = %q, want deterministic cloud/group name", upsert.Name)
	}
	if errs := validation.IsDNS1123Subdomain(upsert.Name); len(errs) != 0 {
		t.Fatalf("Observe upsert name = %q, Kubernetes validation errors = %v", upsert.Name, errs)
	}
	if len(upsert.Finalizers) != 0 {
		t.Fatalf("Observe upsert finalizers = %v, want none", upsert.Finalizers)
	}
	wantSpec := nsxv1alpha.NSXGroupSpec{
		NetworkCloudFQDN: "nsx-a.example.test:8443",
		GroupID:          "App/Web_GROUP",
		DisplayName:      "App Web",
		Mode:             nsxv1alpha.NSXGroupModeObserve,
		CIDRs:            []string{"10.20.0.0/24", "10.20.1.0/24"},
		SegmentPaths:     []string{"/infra/segments/web", "/infra/segments/db"},
	}
	if !reflect.DeepEqual(upsert.Spec, wantSpec) {
		t.Fatalf("Observe upsert spec = %#v, want %#v", upsert.Spec, wantSpec)
	}
	if len(plan.GroupStatuses) != 1 {
		t.Fatalf("GroupStatuses = %#v, want one status update", plan.GroupStatuses)
	}
	pendingStatus, found := plan.KubeWrites.GroupStatusesAfterGroupWrite[createKey]
	if !found {
		t.Fatalf("GroupStatusesAfterGroupWrite = %#v, want status tied to create result", plan.KubeWrites.GroupStatusesAfterGroupWrite)
	}
	if pendingStatus.Name != "nsx-a.example.test-8443-app-web-group" {
		t.Fatalf("pending status name = %q, want import name", pendingStatus.Name)
	}
	if len(plan.KubeWrites.GroupStatusUpdates) != 0 {
		t.Fatalf("GroupStatusUpdates = %#v, want no direct status without gathered resource version", plan.KubeWrites.GroupStatusUpdates)
	}
	if plan.GroupStatuses[0].Name != "nsx-a.example.test-8443-app-web-group" {
		t.Fatalf("Group status name = %q, want observe upsert name", plan.GroupStatuses[0].Name)
	}
	if plan.GroupStatuses[0].Status.UnsupportedReason != "" {
		t.Fatalf("UnsupportedReason = %q for supported remote expression, want empty", plan.GroupStatuses[0].Status.UnsupportedReason)
	}
	requireConditionTypes(t, plan.GroupStatuses[0].Status.Conditions, []string{
		nsxv1alpha.ConditionRemotePresent,
		nsxv1alpha.ConditionSpecMatchesRemote,
		nsxv1alpha.ConditionUnsupportedExpression,
		nsxv1alpha.ConditionRealized,
		nsxv1alpha.ConditionSynced,
		nsxv1alpha.ConditionApplying,
		nsxv1alpha.ConditionDeleting,
	})
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionRemotePresent, metav1.ConditionTrue, "RemoteFound", "remote NSX group is present", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionSpecMatchesRemote, metav1.ConditionTrue, "SpecMatches", "local group spec matches remote NSX group", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionUnsupportedExpression, metav1.ConditionFalse, "SupportedExpression", "remote NSX group expression is representable", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionRealized, metav1.ConditionTrue, "Realized", "remote NSX group is realized", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionTrue, "Synced", "local group reflects remote NSX group", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionApplying, metav1.ConditionFalse, "NotApplying", "no NSX write is planned", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionDeleting, metav1.ConditionFalse, "NotDeleting", "no NSX delete is planned", now)
}

func TestProcessManagerSnapshotRemoteOnlyUnsupportedExpressionMarksUnsynced(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 45, 0, 0, time.UTC)
	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
		NetworkCloudFQDN: "nsx-a.example.test",
		RemoteGroups: []stateoperator.RemoteGroup{{
			Key:               stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-unsupported"},
			DisplayName:       "Unsupported App",
			UnsupportedReason: nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression,
		}},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.GroupStatuses) != 1 {
		t.Fatalf("GroupStatuses = %#v, want one unsupported status", plan.GroupStatuses)
	}
	if plan.GroupStatuses[0].Status.UnsupportedReason != nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression {
		t.Fatalf("UnsupportedReason = %q, want %q", plan.GroupStatuses[0].Status.UnsupportedReason, nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression)
	}
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionUnsupportedExpression, metav1.ConditionTrue, string(nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression), "remote NSX group expression is not fully representable: UnsupportedNestedExpression", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionFalse, string(nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression), "remote NSX group expression needs operator support before it can be synced: UnsupportedNestedExpression", now)
}

func TestProcessManagerSnapshotObserveGroupsMirrorRemoteAndDeleteWhenMissing(t *testing.T) {
	now := time.Date(2026, 5, 19, 13, 0, 0, 0, time.UTC)
	drifted := managerGroup("observe-drifted", "nsx-a.example.test", "app-drifted", nsxv1alpha.NSXGroupModeObserve)
	drifted.ResourceVersion = "rv-drifted"
	drifted.Spec.DisplayName = "Old App"
	drifted.Spec.CIDRs = []string{"10.30.0.0/24"}
	drifted.Finalizers = []string{"example.test/keep"}
	missing := managerGroup("observe-missing", "nsx-a.example.test", "app-missing", nsxv1alpha.NSXGroupModeObserve)
	missing.ResourceVersion = "rv-missing"

	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
		NetworkCloudFQDN: "nsx-a.example.test",
		LocalGroups:      []nsxv1alpha.NSXGroup{*missing, *drifted},
		RemoteGroups: []stateoperator.RemoteGroup{{
			Key:         stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-drifted"},
			DisplayName: "Remote App",
			CIDRs:       []string{"10.31.0.0/24"},
		}},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.ObserveUpserts) != 1 {
		t.Fatalf("ObserveUpserts = %#v, want one drift repair", plan.ObserveUpserts)
	}
	if plan.ObserveUpserts[0].Name != "observe-drifted" {
		t.Fatalf("ObserveUpsert name = %q, want existing CR name", plan.ObserveUpserts[0].Name)
	}
	if len(plan.ObserveUpserts[0].Finalizers) != 0 {
		t.Fatalf("ObserveUpsert finalizers = %v, want none", plan.ObserveUpserts[0].Finalizers)
	}
	if plan.ObserveUpserts[0].Spec.DisplayName != "Remote App" || !reflect.DeepEqual(plan.ObserveUpserts[0].Spec.CIDRs, []string{"10.31.0.0/24"}) {
		t.Fatalf("ObserveUpsert spec = %#v, want remote replacement spec", plan.ObserveUpserts[0].Spec)
	}
	updateKey := kubeapi.BatchKey{Operation: "update", Resource: "nsxgroups", Name: "observe-drifted"}
	updateRequest, found := plan.KubeWrites.GroupUpdates[updateKey]
	if !found {
		t.Fatalf("GroupUpdates = %#v, want typed update for drifted observe group", plan.KubeWrites.GroupUpdates)
	}
	if updateRequest.Object.ResourceVersion != "rv-drifted" {
		t.Fatalf("update resourceVersion = %q, want gathered rv-drifted", updateRequest.Object.ResourceVersion)
	}
	if !reflect.DeepEqual(updateRequest.Object.Finalizers, []string{"example.test/keep"}) {
		t.Fatalf("update finalizers = %#v, want gathered unrelated finalizer preserved", updateRequest.Object.Finalizers)
	}
	if updateRequest.Object.Spec.DisplayName != "Remote App" {
		t.Fatalf("update spec displayName = %q, want remote value", updateRequest.Object.Spec.DisplayName)
	}
	if len(plan.ObserveDeletes) != 1 || plan.ObserveDeletes[0] != "observe-missing" {
		t.Fatalf("ObserveDeletes = %#v, want observe-missing", plan.ObserveDeletes)
	}
	deleteKey := kubeapi.BatchKey{Operation: "delete", Resource: "nsxgroups", Name: "observe-missing"}
	if _, found := plan.KubeWrites.GroupDeletes[deleteKey]; !found {
		t.Fatalf("GroupDeletes = %#v, want typed delete for missing observe group", plan.KubeWrites.GroupDeletes)
	}
	if len(plan.GroupStatuses) != 1 || plan.GroupStatuses[0].Name != "observe-drifted" {
		t.Fatalf("GroupStatuses = %#v, want status for remote-present observe", plan.GroupStatuses)
	}
	if _, found := plan.KubeWrites.GroupStatusesAfterGroupWrite[updateKey]; !found {
		t.Fatalf("GroupStatusesAfterGroupWrite = %#v, want status to use update result resourceVersion", plan.KubeWrites.GroupStatusesAfterGroupWrite)
	}
}

func TestProcessManagerSnapshotObserveGroupWithLegacyFinalizerPlansFinalizerRemovalOnly(t *testing.T) {
	now := time.Date(2026, 5, 19, 13, 15, 0, 0, time.UTC)
	observe := managerGroup("observe-legacy", "nsx-a.example.test", "app-legacy", nsxv1alpha.NSXGroupModeObserve)
	observe.Spec.DisplayName = "Legacy App"
	observe.Spec.SegmentPaths = []string{"/infra/segments/web", "/infra/segments/db"}
	observe.Finalizers = []string{stateoperator.GroupFinalizer, "example.test/keep"}

	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
		NetworkCloudFQDN: "nsx-a.example.test",
		LocalGroups:      []nsxv1alpha.NSXGroup{*observe},
		RemoteGroups: []stateoperator.RemoteGroup{{
			Key:         stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-legacy"},
			DisplayName: "Legacy App",
			CIDRs:       []string{"10.0.0.0/24"},
			SegmentPaths: []string{
				"/infra/segments/db",
				"/infra/segments/web",
			},
		}},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.ObserveFinalizerRemovals) != 1 || plan.ObserveFinalizerRemovals[0] != "observe-legacy" {
		t.Fatalf("ObserveFinalizerRemovals = %#v, want observe-legacy", plan.ObserveFinalizerRemovals)
	}
	if len(plan.ObserveUpserts) != 0 {
		t.Fatalf("ObserveUpserts = %#v, want none for matching Observe group", plan.ObserveUpserts)
	}
	if len(plan.ManagedWrites) != 0 || len(plan.ManagedDeletes) != 0 {
		t.Fatalf("managed operations = writes %#v deletes %#v, want none", plan.ManagedWrites, plan.ManagedDeletes)
	}
	statusKey := kubeapi.BatchKey{Operation: "updateStatus", Resource: "nsxgroups", Subresource: "status", Name: "observe-legacy"}
	patchRequest, found := plan.KubeWrites.GroupFinalizersAfterStatusWrite[statusKey]
	if !found {
		t.Fatalf("GroupFinalizersAfterStatusWrite = %#v, want finalizer patch to use status result resource version", plan.KubeWrites.GroupFinalizersAfterStatusWrite)
	}
	if !reflect.DeepEqual(patchRequest.Finalizers, []string{"example.test/keep"}) {
		t.Fatalf("patch finalizers = %#v, want only unrelated finalizer", patchRequest.Finalizers)
	}
}

func TestProcessManagerSnapshotManageGroupsWriteMissingAndDriftedAndOnlyStatusMatching(t *testing.T) {
	oldTime := time.Date(2026, 5, 19, 12, 30, 0, 0, time.UTC)
	now := time.Date(2026, 5, 19, 13, 30, 0, 0, time.UTC)
	drifted := managerGroup("manage-drifted", "nsx-a.example.test", "app-drifted", nsxv1alpha.NSXGroupModeManage)
	drifted.Spec.DisplayName = "Desired Drifted"
	drifted.Spec.CIDRs = []string{"10.40.0.0/24"}
	matching := managerGroup("manage-matching", "nsx-a.example.test", "app-matching", nsxv1alpha.NSXGroupModeManage)
	matching.Spec.DisplayName = "Matching"
	matching.Spec.CIDRs = []string{"10.41.0.0/24"}
	matching.Spec.SegmentPaths = []string{"/infra/segments/web", "/infra/segments/db"}
	matching.Generation = 8
	matching.Status = nsxv1alpha.NSXGroupStatus{Conditions: []metav1.Condition{{
		Type:               nsxv1alpha.ConditionRemotePresent,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 7,
		LastTransitionTime: metav1.NewTime(oldTime),
		Reason:             "PreviousRemoteFound",
		Message:            "previous remote present",
	}}}
	matchingPending := managerGroup("manage-matching-pending", "nsx-a.example.test", "app-matching-pending", nsxv1alpha.NSXGroupModeManage)
	matchingPending.Spec.DisplayName = "Matching Pending"
	matchingPending.Spec.CIDRs = []string{"10.43.0.0/24"}
	matchingFailed := managerGroup("manage-matching-failed", "nsx-a.example.test", "app-matching-failed", nsxv1alpha.NSXGroupModeManage)
	matchingFailed.Spec.DisplayName = "Matching Failed"
	matchingFailed.Spec.CIDRs = []string{"10.44.0.0/24"}
	missing := managerGroup("manage-missing", "nsx-a.example.test", "app-missing", nsxv1alpha.NSXGroupModeManage)
	missing.Spec.DisplayName = "Missing"
	missing.Spec.CIDRs = []string{"10.42.0.0/24"}

	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
		NetworkCloudFQDN: "nsx-a.example.test",
		LocalGroups:      []nsxv1alpha.NSXGroup{*matching, *matchingPending, *matchingFailed, *missing, *drifted},
		RemoteGroups: []stateoperator.RemoteGroup{
			{
				Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-drifted"},
				DisplayName:           "Remote Drifted",
				CIDRs:                 []string{"10.99.0.0/24"},
				IPAddressExpressionID: "existing-ip-expression",
				PathExpressionID:      "existing-path-expression",
			},
			{
				Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-matching"},
				DisplayName:           "Matching",
				CIDRs:                 []string{"10.41.0.0/24"},
				SegmentPaths:          []string{"/infra/segments/db", "/infra/segments/web"},
				IPAddressExpressionID: "matching-ip-expression",
				PathExpressionID:      "matching-path-expression",
			},
			{
				Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-matching-pending"},
				DisplayName:           "Matching Pending",
				CIDRs:                 []string{"10.43.0.0/24"},
				IPAddressExpressionID: "pending-ip-expression",
				Raw:                   nsxclient.Group{State: "IN_PROGRESS"},
			},
			{
				Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-matching-failed"},
				DisplayName:           "Matching Failed",
				CIDRs:                 []string{"10.44.0.0/24"},
				IPAddressExpressionID: "failed-ip-expression",
				Raw:                   nsxclient.Group{State: "FAILED"},
			},
		},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.ManagedWrites) != 2 {
		t.Fatalf("ManagedWrites = %#v, want drifted and missing writes", plan.ManagedWrites)
	}
	if plan.ManagedWrites[0].Name != "manage-drifted" || plan.ManagedWrites[0].IPAddressExpressionID != "existing-ip-expression" {
		t.Fatalf("first ManagedWrite = %#v, want drifted write retaining expression IDs", plan.ManagedWrites[0])
	}
	if plan.ManagedWrites[0].DisplayName != "Desired Drifted" || !reflect.DeepEqual(plan.ManagedWrites[0].CIDRs, []string{"10.40.0.0/24"}) {
		t.Fatalf("drifted ManagedWrite desired values = %#v", plan.ManagedWrites[0])
	}
	if plan.ManagedWrites[1].Name != "manage-missing" || plan.ManagedWrites[1].IPAddressExpressionID != "" {
		t.Fatalf("second ManagedWrite = %#v, want missing write without remote expression IDs", plan.ManagedWrites[1])
	}
	if len(plan.GroupStatuses) != 5 {
		t.Fatalf("GroupStatuses = %#v, want status plans for all manage groups", plan.GroupStatuses)
	}
	requireConditionTypes(t, statusFor(t, plan.GroupStatuses, "manage-drifted").Conditions, []string{
		nsxv1alpha.ConditionRemotePresent,
		nsxv1alpha.ConditionSpecMatchesRemote,
		nsxv1alpha.ConditionUnsupportedExpression,
		nsxv1alpha.ConditionRealized,
		nsxv1alpha.ConditionSynced,
		nsxv1alpha.ConditionApplying,
		nsxv1alpha.ConditionDeleting,
	})
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-drifted").Conditions, nsxv1alpha.ConditionSpecMatchesRemote, metav1.ConditionFalse, "SpecDrifted", "local group spec does not match remote NSX group", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-drifted").Conditions, nsxv1alpha.ConditionRealized, metav1.ConditionTrue, "Realized", "remote NSX group is realized", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-drifted").Conditions, nsxv1alpha.ConditionApplying, metav1.ConditionTrue, "Applying", "managed NSX group update is planned", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-matching").Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionTrue, "Synced", "local group matches remote NSX group", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-matching").Conditions, nsxv1alpha.ConditionRemotePresent, metav1.ConditionTrue, "RemoteFound", "remote NSX group is present", oldTime)
	requireObservedGeneration(t, statusFor(t, plan.GroupStatuses, "manage-matching").Conditions, nsxv1alpha.ConditionRemotePresent, 8)
	requireObservedGeneration(t, statusFor(t, plan.GroupStatuses, "manage-matching").Conditions, nsxv1alpha.ConditionSynced, 8)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-matching").Conditions, nsxv1alpha.ConditionApplying, metav1.ConditionFalse, "NotApplying", "no NSX write is planned", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-matching-pending").Conditions, nsxv1alpha.ConditionRealized, metav1.ConditionUnknown, "RealizationPending", "remote realization is still pending", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-matching-pending").Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown, "RealizationPending", "remote realization is still pending", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-matching-failed").Conditions, nsxv1alpha.ConditionRealized, metav1.ConditionFalse, "RealizationFailed", "remote NSX group is not realized", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-matching-failed").Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionFalse, "RealizationFailed", "remote NSX group is not realized", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-missing").Conditions, nsxv1alpha.ConditionRemotePresent, metav1.ConditionFalse, "RemoteMissing", "remote NSX group is missing", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-missing").Conditions, nsxv1alpha.ConditionSpecMatchesRemote, metav1.ConditionFalse, "RemoteMissing", "remote NSX group is missing", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-missing").Conditions, nsxv1alpha.ConditionRealized, metav1.ConditionUnknown, "RemoteMissing", "remote realization is unknown because the group is missing", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-missing").Conditions, nsxv1alpha.ConditionDeleting, metav1.ConditionFalse, "NotDeleting", "no NSX delete is planned", now)
}

func TestProcessManagerSnapshotSkipsAlreadyCorrectGroupStatus(t *testing.T) {
	oldTime := time.Date(2026, 5, 19, 13, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 19, 14, 0, 0, 0, time.UTC)
	matching := managerGroup("manage-ready", "nsx-a.example.test", "app-ready", nsxv1alpha.NSXGroupModeManage)
	matching.Generation = 9
	matching.Spec.DisplayName = "Ready App"
	matching.Spec.CIDRs = []string{"10.55.0.0/24"}
	matching.Status = alreadySyncedManagedStatus(t, matching.Generation, oldTime)

	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
		NetworkCloudFQDN: "nsx-a.example.test",
		LocalGroups:      []nsxv1alpha.NSXGroup{*matching},
		RemoteGroups: []stateoperator.RemoteGroup{
			{
				Key:         stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-ready"},
				DisplayName: "Ready App",
				CIDRs:       []string{"10.55.0.0/24"},
			},
			{
				Key:         stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "remote-import"},
				DisplayName: "Remote Import",
			},
		},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.ObserveUpserts) != 1 || plan.ObserveUpserts[0].Name != "nsx-a.example.test-remote-import" {
		t.Fatalf("ObserveUpserts = %#v, want remote import preserved", plan.ObserveUpserts)
	}
	if _, found := findGroupStatusPlan(plan.GroupStatuses, "manage-ready"); found {
		t.Fatalf("GroupStatuses = %#v, want no status plan for already-correct manage-ready", plan.GroupStatuses)
	}
	if _, found := findGroupStatusPlan(plan.GroupStatuses, "nsx-a.example.test-remote-import"); !found {
		t.Fatalf("GroupStatuses = %#v, want status plan for remote import", plan.GroupStatuses)
	}
}

func TestProcessManagerSnapshotPlansStatusForMeaningfulGroupDrift(t *testing.T) {
	oldTime := time.Date(2026, 5, 19, 13, 5, 0, 0, time.UTC)
	now := time.Date(2026, 5, 19, 14, 5, 0, 0, time.UTC)

	tests := []struct {
		name               string
		mutate             func(*nsxv1alpha.NSXGroupStatus)
		wantRemoteFoundLTT time.Time
	}{
		{
			name: "condition status",
			mutate: func(status *nsxv1alpha.NSXGroupStatus) {
				status.Conditions[0].Status = metav1.ConditionFalse
			},
			wantRemoteFoundLTT: now,
		},
		{
			name: "condition reason",
			mutate: func(status *nsxv1alpha.NSXGroupStatus) {
				status.Conditions[0].Reason = "StaleReason"
			},
			wantRemoteFoundLTT: oldTime,
		},
		{
			name: "condition message",
			mutate: func(status *nsxv1alpha.NSXGroupStatus) {
				status.Conditions[0].Message = "stale message"
			},
			wantRemoteFoundLTT: oldTime,
		},
		{
			name: "observed generation",
			mutate: func(status *nsxv1alpha.NSXGroupStatus) {
				status.Conditions[0].ObservedGeneration = 8
			},
			wantRemoteFoundLTT: oldTime,
		},
		{
			name: "unsupported reason",
			mutate: func(status *nsxv1alpha.NSXGroupStatus) {
				status.UnsupportedReason = nsxv1alpha.UnsupportedExpressionReasonUnsupportedExpressionType
			},
			wantRemoteFoundLTT: oldTime,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matching := managerGroup("manage-ready", "nsx-a.example.test", "app-ready", nsxv1alpha.NSXGroupModeManage)
			matching.Generation = 9
			matching.Spec.DisplayName = "Ready App"
			matching.Spec.CIDRs = []string{"10.55.0.0/24"}
			matching.Status = alreadySyncedManagedStatus(t, matching.Generation, oldTime)
			tt.mutate(&matching.Status)

			plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
				Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
				NetworkCloudFQDN: "nsx-a.example.test",
				LocalGroups:      []nsxv1alpha.NSXGroup{*matching},
				RemoteGroups: []stateoperator.RemoteGroup{{
					Key:         stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-ready"},
					DisplayName: "Ready App",
					CIDRs:       []string{"10.55.0.0/24"},
				}},
			}, now)
			if err != nil {
				t.Fatalf("ProcessManagerSnapshot() error = %v", err)
			}
			statusPlan, found := findGroupStatusPlan(plan.GroupStatuses, "manage-ready")
			if !found {
				t.Fatalf("GroupStatuses = %#v, want status plan for stale %s", plan.GroupStatuses, tt.name)
			}
			requireCondition(t, statusPlan.Status.Conditions, nsxv1alpha.ConditionRemotePresent, metav1.ConditionTrue, "RemoteFound", "remote NSX group is present", tt.wantRemoteFoundLTT)
		})
	}
}

func TestProcessManagerSnapshotDeletingManageGroupPlansFinalizerRemovalAfterRemoteAbsence(t *testing.T) {
	now := time.Date(2026, 5, 19, 13, 45, 0, 0, time.UTC)
	deletionTime := metav1.NewTime(now.Add(-time.Minute))
	deleting := managerGroup("manage-deleting", "nsx-a.example.test", "app-deleting", nsxv1alpha.NSXGroupModeManage)
	deleting.Finalizers = []string{stateoperator.GroupFinalizer, "example.test/keep"}
	deleting.DeletionTimestamp = &deletionTime

	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
		NetworkCloudFQDN: "nsx-a.example.test",
		LocalGroups:      []nsxv1alpha.NSXGroup{*deleting},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.ManagedDeletes) != 0 {
		t.Fatalf("ManagedDeletes = %#v, want no repeat delete after remote absence confirmed", plan.ManagedDeletes)
	}
	if !reflect.DeepEqual(plan.ManagedFinalizerRemovals, []string{"manage-deleting"}) {
		t.Fatalf("ManagedFinalizerRemovals = %#v, want manage-deleting", plan.ManagedFinalizerRemovals)
	}
	status := statusFor(t, plan.GroupStatuses, "manage-deleting")
	requireCondition(t, status.Conditions, nsxv1alpha.ConditionRemotePresent, metav1.ConditionFalse, "RemoteDeleted", "remote NSX group deletion is confirmed", now)
	requireCondition(t, status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionTrue, "Deleted", "managed NSX group deletion is confirmed", now)
	requireCondition(t, status.Conditions, nsxv1alpha.ConditionDeleting, metav1.ConditionFalse, "Deleted", "managed NSX group deletion is confirmed", now)
}

func TestProcessManagerSnapshotDeletingManageGroupPlansDeleteWhileRemoteExists(t *testing.T) {
	now := time.Date(2026, 5, 19, 14, 0, 0, 0, time.UTC)
	deletionTime := metav1.NewTime(now.Add(-time.Minute))
	deleting := managerGroup("manage-deleting", "nsx-a.example.test", "app-deleting", nsxv1alpha.NSXGroupModeManage)
	deleting.Finalizers = []string{stateoperator.GroupFinalizer}
	deleting.DeletionTimestamp = &deletionTime

	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
		NetworkCloudFQDN: "nsx-a.example.test",
		LocalGroups:      []nsxv1alpha.NSXGroup{*deleting},
		RemoteGroups: []stateoperator.RemoteGroup{{
			Key:         stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-deleting"},
			DisplayName: "Deleting",
		}},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if !reflect.DeepEqual(plan.ManagedDeletes, []stateoperator.ManagedGroupDelete{{GroupID: "app-deleting"}}) {
		t.Fatalf("ManagedDeletes = %#v, want app-deleting delete", plan.ManagedDeletes)
	}
	if len(plan.ManagedFinalizerRemovals) != 0 {
		t.Fatalf("ManagedFinalizerRemovals = %#v, want none while remote exists", plan.ManagedFinalizerRemovals)
	}
	status := statusFor(t, plan.GroupStatuses, "manage-deleting")
	requireCondition(t, status.Conditions, nsxv1alpha.ConditionRemotePresent, metav1.ConditionTrue, "RemoteFound", "remote NSX group is present", now)
	requireCondition(t, status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionUnknown, "Deleting", "managed NSX group delete is planned", now)
	requireCondition(t, status.Conditions, nsxv1alpha.ConditionDeleting, metav1.ConditionTrue, "Deleting", "managed NSX group delete is planned", now)
}

func TestRemoteGroupFromNSXGroupSupportsEmptyExpression(t *testing.T) {
	remote := stateoperator.RemoteGroupFromNSXGroup("nsx-a.example.test", nsxclient.Group{
		Resource: nsxclient.Resource{ID: "empty", DisplayName: "Empty"},
	})
	if remote.Key != (stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "empty"}) {
		t.Fatalf("remote key = %#v, want cloud/group binding", remote.Key)
	}
	if remote.DisplayName != "Empty" {
		t.Fatalf("remote display name = %q, want Empty", remote.DisplayName)
	}
	if len(remote.CIDRs) != 0 || len(remote.SegmentPaths) != 0 {
		t.Fatalf("remote represented spec = cidrs:%v segments:%v, want empty", remote.CIDRs, remote.SegmentPaths)
	}
	if remote.UnsupportedReason != "" {
		t.Fatalf("UnsupportedReason = %q for empty expression, want empty", remote.UnsupportedReason)
	}
}

func TestRemoteGroupFromNSXGroupSupportsIPAddressExpression(t *testing.T) {
	remote := stateoperator.RemoteGroupFromNSXGroup("nsx-a.example.test", nsxclient.Group{
		Resource: nsxclient.Resource{ID: "app-ip", DisplayName: "App IP"},
		Expression: []json.RawMessage{
			rawExpression(t, nsxclient.IPAddressExpression{
				Resource:    nsxclient.Resource{ID: "ip-expression", ResourceType: "IPAddressExpression"},
				IPAddresses: []string{"10.50.0.0/24", "10.51.0.0/24"},
			}),
		},
	})
	if !reflect.DeepEqual(remote.CIDRs, []string{"10.50.0.0/24", "10.51.0.0/24"}) {
		t.Fatalf("remote CIDRs = %#v, want IP expression addresses", remote.CIDRs)
	}
	if remote.IPAddressExpressionID != "ip-expression" {
		t.Fatalf("remote IP expression ID = %q, want ip-expression", remote.IPAddressExpressionID)
	}
	if len(remote.SegmentPaths) != 0 || remote.PathExpressionID != "" {
		t.Fatalf("remote path expression = id:%q paths:%v, want empty", remote.PathExpressionID, remote.SegmentPaths)
	}
	if remote.UnsupportedReason != "" {
		t.Fatalf("UnsupportedReason = %q for IP expression, want empty", remote.UnsupportedReason)
	}
}

func TestRemoteGroupFromNSXGroupSupportsIPOrSegmentExpression(t *testing.T) {
	segmentPaths := []string{"/infra/segments/web", "/infra/segments/db"}
	remote := stateoperator.RemoteGroupFromNSXGroup("nsx-a.example.test", nsxclient.Group{
		Resource: nsxclient.Resource{ID: "app-web", DisplayName: "App Web"},
		Expression: []json.RawMessage{
			rawExpression(t, nsxclient.IPAddressExpression{
				Resource:    nsxclient.Resource{ID: "ip-expression", ResourceType: "IPAddressExpression"},
				IPAddresses: []string{"10.50.0.0/24"},
			}),
			rawExpression(t, map[string]string{
				"resource_type":        "ConjunctionOperator",
				"conjunction_operator": "OR",
			}),
			rawExpression(t, nsxclient.PathExpression{
				Resource: nsxclient.Resource{ID: "path-expression", ResourceType: "PathExpression"},
				Paths:    segmentPaths,
			}),
		},
	})
	if remote.Key != (stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-web"}) {
		t.Fatalf("remote key = %#v, want cloud/group binding", remote.Key)
	}
	if remote.DisplayName != "App Web" || !reflect.DeepEqual(remote.CIDRs, []string{"10.50.0.0/24"}) {
		t.Fatalf("remote parsed values = %#v", remote)
	}
	if !reflect.DeepEqual(remote.SegmentPaths, segmentPaths) {
		t.Fatalf("remote segment paths = %#v, want %v", remote.SegmentPaths, segmentPaths)
	}
	if remote.IPAddressExpressionID != "ip-expression" || remote.PathExpressionID != "path-expression" {
		t.Fatalf("remote expression IDs = ip:%q path:%q", remote.IPAddressExpressionID, remote.PathExpressionID)
	}
	if remote.UnsupportedReason != "" {
		t.Fatalf("UnsupportedReason = %q for representable expressions, want empty", remote.UnsupportedReason)
	}
}

func TestRemoteGroupFromNSXGroupFlagsUnsupportedAndPreservesRepresentableFields(t *testing.T) {
	unsupported := stateoperator.RemoteGroupFromNSXGroup("nsx-a.example.test", nsxclient.Group{
		Resource: nsxclient.Resource{ID: "app-unsupported", DisplayName: "Unsupported App"},
		Expression: []json.RawMessage{
			rawExpression(t, nsxclient.IPAddressExpression{
				Resource:    nsxclient.Resource{ID: "ip-expression", ResourceType: "IPAddressExpression"},
				IPAddresses: []string{"10.52.0.0/24"},
			}),
			rawExpression(t, map[string]string{
				"resource_type":        "ConjunctionOperator",
				"conjunction_operator": "AND",
			}),
			json.RawMessage(`{`),
			rawExpression(t, nsxclient.PathExpression{
				Resource: nsxclient.Resource{ID: "multi-path", ResourceType: "PathExpression"},
				Paths:    []string{"/infra/segments/first", "/infra/segments/second"},
			}),
		},
		ExtendedExpression: []json.RawMessage{rawExpression(t, map[string]string{"resource_type": "Extra"})},
	})
	if unsupported.UnsupportedReason != nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression {
		t.Fatalf("UnsupportedReason = %q, want %q: %#v", unsupported.UnsupportedReason, nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression, unsupported)
	}
	if !reflect.DeepEqual(unsupported.CIDRs, []string{"10.52.0.0/24"}) {
		t.Fatalf("unsupported CIDRs = %#v, want representable IP fields preserved", unsupported.CIDRs)
	}
	if !reflect.DeepEqual(unsupported.SegmentPaths, []string{"/infra/segments/first", "/infra/segments/second"}) {
		t.Fatalf("unsupported segment paths = %#v, want representable paths preserved", unsupported.SegmentPaths)
	}
}

func TestRemoteGroupFromNSXGroupClassifiesUnsupportedReasons(t *testing.T) {
	tests := []struct {
		name       string
		group      nsxclient.Group
		wantReason nsxv1alpha.UnsupportedExpressionReason
		wantCIDRs  []string
		wantPaths  []string
		wantIPID   string
		wantPathID string
	}{
		{
			name: "extended expression is unsupported nested expression",
			group: nsxclient.Group{
				Resource:           nsxclient.Resource{ID: "extended-expression"},
				ExtendedExpression: []json.RawMessage{rawExpression(t, map[string]string{"resource_type": "NestedExpression"})},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression,
		},
		{
			name: "malformed raw expression has unsupported expression type",
			group: nsxclient.Group{
				Resource:   nsxclient.Resource{ID: "malformed"},
				Expression: []json.RawMessage{json.RawMessage(`{`)},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonUnsupportedExpressionType,
		},
		{
			name: "missing resource type has unsupported expression type",
			group: nsxclient.Group{
				Resource:   nsxclient.Resource{ID: "missing-type"},
				Expression: []json.RawMessage{rawExpression(t, map[string][]string{"ip_addresses": {"10.0.0.0/24"}})},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonUnsupportedExpressionType,
		},
		{
			name: "duplicate IP address expression",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "duplicate-ip"},
				Expression: []json.RawMessage{
					rawExpression(t, nsxclient.IPAddressExpression{
						Resource:    nsxclient.Resource{ID: "ip-first", ResourceType: "IPAddressExpression"},
						IPAddresses: []string{"10.0.0.0/24"},
					}),
					rawExpression(t, nsxclient.IPAddressExpression{
						Resource:    nsxclient.Resource{ID: "ip-second", ResourceType: "IPAddressExpression"},
						IPAddresses: []string{"10.1.0.0/24"},
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonMultipleIPAddressExpressions,
			wantCIDRs:  []string{"10.0.0.0/24"},
			wantIPID:   "ip-first",
		},
		{
			name: "invalid IP address expression",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "invalid-ip"},
				Expression: []json.RawMessage{
					rawExpression(t, map[string]any{
						"resource_type": "IPAddressExpression",
						"ip_addresses":  []any{12},
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonInvalidIPAddressExpression,
		},
		{
			name: "missing IP addresses field",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "missing-ip-addresses"},
				Expression: []json.RawMessage{
					rawExpression(t, map[string]any{
						"resource_type": "IPAddressExpression",
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonInvalidIPAddressExpression,
		},
		{
			name: "unsupported IP address expression fields",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "unsupported-ip-fields"},
				Expression: []json.RawMessage{
					rawExpression(t, map[string]any{
						"id":            "ip-expression",
						"resource_type": "IPAddressExpression",
						"ip_addresses":  []string{"10.2.0.0/24"},
						"mac_addresses": []string{"00:11:22:33:44:55"},
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonUnsupportedIPAddressExpressionFields,
			wantCIDRs:  []string{"10.2.0.0/24"},
			wantIPID:   "ip-expression",
		},
		{
			name: "duplicate path expression",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "duplicate-path"},
				Expression: []json.RawMessage{
					rawExpression(t, nsxclient.PathExpression{
						Resource: nsxclient.Resource{ID: "path-first", ResourceType: "PathExpression"},
						Paths:    []string{"/infra/segments/first"},
					}),
					rawExpression(t, nsxclient.PathExpression{
						Resource: nsxclient.Resource{ID: "path-second", ResourceType: "PathExpression"},
						Paths:    []string{"/infra/segments/second"},
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonMultiplePathExpressions,
			wantPaths:  []string{"/infra/segments/first"},
			wantPathID: "path-first",
		},
		{
			name: "invalid path expression",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "invalid-path"},
				Expression: []json.RawMessage{
					rawExpression(t, map[string]any{
						"resource_type": "PathExpression",
						"paths":         []any{12},
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonInvalidPathExpression,
		},
		{
			name: "missing paths field",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "missing-paths"},
				Expression: []json.RawMessage{
					rawExpression(t, map[string]any{
						"resource_type": "PathExpression",
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonInvalidPathExpression,
		},
		{
			name: "unsupported path expression fields",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "unsupported-path-fields"},
				Expression: []json.RawMessage{
					rawExpression(t, map[string]any{
						"id":            "path-expression",
						"resource_type": "PathExpression",
						"paths":         []string{"/infra/segments/app"},
						"external_ids":  []string{"external"},
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonUnsupportedPathExpressionFields,
			wantPaths:  []string{"/infra/segments/app"},
			wantPathID: "path-expression",
		},
		{
			name: "invalid conjunction operator",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "invalid-operator"},
				Expression: []json.RawMessage{
					rawExpression(t, map[string]any{
						"resource_type":        "ConjunctionOperator",
						"conjunction_operator": 12,
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression,
		},
		{
			name: "non OR conjunction operator",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "and-operator"},
				Expression: []json.RawMessage{
					rawExpression(t, map[string]string{
						"resource_type":        "ConjunctionOperator",
						"conjunction_operator": "AND",
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonUnsupportedNestedExpression,
		},
		{
			name: "unknown resource type",
			group: nsxclient.Group{
				Resource: nsxclient.Resource{ID: "unknown-type"},
				Expression: []json.RawMessage{
					rawExpression(t, map[string]string{
						"resource_type": "Condition",
					}),
				},
			},
			wantReason: nsxv1alpha.UnsupportedExpressionReasonUnsupportedExpressionType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote := stateoperator.RemoteGroupFromNSXGroup("nsx-a.example.test", tt.group)
			if remote.UnsupportedReason != tt.wantReason {
				t.Fatalf("UnsupportedReason = %q, want %q for %#v", remote.UnsupportedReason, tt.wantReason, remote)
			}
			if !reflect.DeepEqual(remote.CIDRs, tt.wantCIDRs) {
				t.Fatalf("CIDRs = %#v, want %#v", remote.CIDRs, tt.wantCIDRs)
			}
			if !reflect.DeepEqual(remote.SegmentPaths, tt.wantPaths) {
				t.Fatalf("SegmentPaths = %#v, want %#v", remote.SegmentPaths, tt.wantPaths)
			}
			if remote.IPAddressExpressionID != tt.wantIPID {
				t.Fatalf("IPAddressExpressionID = %q, want %q", remote.IPAddressExpressionID, tt.wantIPID)
			}
			if remote.PathExpressionID != tt.wantPathID {
				t.Fatalf("PathExpressionID = %q, want %q", remote.PathExpressionID, tt.wantPathID)
			}
		})
	}
}

func TestGatherManagerSnapshotRecordsListAndFactoryFailuresWithoutReturningErrors(t *testing.T) {
	cloud := *networkCloud("cloud-a", "nsx-a.example.test")
	snapshot, err := stateoperator.GatherManagerSnapshot(
		context.Background(),
		cloud,
		func(context.Context, kubeapi.ListOptions) (*nsxv1alpha.NSXGroupList, error) {
			return nil, errors.New("kube list failed")
		},
		func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			t.Fatal("manager factory should not be called after local list failure")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("GatherManagerSnapshot() setup error = %v", err)
	}
	if snapshot.GatherError == nil || !strings.Contains(snapshot.GatherError.Error(), "kube list failed") {
		t.Fatalf("GatherError = %v, want local list failure", snapshot.GatherError)
	}

	snapshot, err = stateoperator.GatherManagerSnapshot(
		context.Background(),
		cloud,
		func(context.Context, kubeapi.ListOptions) (*nsxv1alpha.NSXGroupList, error) {
			return &nsxv1alpha.NSXGroupList{}, nil
		},
		func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return nil, errors.New("factory failed")
		},
	)
	if err != nil {
		t.Fatalf("GatherManagerSnapshot() setup error = %v", err)
	}
	if snapshot.GatherError == nil || !strings.Contains(snapshot.GatherError.Error(), "factory failed") {
		t.Fatalf("GatherError = %v, want manager factory failure", snapshot.GatherError)
	}
}

func TestGatherManagerSnapshotListsLocalGroupsByNormalizedFQDNAndUsesNSXPagination(t *testing.T) {
	var listOptions kubeapi.ListOptions
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/policy/api/v1/infra/domains/default/groups" {
			t.Errorf("path = %q, want default-domain groups path", req.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Query().Get("cursor") {
		case "":
			if _, err := io.WriteString(w, `{"results":[{"id":"remote-a","display_name":"Remote A"}],"cursor":"page-2","result_count":1}`); err != nil {
				t.Errorf("write first page: %v", err)
			}
		case "page-2":
			if _, err := io.WriteString(w, `{"results":[{"id":"remote-b","display_name":"Remote B"}],"result_count":1}`); err != nil {
				t.Errorf("write second page: %v", err)
			}
		default:
			t.Errorf("unexpected cursor %q", req.URL.Query().Get("cursor"))
		}
	}))
	t.Cleanup(server.Close)
	managerClient, err := nsxclient.NewClient(nsxclient.Options{
		BaseURL:  server.URL,
		Username: "nsx-admin",
		Password: "nsx-password",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	snapshot, err := stateoperator.GatherManagerSnapshot(
		context.Background(),
		*networkCloud("cloud-a", "HTTPS://NSX-A.Example.Test:8443/"),
		func(_ context.Context, options kubeapi.ListOptions) (*nsxv1alpha.NSXGroupList, error) {
			listOptions = options
			if len(options.Filters) != 1 {
				return nil, fmt.Errorf("filters = %#v, want one network cloud FQDN filter", options.Filters)
			}
			if options.Filters[0].Field() != kubeapi.FieldNetworkCloudFQDN || options.Filters[0].Value() != "nsx-a.example.test:8443" {
				return nil, fmt.Errorf("filter = %#v, want normalized network cloud FQDN", options.Filters[0])
			}
			return &nsxv1alpha.NSXGroupList{Items: []nsxv1alpha.NSXGroup{
				*managerGroup("local-a", "nsx-a.example.test:8443", "local-a", nsxv1alpha.NSXGroupModeManage),
			}}, nil
		},
		func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return managerClient, nil
		},
	)
	if err != nil {
		t.Fatalf("GatherManagerSnapshot() error = %v", err)
	}
	if snapshot.NetworkCloudFQDN != "nsx-a.example.test:8443" {
		t.Fatalf("NetworkCloudFQDN = %q, want normalized host:port", snapshot.NetworkCloudFQDN)
	}
	if len(listOptions.Filters) != 1 {
		t.Fatalf("list filters = %#v, want normalized FQDN field filter", listOptions.Filters)
	}
	localBindings, err := stateoperator.BuildBindings(stateoperator.ManagerSnapshot{LocalGroups: snapshot.LocalGroups})
	if err != nil {
		t.Fatalf("BuildBindings(local snapshot) error = %v", err)
	}
	if _, ok := localBindings.LocalByKey[stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test:8443", GroupID: "local-a"}]; !ok {
		t.Fatalf("local bindings = %#v, want normalized local group binding", localBindings.LocalByKey)
	}
	if len(snapshot.RemoteGroups) != 2 {
		t.Fatalf("RemoteGroups = %#v, want both paginated NSX groups", snapshot.RemoteGroups)
	}
	if snapshot.RemoteGroups[0].Key.GroupID != "remote-a" || snapshot.RemoteGroups[1].Key.GroupID != "remote-b" {
		t.Fatalf("RemoteGroups = %#v, want remote-a then remote-b", snapshot.RemoteGroups)
	}
}

func TestApplyManagerPlanRunsOperationsInExactOrder(t *testing.T) {
	recorder := &operationRecorder{}
	plan := stateoperator.ManagerPlan{
		ObserveUpserts: []nsxv1alpha.NSXGroup{
			{ObjectMeta: metav1.ObjectMeta{Name: "observe-import"}},
		},
		ManagedWrites: []stateoperator.ManagedGroupWrite{
			{
				Name:                  "manage-drifted",
				Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-drifted"},
				DisplayName:           "Drifted",
				CIDRs:                 []string{"10.60.0.0/24"},
				SegmentPaths:          []string{"/infra/segments/web", "/infra/segments/db"},
				IPAddressExpressionID: "ip-expression",
				PathExpressionID:      "path-expression",
			},
			{
				Name:        "manage-missing",
				Key:         stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-missing"},
				DisplayName: "Missing",
				CIDRs:       []string{"10.61.0.0/24"},
			},
		},
		ManagedDeletes: []stateoperator.ManagedGroupDelete{{GroupID: "app-delete"}},
		GroupStatuses: []stateoperator.GroupStatusPlan{
			{Name: "manage-drifted"},
		},
		ObserveFinalizerRemovals: []string{"observe-missing"},
		ObserveDeletes:           []string{"observe-missing"},
		CloudStatus:              &stateoperator.CloudStatusPlan{Name: "cloud-a"},
	}

	err := stateoperator.ApplyManagerPlan(context.Background(), recorder, recorder, plan)
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v", err)
	}
	want := []string{
		"apply-group:observe-import",
		"patch-group:app-drifted",
		"patch-ip:app-drifted:ip-expression",
		"patch-path:app-drifted:path-expression",
		"patch-group:app-missing",
		"add-ip:app-missing:cidrs",
		"delete-group:app-delete",
		"group-status:manage-drifted",
		"remove-finalizer:observe-missing:nsx.ing.com/finalizer",
		"delete-group-cr:observe-missing",
		"cloud-status:cloud-a",
	}
	if !reflect.DeepEqual(recorder.operations, want) {
		t.Fatalf("operations = %v, want %v", recorder.operations, want)
	}
}

func TestApplyManagerPlanWriteDisabledSkipsRemainingNSXWritesButAppliesStatuses(t *testing.T) {
	recorder := &operationRecorder{
		patchGroupErr: nsxclient.WriteDisabledError{
			Method:           "PATCH",
			URL:              "https://nsx-a.example.test/policy/api/v1/infra/domains/default/groups/managed-write",
			Reason:           nsxclient.WriteDisabledReasonGlobalConfig,
			NetworkCloudName: "cloud-a",
			NetworkCloudFQDN: "nsx-a.example.test",
		},
	}
	plan := stateoperator.ManagerPlan{
		ManagedWrites: []stateoperator.ManagedGroupWrite{{
			Name:                  "managed-write",
			Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "managed-write"},
			DisplayName:           "Managed Write",
			CIDRs:                 []string{"10.42.0.0/24"},
			SegmentPaths:          []string{"/infra/segments/web"},
			IPAddressExpressionID: "selected-ip",
			PathExpressionID:      "selected-path",
		}},
		ManagedDeletes: []stateoperator.ManagedGroupDelete{{GroupID: "managed-delete"}},
		GroupStatuses:  []stateoperator.GroupStatusPlan{{Name: "managed-write"}},
		CloudStatus:    &stateoperator.CloudStatusPlan{Name: "cloud-a"},
	}

	err := stateoperator.ApplyManagerPlan(context.Background(), recorder, recorder, plan)
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v", err)
	}
	want := []string{
		"patch-group:managed-write",
		"group-status:managed-write",
		"cloud-status:cloud-a",
	}
	if !reflect.DeepEqual(recorder.operations, want) {
		t.Fatalf("operations = %v, want %v", recorder.operations, want)
	}
}

func TestApplyManagerPlanPatchesOnlyRepresentedGroupWriteFields(t *testing.T) {
	recorder := &operationRecorder{}
	err := stateoperator.ApplyManagerPlan(context.Background(), recorder, recorder, stateoperator.ManagerPlan{
		ManagedWrites: []stateoperator.ManagedGroupWrite{{
			Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "managed-write"},
			DisplayName:           "Managed Write",
			CIDRs:                 []string{"10.42.0.0/24"},
			SegmentPaths:          []string{"/infra/segments/web"},
			IPAddressExpressionID: "selected-ip",
			PathExpressionID:      "selected-path",
		}},
	})
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v", err)
	}
	wantOperations := []string{
		"patch-group:managed-write",
		"patch-ip:managed-write:selected-ip",
		"patch-path:managed-write:selected-path",
	}
	if !reflect.DeepEqual(recorder.operations, wantOperations) {
		t.Fatalf("operations = %v, want %v", recorder.operations, wantOperations)
	}

	groupPatch := recorder.groupPatches["managed-write"]
	if groupPatch == nil {
		t.Fatalf("recorded group patch is nil, want payload for managed-write")
	}
	rawPatch, err := json.Marshal(groupPatch)
	if err != nil {
		t.Fatalf("marshal recorded group patch: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(rawPatch, &payload); err != nil {
		t.Fatalf("unmarshal recorded group patch: %v", err)
	}
	wantPayload := map[string]any{
		"id":            "managed-write",
		"display_name":  "Managed Write",
		"resource_type": "Group",
	}
	if !reflect.DeepEqual(payload, wantPayload) {
		t.Fatalf("group patch payload = %#v, want only %#v", payload, wantPayload)
	}
}

func TestApplyManagerPlanRejectsMissingRequiredClients(t *testing.T) {
	t.Run("kubernetes applier", func(t *testing.T) {
		err := stateoperator.ApplyManagerPlan(context.Background(), nil, &operationRecorder{}, stateoperator.ManagerPlan{})
		if err == nil {
			t.Fatal("ApplyManagerPlan() error = nil, want missing kubernetes applier error")
		}
	})

	t.Run("manager client for managed write", func(t *testing.T) {
		err := stateoperator.ApplyManagerPlan(context.Background(), &operationRecorder{}, nil, stateoperator.ManagerPlan{
			ManagedWrites: []stateoperator.ManagedGroupWrite{{
				Key:         stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-write"},
				DisplayName: "App Write",
			}},
		})
		if err == nil {
			t.Fatal("ApplyManagerPlan() error = nil, want missing manager client error")
		}
	})

	t.Run("manager client for managed delete", func(t *testing.T) {
		err := stateoperator.ApplyManagerPlan(context.Background(), &operationRecorder{}, nil, stateoperator.ManagerPlan{
			ManagedDeletes: []stateoperator.ManagedGroupDelete{{GroupID: "app-delete"}},
		})
		if err == nil {
			t.Fatal("ApplyManagerPlan() error = nil, want missing manager client error")
		}
	})
}

func TestApplyManagerPlanAllowsObserveOnlyPlanWithoutManagerClient(t *testing.T) {
	recorder := &operationRecorder{}
	plan := stateoperator.ManagerPlan{
		ObserveUpserts: []nsxv1alpha.NSXGroup{
			{ObjectMeta: metav1.ObjectMeta{Name: "observe-import"}},
		},
		GroupStatuses: []stateoperator.GroupStatusPlan{
			{Name: "observe-import"},
		},
		ObserveDeletes: []string{"observe-missing"},
		CloudStatus:    &stateoperator.CloudStatusPlan{Name: "cloud-a"},
	}

	err := stateoperator.ApplyManagerPlan(context.Background(), recorder, nil, plan)
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v", err)
	}
	want := []string{
		"apply-group:observe-import",
		"group-status:observe-import",
		"delete-group-cr:observe-missing",
		"cloud-status:cloud-a",
	}
	if !reflect.DeepEqual(recorder.operations, want) {
		t.Fatalf("operations = %v, want %v", recorder.operations, want)
	}
}

func TestApplyManagerPlanRemovesManagedFinalizersAfterStatusUpdates(t *testing.T) {
	recorder := &operationRecorder{}
	plan := stateoperator.ManagerPlan{
		ManagedFinalizerRemovals: []string{"manage-deleted"},
		GroupStatuses: []stateoperator.GroupStatusPlan{
			{Name: "manage-deleted"},
		},
		CloudStatus: &stateoperator.CloudStatusPlan{Name: "cloud-a"},
	}

	err := stateoperator.ApplyManagerPlan(context.Background(), recorder, nil, plan)
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v", err)
	}
	want := []string{
		"group-status:manage-deleted",
		"remove-finalizer:manage-deleted:nsx.ing.com/finalizer",
		"cloud-status:cloud-a",
	}
	if !reflect.DeepEqual(recorder.operations, want) {
		t.Fatalf("operations = %v, want %v", recorder.operations, want)
	}
}

func TestApplyManagerPlanDeletesExistingIPAddressExpressionWhenManagedCIDRsAreEmpty(t *testing.T) {
	recorder := &operationRecorder{}
	err := stateoperator.ApplyManagerPlan(context.Background(), recorder, recorder, stateoperator.ManagerPlan{
		ManagedWrites: []stateoperator.ManagedGroupWrite{{
			Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "empty-cidrs"},
			DisplayName:           "Empty CIDRs",
			IPAddressExpressionID: "ip-expression",
		}},
	})
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v", err)
	}
	want := []string{"patch-group:empty-cidrs", "delete-ip:empty-cidrs:ip-expression"}
	if !reflect.DeepEqual(recorder.operations, want) {
		t.Fatalf("operations = %v, want %v", recorder.operations, want)
	}
}

func TestApplyManagerPlanAddsMissingPathExpressionWhenManagedSegmentPathsAreSet(t *testing.T) {
	segmentPaths := []string{"/infra/segments/web", "/infra/segments/db"}
	recorder := &operationRecorder{}
	err := stateoperator.ApplyManagerPlan(context.Background(), recorder, recorder, stateoperator.ManagerPlan{
		ManagedWrites: []stateoperator.ManagedGroupWrite{{
			Key:          stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "missing-segment"},
			DisplayName:  "Missing Segment",
			SegmentPaths: segmentPaths,
		}},
	})
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v", err)
	}
	want := []string{"patch-group:missing-segment", "add-path:missing-segment:segment"}
	if !reflect.DeepEqual(recorder.operations, want) {
		t.Fatalf("operations = %v, want %v", recorder.operations, want)
	}
	expression := recorder.pathExpressions["missing-segment:segment"]
	if expression == nil {
		t.Fatalf("recorded path expression is nil, want payload for missing-segment:segment")
	}
	if expression.ID != "segment" || expression.ResourceType != "PathExpression" || !reflect.DeepEqual(expression.Paths, segmentPaths) {
		t.Fatalf("path expression = %#v, want id segment with desired segment paths", expression)
	}
}

func TestApplyManagerPlanDeletesExistingPathExpressionWhenManagedSegmentPathsAreRemoved(t *testing.T) {
	recorder := &operationRecorder{}
	err := stateoperator.ApplyManagerPlan(context.Background(), recorder, recorder, stateoperator.ManagerPlan{
		ManagedWrites: []stateoperator.ManagedGroupWrite{{
			Key:              stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "removed-segment"},
			DisplayName:      "Removed Segment",
			PathExpressionID: "existing-segment",
		}},
	})
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v", err)
	}
	want := []string{"patch-group:removed-segment", "delete-path:removed-segment:existing-segment"}
	if !reflect.DeepEqual(recorder.operations, want) {
		t.Fatalf("operations = %v, want %v", recorder.operations, want)
	}
}

func TestDefaultManagerSweepAppliesObserveUpsertStatusAndDeleteThroughTypedKubeAPI(t *testing.T) {
	registry := prometheus.NewRegistry()
	metricsRecorder, err := operatormetrics.NewRecorder(registry, zap.NewNop())
	if err != nil {
		t.Fatalf("construct metrics recorder: %v", err)
	}
	typedClient, stop := startStateoperatorKubeAPIClientWithRecorder(t, metricsRecorder)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cloud := networkCloud("cloud-default", "nsx-a.example.test")
	createdCloud, err := typedClient.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create typed cloud: %v", err)
	}
	cloud = createdCloud
	localObserve := managerGroup("observe-stale", "nsx-a.example.test", "stale", nsxv1alpha.NSXGroupModeObserve)
	localObserve.Finalizers = []string{stateoperator.GroupFinalizer}
	if _, err := typedClient.Groups().Create(ctx, localObserve, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create typed group: %v", err)
	}
	driftedObserve := managerGroup("observe-drifted", "nsx-a.example.test", "remote-replace", nsxv1alpha.NSXGroupModeObserve)
	driftedObserve.Spec.DisplayName = "Old Remote"
	driftedObserve.Spec.CIDRs = []string{"10.70.0.0/24"}
	driftedObserve.Finalizers = []string{stateoperator.GroupFinalizer}
	if _, err := typedClient.Groups().Create(ctx, driftedObserve, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create drifted typed group: %v", err)
	}

	controllerClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(cloud).
		Build()
	managerRecorder := &operationRecorder{listGroups: []*nsxclient.Group{
		{
			Resource: nsxclient.Resource{ID: "remote-import", DisplayName: "Remote Import"},
		},
		{
			Resource: nsxclient.Resource{ID: "remote-replace", DisplayName: "Remote Replacement"},
			Expression: []json.RawMessage{
				rawExpression(t, nsxclient.IPAddressExpression{
					Resource:    nsxclient.Resource{ID: "replacement-ip", ResourceType: "IPAddressExpression"},
					IPAddresses: []string{"10.71.0.0/24"},
				}),
			},
		},
	}}
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       controllerClient,
		KubeClient:   typedClient,
		TickInterval: time.Hour,
		Logger:       zap.NewExample(),
		Recorder:     metricsRecorder,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return managerRecorder, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	operatorErr := make(chan error, 1)
	operatorCtx, stopOperator := context.WithCancel(ctx)
	defer stopOperator()
	go func() {
		operatorErr <- operator.Start(operatorCtx)
	}()

	imported := requireTypedGroupConditionByList(ctx, t, typedClient, "nsx-a.example.test-remote-import", nsxv1alpha.ConditionSynced, metav1.ConditionTrue)
	if len(imported.Finalizers) != 0 {
		t.Fatalf("imported finalizers = %v, want none", imported.Finalizers)
	}
	if imported.Spec.Mode != nsxv1alpha.NSXGroupModeObserve || imported.Spec.DisplayName != "Remote Import" {
		t.Fatalf("imported spec = %#v, want Observe Remote Import", imported.Spec)
	}
	replaced := requireTypedGroupConditionByList(ctx, t, typedClient, "observe-drifted", nsxv1alpha.ConditionSynced, metav1.ConditionTrue)
	if slices.Contains(replaced.Finalizers, stateoperator.GroupFinalizer) {
		t.Fatalf("replaced finalizers = %v, want no %q", replaced.Finalizers, stateoperator.GroupFinalizer)
	}
	if replaced.Spec.DisplayName != "Remote Replacement" || !reflect.DeepEqual(replaced.Spec.CIDRs, []string{"10.71.0.0/24"}) {
		t.Fatalf("replaced spec = %#v, want remote replacement spec", replaced.Spec)
	}
	requireTypedGroupDeletedByList(ctx, t, typedClient, "observe-stale")

	if groupGets := counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "groups.get"}); groupGets != 0 {
		t.Fatalf("typed kube api groups.get count = %v, want zero during manager sweep verification", groupGets)
	}
	if cloudGets := counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "network_clouds.get"}); cloudGets != 0 {
		t.Fatalf("typed kube api networkclouds.get count = %v, want zero during manager sweep verification", cloudGets)
	}
	t.Logf(
		"typed kube api manager sweep counts: groups.list=%v groups.create=%v groups.update=%v groups.update_status=%v groups.patch=%v groups.delete=%v networkclouds.update_status=%v groups.get=%v networkclouds.get=%v",
		counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "groups.list"}),
		counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "groups.create"}),
		counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "groups.update"}),
		counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "groups.update_status"}),
		counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "groups.apply"}),
		counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "groups.delete"}),
		counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "network_clouds.update_status"}),
		counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "groups.get"}),
		counterValueOrZero(t, registry, "nsx_operator_kubernetes_api_calls_total", map[string]string{"function": "network_clouds.get"}),
	)

	stopOperator()
	if err := <-operatorErr; err != nil {
		t.Fatalf("operator Start() error = %v", err)
	}
	expectedMetrics := `
# HELP nsx_operator_nsx_group_cr_creates_needed_total Last manager sweep total new group CRs that need to be created.
# TYPE nsx_operator_nsx_group_cr_creates_needed_total gauge
nsx_operator_nsx_group_cr_creates_needed_total{manager="nsx-a.example.test"} 1
# HELP nsx_operator_nsx_group_cr_updates_needed_total Last manager sweep total group CR updates needed by mode.
# TYPE nsx_operator_nsx_group_cr_updates_needed_total gauge
nsx_operator_nsx_group_cr_updates_needed_total{manager="nsx-a.example.test",mode="manage"} 0
nsx_operator_nsx_group_cr_updates_needed_total{manager="nsx-a.example.test",mode="observe"} 4
# HELP nsx_operator_nsx_groups_listed_total Last manager sweep total groups listed from NSX.
# TYPE nsx_operator_nsx_groups_listed_total gauge
nsx_operator_nsx_groups_listed_total{manager="nsx-a.example.test"} 2
# HELP nsx_operator_nsx_groups_manage_total Last manager sweep total manage groups considered for this manager.
# TYPE nsx_operator_nsx_groups_manage_total gauge
nsx_operator_nsx_groups_manage_total{manager="nsx-a.example.test"} 0
# HELP nsx_operator_nsx_groups_observe_total Last manager sweep total observe groups considered for this manager.
# TYPE nsx_operator_nsx_groups_observe_total gauge
nsx_operator_nsx_groups_observe_total{manager="nsx-a.example.test"} 3
`
	if err := testutil.GatherAndCompare(
		registry, strings.NewReader(expectedMetrics),
		"nsx_operator_nsx_group_cr_creates_needed_total",
		"nsx_operator_nsx_group_cr_updates_needed_total",
		"nsx_operator_nsx_groups_listed_total",
		"nsx_operator_nsx_groups_manage_total",
		"nsx_operator_nsx_groups_observe_total",
	); err != nil {
		t.Fatalf("gather manager sweep metrics: %v", err)
	}
	for _, operation := range managerRecorder.operations {
		if strings.HasPrefix(operation, "delete-group:") {
			t.Fatalf("manager operations = %v, want no NSX delete for Observe sweep", managerRecorder.operations)
		}
	}
}

func TestDefaultManagerSweepLogsUnsupportedRemoteReason(t *testing.T) {
	typedClient, stop := startStateoperatorKubeAPIClient(t)
	t.Cleanup(stop)
	registry := prometheus.NewRegistry()
	metricsRecorder, err := operatormetrics.NewRecorder(registry, zap.NewNop())
	if err != nil {
		t.Fatalf("construct metrics recorder: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cloud := networkCloud("cloud-unsupported", "nsx-a.example.test")
	createdCloud, err := typedClient.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create typed cloud: %v", err)
	}
	cloud = createdCloud
	controllerClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(cloud).
		Build()
	managerRecorder := &operationRecorder{listGroups: []*nsxclient.Group{{
		Resource: nsxclient.Resource{ID: "remote-unsupported", DisplayName: "Remote Unsupported"},
		Expression: []json.RawMessage{
			rawExpression(t, map[string]string{"resource_type": "Condition"}),
		},
	}}}
	core, logs := observer.New(zapcore.DebugLevel)
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       controllerClient,
		KubeClient:   typedClient,
		TickInterval: time.Hour,
		Logger:       zap.New(core),
		Recorder:     metricsRecorder,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return managerRecorder, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	operatorErr := make(chan error, 1)
	operatorCtx, stopOperator := context.WithCancel(ctx)
	defer stopOperator()
	go func() {
		operatorErr <- operator.Start(operatorCtx)
	}()

	requireTypedGroupCondition(ctx, t, typedClient, "nsx-a.example.test-remote-unsupported", nsxv1alpha.ConditionUnsupportedExpression, metav1.ConditionTrue)
	stopOperator()
	if err := <-operatorErr; err != nil {
		t.Fatalf("operator Start() error = %v", err)
	}

	requireObservedLogField(t, logs, "default manager remote group has unsupported expression", "networkCloudFQDN", "nsx-a.example.test")
	requireObservedLogField(t, logs, "default manager remote group has unsupported expression", "groupID", "remote-unsupported")
	requireObservedLogField(t, logs, "default manager remote group has unsupported expression", "unsupportedReason", string(nsxv1alpha.UnsupportedExpressionReasonUnsupportedExpressionType))
}

func TestDefaultManagerSweepRepairsManagedDriftWithoutRewritingSpec(t *testing.T) {
	typedClient, stop := startStateoperatorKubeAPIClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cloud := networkCloud("cloud-manage", "nsx-a.example.test")
	createdCloud, err := typedClient.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create typed cloud: %v", err)
	}
	cloud = createdCloud
	desiredSegments := []string{"/infra/segments/desired", "/infra/segments/extra"}
	localManage := managerGroup("manage-drifted", "nsx-a.example.test", "app-drifted", nsxv1alpha.NSXGroupModeManage)
	localManage.Spec.DisplayName = "Desired App"
	localManage.Spec.CIDRs = []string{"10.80.0.0/24"}
	localManage.Spec.SegmentPaths = desiredSegments
	localManage.Generation = 6
	if _, err := typedClient.Groups().Create(ctx, localManage, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create typed managed group: %v", err)
	}

	remoteSegment := "/infra/segments/remote"
	managerRecorder := &operationRecorder{listGroups: []*nsxclient.Group{{
		Resource: nsxclient.Resource{ID: "app-drifted", DisplayName: "Remote App"},
		Expression: []json.RawMessage{
			rawExpression(t, nsxclient.IPAddressExpression{
				Resource:    nsxclient.Resource{ID: "remote-ip", ResourceType: "IPAddressExpression"},
				IPAddresses: []string{"10.99.0.0/24"},
			}),
			rawExpression(t, nsxclient.PathExpression{
				Resource: nsxclient.Resource{ID: "remote-path", ResourceType: "PathExpression"},
				Paths:    []string{remoteSegment},
			}),
		},
	}}}
	controllerClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(cloud).
		Build()
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       controllerClient,
		KubeClient:   typedClient,
		TickInterval: time.Hour,
		Logger:       zap.NewNop(),
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return managerRecorder, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	operatorErr := make(chan error, 1)
	operatorCtx, stopOperator := context.WithCancel(ctx)
	defer stopOperator()
	go func() {
		operatorErr <- operator.Start(operatorCtx)
	}()

	requireTypedGroupCondition(ctx, t, typedClient, "manage-drifted", nsxv1alpha.ConditionApplying, metav1.ConditionTrue)
	updated, err := typedClient.Groups().Get(ctx, "manage-drifted", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get managed group after sweep: %v", err)
	}
	if !reflect.DeepEqual(updated.Spec, localManage.Spec) {
		t.Fatalf("managed spec = %#v, want desired spec preserved %#v", updated.Spec, localManage.Spec)
	}
	groupPatch := managerRecorder.groupPatches["app-drifted"]
	if groupPatch == nil {
		t.Fatalf("patched group missing; operations = %v", managerRecorder.operations)
	}
	if groupPatch.DisplayName != "Desired App" {
		t.Fatalf("patched group = %#v, want desired display name", groupPatch)
	}
	ipExpression := managerRecorder.ipExpressions["app-drifted:remote-ip"]
	if ipExpression == nil {
		t.Fatalf("patched IP expression missing; operations = %v", managerRecorder.operations)
	}
	if got := ipExpression.IPAddresses; !reflect.DeepEqual(got, []string{"10.80.0.0/24"}) {
		t.Fatalf("patched IP expression addresses = %v, want desired CIDRs", got)
	}
	pathExpression := managerRecorder.pathExpressions["app-drifted:remote-path"]
	if pathExpression == nil {
		t.Fatalf("patched path expression missing; operations = %v", managerRecorder.operations)
	}
	if got := pathExpression.Paths; !reflect.DeepEqual(got, desiredSegments) {
		t.Fatalf("patched path expression paths = %v, want desired segment paths", got)
	}

	stopOperator()
	if err := <-operatorErr; err != nil {
		t.Fatalf("operator Start() error = %v", err)
	}
}

func TestDefaultManagerSweepRemovesManagedFinalizerAfterConfirmedRemoteAbsence(t *testing.T) {
	typedClient, stop := startStateoperatorKubeAPIClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cloud := networkCloud("cloud-delete", "nsx-a.example.test")
	createdCloud, err := typedClient.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create typed cloud: %v", err)
	}
	cloud = createdCloud
	localManage := managerGroup("manage-deleted", "nsx-a.example.test", "app-deleted", nsxv1alpha.NSXGroupModeManage)
	localManage.Finalizers = []string{stateoperator.GroupFinalizer, "example.test/keep"}
	if _, err := typedClient.Groups().Create(ctx, localManage, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create typed managed group: %v", err)
	}
	if err := typedClient.Groups().Delete(ctx, localManage.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete typed managed group: %v", err)
	}

	managerRecorder := &operationRecorder{}
	controllerClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(cloud).
		Build()
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       controllerClient,
		KubeClient:   typedClient,
		TickInterval: time.Hour,
		Logger:       zap.NewNop(),
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return managerRecorder, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	operatorErr := make(chan error, 1)
	operatorCtx, stopOperator := context.WithCancel(ctx)
	defer stopOperator()
	go func() {
		operatorErr <- operator.Start(operatorCtx)
	}()

	requireTypedGroupFinalizerRemoved(ctx, t, typedClient, "manage-deleted", stateoperator.GroupFinalizer)
	stopOperator()
	if err := <-operatorErr; err != nil {
		t.Fatalf("operator Start() error = %v", err)
	}
	for _, operation := range managerRecorder.operations {
		if strings.HasPrefix(operation, "delete-group:") {
			t.Fatalf("manager operations = %v, want no NSX delete after absence confirmation", managerRecorder.operations)
		}
	}
}

func TestDefaultManagerSweepUpdatesCloudStatusWhenGatherFails(t *testing.T) {
	typedClient, stop := startStateoperatorKubeAPIClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cloud := networkCloud("cloud-gather-failed", "nsx-failed.example.test")
	createdCloud, err := typedClient.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create typed cloud: %v", err)
	}
	cloud = createdCloud
	controllerClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(cloud).
		Build()
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       controllerClient,
		KubeClient:   typedClient,
		TickInterval: time.Hour,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return nil, errors.New("missing credentials")
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	operatorErr := make(chan error, 1)
	operatorCtx, stopOperator := context.WithCancel(ctx)
	defer stopOperator()
	go func() {
		operatorErr <- operator.Start(operatorCtx)
	}()

	requireTypedCloudCondition(ctx, t, typedClient, "cloud-gather-failed", nsxv1alpha.ConditionReachable, metav1.ConditionFalse)

	stopOperator()
	if err := <-operatorErr; err != nil {
		t.Fatalf("operator Start() error = %v", err)
	}
}

func TestLifecycleObserveAndManageDeletionDifferAgainstMockAPI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	mock := startStateoperatorMockAPI(t, ctx)
	managerClient := newStateoperatorMockAPIClient(t, mock.BaseURL())
	if err := managerClient.PatchGroup(ctx, "observe-remote", &nsxclient.GroupPatch{DisplayName: "Observe Remote", ResourceType: "Group"}); err != nil {
		t.Fatalf("seed observe remote group: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	if err := managerClient.PatchGroup(ctx, "manage-remote", &nsxclient.GroupPatch{DisplayName: "Manage Remote", ResourceType: "Group"}); err != nil {
		t.Fatalf("seed manage remote group: %v\nmockapi logs:\n%s", err, mock.Logs())
	}

	clients, stopClients := startStateoperatorClients(t)
	t.Cleanup(stopClients)
	cloud := networkCloud("cloud-lifecycle", "nsx-a.example.test")
	if _, err := clients.typed.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create lifecycle cloud: %v", err)
	}

	observe := managerGroup("observe-delete", "nsx-a.example.test", "observe-remote", nsxv1alpha.NSXGroupModeObserve)
	observe.Finalizers = []string{stateoperator.GroupFinalizer}
	if _, err := clients.typed.Groups().Create(ctx, observe, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create observe group: %v", err)
	}
	if err := clients.typed.Groups().Delete(ctx, observe.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete observe group: %v", err)
	}
	observeReconciler := stateoperator.GroupReconciler{
		Client: clients.controller,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			t.Fatal("observe deletion constructed NSX manager client")
			return nil, nil
		},
	}
	if _, err := observeReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: crclient.ObjectKey{Name: observe.Name}}); err != nil {
		t.Fatalf("reconcile observe deletion: %v", err)
	}
	requireTypedGroupDeleted(ctx, t, clients.typed, observe.Name)
	requireMockAPIGroupPresent(ctx, t, managerClient, "observe-remote")

	manage := managerGroup("manage-delete", "nsx-a.example.test", "manage-remote", nsxv1alpha.NSXGroupModeManage)
	manage.Finalizers = []string{stateoperator.GroupFinalizer}
	if _, err := clients.typed.Groups().Create(ctx, manage, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create manage group: %v", err)
	}
	if err := clients.typed.Groups().Delete(ctx, manage.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete manage group: %v", err)
	}
	manageReconciler := stateoperator.GroupReconciler{
		Client: clients.controller,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return managerClient, nil
		},
		Clock: newManualClock(time.Date(2026, 5, 19, 5, 0, 0, 0, time.UTC)),
	}
	if _, err := manageReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: crclient.ObjectKey{Name: manage.Name}}); err != nil {
		t.Fatalf("reconcile manage deletion: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	deletingManage, err := clients.typed.Groups().Get(ctx, manage.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deleting manage group: %v", err)
	}
	if !slices.Contains(deletingManage.Finalizers, stateoperator.GroupFinalizer) {
		t.Fatalf("manage finalizers after delete submit = %v, want %q kept", deletingManage.Finalizers, stateoperator.GroupFinalizer)
	}
	requireMockAPIGroupAbsent(ctx, t, managerClient, "manage-remote")

	operator, err := stateoperator.New(stateoperator.Options{
		Client:       clients.controller,
		KubeClient:   clients.typed,
		TickInterval: time.Hour,
		Logger:       zap.NewNop(),
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return managerClient, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	operatorErr := make(chan error, 1)
	operatorCtx, stopOperator := context.WithCancel(ctx)
	go func() {
		operatorErr <- operator.Start(operatorCtx)
	}()
	requireTypedGroupDeleted(ctx, t, clients.typed, manage.Name)
	stopOperator()
	if err := <-operatorErr; err != nil {
		t.Fatalf("operator Start() error = %v", err)
	}
	requireMockAPIGroupPresent(ctx, t, managerClient, "observe-remote")
	requireMockAPIGroupAbsent(ctx, t, managerClient, "manage-remote")
}

func TestLifecycleObserveMissingRemoteDeletesCRAgainstMockAPIWithoutNSXDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	mock := startStateoperatorMockAPI(t, ctx)
	managerClient := newStateoperatorMockAPIClient(t, mock.BaseURL())
	if err := managerClient.PatchGroup(ctx, "observe-missing-remote", &nsxclient.GroupPatch{DisplayName: "Observe Missing Remote", ResourceType: "Group"}); err != nil {
		t.Fatalf("seed observe remote group: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	requireMockAPIGroupPresent(ctx, t, managerClient, "observe-missing-remote")
	if err := managerClient.DeleteGroup(ctx, "observe-missing-remote"); err != nil {
		t.Fatalf("delete observe remote outside operator: %v\nmockapi logs:\n%s", err, mock.Logs())
	}
	requireMockAPIGroupAbsent(ctx, t, managerClient, "observe-missing-remote")

	clients, stopClients := startStateoperatorClients(t)
	t.Cleanup(stopClients)
	cloud := networkCloud("cloud-observe-missing", "nsx-a.example.test")
	if _, err := clients.typed.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create observe missing cloud: %v", err)
	}
	observe := managerGroup("observe-missing-remote", "nsx-a.example.test", "observe-missing-remote", nsxv1alpha.NSXGroupModeObserve)
	observe.Finalizers = []string{stateoperator.GroupFinalizer}
	if _, err := clients.typed.Groups().Create(ctx, observe, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create observe missing group: %v", err)
	}

	recordingClient := &deleteRecordingManagerClient{ManagerClient: managerClient}
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       clients.controller,
		KubeClient:   clients.typed,
		TickInterval: time.Hour,
		Logger:       zap.NewNop(),
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return recordingClient, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	operatorErr := make(chan error, 1)
	operatorCtx, stopOperator := context.WithCancel(ctx)
	go func() {
		operatorErr <- operator.Start(operatorCtx)
	}()
	requireTypedGroupDeleted(ctx, t, clients.typed, observe.Name)
	stopOperator()
	if err := <-operatorErr; err != nil {
		t.Fatalf("operator Start() error = %v", err)
	}
	if calls := recordingClient.deleteCalls(); len(calls) != 0 {
		t.Fatalf("operator NSX deletes = %v, want none for missing Observe remote", calls)
	}
	requireMockAPIGroupAbsent(ctx, t, managerClient, "observe-missing-remote")
}

func TestNetworkCloudAddAndRemoveLifecycleAgainstPublicMockAPI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)

	mock := startStateoperatorMockAPI(t, ctx)
	managerClient := newStateoperatorMockAPIClient(t, mock.BaseURL())
	if err := managerClient.PatchGroup(ctx, "networkcloud-live-remote", &nsxclient.GroupPatch{
		ID:           "networkcloud-live-remote",
		DisplayName:  "NetworkCloud Live Remote",
		ResourceType: "Group",
	}); err != nil {
		t.Fatalf("seed networkcloud remote group: %v\nmockapi logs:\n%s", err, mock.Logs())
	}

	clients, stopClients := startStateoperatorClients(t)
	t.Cleanup(stopClients)
	cloud := networkCloud("cloud-public-mockapi", mock.BaseURL())
	if _, err := clients.typed.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create public mockapi cloud: %v", err)
	}

	constructed := make(chan string, 4)
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       clients.controller,
		KubeClient:   clients.typed,
		TickInterval: time.Hour,
		Logger:       zap.NewNop(),
		ManagerClientFactory: func(_ context.Context, gotCloud nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			constructed <- names.NormalizeNetworkCloudFQDN(gotCloud.Spec.NetworkCloudFQDN)
			return managerClient, nil
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	operatorErr := make(chan error, 1)
	operatorCtx, stopOperator := context.WithCancel(ctx)
	go func() {
		operatorErr <- operator.Start(operatorCtx)
	}()
	requireTypedCloudCondition(ctx, t, clients.typed, cloud.Name, nsxv1alpha.ConditionSwept, metav1.ConditionTrue)
	normalizedFQDN := names.NormalizeNetworkCloudFQDN(mock.BaseURL())
	select {
	case gotFQDN := <-constructed:
		if gotFQDN != normalizedFQDN {
			t.Fatalf("constructed manager client for FQDN %q, want %q", gotFQDN, normalizedFQDN)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for manager client construction: %v", ctx.Err())
	}
	imported := requireTypedGroupByRemoteID(ctx, t, clients.typed, "networkcloud-live-remote")
	if imported.Spec.NetworkCloudFQDN != normalizedFQDN {
		t.Fatalf("imported group networkCloudFQDN = %q, want %q", imported.Spec.NetworkCloudFQDN, normalizedFQDN)
	}
	if imported.Spec.Mode != nsxv1alpha.NSXGroupModeObserve {
		t.Fatalf("imported group mode = %q, want Observe", imported.Spec.Mode)
	}
	stopOperator()
	if err := <-operatorErr; err != nil {
		t.Fatalf("operator Start() error = %v", err)
	}

	if err := clients.typed.NetworkClouds().Delete(ctx, cloud.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete public mockapi cloud: %v", err)
	}
	if _, err := clients.typed.NetworkClouds().Get(ctx, cloud.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("get deleted public mockapi cloud error = %v, want NotFound", err)
	}
	if _, err := clients.typed.Groups().Get(ctx, imported.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("get imported group after cloud deletion: %v", err)
	}

	sweepAfterDeleteCalled := make(chan struct{}, 1)
	secondOperator, err := stateoperator.New(stateoperator.Options{
		Client:       clients.controller,
		KubeClient:   clients.typed,
		TickInterval: time.Hour,
		Logger:       zap.NewNop(),
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			sweepAfterDeleteCalled <- struct{}{}
			return managerClient, nil
		},
	})
	if err != nil {
		t.Fatalf("New() second operator error = %v", err)
	}
	secondErr := make(chan error, 1)
	secondCtx, stopSecond := context.WithCancel(ctx)
	go func() {
		secondErr <- secondOperator.Start(secondCtx)
	}()
	requireNotClosed(t, sweepAfterDeleteCalled, "default manager sweep after public mockapi cloud deletion")
	stopSecond()
	if err := <-secondErr; err != nil {
		t.Fatalf("second operator Start() error = %v", err)
	}
}

func TestLifecycleCloudDeletionLeavesChildGroupsAndStopsDefaultSweepThroughTypedKubeAPI(t *testing.T) {
	clients, stop := startStateoperatorClients(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cloud := networkCloud("cloud-delete", "nsx-a.example.test")
	if _, err := clients.typed.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create cloud: %v", err)
	}
	child := managerGroup("child-survives", "nsx-a.example.test", "child-survives", nsxv1alpha.NSXGroupModeManage)
	child.Finalizers = []string{stateoperator.GroupFinalizer}
	if _, err := clients.typed.Groups().Create(ctx, child, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create child group: %v", err)
	}

	firstSweepRecorder := &operationRecorder{}
	firstOperator, err := stateoperator.New(stateoperator.Options{
		Client:       clients.controller,
		KubeClient:   clients.typed,
		TickInterval: time.Hour,
		Logger:       zap.NewNop(),
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return firstSweepRecorder, nil
		},
	})
	if err != nil {
		t.Fatalf("New() first operator error = %v", err)
	}
	firstErr := make(chan error, 1)
	firstCtx, stopFirst := context.WithCancel(ctx)
	go func() {
		firstErr <- firstOperator.Start(firstCtx)
	}()
	requireTypedCloudCondition(ctx, t, clients.typed, cloud.Name, nsxv1alpha.ConditionSwept, metav1.ConditionTrue)
	stopFirst()
	if err := <-firstErr; err != nil {
		t.Fatalf("first operator Start() error = %v", err)
	}
	if len(firstSweepRecorder.operations) == 0 {
		t.Fatalf("first sweep operations = %v, want default manager sweep to run", firstSweepRecorder.operations)
	}

	if err := clients.typed.NetworkClouds().Delete(ctx, cloud.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete cloud: %v", err)
	}
	if _, err := clients.typed.Groups().Get(ctx, child.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("get child group after cloud deletion: %v", err)
	}

	clock := newManualClock(time.Date(2026, 5, 19, 5, 30, 0, 0, time.UTC))
	sweepAfterDeleteCalled := make(chan struct{}, 1)
	secondOperator, err := stateoperator.New(stateoperator.Options{
		Client:       clients.controller,
		KubeClient:   clients.typed,
		TickInterval: time.Hour,
		Logger:       zap.NewNop(),
		Clock:        clock,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			sweepAfterDeleteCalled <- struct{}{}
			return &operationRecorder{}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() second operator error = %v", err)
	}
	secondErr := make(chan error, 1)
	secondCtx, stopSecond := context.WithCancel(ctx)
	go func() {
		secondErr <- secondOperator.Start(secondCtx)
	}()
	_ = clock.RequireNextTimerDuration(t)
	requireNotClosed(t, sweepAfterDeleteCalled, "default manager sweep after cloud deletion")
	stopSecond()
	if err := <-secondErr; err != nil {
		t.Fatalf("second operator Start() error = %v", err)
	}
	if _, err := clients.typed.Groups().Get(ctx, child.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("get child group after skipped sweep: %v", err)
	}
}

func managerGroup(name string, fqdn string, groupID string, mode nsxv1alpha.NSXGroupMode) *nsxv1alpha.NSXGroup {
	return &nsxv1alpha.NSXGroup{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: nsxv1alpha.NSXGroupSpec{
			NetworkCloudFQDN: fqdn,
			GroupID:          groupID,
			DisplayName:      "Group " + groupID,
			Mode:             mode,
			CIDRs:            []string{"10.0.0.0/24"},
		},
	}
}

type operationRecorder struct {
	operations      []string
	listGroups      []*nsxclient.Group
	groupPatches    map[string]*nsxclient.GroupPatch
	ipExpressions   map[string]*nsxclient.IPAddressExpressionPatch
	pathExpressions map[string]*nsxclient.PathExpressionPatch
	patchGroupErr   error
	deleteGroupErr  error
}

type deleteRecordingManagerClient struct {
	stateoperator.ManagerClient
	mu      sync.Mutex
	deletes []string
}

func (c *deleteRecordingManagerClient) DeleteGroup(ctx context.Context, groupID string) error {
	c.mu.Lock()
	c.deletes = append(c.deletes, groupID)
	c.mu.Unlock()
	return fmt.Errorf("unexpected operator nsx delete for %q", groupID)
}

func (c *deleteRecordingManagerClient) deleteCalls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.deletes...)
}

func (r *operationRecorder) ApplyGroup(_ context.Context, group nsxv1alpha.NSXGroup) error {
	r.operations = append(r.operations, "apply-group:"+group.Name)
	return nil
}

func (r *operationRecorder) ApplyManagerKubeWrites(_ context.Context, writes stateoperator.ManagerKubeWritePlan) error {
	for _, key := range sortedTestBatchKeys(writes.GroupCreates) {
		request := writes.GroupCreates[key]
		r.operations = append(r.operations, "apply-group:"+request.Object.Name)
	}
	for _, key := range sortedTestBatchKeys(writes.GroupUpdates) {
		request := writes.GroupUpdates[key]
		r.operations = append(r.operations, "apply-group:"+request.Object.Name)
	}
	for _, key := range sortedTestBatchKeys(writes.GroupStatusUpdates) {
		request := writes.GroupStatusUpdates[key]
		r.operations = append(r.operations, "group-status:"+request.Name)
	}
	for _, key := range sortedTestBatchKeys(writes.GroupStatusesAfterGroupWrite) {
		pending := writes.GroupStatusesAfterGroupWrite[key]
		r.operations = append(r.operations, "group-status:"+pending.Name)
	}
	for _, key := range sortedTestBatchKeys(writes.GroupFinalizerPatches) {
		request := writes.GroupFinalizerPatches[key]
		r.operations = append(r.operations, "remove-finalizer:"+request.Name+":"+stateoperator.GroupFinalizer)
	}
	for _, key := range sortedTestBatchKeys(writes.GroupFinalizersAfterGroupWrite) {
		pending := writes.GroupFinalizersAfterGroupWrite[key]
		r.operations = append(r.operations, "remove-finalizer:"+pending.Name+":"+stateoperator.GroupFinalizer)
	}
	for _, key := range sortedTestBatchKeys(writes.GroupFinalizersAfterStatusWrite) {
		pending := writes.GroupFinalizersAfterStatusWrite[key]
		r.operations = append(r.operations, "remove-finalizer:"+pending.Name+":"+stateoperator.GroupFinalizer)
	}
	for _, key := range sortedTestBatchKeys(writes.GroupDeletes) {
		request := writes.GroupDeletes[key]
		r.operations = append(r.operations, "delete-group-cr:"+request.Name)
	}
	for _, key := range sortedTestBatchKeys(writes.CloudStatusUpdates) {
		request := writes.CloudStatusUpdates[key]
		r.operations = append(r.operations, "cloud-status:"+request.Name)
	}
	return nil
}

func sortedTestBatchKeys[T any](items map[kubeapi.BatchKey]T) []kubeapi.BatchKey {
	keys := make([]kubeapi.BatchKey, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a kubeapi.BatchKey, b kubeapi.BatchKey) int {
		if a.Operation != b.Operation {
			return strings.Compare(a.Operation, b.Operation)
		}
		if a.Resource != b.Resource {
			return strings.Compare(a.Resource, b.Resource)
		}
		if a.Subresource != b.Subresource {
			return strings.Compare(a.Subresource, b.Subresource)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return keys
}

func (r *operationRecorder) UpdateGroupStatus(_ context.Context, name string, _ nsxv1alpha.NSXGroupStatus) error {
	r.operations = append(r.operations, "group-status:"+name)
	return nil
}

func (r *operationRecorder) DeleteGroupCR(_ context.Context, name string) error {
	r.operations = append(r.operations, "delete-group-cr:"+name)
	return nil
}

func (r *operationRecorder) RemoveGroupFinalizer(_ context.Context, name string, finalizer string) error {
	r.operations = append(r.operations, "remove-finalizer:"+name+":"+finalizer)
	return nil
}

func (r *operationRecorder) UpdateCloudStatus(_ context.Context, name string, _ nsxv1alpha.NSXNetworkCloudStatus) error {
	r.operations = append(r.operations, "cloud-status:"+name)
	return nil
}

func (r *operationRecorder) ListGroups(context.Context) ([]*nsxclient.Group, error) {
	return r.listGroups, nil
}

func (r *operationRecorder) PatchGroup(_ context.Context, groupID string, group *nsxclient.GroupPatch) error {
	r.operations = append(r.operations, "patch-group:"+groupID)
	if r.groupPatches == nil {
		r.groupPatches = map[string]*nsxclient.GroupPatch{}
	}
	copied := *group
	r.groupPatches[groupID] = &copied
	return r.patchGroupErr
}

func (r *operationRecorder) PatchGroupIPAddressExpression(_ context.Context, groupID string, expressionID string, expression *nsxclient.IPAddressExpressionPatch) error {
	r.operations = append(r.operations, "patch-ip:"+groupID+":"+expressionID)
	r.recordIPAddressExpression(groupID, expressionID, expression)
	return nil
}

func (r *operationRecorder) AddGroupIPAddressExpression(_ context.Context, groupID string, expressionID string, expression *nsxclient.IPAddressExpressionPatch) error {
	r.operations = append(r.operations, "add-ip:"+groupID+":"+expressionID)
	r.recordIPAddressExpression(groupID, expressionID, expression)
	return nil
}

func (r *operationRecorder) DeleteGroupIPAddressExpression(_ context.Context, groupID string, expressionID string) error {
	r.operations = append(r.operations, "delete-ip:"+groupID+":"+expressionID)
	return nil
}

func (r *operationRecorder) PatchGroupPathExpression(_ context.Context, groupID string, expressionID string, expression *nsxclient.PathExpressionPatch) error {
	r.operations = append(r.operations, "patch-path:"+groupID+":"+expressionID)
	r.recordPathExpression(groupID, expressionID, expression)
	return nil
}

func (r *operationRecorder) AddGroupPathExpression(_ context.Context, groupID string, expressionID string, expression *nsxclient.PathExpressionPatch) error {
	r.operations = append(r.operations, "add-path:"+groupID+":"+expressionID)
	r.recordPathExpression(groupID, expressionID, expression)
	return nil
}

func (r *operationRecorder) DeleteGroupPathExpression(_ context.Context, groupID string, expressionID string) error {
	r.operations = append(r.operations, "delete-path:"+groupID+":"+expressionID)
	return nil
}

func (r *operationRecorder) DeleteGroup(_ context.Context, groupID string) error {
	r.operations = append(r.operations, "delete-group:"+groupID)
	return r.deleteGroupErr
}

func (r *operationRecorder) recordPathExpression(groupID string, expressionID string, expression *nsxclient.PathExpressionPatch) {
	if r.pathExpressions == nil {
		r.pathExpressions = map[string]*nsxclient.PathExpressionPatch{}
	}
	copied := *expression
	copied.Paths = append([]string(nil), expression.Paths...)
	r.pathExpressions[groupID+":"+expressionID] = &copied
}

func (r *operationRecorder) recordIPAddressExpression(groupID string, expressionID string, expression *nsxclient.IPAddressExpressionPatch) {
	if r.ipExpressions == nil {
		r.ipExpressions = map[string]*nsxclient.IPAddressExpressionPatch{}
	}
	copied := *expression
	copied.IPAddresses = append([]string(nil), expression.IPAddresses...)
	r.ipExpressions[groupID+":"+expressionID] = &copied
}

func rawExpression(t *testing.T, value any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal expression: %v", err)
	}
	return raw
}

func startStateoperatorKubeAPIClient(t *testing.T) (*kubeapi.Client, func()) {
	t.Helper()

	clients, stop := startStateoperatorClients(t)
	return clients.typed, stop
}

func startStateoperatorKubeAPIClientWithRecorder(t *testing.T, recorder operatormetrics.Recorder) (*kubeapi.Client, func()) {
	t.Helper()

	clients, stop := startStateoperatorClientsWithRecorder(t, recorder)
	return clients.typed, stop
}

type stateoperatorClients struct {
	typed      *kubeapi.Client
	controller crclient.Client
}

func startStateoperatorClients(t *testing.T) (stateoperatorClients, func()) {
	t.Helper()
	return startStateoperatorClientsWithRecorder(t, operatormetrics.NopRecorder{})
}

func startStateoperatorClientsWithRecorder(t *testing.T, recorder operatormetrics.Recorder) (stateoperatorClients, func()) {
	t.Helper()

	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}
	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{stateoperatorRepoPath(t, "crds")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	typedClient, err := kubeapi.NewClient(kubeapi.Options{Config: restConfig, Recorder: recorder})
	if err != nil {
		if stopErr := testEnvironment.Stop(); stopErr != nil {
			t.Errorf("stop envtest API server after typed client failure: %v", stopErr)
		}
		t.Fatalf("construct typed kube client: %v", err)
	}
	scheme := newScheme(t)
	controllerClient, err := crclient.New(restConfig, crclient.Options{Scheme: scheme})
	if err != nil {
		if stopErr := testEnvironment.Stop(); stopErr != nil {
			t.Errorf("stop envtest API server after controller client failure: %v", stopErr)
		}
		t.Fatalf("construct controller client: %v", err)
	}
	return stateoperatorClients{
			typed:      typedClient,
			controller: controllerClient,
		}, func() {
			if err := testEnvironment.Stop(); err != nil {
				t.Errorf("stop envtest API server: %v", err)
			}
		}
}

const (
	stateoperatorMockAPIUsername = mockapi.Username
	stateoperatorMockAPIPassword = mockapi.Password
)

func startStateoperatorMockAPI(t *testing.T, ctx context.Context) mockapi.Process {
	t.Helper()
	return mockapi.Start(t, ctx)
}

func newStateoperatorMockAPIClient(t *testing.T, baseURL string) *nsxclient.Client {
	t.Helper()

	client, err := nsxclient.NewClient(nsxclient.Options{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		Username:   stateoperatorMockAPIUsername,
		Password:   stateoperatorMockAPIPassword,
		Logger:     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("construct mockapi nsx client: %v", err)
	}
	return client
}

func requireMockAPIGroupPresent(ctx context.Context, t *testing.T, client stateoperator.ManagerClient, groupID string) {
	t.Helper()

	requireMockAPIGroupPresence(ctx, t, client, groupID, true)
}

func requireMockAPIGroupAbsent(ctx context.Context, t *testing.T, client stateoperator.ManagerClient, groupID string) {
	t.Helper()

	requireMockAPIGroupPresence(ctx, t, client, groupID, false)
}

func requireMockAPIGroupPresence(ctx context.Context, t *testing.T, client stateoperator.ManagerClient, groupID string, present bool) {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		groups, err := client.ListGroups(ctx)
		if err != nil {
			t.Fatalf("list mockapi groups: %v", err)
		}
		found := false
		for _, group := range groups {
			if group != nil && group.ID == groupID {
				found = true
				break
			}
		}
		if found == present {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for mockapi group %q present=%t; groups = %#v", groupID, present, groups)
		case <-ticker.C:
		}
	}
}

func requireTypedGroupByRemoteID(ctx context.Context, t *testing.T, typedClient *kubeapi.Client, groupID string) nsxv1alpha.NSXGroup {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		groups, err := typedClient.Groups().List(ctx, kubeapi.ListOptions{})
		if err != nil {
			t.Fatalf("list typed groups: %v", err)
		}
		for _, group := range groups.Items {
			if group.Spec.GroupID == groupID {
				return group
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for typed group with groupID %q; groups = %#v", groupID, groups.Items)
		case <-ticker.C:
		}
	}
}

func requireTypedGroupCondition(
	ctx context.Context,
	t *testing.T,
	typedClient *kubeapi.Client,
	name string,
	conditionType string,
	status metav1.ConditionStatus,
) {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		group, err := typedClient.Groups().Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			for _, condition := range group.Status.Conditions {
				if condition.Type == conditionType && condition.Status == status {
					return
				}
			}
		} else if !apierrors.IsNotFound(err) {
			t.Fatalf("get typed group %q: %v", name, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for group %q condition %s=%s", name, conditionType, status)
		case <-ticker.C:
		}
	}
}

func requireTypedGroupConditionByList(
	ctx context.Context,
	t *testing.T,
	typedClient *kubeapi.Client,
	name string,
	conditionType string,
	status metav1.ConditionStatus,
) nsxv1alpha.NSXGroup {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		groups, err := typedClient.Groups().List(ctx, kubeapi.ListOptions{})
		if err != nil {
			t.Fatalf("list typed groups while waiting for %q: %v", name, err)
		}
		for _, group := range groups.Items {
			if group.Name != name {
				continue
			}
			for _, condition := range group.Status.Conditions {
				if condition.Type == conditionType && condition.Status == status {
					return group
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for group %q condition %s=%s via list", name, conditionType, status)
		case <-ticker.C:
		}
	}
}

func requireTypedGroupDeleted(ctx context.Context, t *testing.T, typedClient *kubeapi.Client, name string) {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := typedClient.Groups().Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return
		}
		if err != nil {
			t.Fatalf("get typed group %q: %v", name, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for group %q to be deleted", name)
		case <-ticker.C:
		}
	}
}

func requireTypedGroupDeletedByList(ctx context.Context, t *testing.T, typedClient *kubeapi.Client, name string) {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		groups, err := typedClient.Groups().List(ctx, kubeapi.ListOptions{})
		if err != nil {
			t.Fatalf("list typed groups while waiting for delete %q: %v", name, err)
		}
		found := false
		for _, group := range groups.Items {
			if group.Name == name {
				found = true
				break
			}
		}
		if !found {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for group %q to be deleted via list", name)
		case <-ticker.C:
		}
	}
}

func requireTypedGroupFinalizerRemoved(ctx context.Context, t *testing.T, typedClient *kubeapi.Client, name string, finalizer string) {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		group, err := typedClient.Groups().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get typed group %q: %v", name, err)
		}
		if !slices.Contains(group.Finalizers, finalizer) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for group %q finalizers to remove %q; finalizers = %v", name, finalizer, group.Finalizers)
		case <-ticker.C:
		}
	}
}

func counterValueOrZero(t *testing.T, registry *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather registry: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metric.GetCounter() != nil && metricLabelsContain(metric.GetLabel(), labels) {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func metricLabelsContain(actual []*dto.LabelPair, want map[string]string) bool {
	for key, value := range want {
		found := false
		for _, label := range actual {
			if label.GetName() == key && label.GetValue() == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func requireTypedCloudCondition(
	ctx context.Context,
	t *testing.T,
	typedClient *kubeapi.Client,
	name string,
	conditionType string,
	status metav1.ConditionStatus,
) {
	t.Helper()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		cloud, err := typedClient.NetworkClouds().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get typed cloud %q: %v", name, err)
		}
		for _, condition := range cloud.Status.Conditions {
			if condition.Type == conditionType && condition.Status == status {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for cloud %q condition %s=%s", name, conditionType, status)
		case <-ticker.C:
		}
	}
}

func requireObservedLogField(t *testing.T, logs *observer.ObservedLogs, message string, key string, want string) {
	t.Helper()

	for _, entry := range logs.All() {
		if entry.Message != message {
			continue
		}
		fields := entry.ContextMap()
		got, ok := fields[key]
		if ok && got == want {
			return
		}
	}
	t.Fatalf("log %q field %s=%q not found; logs=%#v", message, key, want, logs.All())
}

func stateoperatorRepoPath(t *testing.T, elements ...string) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current test file path")
	}
	parts := append([]string{filepath.Dir(filename), "..", ".."}, elements...)
	return filepath.Clean(filepath.Join(parts...))
}

func statusFor(t *testing.T, plans []stateoperator.GroupStatusPlan, name string) nsxv1alpha.NSXGroupStatus {
	t.Helper()

	plan, found := findGroupStatusPlan(plans, name)
	if found {
		return plan.Status
	}
	t.Fatalf("status plan %q not found in %#v", name, plans)
	return nsxv1alpha.NSXGroupStatus{}
}

func findGroupStatusPlan(plans []stateoperator.GroupStatusPlan, name string) (stateoperator.GroupStatusPlan, bool) {
	for _, plan := range plans {
		if plan.Name == name {
			return plan, true
		}
	}
	return stateoperator.GroupStatusPlan{}, false
}

func alreadySyncedManagedStatus(t *testing.T, observedGeneration int64, transitionTime time.Time) nsxv1alpha.NSXGroupStatus {
	t.Helper()

	status, err := statuscondition.BuildGroupStatus(
		nsxv1alpha.NSXGroupStatus{},
		observedGeneration,
		transitionTime,
		statuscondition.RemotePresent(metav1.ConditionTrue, "RemoteFound", "remote NSX group is present"),
		statuscondition.SpecMatchesRemote(metav1.ConditionTrue, "SpecMatches", "local group matches remote NSX group"),
		statuscondition.UnsupportedExpression(metav1.ConditionFalse, "SupportedExpression", "remote NSX group expression is representable"),
		statuscondition.Realized(metav1.ConditionTrue, "Realized", "remote NSX group is realized"),
		statuscondition.Synced(metav1.ConditionTrue, metav1.ConditionTrue, metav1.ConditionFalse, metav1.ConditionTrue, "Synced", "local group matches remote NSX group"),
		statuscondition.Applying(metav1.ConditionFalse, "NotApplying", "no NSX write is planned"),
		statuscondition.Deleting(metav1.ConditionFalse, "NotDeleting", "no NSX delete is planned"),
	)
	if err != nil {
		t.Fatalf("build already-synced managed status: %v", err)
	}
	return status
}

func alreadySweptCloudStatus(t *testing.T, observedGeneration int64, transitionTime time.Time) nsxv1alpha.NSXNetworkCloudStatus {
	t.Helper()

	status, err := statuscondition.BuildNetworkCloudStatus(
		nsxv1alpha.NSXNetworkCloudStatus{},
		observedGeneration,
		transitionTime,
		statuscondition.Reachable(metav1.ConditionTrue, "GatherSucceeded", "NSX manager gather completed"),
		statuscondition.Swept(metav1.ConditionTrue, "SweepPlanned", "manager snapshot was processed"),
	)
	if err != nil {
		t.Fatalf("build already-swept cloud status: %v", err)
	}
	return status
}

func requireCondition(
	t *testing.T,
	conditions []metav1.Condition,
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
	message string,
	now time.Time,
) {
	t.Helper()

	for _, condition := range conditions {
		if condition.Type != conditionType {
			continue
		}
		if condition.Status != status || condition.Reason != reason || condition.Message != message {
			t.Fatalf("condition %s = %#v, want status=%s reason=%s message=%q", conditionType, condition, status, reason, message)
		}
		if !condition.LastTransitionTime.Time.Equal(now) {
			t.Fatalf("condition %s LastTransitionTime = %s, want %s", conditionType, condition.LastTransitionTime.Time, now)
		}
		return
	}
	t.Fatalf("condition %s not found in %#v", conditionType, conditions)
}

func requireObservedGeneration(t *testing.T, conditions []metav1.Condition, conditionType string, observedGeneration int64) {
	t.Helper()

	for _, condition := range conditions {
		if condition.Type == conditionType {
			if condition.ObservedGeneration != observedGeneration {
				t.Fatalf("condition %s ObservedGeneration = %d, want %d", conditionType, condition.ObservedGeneration, observedGeneration)
			}
			return
		}
	}
	t.Fatalf("condition %s not found in %#v", conditionType, conditions)
}

func requireConditionTypes(t *testing.T, conditions []metav1.Condition, want []string) {
	t.Helper()

	got := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		got = append(got, condition.Type)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("condition types = %v, want %v", got, want)
	}
}

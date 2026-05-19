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
	"strings"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
	"github.com/djosh34/nsx-operator/internal/stateoperator"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestProcessManagerSnapshotGatherFailureOnlyPlansCloudStatus(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
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
	requireCondition(t, gotConditions, nsxv1alpha.ConditionReachable, metav1.ConditionFalse, "GatherFailed", "nsx unavailable", now)
	requireCondition(t, gotConditions, nsxv1alpha.ConditionSwept, metav1.ConditionFalse, "GatherFailed", "nsx unavailable", now)
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

func TestProcessManagerSnapshotImportsRemoteOnlyGroupsAsObserveUpserts(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 30, 0, 0, time.UTC)
	segmentPath := "/infra/segments/web"
	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "NSX-A.Example.Test"),
		NetworkCloudFQDN: "nsx-a.example.test",
		RemoteGroups: []stateoperator.RemoteGroup{{
			Key:         stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-web"},
			DisplayName: "App Web",
			CIDRs:       []string{"10.20.0.0/24", "10.20.1.0/24"},
			SegmentPath: &segmentPath,
		}},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.ObserveUpserts) != 1 {
		t.Fatalf("ObserveUpserts = %#v, want one remote-only import", plan.ObserveUpserts)
	}
	upsert := plan.ObserveUpserts[0]
	if upsert.Name != "nsx-a.example.test--app-web" {
		t.Fatalf("Observe upsert name = %q, want deterministic cloud/group name", upsert.Name)
	}
	wantSpec := nsxv1alpha.NSXGroupSpec{
		NetworkCloudFQDN: "nsx-a.example.test",
		GroupID:          "app-web",
		DisplayName:      "App Web",
		Mode:             nsxv1alpha.NSXGroupModeObserve,
		CIDRs:            []string{"10.20.0.0/24", "10.20.1.0/24"},
		SegmentPath:      &segmentPath,
	}
	if !reflect.DeepEqual(upsert.Spec, wantSpec) {
		t.Fatalf("Observe upsert spec = %#v, want %#v", upsert.Spec, wantSpec)
	}
	if len(plan.GroupStatuses) != 1 {
		t.Fatalf("GroupStatuses = %#v, want one status update", plan.GroupStatuses)
	}
	if plan.GroupStatuses[0].Name != "nsx-a.example.test--app-web" {
		t.Fatalf("Group status name = %q, want observe upsert name", plan.GroupStatuses[0].Name)
	}
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionRemotePresent, metav1.ConditionTrue, "RemoteFound", "remote NSX group is present", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionSpecMatchesRemote, metav1.ConditionTrue, "SpecMatches", "local group spec matches remote NSX group", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionUnsupportedExpression, metav1.ConditionFalse, "SupportedExpression", "remote NSX group expression is representable", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionTrue, "Synced", "local group reflects remote NSX group", now)
}

func TestProcessManagerSnapshotRemoteOnlyUnsupportedExpressionMarksUnsynced(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 45, 0, 0, time.UTC)
	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
		NetworkCloudFQDN: "nsx-a.example.test",
		RemoteGroups: []stateoperator.RemoteGroup{{
			Key:                   stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-unsupported"},
			DisplayName:           "Unsupported App",
			UnsupportedExpression: true,
		}},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.GroupStatuses) != 1 {
		t.Fatalf("GroupStatuses = %#v, want one unsupported status", plan.GroupStatuses)
	}
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionUnsupportedExpression, metav1.ConditionTrue, "UnsupportedExpression", "remote NSX group expression is not fully representable", now)
	requireCondition(t, plan.GroupStatuses[0].Status.Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionFalse, "UnsupportedExpression", "remote NSX group expression needs operator support before it can be synced", now)
}

func TestProcessManagerSnapshotObserveGroupsMirrorRemoteAndDeleteWhenMissing(t *testing.T) {
	now := time.Date(2026, 5, 19, 13, 0, 0, 0, time.UTC)
	drifted := managerGroup("observe-drifted", "nsx-a.example.test", "app-drifted", nsxv1alpha.NSXGroupModeObserve)
	drifted.Spec.DisplayName = "Old App"
	drifted.Spec.CIDRs = []string{"10.30.0.0/24"}
	missing := managerGroup("observe-missing", "nsx-a.example.test", "app-missing", nsxv1alpha.NSXGroupModeObserve)

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
	if plan.ObserveUpserts[0].Spec.DisplayName != "Remote App" || !reflect.DeepEqual(plan.ObserveUpserts[0].Spec.CIDRs, []string{"10.31.0.0/24"}) {
		t.Fatalf("ObserveUpsert spec = %#v, want remote replacement spec", plan.ObserveUpserts[0].Spec)
	}
	if len(plan.ObserveDeletes) != 1 || plan.ObserveDeletes[0] != "observe-missing" {
		t.Fatalf("ObserveDeletes = %#v, want observe-missing", plan.ObserveDeletes)
	}
	if len(plan.GroupStatuses) != 1 || plan.GroupStatuses[0].Name != "observe-drifted" {
		t.Fatalf("GroupStatuses = %#v, want status for remote-present observe", plan.GroupStatuses)
	}
}

func TestProcessManagerSnapshotManageGroupsWriteMissingAndDriftedAndOnlyStatusMatching(t *testing.T) {
	now := time.Date(2026, 5, 19, 13, 30, 0, 0, time.UTC)
	drifted := managerGroup("manage-drifted", "nsx-a.example.test", "app-drifted", nsxv1alpha.NSXGroupModeManage)
	drifted.Spec.DisplayName = "Desired Drifted"
	drifted.Spec.CIDRs = []string{"10.40.0.0/24"}
	matching := managerGroup("manage-matching", "nsx-a.example.test", "app-matching", nsxv1alpha.NSXGroupModeManage)
	matching.Spec.DisplayName = "Matching"
	matching.Spec.CIDRs = []string{"10.41.0.0/24"}
	missing := managerGroup("manage-missing", "nsx-a.example.test", "app-missing", nsxv1alpha.NSXGroupModeManage)
	missing.Spec.DisplayName = "Missing"
	missing.Spec.CIDRs = []string{"10.42.0.0/24"}

	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", "nsx-a.example.test"),
		NetworkCloudFQDN: "nsx-a.example.test",
		LocalGroups:      []nsxv1alpha.NSXGroup{*matching, *missing, *drifted},
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
				IPAddressExpressionID: "matching-ip-expression",
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
	if len(plan.GroupStatuses) != 3 {
		t.Fatalf("GroupStatuses = %#v, want status plans for all manage groups", plan.GroupStatuses)
	}
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-drifted").Conditions, nsxv1alpha.ConditionApplying, metav1.ConditionTrue, "Applying", "managed NSX group update is planned", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-matching").Conditions, nsxv1alpha.ConditionSynced, metav1.ConditionTrue, "Synced", "local group matches remote NSX group", now)
	requireCondition(t, statusFor(t, plan.GroupStatuses, "manage-missing").Conditions, nsxv1alpha.ConditionRemotePresent, metav1.ConditionFalse, "RemoteMissing", "remote NSX group is missing", now)
}

func TestRemoteGroupFromNSXGroupParsesExpressionsAndFlagsUnsupported(t *testing.T) {
	segmentPath := "/infra/segments/web"
	remote := stateoperator.RemoteGroupFromNSXGroup("nsx-a.example.test", nsxclient.Group{
		Resource: nsxclient.Resource{ID: "app-web", DisplayName: "App Web"},
		Expression: []json.RawMessage{
			rawExpression(t, nsxclient.IPAddressExpression{
				Resource:    nsxclient.Resource{ID: "ip-expression", ResourceType: "IPAddressExpression"},
				IPAddresses: []string{"10.50.0.0/24"},
			}),
			rawExpression(t, nsxclient.PathExpression{
				Resource: nsxclient.Resource{ID: "path-expression", ResourceType: "PathExpression"},
				Paths:    []string{segmentPath},
			}),
		},
	})
	if remote.Key != (stateoperator.BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: "app-web"}) {
		t.Fatalf("remote key = %#v, want cloud/group binding", remote.Key)
	}
	if remote.DisplayName != "App Web" || !reflect.DeepEqual(remote.CIDRs, []string{"10.50.0.0/24"}) {
		t.Fatalf("remote parsed values = %#v", remote)
	}
	if remote.SegmentPath == nil || *remote.SegmentPath != segmentPath {
		t.Fatalf("remote segment path = %#v, want %q", remote.SegmentPath, segmentPath)
	}
	if remote.IPAddressExpressionID != "ip-expression" || remote.PathExpressionID != "path-expression" {
		t.Fatalf("remote expression IDs = ip:%q path:%q", remote.IPAddressExpressionID, remote.PathExpressionID)
	}
	if remote.UnsupportedExpression {
		t.Fatalf("UnsupportedExpression = true for representable expressions")
	}

	unsupported := stateoperator.RemoteGroupFromNSXGroup("nsx-a.example.test", nsxclient.Group{
		Resource: nsxclient.Resource{ID: "app-unsupported", DisplayName: "Unsupported App"},
		Expression: []json.RawMessage{
			rawExpression(t, map[string]string{"resource_type": "ConjunctionOperator", "id": "and"}),
			json.RawMessage(`{`),
			rawExpression(t, nsxclient.PathExpression{
				Resource: nsxclient.Resource{ID: "multi-path", ResourceType: "PathExpression"},
				Paths:    []string{"/infra/segments/first", "/infra/segments/second"},
			}),
		},
		ExtendedExpression: []json.RawMessage{rawExpression(t, map[string]string{"resource_type": "Extra"})},
	})
	if !unsupported.UnsupportedExpression {
		t.Fatalf("UnsupportedExpression = false, want true for unknown expression: %#v", unsupported)
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
	segmentPath := "/infra/segments/web"
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
				SegmentPath:           &segmentPath,
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
		ObserveDeletes: []string{"observe-missing"},
		CloudStatus:    &stateoperator.CloudStatusPlan{Name: "cloud-a"},
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
		"delete-group-cr:observe-missing",
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

func TestDefaultManagerSweepAppliesObserveUpsertStatusAndDeleteThroughTypedKubeAPI(t *testing.T) {
	typedClient, stop := startStateoperatorKubeAPIClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cloud := networkCloud("cloud-default", "nsx-a.example.test")
	if _, err := typedClient.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create typed cloud: %v", err)
	}
	localObserve := managerGroup("observe-stale", "nsx-a.example.test", "stale", nsxv1alpha.NSXGroupModeObserve)
	if _, err := typedClient.Groups().Create(ctx, localObserve, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create typed group: %v", err)
	}

	controllerClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(cloud).
		Build()
	operator, err := stateoperator.New(stateoperator.Options{
		Client:       controllerClient,
		KubeClient:   typedClient,
		TickInterval: time.Hour,
		Logger:       zap.NewExample(),
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return &operationRecorder{listGroups: []*nsxclient.Group{{
				Resource: nsxclient.Resource{ID: "remote-import", DisplayName: "Remote Import"},
			}}}, nil
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

	requireTypedGroupCondition(ctx, t, typedClient, "nsx-a.example.test--remote-import", nsxv1alpha.ConditionSynced, metav1.ConditionTrue)
	requireTypedGroupDeleted(ctx, t, typedClient, "observe-stale")

	stopOperator()
	if err := <-operatorErr; err != nil {
		t.Fatalf("operator Start() error = %v", err)
	}
}

func TestDefaultManagerSweepUpdatesCloudStatusWhenGatherFails(t *testing.T) {
	typedClient, stop := startStateoperatorKubeAPIClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cloud := networkCloud("cloud-gather-failed", "nsx-failed.example.test")
	if _, err := typedClient.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create typed cloud: %v", err)
	}
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
	operations []string
	listGroups []*nsxclient.Group
}

func (r *operationRecorder) ApplyGroup(_ context.Context, group nsxv1alpha.NSXGroup) error {
	r.operations = append(r.operations, "apply-group:"+group.Name)
	return nil
}

func (r *operationRecorder) UpdateGroupStatus(_ context.Context, name string, _ nsxv1alpha.NSXGroupStatus) error {
	r.operations = append(r.operations, "group-status:"+name)
	return nil
}

func (r *operationRecorder) DeleteGroupCR(_ context.Context, name string) error {
	r.operations = append(r.operations, "delete-group-cr:"+name)
	return nil
}

func (r *operationRecorder) UpdateCloudStatus(_ context.Context, name string, _ nsxv1alpha.NSXNetworkCloudStatus) error {
	r.operations = append(r.operations, "cloud-status:"+name)
	return nil
}

func (r *operationRecorder) ListGroups(context.Context) ([]*nsxclient.Group, error) {
	return r.listGroups, nil
}

func (r *operationRecorder) PatchGroup(_ context.Context, groupID string, _ *nsxclient.Group) error {
	r.operations = append(r.operations, "patch-group:"+groupID)
	return nil
}

func (r *operationRecorder) PatchGroupIPAddressExpression(_ context.Context, groupID string, expressionID string, _ *nsxclient.IPAddressExpression) error {
	r.operations = append(r.operations, "patch-ip:"+groupID+":"+expressionID)
	return nil
}

func (r *operationRecorder) AddGroupIPAddressExpression(_ context.Context, groupID string, expressionID string, _ *nsxclient.IPAddressExpression) error {
	r.operations = append(r.operations, "add-ip:"+groupID+":"+expressionID)
	return nil
}

func (r *operationRecorder) DeleteGroupIPAddressExpression(_ context.Context, groupID string, expressionID string) error {
	r.operations = append(r.operations, "delete-ip:"+groupID+":"+expressionID)
	return nil
}

func (r *operationRecorder) PatchGroupPathExpression(_ context.Context, groupID string, expressionID string, _ *nsxclient.PathExpression) error {
	r.operations = append(r.operations, "patch-path:"+groupID+":"+expressionID)
	return nil
}

func (r *operationRecorder) DeleteGroup(_ context.Context, groupID string) error {
	r.operations = append(r.operations, "delete-group:"+groupID)
	return nil
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

	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}
	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{stateoperatorRepoPath(t, "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	typedClient, err := kubeapi.NewClient(kubeapi.Options{Config: restConfig})
	if err != nil {
		if stopErr := testEnvironment.Stop(); stopErr != nil {
			t.Errorf("stop envtest API server after client failure: %v", stopErr)
		}
		t.Fatalf("construct typed kube client: %v", err)
	}
	return typedClient, func() {
		if err := testEnvironment.Stop(); err != nil {
			t.Errorf("stop envtest API server: %v", err)
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

	for _, plan := range plans {
		if plan.Name == name {
			return plan.Status
		}
	}
	t.Fatalf("status plan %q not found in %#v", name, plans)
	return nsxv1alpha.NSXGroupStatus{}
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

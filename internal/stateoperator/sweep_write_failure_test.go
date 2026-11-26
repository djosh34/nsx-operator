package stateoperator_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
	"github.com/djosh34/nsx-operator/internal/stateoperator"
	"github.com/djosh34/nsx-operator/internal/statuscondition"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const failingWritesGroupsPath = "/policy/api/v1/infra/domains/default/groups"

// failingWritesNSX is an NSX manager fake that serves a fixed group list and
// answers every mutating request with HTTP 500.
type failingWritesNSX struct {
	t        *testing.T
	listBody string

	mu       sync.Mutex
	putPaths []string
}

func newFailingWritesNSX(t *testing.T, listBody string) (*failingWritesNSX, *httptest.Server) {
	t.Helper()
	fake := &failingWritesNSX{t: t, listBody: listBody}
	server := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(server.Close)
	return fake, server
}

func (f *failingWritesNSX) handle(w http.ResponseWriter, req *http.Request) {
	switch {
	case req.Method == http.MethodGet && req.URL.Path == failingWritesGroupsPath:
		w.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(w, f.listBody)
		if err != nil {
			f.t.Errorf("write nsx group list: %v", err)
		}
	case req.Method == http.MethodPut && strings.HasPrefix(req.URL.Path, failingWritesGroupsPath+"/"):
		f.mu.Lock()
		f.putPaths = append(f.putPaths, req.URL.Path)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, err := io.WriteString(w, `{"error_code":500,"error_message":"mock put failure"}`)
		if err != nil {
			f.t.Errorf("write nsx put failure: %v", err)
		}
	default:
		f.t.Errorf("unexpected nsx request %s %s", req.Method, req.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *failingWritesNSX) puts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.putPaths...)
}

func newFailingWritesRunner(t *testing.T, typed *kubeapi.Client, baseURL string, now time.Time) *stateoperator.DefaultReconcilePassRunner {
	t.Helper()
	managerClient, err := nsxclient.NewClient(nsxclient.Options{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		Username:   "nsx-admin",
		Password:   "nsx-password",
		Logger:     zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("construct nsx client: %v", err)
	}
	return stateoperator.NewDefaultReconcilePassRunner(stateoperator.ReconcilePassRunnerOptions{
		KubeClient: stateoperator.NewKubeReconcilePassClient(typed, zap.NewNop()),
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			return managerClient, nil
		},
		Logger: zap.NewNop(),
		Clock:  &fixedClock{now: now},
	})
}

// createPreviouslySyncedManageGroup creates a Manage NSXGroup whose status says it
// was fully synced with NSX on an earlier pass.
func createPreviouslySyncedManageGroup(ctx context.Context, t *testing.T, typed *kubeapi.Client, name string, fqdn string, groupID string, syncedAt time.Time) {
	t.Helper()
	group := managerGroup(name, fqdn, groupID, nsxv1alpha.NSXGroupModeManage)
	group.Finalizers = []string{stateoperator.GroupFinalizer}
	created, err := typed.Groups().Create(ctx, group, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create manage group %q: %v", name, err)
	}
	status, err := statuscondition.BuildGroupStatus(
		nsxv1alpha.NSXGroupStatus{},
		created.Generation,
		syncedAt,
		statuscondition.RemotePresent(metav1.ConditionTrue, "RemoteFound", "remote NSX group is present"),
		statuscondition.SpecMatchesRemote(metav1.ConditionTrue, "SpecMatches", "local group matches remote NSX group"),
		statuscondition.UnsupportedExpression(metav1.ConditionFalse, "SupportedExpression", "remote NSX group expression is representable"),
		statuscondition.Realized(metav1.ConditionTrue, "Realized", "remote NSX group is realized"),
		statuscondition.Synced(metav1.ConditionTrue, metav1.ConditionTrue, metav1.ConditionFalse, metav1.ConditionTrue, "Synced", "local group matches remote NSX group"),
		statuscondition.Applying(metav1.ConditionFalse, "NotApplying", "no NSX write is planned"),
		statuscondition.Deleting(metav1.ConditionFalse, "NotDeleting", "no NSX delete is planned"),
	)
	if err != nil {
		t.Fatalf("build synced status for %q: %v", name, err)
	}
	_, err = typed.Groups().UpdateStatus(ctx, name, *status, kubeapi.StatusUpdateOptions{ResourceVersion: created.ResourceVersion})
	if err != nil {
		t.Fatalf("seed synced status for %q: %v", name, err)
	}
}

func requireGroupConditionState(t *testing.T, group *nsxv1alpha.NSXGroup, conditionType string, status metav1.ConditionStatus, reason string, messageContains string) {
	t.Helper()
	for _, condition := range group.Status.Conditions {
		if condition.Type != conditionType {
			continue
		}
		if condition.Status != status || condition.Reason != reason || !strings.Contains(condition.Message, messageContains) {
			t.Fatalf(
				"group %q condition %s = %s/%s/%q, want %s/%s containing %q; all conditions = %#v",
				group.Name, conditionType, condition.Status, condition.Reason, condition.Message, status, reason, messageContains, group.Status.Conditions,
			)
		}
		return
	}
	t.Fatalf("group %q has no %s condition; conditions = %#v", group.Name, conditionType, group.Status.Conditions)
}

func TestManagedRecreateFailureMarksGroupNotSyncedAndRemoteAbsent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	clients, stopClients := startStateoperatorClients(t)
	t.Cleanup(stopClients)

	const fqdn = "nsx-a.example.test"
	syncedAt := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	_, err := clients.typed.NetworkClouds().Create(ctx, networkCloud("cloud-recreate", fqdn), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create cloud: %v", err)
	}
	createPreviouslySyncedManageGroup(ctx, t, clients.typed, "manage-recreate", fqdn, "app-recreate", syncedAt)

	// The managed group was deleted in NSX: the list is empty and the recreate PUT fails.
	fake, server := newFailingWritesNSX(t, `{"results":[],"result_count":0}`)
	runner := newFailingWritesRunner(t, clients.typed, server.URL, syncedAt.Add(time.Hour))

	err = runner.RunReconcilePass(ctx, stateoperator.ReconcileTrigger{Kind: stateoperator.ReconcileTriggerGroup, Name: "manage-recreate"})
	if err != nil {
		t.Fatalf("RunReconcilePass() error = %v, want per-group NSX write failure recorded on status instead of failing the pass", err)
	}
	if puts := fake.puts(); len(puts) != 1 || puts[0] != failingWritesGroupsPath+"/app-recreate" {
		t.Fatalf("nsx PUT requests = %v, want one recreate PUT for app-recreate", puts)
	}

	group, err := clients.typed.Groups().Get(ctx, "manage-recreate", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get manage group: %v", err)
	}
	requireGroupConditionState(t, group, nsxv1alpha.ConditionSynced, metav1.ConditionFalse, "WriteFailed", "500")
	requireGroupConditionState(t, group, nsxv1alpha.ConditionRemotePresent, metav1.ConditionFalse, "RemoteMissing", "")
	requireGroupConditionState(t, group, nsxv1alpha.ConditionApplying, metav1.ConditionFalse, "WriteFailed", "500")
}

func TestManagedWriteFailureDoesNotBlockOtherGroupsOnManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	clients, stopClients := startStateoperatorClients(t)
	t.Cleanup(stopClients)

	const fqdn = "nsx-a.example.test"
	syncedAt := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	_, err := clients.typed.NetworkClouds().Create(ctx, networkCloud("cloud-continue", fqdn), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create cloud: %v", err)
	}
	createPreviouslySyncedManageGroup(ctx, t, clients.typed, "manage-failing", fqdn, "app-failing", syncedAt)

	// NSX has a new group to import in Observe mode; the managed group is gone and its PUT fails.
	fake, server := newFailingWritesNSX(t, `{"results":[{"id":"observe-new","display_name":"Observe New"}],"result_count":1}`)
	runner := newFailingWritesRunner(t, clients.typed, server.URL, syncedAt.Add(time.Hour))

	err = runner.RunReconcilePass(ctx, stateoperator.ReconcileTrigger{Kind: stateoperator.ReconcileTriggerSweep, Sweep: stateoperator.SweepContext{ID: "sweep-write-failure"}})
	if err != nil {
		t.Fatalf("RunReconcilePass() error = %v", err)
	}
	if puts := fake.puts(); len(puts) != 1 {
		t.Fatalf("nsx PUT requests = %v, want one failing PUT", puts)
	}

	groups, err := clients.typed.Groups().List(ctx, kubeapi.ListOptions{})
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	var imported *nsxv1alpha.NSXGroup
	for index := range groups.Items {
		if groups.Items[index].Spec.GroupID == "observe-new" {
			imported = &groups.Items[index]
		}
	}
	if imported == nil {
		t.Fatalf("observe-new was not imported after a different group's NSX write failed; groups = %#v", groups.Items)
	}
	if imported.Spec.Mode != nsxv1alpha.NSXGroupModeObserve {
		t.Fatalf("imported group mode = %s, want Observe", imported.Spec.Mode)
	}
	requireGroupConditionState(t, imported, nsxv1alpha.ConditionSynced, metav1.ConditionTrue, "Synced", "")

	cloud, err := clients.typed.NetworkClouds().Get(ctx, "cloud-continue", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get cloud: %v", err)
	}
	requireCloudConditionStatus(t, cloud, nsxv1alpha.ConditionReachable, metav1.ConditionTrue)

	failing, err := clients.typed.Groups().Get(ctx, "manage-failing", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get failing manage group: %v", err)
	}
	requireGroupConditionState(t, failing, nsxv1alpha.ConditionSynced, metav1.ConditionFalse, "WriteFailed", "500")
}

func requireCloudConditionStatus(t *testing.T, cloud *nsxv1alpha.NSXNetworkCloud, conditionType string, status metav1.ConditionStatus) {
	t.Helper()
	for _, condition := range cloud.Status.Conditions {
		if condition.Type == conditionType && condition.Status == status {
			return
		}
	}
	t.Fatalf("cloud %q conditions = %#v, want %s=%s", cloud.Name, cloud.Status.Conditions, conditionType, status)
}

type capturingKubeApplier struct {
	writes []stateoperator.ManagerKubeWritePlan
}

func (a *capturingKubeApplier) ApplyManagerKubeWrites(_ context.Context, writes stateoperator.ManagerKubeWritePlan) (*stateoperator.ManagerKubeApplyResult, error) {
	a.writes = append(a.writes, writes)
	return &stateoperator.ManagerKubeApplyResult{}, nil
}

func TestApplyManagerPlanContinuesOtherNSXWritesAfterGroupWriteFailure(t *testing.T) {
	const fqdn = "nsx-a.example.test"
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	drifted := managerGroup("manage-drifted", fqdn, "app-drifted", nsxv1alpha.NSXGroupModeManage)
	drifted.Finalizers = []string{stateoperator.GroupFinalizer}
	missing := managerGroup("manage-missing", fqdn, "app-missing", nsxv1alpha.NSXGroupModeManage)
	missing.Finalizers = []string{stateoperator.GroupFinalizer}
	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-a", fqdn),
		NetworkCloudFQDN: fqdn,
		LocalGroups:      []nsxv1alpha.NSXGroup{*drifted, *missing},
		RemoteGroups: []stateoperator.RemoteGroup{{
			Key:         stateoperator.BindingKey{NetworkCloudFQDN: fqdn, GroupID: "app-drifted"},
			DisplayName: "Old Name",
			CIDRs:       []string{"10.0.0.0/24"},
		}},
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}

	manager := &operationRecorder{patchGroupErr: errors.New("patch rejected by nsx")}
	kube := &capturingKubeApplier{}
	err = stateoperator.ApplyManagerPlan(context.Background(), kube, manager, *plan)
	if err != nil {
		t.Fatalf("ApplyManagerPlan() error = %v, want group write failure recorded on status", err)
	}
	if !slices.Contains(manager.operations, "put-group:app-missing") {
		t.Fatalf("nsx operations = %v, want app-missing PUT after app-drifted PATCH failed", manager.operations)
	}
	if len(kube.writes) != 1 {
		t.Fatalf("kube apply calls = %d, want 1", len(kube.writes))
	}
	var failedStatus *nsxv1alpha.NSXGroupStatus
	for _, request := range kube.writes[0].GroupStatusUpdates {
		if request.Name == "manage-drifted" {
			status := request.Status
			failedStatus = &status
		}
	}
	if failedStatus == nil {
		t.Fatalf("group status updates = %#v, want a status for manage-drifted", kube.writes[0].GroupStatusUpdates)
	}
	requireGroupConditionState(t, &nsxv1alpha.NSXGroup{ObjectMeta: metav1.ObjectMeta{Name: "manage-drifted"}, Status: *failedStatus},
		nsxv1alpha.ConditionSynced, metav1.ConditionFalse, "WriteFailed", "patch rejected by nsx")
}

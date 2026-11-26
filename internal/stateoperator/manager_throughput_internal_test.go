package stateoperator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// concurrentNSXWriter is a fake NSX manager that records every write, how many run at
// once, and fails the groups listed in failures with their own error.
type concurrentNSXWriter struct {
	delay    time.Duration
	failures map[string]error
	disabled bool

	mu          sync.Mutex
	attempts    map[string]int
	inFlight    int
	maxInFlight int
}

func newConcurrentNSXWriter(delay time.Duration, failures map[string]error) *concurrentNSXWriter {
	return &concurrentNSXWriter{delay: delay, failures: failures, attempts: map[string]int{}}
}

func (w *concurrentNSXWriter) write(groupID string) error {
	w.mu.Lock()
	w.attempts[groupID]++
	w.inFlight++
	w.maxInFlight = max(w.maxInFlight, w.inFlight)
	w.mu.Unlock()
	time.Sleep(w.delay)
	w.mu.Lock()
	w.inFlight--
	w.mu.Unlock()
	if w.disabled {
		return &nsxclient.WriteDisabledError{Method: http.MethodPut, Reason: nsxclient.WriteDisabledReasonGlobalConfig}
	}
	return w.failures[groupID]
}

func (w *concurrentNSXWriter) ListGroups(context.Context) ([]*nsxclient.Group, error) {
	return nil, nil
}

func (w *concurrentNSXWriter) PatchGroup(_ context.Context, groupID string, _ *nsxclient.GroupPatch) error {
	return w.write(groupID)
}

func (w *concurrentNSXWriter) PutGroup(_ context.Context, groupID string, group *nsxclient.Group) (*nsxclient.Group, error) {
	err := w.write(groupID)
	if err != nil {
		return nil, err
	}
	stored := *group
	return &stored, nil
}

func (w *concurrentNSXWriter) DeleteGroup(_ context.Context, groupID string) error {
	return w.write(groupID)
}

func (w *concurrentNSXWriter) PatchGroupIPAddressExpression(context.Context, string, string, *nsxclient.IPAddressExpressionPatch) error {
	return nil
}

func (w *concurrentNSXWriter) AddGroupIPAddressExpression(context.Context, string, string, *nsxclient.IPAddressExpressionPatch) error {
	return nil
}

func (w *concurrentNSXWriter) DeleteGroupIPAddressExpression(context.Context, string, string) error {
	return nil
}

func (w *concurrentNSXWriter) PatchGroupPathExpression(context.Context, string, string, *nsxclient.PathExpressionPatch) error {
	return nil
}

func (w *concurrentNSXWriter) AddGroupPathExpression(context.Context, string, string, *nsxclient.PathExpressionPatch) error {
	return nil
}

func (w *concurrentNSXWriter) DeleteGroupPathExpression(context.Context, string, string) error {
	return nil
}

func throughputKey(groupID string) BindingKey {
	return BindingKey{NetworkCloudFQDN: "nsx-a.example.test", GroupID: groupID}
}

func TestApplyManagerNSXWritesConcurrentlyKeepsEveryPerGroupFailure(t *testing.T) {
	writes := ManagerNSXWritePlan{
		Patches: map[BindingKey]ManagedGroupPatch{},
		Puts:    map[BindingKey]ManagedGroupPut{},
		Deletes: map[BindingKey]ManagedGroupDelete{},
	}
	failures := map[string]error{}
	for index := range 120 {
		id := fmt.Sprintf("put-%03d", index)
		writes.Puts[throughputKey(id)] = ManagedGroupPut{Key: throughputKey(id), DisplayName: id}
		if index%3 == 0 {
			failures[id] = fmt.Errorf("nsx rejected %s", id)
		}
	}
	for index := range 30 {
		id := fmt.Sprintf("patch-%03d", index)
		writes.Patches[throughputKey(id)] = ManagedGroupPatch{Key: throughputKey(id), DisplayName: id}
		if index%5 == 0 {
			failures[id] = fmt.Errorf("nsx rejected %s", id)
		}
	}
	for index := range 30 {
		id := fmt.Sprintf("delete-%03d", index)
		writes.Deletes[throughputKey(id)] = ManagedGroupDelete{GroupID: id}
		if index%7 == 0 {
			failures[id] = fmt.Errorf("nsx rejected %s", id)
		}
	}
	writer := newConcurrentNSXWriter(2*time.Millisecond, failures)

	const workers = 8
	gotFailures, created, err := applyManagerNSXWrites(context.Background(), writer, writes, workers, zap.NewNop(), nil)
	if err != nil {
		t.Fatalf("applyManagerNSXWrites() error = %v", err)
	}

	if writer.maxInFlight < 2 || writer.maxInFlight > workers {
		t.Fatalf("max concurrent NSX writes = %d, want between 2 and %d", writer.maxInFlight, workers)
	}
	total := len(writes.Puts) + len(writes.Patches) + len(writes.Deletes)
	if len(writer.attempts) != total {
		t.Fatalf("attempted %d distinct groups, want all %d", len(writer.attempts), total)
	}
	for id, count := range writer.attempts {
		if count != 1 {
			t.Fatalf("group %s written %d times, want exactly once", id, count)
		}
	}
	if len(gotFailures) != len(failures) {
		t.Fatalf("failure count = %d, want %d (no lost or extra failures): %v", len(gotFailures), len(failures), gotFailures)
	}
	for id, wantErr := range failures {
		gotErr, ok := gotFailures[throughputKey(id)]
		if !ok {
			t.Fatalf("failure for %s missing", id)
		}
		if !errors.Is(gotErr, wantErr) {
			t.Fatalf("failure for %s = %v, want it to wrap its own error %v", id, gotErr, wantErr)
		}
	}
	for key := range writes.Puts {
		_, failed := failures[key.GroupID]
		_, returned := created[key]
		if failed == returned {
			t.Fatalf("create result for %s returned=%v, want returned only for successful creates", key.GroupID, returned)
		}
	}
}

func TestApplyManagerNSXWritesConcurrentlyStopsOnceWritesAreDisabled(t *testing.T) {
	writes := ManagerNSXWritePlan{Puts: map[BindingKey]ManagedGroupPut{}}
	for index := range 200 {
		id := fmt.Sprintf("put-%03d", index)
		writes.Puts[throughputKey(id)] = ManagedGroupPut{Key: throughputKey(id), DisplayName: id}
	}
	writer := newConcurrentNSXWriter(time.Millisecond, nil)
	writer.disabled = true

	const workers = 4
	failures, created, err := applyManagerNSXWrites(context.Background(), writer, writes, workers, zap.NewNop(), nil)
	if err != nil {
		t.Fatalf("applyManagerNSXWrites() error = %v", err)
	}
	if len(failures) != 0 || len(created) != 0 {
		t.Fatalf("failures = %v, created = %v; want none when writes are disabled", failures, created)
	}
	if len(writer.attempts) > workers {
		t.Fatalf("attempted %d writes after writes were disabled, want at most one per worker (%d)", len(writer.attempts), workers)
	}
}

func budgetSnapshot(now time.Time, creates int, deleting []string) ManagerSnapshot {
	snapshot := ManagerSnapshot{
		Cloud: nsxv1alpha.NSXNetworkCloud{
			ObjectMeta: metav1.ObjectMeta{Name: "cloud-a"},
			Spec:       nsxv1alpha.NSXNetworkCloudSpec{NetworkCloudFQDN: "nsx-a.example.test"},
		},
		NetworkCloudFQDN: "nsx-a.example.test",
	}
	for index := range creates {
		name := fmt.Sprintf("group-%02d", index)
		snapshot.LocalGroups = append(snapshot.LocalGroups, nsxv1alpha.NSXGroup{
			ObjectMeta: metav1.ObjectMeta{Name: name, ResourceVersion: "1"},
			Spec: nsxv1alpha.NSXGroupSpec{
				NetworkCloudFQDN: "nsx-a.example.test",
				GroupID:          name,
				DisplayName:      name,
				Mode:             nsxv1alpha.NSXGroupModeManage,
				CIDRs:            []string{"10.0.0.1/32"},
			},
		})
	}
	deletedAt := metav1.NewTime(now)
	for _, name := range deleting {
		snapshot.LocalGroups = append(snapshot.LocalGroups, nsxv1alpha.NSXGroup{
			ObjectMeta: metav1.ObjectMeta{Name: name, ResourceVersion: "1", DeletionTimestamp: &deletedAt, Finalizers: []string{GroupFinalizer}},
			Spec: nsxv1alpha.NSXGroupSpec{
				NetworkCloudFQDN: "nsx-a.example.test",
				GroupID:          name,
				DisplayName:      name,
				Mode:             nsxv1alpha.NSXGroupModeManage,
			},
		})
		snapshot.RemoteGroups = append(snapshot.RemoteGroups, RemoteGroup{Key: throughputKey(name), DisplayName: name})
	}
	return snapshot
}

func TestGroupWriteBudgetTakesDeletingGroupsFirstAndRotatesTheRest(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	budget := &groupWriteBudget{maxGroups: 4}

	firstGroups := []string{"zz-deleting-a", "zz-deleting-b", "group-00", "group-01"}
	secondGroups := []string{"zz-deleting-a", "zz-deleting-b", "group-02", "group-03"}
	thirdGroups := []string{"zz-deleting-a", "zz-deleting-b", "group-04", "group-00"}
	for passIndex, wantGroups := range [][]string{firstGroups, secondGroups, thirdGroups} {
		snapshot := budgetSnapshot(now, 5, []string{"zz-deleting-a", "zz-deleting-b"})
		plan, err := ProcessManagerSnapshot(snapshot, now)
		if err != nil {
			t.Fatalf("ProcessManagerSnapshot() error = %v", err)
		}
		cloudStatusWrites := len(plan.KubeWrites.CloudStatusUpdates)

		deferred := budget.limit(plan, &snapshot)
		if deferred != 3 {
			t.Fatalf("pass %d deferred = %d, want 3 of 7 groups", passIndex, deferred)
		}
		got := groupsWithWrites(plan)
		gotNames := mapKeys(got)
		slices.Sort(gotNames)
		want := slices.Clone(wantGroups)
		slices.Sort(want)
		if !slices.Equal(gotNames, want) {
			t.Fatalf("pass %d groups with writes = %v, want %v", passIndex, gotNames, want)
		}
		for key := range plan.NSXWrites.Puts {
			if !slices.Contains(wantGroups, key.GroupID) {
				t.Fatalf("pass %d kept NSX put for deferred group %s", passIndex, key.GroupID)
			}
		}
		if len(plan.NSXWrites.Deletes) != 2 {
			t.Fatalf("pass %d NSX deletes = %v, want both deleting groups", passIndex, plan.NSXWrites.Deletes)
		}
		for _, status := range plan.GroupStatuses {
			if !slices.Contains(wantGroups, status.Name) {
				t.Fatalf("pass %d kept status plan for deferred group %s", passIndex, status.Name)
			}
		}
		if len(plan.KubeWrites.CloudStatusUpdates) != cloudStatusWrites {
			t.Fatalf("pass %d cloud status writes = %d, want %d kept", passIndex, len(plan.KubeWrites.CloudStatusUpdates), cloudStatusWrites)
		}
	}
}

func TestGroupWriteBudgetLeavesSmallPlansAlone(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	snapshot := budgetSnapshot(now, 3, nil)
	plan, err := ProcessManagerSnapshot(snapshot, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	for _, maxGroups := range []int{0, 3, 10} {
		budget := &groupWriteBudget{maxGroups: maxGroups}
		deferred := budget.limit(plan, &snapshot)
		if deferred != 0 || len(plan.NSXWrites.Puts) != 3 {
			t.Fatalf("maxGroups %d: deferred = %d, puts = %d; want nothing deferred", maxGroups, deferred, len(plan.NSXWrites.Puts))
		}
	}
}

type capturingManagerKubeApplier struct {
	writes ManagerKubeWritePlan
}

func (a *capturingManagerKubeApplier) ApplyManagerKubeWrites(_ context.Context, writes ManagerKubeWritePlan) (*ManagerKubeApplyResult, error) {
	a.writes = writes
	return &ManagerKubeApplyResult{}, nil
}

func syncedCondition(t *testing.T, status nsxv1alpha.NSXGroupStatus) metav1.Condition {
	t.Helper()
	for _, condition := range status.Conditions {
		if condition.Type == nsxv1alpha.ConditionSynced {
			return condition
		}
	}
	t.Fatalf("status %#v has no Synced condition", status)
	return metav1.Condition{}
}

func TestApplyManagerPlanMarksCreatedGroupsSyncedFromCreateResponse(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	snapshot := budgetSnapshot(now, 3, nil)
	plan, err := ProcessManagerSnapshot(snapshot, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	writer := newConcurrentNSXWriter(0, map[string]error{"group-01": errors.New("nsx rejected group-01")})
	applier := &capturingManagerKubeApplier{}

	err = applyManagerPlan(context.Background(), applier, writer, plan, 4, zap.NewNop(), nil)
	if err != nil {
		t.Fatalf("applyManagerPlan() error = %v", err)
	}

	for _, name := range []string{"group-00", "group-01", "group-02"} {
		statusKey := groupBatchKey("updateStatus", "status", name)
		request, ok := applier.writes.GroupStatusUpdates[statusKey]
		if !ok {
			t.Fatalf("no status write for %s", name)
		}
		synced := syncedCondition(t, request.Status)
		if name == "group-01" {
			if synced.Status != metav1.ConditionFalse || synced.Reason != "WriteFailed" {
				t.Fatalf("%s Synced = %s/%s, want False/WriteFailed for the failed create", name, synced.Status, synced.Reason)
			}
		} else if synced.Status != metav1.ConditionTrue {
			t.Fatalf("%s Synced = %s/%s, want True from the create response", name, synced.Status, synced.Reason)
		}
		if _, waits := applier.writes.GroupFinalizersAfterStatusWrite[statusKey]; !waits {
			t.Fatalf("finalizer add for %s does not wait for its status write", name)
		}
	}
}

func TestKubeAPIAdapterKeepsApplyingAfterItemFailures(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		mu.Lock()
		requests = append(requests, req.Method+" "+req.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(req.URL.Path, "/conflict-group/status") {
			w.WriteHeader(http.StatusConflict)
			_, writeErr := io.WriteString(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Conflict","code":409}`)
			if writeErr != nil {
				t.Errorf("write conflict: %v", writeErr)
			}
			return
		}
		if req.Method == http.MethodPatch {
			name := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
			writeErr := json.NewEncoder(w).Encode(&nsxv1alpha.NSXGroup{ObjectMeta: metav1.ObjectMeta{Name: name, ResourceVersion: "rv-patched"}})
			if writeErr != nil {
				t.Errorf("write patch response: %v", writeErr)
			}
			return
		}
		_, writeErr := w.Write(body)
		if writeErr != nil {
			t.Errorf("echo body: %v", writeErr)
		}
	}))
	t.Cleanup(server.Close)
	kubeClient, err := kubeapi.NewClient(kubeapi.Options{
		Config:      &rest.Config{Host: server.URL},
		BatchConfig: kubeapi.BatchConfig{NumParallelWorkers: 4, MaxRequestsPerSecond: 1000, MaxRequestsInFlight: 4},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	conflictStatus := groupBatchKey("updateStatus", "status", "conflict-group")
	okStatus := groupBatchKey("updateStatus", "status", "ok-group")
	writes := ManagerKubeWritePlan{}
	ensureManagerKubeWrites(&writes)
	writes.GroupStatusUpdates[conflictStatus] = kubeapi.GroupStatusUpdateRequest{Name: "conflict-group", Options: kubeapi.StatusUpdateOptions{ResourceVersion: "1"}}
	writes.GroupStatusUpdates[okStatus] = kubeapi.GroupStatusUpdateRequest{Name: "ok-group", Options: kubeapi.StatusUpdateOptions{ResourceVersion: "1"}}
	writes.GroupFinalizersAfterStatusWrite[conflictStatus] = GroupFinalizerAfterStatusWrite{Name: "conflict-group", Finalizers: []string{GroupFinalizer}}
	writes.GroupFinalizersAfterStatusWrite[okStatus] = GroupFinalizerAfterStatusWrite{Name: "ok-group", Finalizers: []string{GroupFinalizer}}
	writes.GroupFinalizerPatches[groupBatchKey("patchFinalizers", "finalizers", "deleting-group")] = kubeapi.GroupFinalizerPatchRequest{Name: "deleting-group", Finalizers: []string{}}
	writes.CloudStatusUpdates[networkCloudBatchKey("updateStatus", "status", "cloud-a")] = kubeapi.NetworkCloudStatusUpdateRequest{Name: "cloud-a", Options: kubeapi.StatusUpdateOptions{ResourceVersion: "1"}}

	_, err = (&kubeAPIAdapter{client: kubeClient}).ApplyManagerKubeWrites(context.Background(), writes)
	if err == nil || !strings.Contains(err.Error(), "update manager group statuses") {
		t.Fatalf("ApplyManagerKubeWrites() error = %v, want the status conflict reported", err)
	}
	mu.Lock()
	got := slices.Clone(requests)
	mu.Unlock()
	for _, want := range []string{
		"PATCH /apis/nsx.ing.com/v1alpha/nsxgroups/deleting-group",
		"PATCH /apis/nsx.ing.com/v1alpha/nsxgroups/ok-group",
		"PUT /apis/nsx.ing.com/v1alpha/nsxnetworkclouds/cloud-a/status",
	} {
		if !slices.Contains(got, want) {
			t.Fatalf("requests %v miss %q: one failed status write must not stop the other writes", got, want)
		}
	}
	if slices.Contains(got, "PATCH /apis/nsx.ing.com/v1alpha/nsxgroups/conflict-group") {
		t.Fatalf("requests %v patch conflict-group's finalizer although its status write failed", got)
	}
}

// serializedPassKubeClient reports how many passes are between their first and last
// gather call at once.
type serializedPassKubeClient struct {
	active    atomic.Int32
	maxActive atomic.Int32
}

func (c *serializedPassKubeClient) ListNetworkClouds(context.Context) (*nsxv1alpha.NSXNetworkCloudList, error) {
	active := c.active.Add(1)
	for {
		previous := c.maxActive.Load()
		if active <= previous || c.maxActive.CompareAndSwap(previous, active) {
			break
		}
	}
	return &nsxv1alpha.NSXNetworkCloudList{}, nil
}

func (c *serializedPassKubeClient) ListGroups(context.Context) (*nsxv1alpha.NSXGroupList, error) {
	time.Sleep(5 * time.Millisecond)
	c.active.Add(-1)
	return &nsxv1alpha.NSXGroupList{}, nil
}

func (c *serializedPassKubeClient) ApplyManagerKubeWrites(context.Context, ManagerKubeWritePlan) (*ManagerKubeApplyResult, error) {
	return &ManagerKubeApplyResult{}, nil
}

func TestDefaultReconcilePassRunnerSerializesPasses(t *testing.T) {
	kubeClient := &serializedPassKubeClient{}
	runner := NewDefaultReconcilePassRunner(ReconcilePassRunnerOptions{
		KubeClient: kubeClient,
		ManagerClientFactory: func(context.Context, nsxv1alpha.NSXNetworkCloud) (ManagerClient, error) {
			return newConcurrentNSXWriter(0, nil), nil
		},
	})
	var waitGroup sync.WaitGroup
	errs := make(chan error, 8)
	for index := range 8 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			trigger := ReconcileTrigger{Kind: ReconcileTriggerSweep}
			if index%2 == 1 {
				trigger = ReconcileTrigger{Kind: ReconcileTriggerManager, Name: "nsx-a.example.test"}
			}
			errs <- runner.RunReconcilePass(context.Background(), trigger)
		}()
	}
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("RunReconcilePass() error = %v", err)
		}
	}
	if got := kubeClient.maxActive.Load(); got != 1 {
		t.Fatalf("concurrent passes = %d, want 1: sweep and event passes must not overlap", got)
	}
}

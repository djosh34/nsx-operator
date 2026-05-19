//go:build largechaos

package stateoperator_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/stateoperator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	largeRemoteGroupCount     = 2000
	largeKubernetesGroupCount = 10000
)

func TestLargeRemoteGroupsPlanObserveUpserts(t *testing.T) {
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()

	remoteGroups := make([]stateoperator.RemoteGroup, 0, largeRemoteGroupCount)
	for i := range largeRemoteGroupCount {
		groupID := fmt.Sprintf("remote-%04d", i)
		remoteGroups = append(remoteGroups, stateoperator.RemoteGroup{
			Key: stateoperator.BindingKey{
				NetworkCloudFQDN: "nsx-large.example.test",
				GroupID:          groupID,
			},
			DisplayName: "Remote " + groupID,
			CIDRs:       []string{fmt.Sprintf("10.%d.%d.0/24", i/256, i%256)},
		})
	}

	now := time.Date(2026, 5, 19, 6, 0, 0, 0, time.UTC)
	plan, err := stateoperator.ProcessManagerSnapshot(stateoperator.ManagerSnapshot{
		Cloud:            *networkCloud("cloud-large", "nsx-large.example.test"),
		NetworkCloudFQDN: "nsx-large.example.test",
		RemoteGroups:     remoteGroups,
	}, now)
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.ObserveUpserts) != largeRemoteGroupCount {
		t.Fatalf("ObserveUpserts = %d, want %d", len(plan.ObserveUpserts), largeRemoteGroupCount)
	}
	if len(plan.GroupStatuses) != largeRemoteGroupCount {
		t.Fatalf("GroupStatuses = %d, want %d", len(plan.GroupStatuses), largeRemoteGroupCount)
	}
	requireLargeObserveUpsert(t, plan.ObserveUpserts[0], "nsx-large.example.test--remote-0000", "remote-0000")
	requireLargeObserveUpsert(t, plan.ObserveUpserts[len(plan.ObserveUpserts)-1], "nsx-large.example.test--remote-1999", "remote-1999")

	seen := make(map[string]struct{}, len(plan.ObserveUpserts))
	for _, group := range plan.ObserveUpserts {
		if _, exists := seen[group.Name]; exists {
			t.Fatalf("duplicate observe upsert name %q", group.Name)
		}
		seen[group.Name] = struct{}{}
		if group.Spec.Mode != nsxv1alpha.NSXGroupModeObserve {
			t.Fatalf("upsert %q mode = %q, want Observe", group.Name, group.Spec.Mode)
		}
		if group.Spec.NetworkCloudFQDN != "nsx-large.example.test" {
			t.Fatalf("upsert %q networkCloudFQDN = %q, want nsx-large.example.test", group.Name, group.Spec.NetworkCloudFQDN)
		}
	}

	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	t.Logf("large remote evidence: planned %d observe upserts and statuses in %s; heap_alloc_delta_bytes=%d", largeRemoteGroupCount, time.Since(started), int64(after.HeapAlloc)-int64(before.HeapAlloc))
}

func TestLargeMixedGroupsThroughRealKubernetesClient(t *testing.T) {
	clients, stop := startStateoperatorClients(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	cloud := networkCloud("cloud-k8s-large", "nsx-k8s-large.example.test")
	if _, err := clients.typed.NetworkClouds().Create(ctx, cloud, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create large cloud: %v", err)
	}

	started := time.Now()
	for i := range largeKubernetesGroupCount {
		mode := nsxv1alpha.NSXGroupModeObserve
		if i%2 == 0 {
			mode = nsxv1alpha.NSXGroupModeManage
		}
		groupID := fmt.Sprintf("app-%05d", i)
		group := managerGroup("large-"+groupID, "nsx-k8s-large.example.test", groupID, mode)
		if _, err := clients.typed.Groups().Create(ctx, group, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create large group %d: %v", i, err)
		}
		if i > 0 && i%1000 == 0 {
			t.Logf("created %d/%d large Kubernetes groups", i, largeKubernetesGroupCount)
		}
	}

	allCount := countGroupsByPages(ctx, t, clients.typed, []kubeapi.FieldFilter{
		kubeapi.FilterBy(kubeapi.FieldNetworkCloudFQDN, "nsx-k8s-large.example.test"),
	}, 1000)
	if allCount != largeKubernetesGroupCount {
		t.Fatalf("paged group count = %d, want %d", allCount, largeKubernetesGroupCount)
	}
	manageCount := countGroupsByPages(ctx, t, clients.typed, []kubeapi.FieldFilter{
		kubeapi.FilterBy(kubeapi.FieldNetworkCloudFQDN, "nsx-k8s-large.example.test"),
		kubeapi.FilterBy(kubeapi.FieldGroupMode, string(nsxv1alpha.NSXGroupModeManage)),
	}, 750)
	if manageCount != largeKubernetesGroupCount/2 {
		t.Fatalf("paged manage group count = %d, want %d", manageCount, largeKubernetesGroupCount/2)
	}

	snapshot, err := stateoperator.GatherManagerSnapshot(ctx, *cloud, clients.typed.Groups().List, func(context.Context, nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
		return &operationRecorder{}, nil
	})
	if err != nil {
		t.Fatalf("GatherManagerSnapshot() error = %v", err)
	}
	if snapshot.GatherError != nil {
		t.Fatalf("GatherManagerSnapshot() gather error = %v", snapshot.GatherError)
	}
	if len(snapshot.LocalGroups) != largeKubernetesGroupCount {
		t.Fatalf("snapshot local groups = %d, want %d", len(snapshot.LocalGroups), largeKubernetesGroupCount)
	}
	plan, err := stateoperator.ProcessManagerSnapshot(snapshot, time.Date(2026, 5, 19, 6, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ProcessManagerSnapshot() error = %v", err)
	}
	if len(plan.ManagedWrites) != largeKubernetesGroupCount/2 {
		t.Fatalf("managed writes = %d, want %d", len(plan.ManagedWrites), largeKubernetesGroupCount/2)
	}
	if len(plan.ObserveDeletes) != largeKubernetesGroupCount/2 {
		t.Fatalf("observe deletes = %d, want %d", len(plan.ObserveDeletes), largeKubernetesGroupCount/2)
	}
	t.Logf("large Kubernetes evidence: created and paged %d real CRs, gathered %d local groups, planned %d managed writes and %d observe deletes in %s", largeKubernetesGroupCount, len(snapshot.LocalGroups), len(plan.ManagedWrites), len(plan.ObserveDeletes), time.Since(started))
}

func requireLargeObserveUpsert(t *testing.T, group nsxv1alpha.NSXGroup, wantName string, wantGroupID string) {
	t.Helper()

	if group.Name != wantName {
		t.Fatalf("observe upsert name = %q, want %q", group.Name, wantName)
	}
	if group.Spec.GroupID != wantGroupID {
		t.Fatalf("observe upsert groupID = %q, want %q", group.Spec.GroupID, wantGroupID)
	}
}

func countGroupsByPages(ctx context.Context, t *testing.T, client *kubeapi.Client, filters []kubeapi.FieldFilter, limit int64) int {
	t.Helper()

	count := 0
	continueToken := ""
	for {
		page, err := client.Groups().List(ctx, kubeapi.ListOptions{
			Filters:  filters,
			Limit:    limit,
			Continue: continueToken,
		})
		if err != nil {
			t.Fatalf("list groups page after %d items: %v", count, err)
		}
		count += len(page.Items)
		if page.Continue == "" {
			return count
		}
		continueToken = page.Continue
	}
}

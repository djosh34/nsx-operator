package stateoperator_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
	"github.com/djosh34/nsx-operator/internal/stateoperator"
)

func TestDefaultReconcilePassRunnerSweepGathersKubernetesStateOnce(t *testing.T) {
	kube := &recordingPassKubeClient{
		clouds: []nsxv1alpha.NSXNetworkCloud{
			*networkCloud("cloud-a", "nsx-a.example.test"),
			*networkCloud("cloud-b", "nsx-b.example.test"),
		},
		groups: []nsxv1alpha.NSXGroup{
			*managerGroup("local-a", "nsx-a.example.test", "local-a", nsxv1alpha.NSXGroupModeManage),
			*managerGroup("local-b", "nsx-b.example.test", "local-b", nsxv1alpha.NSXGroupModeManage),
		},
	}
	managerClients := map[string]*operationRecorder{
		"cloud-a": {listGroups: []*nsxclient.Group{{
			Resource: nsxclient.Resource{ID: "remote-a", DisplayName: "Remote A"},
		}}},
		"cloud-b": {listGroups: []*nsxclient.Group{{
			Resource: nsxclient.Resource{ID: "remote-b", DisplayName: "Remote B"},
		}}},
	}
	var requestedClouds []string
	var requestedCloudsMu sync.Mutex
	runner := stateoperator.NewDefaultReconcilePassRunner(stateoperator.ReconcilePassRunnerOptions{
		KubeClient: kube,
		Clock:      &fixedClock{now: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)},
		ManagerClientFactory: func(_ context.Context, cloud nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			requestedCloudsMu.Lock()
			requestedClouds = append(requestedClouds, cloud.Name)
			requestedCloudsMu.Unlock()
			managerClient := managerClients[cloud.Name]
			if managerClient == nil {
				return nil, fmt.Errorf("unexpected manager client for %q", cloud.Name)
			}
			return managerClient, nil
		},
	})

	err := runner.RunReconcilePass(context.Background(), stateoperator.ReconcileTrigger{Kind: stateoperator.ReconcileTriggerSweep})
	if err != nil {
		t.Fatalf("RunReconcilePass() error = %v", err)
	}
	cloudListCalls, groupListCalls, kubeApplyCallCount := kube.counts()
	if cloudListCalls != 1 {
		t.Fatalf("cloud list calls = %d, want 1", cloudListCalls)
	}
	if groupListCalls != 1 {
		t.Fatalf("group list calls = %d, want 1", groupListCalls)
	}
	if kubeApplyCallCount != 2 {
		t.Fatalf("ApplyManagerKubeWrites call count = %d, want one per selected cloud", kubeApplyCallCount)
	}
	requestedCloudsMu.Lock()
	slices.Sort(requestedClouds)
	gotRequestedClouds := append([]string(nil), requestedClouds...)
	requestedCloudsMu.Unlock()
	if !slices.Equal(gotRequestedClouds, []string{"cloud-a", "cloud-b"}) {
		t.Fatalf("requested manager clouds = %v, want both clouds", gotRequestedClouds)
	}
}

func TestDefaultReconcilePassRunnerNetworkCloudEventNarrowsFromGatheredSnapshot(t *testing.T) {
	kube := &recordingPassKubeClient{
		clouds: []nsxv1alpha.NSXNetworkCloud{
			*networkCloud("cloud-a", "nsx-a.example.test"),
			*networkCloud("cloud-b", "nsx-b.example.test"),
		},
		groups: []nsxv1alpha.NSXGroup{
			*managerGroup("local-a", "nsx-a.example.test", "local-a", nsxv1alpha.NSXGroupModeManage),
			*managerGroup("local-b", "nsx-b.example.test", "local-b", nsxv1alpha.NSXGroupModeManage),
		},
	}
	var requestedClouds []string
	runner := stateoperator.NewDefaultReconcilePassRunner(stateoperator.ReconcilePassRunnerOptions{
		KubeClient: kube,
		Clock:      &fixedClock{now: time.Date(2026, 5, 20, 8, 15, 0, 0, time.UTC)},
		ManagerClientFactory: func(_ context.Context, cloud nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			requestedClouds = append(requestedClouds, cloud.Name)
			return &operationRecorder{}, nil
		},
	})

	err := runner.RunReconcilePass(context.Background(), stateoperator.ReconcileTrigger{
		Kind: stateoperator.ReconcileTriggerNetworkCloud,
		Name: "cloud-b",
	})
	if err != nil {
		t.Fatalf("RunReconcilePass() error = %v", err)
	}
	cloudListCalls, groupListCalls, kubeApplyCallCount := kube.counts()
	if cloudListCalls != 1 || groupListCalls != 1 {
		t.Fatalf("gather calls clouds=%d groups=%d, want one each", cloudListCalls, groupListCalls)
	}
	if !slices.Equal(requestedClouds, []string{"cloud-b"}) {
		t.Fatalf("requested manager clouds = %v, want [cloud-b]", requestedClouds)
	}
	if kubeApplyCallCount != 1 {
		t.Fatalf("ApplyManagerKubeWrites call count = %d, want one selected cloud", kubeApplyCallCount)
	}
}

func TestDefaultReconcilePassRunnerGroupEventNarrowsToGroupsCloud(t *testing.T) {
	kube := &recordingPassKubeClient{
		clouds: []nsxv1alpha.NSXNetworkCloud{
			*networkCloud("cloud-a", "nsx-a.example.test"),
			*networkCloud("cloud-b", "NSX-B.Example.Test"),
		},
		groups: []nsxv1alpha.NSXGroup{
			*managerGroup("group-a", "nsx-a.example.test", "app-a", nsxv1alpha.NSXGroupModeManage),
			*managerGroup("group-b", "nsx-b.example.test", "app-b", nsxv1alpha.NSXGroupModeManage),
		},
	}
	var requestedClouds []string
	runner := stateoperator.NewDefaultReconcilePassRunner(stateoperator.ReconcilePassRunnerOptions{
		KubeClient: kube,
		Clock:      &fixedClock{now: time.Date(2026, 5, 20, 8, 30, 0, 0, time.UTC)},
		ManagerClientFactory: func(_ context.Context, cloud nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			requestedClouds = append(requestedClouds, cloud.Name)
			return &operationRecorder{}, nil
		},
	})

	err := runner.RunReconcilePass(context.Background(), stateoperator.ReconcileTrigger{
		Kind: stateoperator.ReconcileTriggerGroup,
		Name: "group-b",
	})
	if err != nil {
		t.Fatalf("RunReconcilePass() error = %v", err)
	}
	cloudListCalls, groupListCalls, kubeApplyCallCount := kube.counts()
	if cloudListCalls != 1 || groupListCalls != 1 {
		t.Fatalf("gather calls clouds=%d groups=%d, want one each", cloudListCalls, groupListCalls)
	}
	if !slices.Equal(requestedClouds, []string{"cloud-b"}) {
		t.Fatalf("requested manager clouds = %v, want [cloud-b]", requestedClouds)
	}
	if kubeApplyCallCount != 1 {
		t.Fatalf("ApplyManagerKubeWrites call count = %d, want one selected cloud", kubeApplyCallCount)
	}
}

func TestDefaultReconcilePassRunnerMissingEventObjectIsNoopAfterGather(t *testing.T) {
	kube := &recordingPassKubeClient{
		clouds: []nsxv1alpha.NSXNetworkCloud{*networkCloud("cloud-a", "nsx-a.example.test")},
		groups: []nsxv1alpha.NSXGroup{*managerGroup("group-a", "nsx-a.example.test", "app-a", nsxv1alpha.NSXGroupModeManage)},
	}
	var requestedClouds []string
	runner := stateoperator.NewDefaultReconcilePassRunner(stateoperator.ReconcilePassRunnerOptions{
		KubeClient: kube,
		Clock:      &fixedClock{now: time.Date(2026, 5, 20, 8, 45, 0, 0, time.UTC)},
		ManagerClientFactory: func(_ context.Context, cloud nsxv1alpha.NSXNetworkCloud) (stateoperator.ManagerClient, error) {
			requestedClouds = append(requestedClouds, cloud.Name)
			return &operationRecorder{}, nil
		},
	})

	err := runner.RunReconcilePass(context.Background(), stateoperator.ReconcileTrigger{
		Kind: stateoperator.ReconcileTriggerGroup,
		Name: "missing-group",
	})
	if err != nil {
		t.Fatalf("RunReconcilePass() error = %v", err)
	}
	cloudListCalls, groupListCalls, kubeApplyCallCount := kube.counts()
	if cloudListCalls != 1 || groupListCalls != 1 {
		t.Fatalf("gather calls clouds=%d groups=%d, want one each", cloudListCalls, groupListCalls)
	}
	if len(requestedClouds) != 0 {
		t.Fatalf("requested manager clouds = %v, want none", requestedClouds)
	}
	if kubeApplyCallCount != 0 {
		t.Fatalf("ApplyManagerKubeWrites call count = %d, want none", kubeApplyCallCount)
	}
}

type recordingPassKubeClient struct {
	mu sync.Mutex

	clouds []nsxv1alpha.NSXNetworkCloud
	groups []nsxv1alpha.NSXGroup

	cloudListCalls     int
	groupListCalls     int
	kubeApplyCallCount int
}

func (c *recordingPassKubeClient) ListNetworkClouds(_ context.Context) (*nsxv1alpha.NSXNetworkCloudList, error) {
	c.mu.Lock()
	c.cloudListCalls++
	c.mu.Unlock()
	return &nsxv1alpha.NSXNetworkCloudList{Items: append([]nsxv1alpha.NSXNetworkCloud(nil), c.clouds...)}, nil
}

func (c *recordingPassKubeClient) ListGroups(_ context.Context) (*nsxv1alpha.NSXGroupList, error) {
	c.mu.Lock()
	c.groupListCalls++
	c.mu.Unlock()
	return &nsxv1alpha.NSXGroupList{Items: append([]nsxv1alpha.NSXGroup(nil), c.groups...)}, nil
}

func (c *recordingPassKubeClient) ApplyManagerKubeWrites(_ context.Context, _ stateoperator.ManagerKubeWritePlan) (*stateoperator.ManagerKubeApplyResult, error) {
	c.mu.Lock()
	c.kubeApplyCallCount++
	c.mu.Unlock()
	return &stateoperator.ManagerKubeApplyResult{}, nil
}

func (c *recordingPassKubeClient) counts() (int, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cloudListCalls, c.groupListCalls, c.kubeApplyCallCount
}

type fixedClock struct {
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	return c.now
}

func (c *fixedClock) NewTimer(time.Duration) stateoperator.Timer {
	return &manualTimer{ch: make(chan time.Time)}
}

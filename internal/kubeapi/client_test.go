package kubeapi_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/kubeapi"
	"github.com/djosh34/nsx-operator/internal/operatormetrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestNetworkCloudClientCreatesGetsAndListsByFieldSelector(t *testing.T) {
	client, stop := startClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	created, err := client.NetworkClouds().Create(ctx, &nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "cloud-a"},
		Spec: nsxv1alpha.NSXNetworkCloudSpec{
			NetworkCloudFQDN: "nsx-a.example.net",
			NetworkCloudID:   "cloud-a",
			Name:             "Cloud A",
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Kind != "NSXNetworkCloud" || created.APIVersion != nsxv1alpha.SchemeGroupVersion.String() {
		t.Fatalf("created type meta = %s/%s, want %s/NSXNetworkCloud", created.APIVersion, created.Kind, nsxv1alpha.SchemeGroupVersion.String())
	}

	got, err := client.NetworkClouds().Get(ctx, "cloud-a", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Spec.NetworkCloudFQDN != "nsx-a.example.net" {
		t.Fatalf("Get() spec.networkCloudFQDN = %q, want nsx-a.example.net", got.Spec.NetworkCloudFQDN)
	}

	list, err := client.NetworkClouds().List(ctx, kubeapi.ListOptions{
		Filters: []kubeapi.FieldFilter{
			kubeapi.FilterBy(kubeapi.FieldNetworkCloudFQDN, "nsx-a.example.net"),
		},
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "cloud-a" {
		t.Fatalf("List() items = %#v, want only cloud-a", list.Items)
	}
	t.Logf("typed network cloud create/get/list with field selector returned %s", list.Items[0].Name)
}

func TestGroupClientCreatesGetsAndListsBySelectableFields(t *testing.T) {
	client, stop := startClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	first := &nsxv1alpha.NSXGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "group-a"},
		Spec: nsxv1alpha.NSXGroupSpec{
			NetworkCloudFQDN: "nsx-a.example.net",
			GroupID:          "app-a",
			DisplayName:      "App A",
			Mode:             nsxv1alpha.NSXGroupModeManage,
			CIDRs:            []string{"10.0.0.0/24"},
		},
	}
	second := &nsxv1alpha.NSXGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "group-b"},
		Spec: nsxv1alpha.NSXGroupSpec{
			NetworkCloudFQDN: "nsx-b.example.net",
			GroupID:          "app-b",
			DisplayName:      "App B",
			Mode:             nsxv1alpha.NSXGroupModeObserve,
			CIDRs:            []string{"10.1.0.0/24"},
		},
	}
	if _, err := client.Groups().Create(ctx, first, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	if _, err := client.Groups().Create(ctx, second, metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	got, err := client.Groups().Get(ctx, "group-a", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Spec.GroupID != "app-a" {
		t.Fatalf("Get() spec.groupID = %q, want app-a", got.Spec.GroupID)
	}

	requireGroupNames(t, ctx, client, kubeapi.FilterBy(kubeapi.FieldGroupID, "app-b"), []string{"group-b"})
	requireGroupNames(t, ctx, client, kubeapi.FilterBy(kubeapi.FieldGroupMode, string(nsxv1alpha.NSXGroupModeManage)), []string{"group-a"})
	requireGroupNames(t, ctx, client, kubeapi.FilterBy(kubeapi.FieldNetworkCloudFQDN, "nsx-a.example.net"), []string{"group-a"})
}

func TestTypedClientRecordsKubernetesAPIMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder, err := operatormetrics.NewRecorder(registry, zap.NewNop())
	if err != nil {
		t.Fatalf("construct recorder: %v", err)
	}
	client, stop := startClientWithLoggerAndRecorder(t, zap.NewNop(), recorder)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	created, err := client.Groups().Create(ctx, group("group-metrics", "nsx-metrics.example.net", "app-metrics", nsxv1alpha.NSXGroupModeManage), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Groups().Create() error = %v", err)
	}
	if _, err := client.Groups().List(ctx, kubeapi.ListOptions{}); err != nil {
		t.Fatalf("Groups().List() error = %v", err)
	}
	created.Spec.DisplayName = "Metrics Updated"
	updated, err := client.Groups().Update(ctx, created, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Groups().Update() error = %v", err)
	}
	if _, err := client.Groups().UpdateStatus(ctx, "group-metrics", nsxv1alpha.NSXGroupStatus{}, kubeapi.StatusUpdateOptions{ResourceVersion: updated.ResourceVersion}); err != nil {
		t.Fatalf("Groups().UpdateStatus() error = %v", err)
	}
	if err := client.Groups().Delete(ctx, "group-metrics", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Groups().Delete() error = %v", err)
	}

	expected := `
# HELP nsx_operator_kubernetes_api_calls_total Total Kubernetes API calls by typed client function.
# TYPE nsx_operator_kubernetes_api_calls_total counter
nsx_operator_kubernetes_api_calls_total{function="groups.create"} 1
nsx_operator_kubernetes_api_calls_total{function="groups.delete"} 1
nsx_operator_kubernetes_api_calls_total{function="groups.list"} 1
nsx_operator_kubernetes_api_calls_total{function="groups.update"} 1
nsx_operator_kubernetes_api_calls_total{function="groups.update_status"} 1
`
	if err := testutil.GatherAndCompare(registry, strings.NewReader(expected), "nsx_operator_kubernetes_api_calls_total"); err != nil {
		t.Fatalf("gather kubernetes api call metrics: %v", err)
	}
	for _, function := range []string{"groups.create", "groups.list", "groups.update", "groups.update_status", "groups.delete"} {
		responseBytes := counterValue(t, registry, "nsx_operator_kubernetes_api_bytes_total", map[string]string{"function": function, "direction": "response"})
		if responseBytes <= 0 {
			t.Fatalf("response bytes for %s = %f, want positive", function, responseBytes)
		}
		histogramCount := histogramCount(t, registry, "nsx_operator_kubernetes_api_round_trip_seconds", map[string]string{"function": function})
		if histogramCount != 1 {
			t.Fatalf("round trip count for %s = %d, want 1", function, histogramCount)
		}
	}
}

func requireGroupNames(t *testing.T, ctx context.Context, client *kubeapi.Client, filter kubeapi.FieldFilter, want []string) {
	t.Helper()
	list, err := client.Groups().List(ctx, kubeapi.ListOptions{Filters: []kubeapi.FieldFilter{filter}})
	if err != nil {
		t.Fatalf("Groups().List() error = %v", err)
	}
	got := make([]string, 0, len(list.Items))
	for i := range list.Items {
		got = append(got, list.Items[i].Name)
	}
	if len(got) != len(want) {
		t.Fatalf("Groups().List() names = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("Groups().List() names = %v, want %v", got, want)
		}
	}
	t.Logf("typed group list with filter returned %v", got)
}

func TestFieldFilterExposesTypedFieldAndValue(t *testing.T) {
	filter := kubeapi.FilterBy(kubeapi.FieldNetworkCloudFQDN, "nsx-a.example.net")
	if filter.Field() != kubeapi.FieldNetworkCloudFQDN {
		t.Fatalf("Field() = %q, want %q", filter.Field(), kubeapi.FieldNetworkCloudFQDN)
	}
	if filter.Value() != "nsx-a.example.net" {
		t.Fatalf("Value() = %q, want nsx-a.example.net", filter.Value())
	}
}

func TestUpdateRequiresResourceVersionAndPersistsFetchedObjectChanges(t *testing.T) {
	client, stop := startClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	created, err := client.NetworkClouds().Create(ctx, networkCloud("cloud-update", "nsx-update.example.net"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	withoutResourceVersion := created.DeepCopy()
	withoutResourceVersion.ResourceVersion = ""
	withoutResourceVersion.Spec.Name = "Should Not Persist"
	if _, err := client.NetworkClouds().Update(ctx, withoutResourceVersion, metav1.UpdateOptions{}); err == nil {
		t.Fatal("Update() error = nil, want resourceVersion validation error")
	}
	unchanged, err := client.NetworkClouds().Get(ctx, "cloud-update", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() after rejected update error = %v", err)
	}
	if unchanged.Spec.Name != "Cloud cloud-update" {
		t.Fatalf("name after rejected update = %q, want original", unchanged.Spec.Name)
	}

	unchanged.Spec.Name = "Cloud Update Renamed"
	updated, err := client.NetworkClouds().Update(ctx, unchanged, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Update() with resourceVersion error = %v", err)
	}
	if updated.Spec.Name != "Cloud Update Renamed" {
		t.Fatalf("updated spec.name = %q, want Cloud Update Renamed", updated.Spec.Name)
	}

	createdGroup, err := client.Groups().Create(ctx, group("group-update", "nsx-update.example.net", "app-update", nsxv1alpha.NSXGroupModeObserve), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Groups().Create() error = %v", err)
	}
	createdGroup.Spec.Mode = nsxv1alpha.NSXGroupModeManage
	updatedGroup, err := client.Groups().Update(ctx, createdGroup, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Groups().Update() error = %v", err)
	}
	if updatedGroup.Spec.Mode != nsxv1alpha.NSXGroupModeManage {
		t.Fatalf("updated group mode = %q, want Manage", updatedGroup.Spec.Mode)
	}
}

func TestStatusUpdateStoresStatusAndPreservesSpec(t *testing.T) {
	client, stop := startClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	created, err := client.Groups().Create(ctx, group("group-status", "nsx-status.example.net", "app-status", nsxv1alpha.NSXGroupModeManage), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	status := nsxv1alpha.NSXGroupStatus{Conditions: []metav1.Condition{{
		Type:               nsxv1alpha.ConditionSynced,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: created.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "Verified",
		Message:            "status update through typed kubeapi client",
	}}}
	updated, err := client.Groups().UpdateStatus(ctx, "group-status", status, kubeapi.StatusUpdateOptions{ResourceVersion: created.ResourceVersion})
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
	if len(updated.Status.Conditions) != 1 || updated.Status.Conditions[0].Type != nsxv1alpha.ConditionSynced {
		t.Fatalf("updated status conditions = %#v, want Synced condition", updated.Status.Conditions)
	}
	got, err := client.Groups().Get(ctx, "group-status", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() after status update error = %v", err)
	}
	if got.Spec.DisplayName != "Group app-status" || got.Spec.GroupID != "app-status" {
		t.Fatalf("spec after status update = %#v, want original group spec", got.Spec)
	}
	if len(got.Status.Conditions) != 1 || got.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Fatalf("stored status conditions = %#v, want true condition", got.Status.Conditions)
	}
	t.Logf("typed UpdateStatus stored %s and preserved display_name %q", got.Status.Conditions[0].Type, got.Spec.DisplayName)

	createdCloud, err := client.NetworkClouds().Create(ctx, networkCloud("cloud-status", "nsx-cloud-status.example.net"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("NetworkClouds().Create() error = %v", err)
	}
	cloudStatus := nsxv1alpha.NSXNetworkCloudStatus{Conditions: []metav1.Condition{{
		Type:               nsxv1alpha.ConditionReachable,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: createdCloud.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "Verified",
		Message:            "network cloud status update through typed kubeapi client",
	}}}
	updatedCloud, err := client.NetworkClouds().UpdateStatus(ctx, "cloud-status", cloudStatus, kubeapi.StatusUpdateOptions{ResourceVersion: createdCloud.ResourceVersion})
	if err != nil {
		t.Fatalf("NetworkClouds().UpdateStatus() error = %v", err)
	}
	if updatedCloud.Spec.NetworkCloudFQDN != "nsx-cloud-status.example.net" {
		t.Fatalf("network cloud spec after status update = %#v, want original fqdn", updatedCloud.Spec)
	}
	if len(updatedCloud.Status.Conditions) != 1 || updatedCloud.Status.Conditions[0].Type != nsxv1alpha.ConditionReachable {
		t.Fatalf("network cloud status conditions = %#v, want Reachable condition", updatedCloud.Status.Conditions)
	}
}

func TestApplyRequiresFieldManagerAndUsesServerSideApply(t *testing.T) {
	client, stop := startClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	object := group("group-apply", "nsx-apply.example.net", "app-apply", nsxv1alpha.NSXGroupModeObserve)
	if _, err := client.Groups().Apply(ctx, object, kubeapi.ApplyOptions{}); err == nil {
		t.Fatal("Apply() error = nil, want fieldManager validation error")
	}
	applied, err := client.Groups().Apply(ctx, object, kubeapi.ApplyOptions{FieldManager: "kubeapi-test", Force: true})
	if err != nil {
		t.Fatalf("Apply() create error = %v", err)
	}
	if applied.Spec.Mode != nsxv1alpha.NSXGroupModeObserve {
		t.Fatalf("applied mode = %q, want Observe", applied.Spec.Mode)
	}

	applied.Spec.Mode = nsxv1alpha.NSXGroupModeManage
	applied.Spec.CIDRs = []string{"10.44.0.0/24"}
	reapplied, err := client.Groups().Apply(ctx, applied, kubeapi.ApplyOptions{FieldManager: "kubeapi-test", Force: true})
	if err != nil {
		t.Fatalf("Apply() update error = %v", err)
	}
	if reapplied.Spec.Mode != nsxv1alpha.NSXGroupModeManage || len(reapplied.Spec.CIDRs) != 1 || reapplied.Spec.CIDRs[0] != "10.44.0.0/24" {
		t.Fatalf("reapplied spec = %#v, want managed mode and new cidr", reapplied.Spec)
	}

	cloud, err := client.NetworkClouds().Apply(ctx, networkCloud("cloud-apply", "nsx-cloud-apply.example.net"), kubeapi.ApplyOptions{FieldManager: "kubeapi-test", Force: true})
	if err != nil {
		t.Fatalf("NetworkClouds().Apply() error = %v", err)
	}
	if cloud.Spec.NetworkCloudFQDN != "nsx-cloud-apply.example.net" {
		t.Fatalf("applied network cloud spec = %#v, want requested fqdn", cloud.Spec)
	}
}

func TestDeleteRemovesTypedObject(t *testing.T) {
	client, stop := startClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	if _, err := client.NetworkClouds().Create(ctx, networkCloud("cloud-delete", "nsx-delete.example.net"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := client.NetworkClouds().Delete(ctx, "cloud-delete", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	_, err := client.NetworkClouds().Get(ctx, "cloud-delete", metav1.GetOptions{})
	if err == nil {
		t.Fatal("Get() error = nil, want not found after delete")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("Get() error = %T %[1]v, want Kubernetes not found", err)
	}

	if _, err := client.Groups().Create(ctx, group("group-delete", "nsx-delete.example.net", "app-delete", nsxv1alpha.NSXGroupModeManage), metav1.CreateOptions{}); err != nil {
		t.Fatalf("Groups().Create() error = %v", err)
	}
	if err := client.Groups().Delete(ctx, "group-delete", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("Groups().Delete() error = %v", err)
	}
	_, err = client.Groups().Get(ctx, "group-delete", metav1.GetOptions{})
	if err == nil {
		t.Fatal("Groups().Get() error = nil, want not found after delete")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("Groups().Get() error = %T %[1]v, want Kubernetes not found", err)
	}
}

func TestWatchEmitsTypedEventsForFieldSelector(t *testing.T) {
	client, stop := startClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	watcher, err := client.Groups().Watch(ctx, kubeapi.ListOptions{
		Filters: []kubeapi.FieldFilter{kubeapi.FilterBy(kubeapi.FieldGroupID, "watched-app")},
	})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	t.Cleanup(watcher.Stop)

	if _, err := client.Groups().Create(ctx, group("group-watch", "nsx-watch.example.net", "watched-app", nsxv1alpha.NSXGroupModeManage), metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create() watched group error = %v", err)
	}
	event := requireWatchEvent(t, ctx, watcher.ResultChan())
	object, ok := event.Object.(*nsxv1alpha.NSXGroup)
	if !ok {
		t.Fatalf("watch event object = %T, want *NSXGroup", event.Object)
	}
	if event.Type != watch.Added || object.Name != "group-watch" {
		t.Fatalf("watch event = %s/%s, want ADDED/group-watch", event.Type, object.Name)
	}

	cloudWatcher, err := client.NetworkClouds().Watch(ctx, kubeapi.ListOptions{
		Filters: []kubeapi.FieldFilter{kubeapi.FilterBy(kubeapi.FieldNetworkCloudID, "cloud-watch")},
	})
	if err != nil {
		t.Fatalf("NetworkClouds().Watch() error = %v", err)
	}
	t.Cleanup(cloudWatcher.Stop)
	if _, err := client.NetworkClouds().Create(ctx, networkCloud("cloud-watch", "nsx-cloud-watch.example.net"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create() watched network cloud error = %v", err)
	}
	cloudEvent := requireWatchEvent(t, ctx, cloudWatcher.ResultChan())
	cloudObject, ok := cloudEvent.Object.(*nsxv1alpha.NSXNetworkCloud)
	if !ok {
		t.Fatalf("network cloud watch event object = %T, want *NSXNetworkCloud", cloudEvent.Object)
	}
	if cloudEvent.Type != watch.Added || cloudObject.Name != "cloud-watch" {
		t.Fatalf("network cloud watch event = %s/%s, want ADDED/cloud-watch", cloudEvent.Type, cloudObject.Name)
	}
}

func TestInvalidResourceSpecificFiltersFailBeforeRequest(t *testing.T) {
	client, stop := startClient(t)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	_, cloudErr := client.NetworkClouds().List(ctx, kubeapi.ListOptions{
		Filters: []kubeapi.FieldFilter{kubeapi.FilterBy(kubeapi.FieldGroupID, "app")},
	})
	if cloudErr == nil {
		t.Fatal("NetworkClouds().List() error = nil, want invalid field error")
	}
	_, groupErr := client.Groups().List(ctx, kubeapi.ListOptions{
		Filters: []kubeapi.FieldFilter{kubeapi.FilterBy(kubeapi.FieldNetworkCloudID, "cloud")},
	})
	if groupErr == nil {
		t.Fatal("Groups().List() error = nil, want invalid field error")
	}
	if !strings.Contains(cloudErr.Error(), "not selectable") || !strings.Contains(groupErr.Error(), "not selectable") {
		t.Fatalf("invalid filter errors = %q / %q, want not selectable errors", cloudErr, groupErr)
	}
}

func TestConstructorValidationNilLoggerAndStructuredLogs(t *testing.T) {
	if _, err := kubeapi.NewClient(kubeapi.Options{}); err == nil {
		t.Fatal("NewClient() error = nil, want nil config validation error")
	}

	observedCore, logs := observer.New(zapcore.DebugLevel)
	client, stop := startClientWithLogger(t, zap.New(observedCore))
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	if _, err := client.NetworkClouds().Create(ctx, networkCloud("cloud-logs", "nsx-logs.example.net"), metav1.CreateOptions{}); err != nil {
		t.Fatalf("Create() with observed logger error = %v", err)
	}
	if logs.FilterMessage("creating typed kubernetes resource").Len() == 0 {
		t.Fatalf("observed logs did not include create info entry: %#v", logs.All())
	}
	if logs.FilterField(zap.String("resource", "nsxnetworkclouds")).Len() == 0 {
		t.Fatalf("observed logs did not include structured resource field: %#v", logs.All())
	}

	nilLoggerClient, nilLoggerStop := startClientWithLogger(t, nil)
	t.Cleanup(nilLoggerStop)
	if _, err := nilLoggerClient.NetworkClouds().List(ctx, kubeapi.ListOptions{}); err != nil {
		t.Fatalf("List() with nil logger client error = %v", err)
	}
}

func requireWatchEvent(t *testing.T, ctx context.Context, events <-chan watch.Event) watch.Event {
	t.Helper()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("watch channel closed before event")
			}
			if event.Type == watch.Error {
				t.Fatalf("watch returned error event: %#v", event.Object)
			}
			return event
		case <-ctx.Done():
			t.Fatalf("wait for watch event: %v", ctx.Err())
		}
	}
}

func startClient(t *testing.T) (*kubeapi.Client, func()) {
	t.Helper()
	return startClientWithLogger(t, zap.NewNop())
}

func startClientWithLogger(t *testing.T, logger *zap.Logger) (*kubeapi.Client, func()) {
	t.Helper()
	return startClientWithLoggerAndRecorder(t, logger, operatormetrics.NopRecorder{})
}

func startClientWithLoggerAndRecorder(t *testing.T, logger *zap.Logger, recorder operatormetrics.Recorder) (*kubeapi.Client, func()) {
	t.Helper()
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}
	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{repoPath(t, "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	client, err := kubeapi.NewClient(kubeapi.Options{
		Config:   restConfig,
		Logger:   logger,
		Recorder: recorder,
	})
	if err != nil {
		if stopErr := testEnvironment.Stop(); stopErr != nil {
			t.Errorf("stop envtest API server after NewClient failure: %v", stopErr)
		}
		t.Fatalf("NewClient() error = %v", err)
	}
	return client, func() {
		if err := testEnvironment.Stop(); err != nil {
			t.Errorf("stop envtest API server: %v", err)
		}
	}
}

func counterValue(t *testing.T, registry *prometheus.Registry, name string, labels map[string]string) float64 {
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
	t.Fatalf("missing counter %s with labels %v", name, labels)
	return 0
}

func histogramCount(t *testing.T, registry *prometheus.Registry, name string, labels map[string]string) uint64 {
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
			if metric.GetHistogram() != nil && metricLabelsContain(metric.GetLabel(), labels) {
				return metric.GetHistogram().GetSampleCount()
			}
		}
	}
	t.Fatalf("missing histogram %s with labels %v", name, labels)
	return 0
}

func metricLabelsContain(pairs []*dto.LabelPair, labels map[string]string) bool {
	for key, want := range labels {
		found := false
		for _, pair := range pairs {
			if pair.GetName() == key && pair.GetValue() == want {
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

func networkCloud(name string, fqdn string) *nsxv1alpha.NSXNetworkCloud {
	return &nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: nsxv1alpha.NSXNetworkCloudSpec{
			NetworkCloudFQDN: fqdn,
			NetworkCloudID:   name,
			Name:             "Cloud " + name,
		},
	}
}

func group(name string, fqdn string, groupID string, mode nsxv1alpha.NSXGroupMode) *nsxv1alpha.NSXGroup {
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

func repoPath(t *testing.T, elements ...string) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current test file path")
	}
	parts := append([]string{filepath.Dir(filename), "..", ".."}, elements...)
	return filepath.Clean(filepath.Join(parts...))
}

package v1alpha

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestCRDsInstallStatusSubresourceSelectableFieldsAndSchema(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Fatalf("KUBEBUILDER_ASSETS is required; run through make test or set it with setup-envtest use 1.32.x -p path")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	testEnvironment := &envtest.Environment{
		CRDDirectoryPaths:     []string{repoPath(t, "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := testEnvironment.Start()
	if err != nil {
		t.Fatalf("start envtest API server: %v", err)
	}
	defer func() {
		if err := testEnvironment.Stop(); err != nil {
			t.Errorf("stop envtest API server: %v", err)
		}
	}()

	serverVersion := requireKubernetesAtLeast(t, restConfig, 1, 32)
	t.Logf("envtest Kubernetes API server version: %s", serverVersion.String())

	extensionsClient, err := apiextensionsclient.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("create apiextensions client: %v", err)
	}
	requireEstablishedCRD(ctx, t, extensionsClient, "nsxnetworkclouds.nsx.ing.com")
	requireEstablishedCRD(ctx, t, extensionsClient, "nsxgroups.nsx.ing.com")
	requireStatusSchemaConditionsOnly(ctx, t, extensionsClient, "nsxnetworkclouds.nsx.ing.com")
	requireStatusSchemaConditionsOnly(ctx, t, extensionsClient, "nsxgroups.nsx.ing.com")

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}

	clouds := dynamicClient.Resource(schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "nsxnetworkclouds",
	})
	groups := dynamicClient.Resource(schema.GroupVersionResource{
		Group:    GroupName,
		Version:  Version,
		Resource: "nsxgroups",
	})

	cloudA := createObject(ctx, t, clouds, networkCloudObject("cloud-a", "nsx-a.example.net", "cloud-a", "Cloud A"))
	createObject(ctx, t, clouds, networkCloudObject("cloud-b", "nsx-b.example.net", "cloud-b", "Cloud B"))
	groupA := createObject(ctx, t, groups, groupObject("group-a", "nsx-a.example.net", "app-a", NSXGroupModeManage, "App A", []string{"10.0.0.0/24"}, nil))
	createObject(ctx, t, groups, groupObject("group-b", "nsx-b.example.net", "app-b", NSXGroupModeObserve, "App B", []string{"10.1.0.0/24"}, nil))

	updateStatusAndRequireSpecUnchanged(ctx, t, clouds, cloudA, "Reachable")
	updateStatusAndRequireSpecUnchanged(ctx, t, groups, groupA, "Synced")

	requireNames(ctx, t, clouds, "spec.networkCloudFQDN=nsx-a.example.net", []string{"cloud-a"})
	requireNames(ctx, t, clouds, "spec.networkCloudId=cloud-b", []string{"cloud-b"})
	requireNames(ctx, t, groups, "spec.networkCloudFQDN=nsx-a.example.net", []string{"group-a"})
	requireNames(ctx, t, groups, "spec.groupID=app-b", []string{"group-b"})
	requireNames(ctx, t, groups, "spec.mode=Manage", []string{"group-a"})

	requireCreateRejected(ctx, t, groups, groupObject("invalid-mode", "nsx-a.example.net", "bad-mode", NSXGroupMode("Invalid"), "Invalid", []string{"10.2.0.0/24"}, nil))
	requireCreateRejected(ctx, t, groups, map[string]any{
		"apiVersion": SchemeGroupVersion.String(),
		"kind":       "NSXGroup",
		"metadata": map[string]any{
			"name": "invalid-segment-path",
		},
		"spec": map[string]any{
			"networkCloudFQDN": "nsx-a.example.net",
			"groupID":          "invalid-segment-path",
			"display_name":     "Invalid Segment Path",
			"mode":             string(NSXGroupModeManage),
			"cidrs":            []any{"10.3.0.0/24"},
			"segment_path":     12,
		},
	})
	requireCreateRejected(ctx, t, groups, map[string]any{
		"apiVersion": SchemeGroupVersion.String(),
		"kind":       "NSXGroup",
		"metadata": map[string]any{
			"name": "missing-cidrs",
		},
		"spec": map[string]any{
			"networkCloudFQDN": "nsx-a.example.net",
			"groupID":          "missing-cidrs",
			"display_name":     "Missing CIDRs",
			"mode":             string(NSXGroupModeManage),
		},
	})
	requireCreateRejected(ctx, t, clouds, map[string]any{
		"apiVersion": SchemeGroupVersion.String(),
		"kind":       "NSXNetworkCloud",
		"metadata": map[string]any{
			"name": "missing-fqdn",
		},
		"spec": map[string]any{
			"networkCloudId": "missing-fqdn",
			"name":           "Missing FQDN",
		},
	})
}

type resourceClient interface {
	Create(context.Context, *unstructured.Unstructured, metav1.CreateOptions, ...string) (*unstructured.Unstructured, error)
	Get(context.Context, string, metav1.GetOptions, ...string) (*unstructured.Unstructured, error)
	List(context.Context, metav1.ListOptions) (*unstructured.UnstructuredList, error)
	UpdateStatus(context.Context, *unstructured.Unstructured, metav1.UpdateOptions) (*unstructured.Unstructured, error)
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

func requireKubernetesAtLeast(t *testing.T, config *rest.Config, major int, minor int) *utilversion.Version {
	t.Helper()
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		t.Fatalf("create discovery client: %v", err)
	}
	serverInfo, err := discoveryClient.ServerVersion()
	if err != nil {
		t.Fatalf("read Kubernetes server version: %v", err)
	}
	serverVersion, err := utilversion.ParseSemantic(serverInfo.GitVersion)
	if err != nil {
		t.Fatalf("parse Kubernetes server version %q: %v", serverInfo.GitVersion, err)
	}
	requiredVersion := utilversion.MustParseSemantic(fmt.Sprintf("v%d.%d.0", major, minor))
	if !serverVersion.AtLeast(requiredVersion) {
		t.Fatalf("Kubernetes server version = %s, want at least %s", serverVersion.String(), requiredVersion.String())
	}
	return serverVersion
}

func requireEstablishedCRD(ctx context.Context, t *testing.T, client *apiextensionsclient.Clientset, name string) {
	t.Helper()
	err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 20*time.Second, true, func(ctx context.Context) (bool, error) {
		crd, err := client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("get CRD %s: %w", name, err)
		}
		for _, condition := range crd.Status.Conditions {
			if condition.Type == apiextensionsv1.Established && condition.Status == apiextensionsv1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("wait for CRD %s Established: %v", name, err)
	}
	t.Logf("CRD %s is Established", name)
}

func requireStatusSchemaConditionsOnly(ctx context.Context, t *testing.T, client *apiextensionsclient.Clientset, name string) {
	t.Helper()
	crd, err := client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get CRD %s for status schema verification: %v", name, err)
	}
	if len(crd.Spec.Versions) != 1 {
		t.Fatalf("CRD %s versions = %d, want exactly one served version", name, len(crd.Spec.Versions))
	}
	schema := crd.Spec.Versions[0].Schema
	if schema == nil || schema.OpenAPIV3Schema == nil {
		t.Fatalf("CRD %s missing OpenAPI v3 schema", name)
	}
	statusSchema, ok := schema.OpenAPIV3Schema.Properties["status"]
	if !ok {
		t.Fatalf("CRD %s schema has no status property", name)
	}
	gotProperties := make([]string, 0, len(statusSchema.Properties))
	for property := range statusSchema.Properties {
		gotProperties = append(gotProperties, property)
	}
	slices.Sort(gotProperties)
	if !slices.Equal(gotProperties, []string{"conditions"}) {
		t.Fatalf("CRD %s status properties = %v, want only conditions", name, gotProperties)
	}
	if _, ok := statusSchema.Properties["conditions"]; !ok {
		t.Fatalf("CRD %s status.conditions schema is missing", name)
	}
	t.Logf("CRD %s status schema exposes only conditions", name)
}

func createObject(ctx context.Context, t *testing.T, client resourceClient, object map[string]any) *unstructured.Unstructured {
	t.Helper()
	created, err := client.Create(ctx, &unstructured.Unstructured{Object: object}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create %s/%s: %v", object["kind"], objectName(object), err)
	}
	return created
}

func updateStatusAndRequireSpecUnchanged(ctx context.Context, t *testing.T, client resourceClient, object *unstructured.Unstructured, conditionType string) {
	t.Helper()
	displayField := "display_name"
	originalDisplayName, displayNameFound, err := unstructured.NestedString(object.Object, "spec", displayField)
	if err != nil {
		t.Fatalf("read original display_name from %s: %v", object.GetName(), err)
	}
	if !displayNameFound {
		displayField = "name"
		originalDisplayName, displayNameFound, err = unstructured.NestedString(object.Object, "spec", displayField)
		if err != nil {
			t.Fatalf("read original name from %s: %v", object.GetName(), err)
		}
	}
	statusObject := object.DeepCopy()
	if err := unstructured.SetNestedField(statusObject.Object, "changed through status", "spec", displayField); err != nil {
		t.Fatalf("try mutating spec before status update for %s: %v", object.GetName(), err)
	}
	condition := []any{map[string]any{
		"type":               conditionType,
		"status":             "True",
		"lastTransitionTime": time.Now().UTC().Format(time.RFC3339),
		"reason":             "Verified",
		"message":            "status subresource accepts conditions",
	}}
	if err := unstructured.SetNestedSlice(statusObject.Object, condition, "status", "conditions"); err != nil {
		t.Fatalf("set status conditions on %s: %v", object.GetName(), err)
	}
	if err := unstructured.SetNestedField(statusObject.Object, true, "status", "synced"); err != nil {
		t.Fatalf("set synthetic status synced field on %s: %v", object.GetName(), err)
	}
	if err := unstructured.SetNestedField(statusObject.Object, map[string]any{"id": "remote-id"}, "status", "remoteObject"); err != nil {
		t.Fatalf("set synthetic status remoteObject field on %s: %v", object.GetName(), err)
	}
	updated, err := client.UpdateStatus(ctx, statusObject, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("update status for %s: %v", object.GetName(), err)
	}
	conditions, conditionsFound, err := unstructured.NestedSlice(updated.Object, "status", "conditions")
	if err != nil {
		t.Fatalf("read status conditions for %s: %v", object.GetName(), err)
	}
	if !conditionsFound || len(conditions) != 1 {
		t.Fatalf("status conditions for %s = %#v, want one condition", object.GetName(), conditions)
	}
	statusMap, statusFound, err := unstructured.NestedMap(updated.Object, "status")
	if err != nil {
		t.Fatalf("read status map for %s: %v", object.GetName(), err)
	}
	if !statusFound {
		t.Fatalf("status map for %s missing after status update", object.GetName())
	}
	gotStatusProperties := make([]string, 0, len(statusMap))
	for property := range statusMap {
		gotStatusProperties = append(gotStatusProperties, property)
	}
	slices.Sort(gotStatusProperties)
	if !slices.Equal(gotStatusProperties, []string{"conditions"}) {
		t.Fatalf("stored status properties for %s = %v, want only conditions", object.GetName(), gotStatusProperties)
	}
	if displayNameFound {
		current, currentFound, err := unstructured.NestedString(updated.Object, "spec", displayField)
		if err != nil {
			t.Fatalf("read %s after status update for %s: %v", displayField, object.GetName(), err)
		}
		if !currentFound || current != originalDisplayName {
			t.Fatalf("status update changed spec.%s for %s to %q, want %q", displayField, object.GetName(), current, originalDisplayName)
		}
	}
	t.Logf("status subresource update for %s kept spec unchanged and stored %s condition", object.GetName(), conditionType)
}

func requireNames(ctx context.Context, t *testing.T, client resourceClient, fieldSelector string, want []string) {
	t.Helper()
	list, err := client.List(ctx, metav1.ListOptions{FieldSelector: fieldSelector})
	if err != nil {
		t.Fatalf("list with field selector %q: %v", fieldSelector, err)
	}
	got := make([]string, 0, len(list.Items))
	for i := range list.Items {
		got = append(got, list.Items[i].GetName())
	}
	if !slices.Equal(got, want) {
		t.Fatalf("list with field selector %q returned names %v, want %v", fieldSelector, got, want)
	}
	t.Logf("field selector %q returned %v", fieldSelector, got)
}

func requireCreateRejected(ctx context.Context, t *testing.T, client resourceClient, object map[string]any) {
	t.Helper()
	created, err := client.Create(ctx, &unstructured.Unstructured{Object: object}, metav1.CreateOptions{})
	if err == nil {
		t.Fatalf("create invalid %s/%s unexpectedly succeeded: %#v", object["kind"], objectName(object), created.Object)
	}
	t.Logf("invalid %s/%s rejected: %v", object["kind"], objectName(object), err)
}

func networkCloudObject(name string, fqdn string, cloudID string, displayName string) map[string]any {
	return map[string]any{
		"apiVersion": SchemeGroupVersion.String(),
		"kind":       "NSXNetworkCloud",
		"metadata": map[string]any{
			"name": name,
		},
		"spec": map[string]any{
			"networkCloudFQDN": fqdn,
			"networkCloudId":   cloudID,
			"name":             displayName,
		},
	}
}

func groupObject(name string, fqdn string, groupID string, mode NSXGroupMode, displayName string, cidrs []string, segmentPath *string) map[string]any {
	spec := map[string]any{
		"networkCloudFQDN": fqdn,
		"groupID":          groupID,
		"display_name":     displayName,
		"mode":             string(mode),
		"cidrs":            stringSliceToAny(cidrs),
		"segment_path":     segmentPath,
	}
	return map[string]any{
		"apiVersion": SchemeGroupVersion.String(),
		"kind":       "NSXGroup",
		"metadata": map[string]any{
			"name": name,
		},
		"spec": spec,
	}
}

func stringSliceToAny(values []string) []any {
	result := make([]any, len(values))
	for i := range values {
		result[i] = values[i]
	}
	return result
}

func objectName(object map[string]any) string {
	metadataValue, ok := object["metadata"]
	if !ok {
		return ""
	}
	metadata, ok := metadataValue.(map[string]any)
	if !ok {
		return ""
	}
	nameValue, ok := metadata["name"]
	if !ok {
		return ""
	}
	name, ok := nameValue.(string)
	if !ok {
		return ""
	}
	return name
}

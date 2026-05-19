package kubeapi

import (
	"context"
	"encoding/json"
	"fmt"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type GroupApplyRequest struct {
	Object  *nsxv1alpha.NSXGroup
	Options ApplyOptions
}

type GroupUpdateRequest struct {
	Object  *nsxv1alpha.NSXGroup
	Options metav1.UpdateOptions
}

type GroupFinalizerPatchRequest struct {
	Name            string
	ResourceVersion string
	Finalizers      []string
	Options         metav1.PatchOptions
}

type GroupStatusUpdateRequest struct {
	Name    string
	Status  nsxv1alpha.NSXGroupStatus
	Options StatusUpdateOptions
}

type GroupCreateRequest struct {
	Object  *nsxv1alpha.NSXGroup
	Options metav1.CreateOptions
}

type GroupDeleteRequest struct {
	Name    string
	Options metav1.DeleteOptions
}

type NetworkCloudApplyRequest struct {
	Object  *nsxv1alpha.NSXNetworkCloud
	Options ApplyOptions
}

type NetworkCloudUpdateRequest struct {
	Object  *nsxv1alpha.NSXNetworkCloud
	Options metav1.UpdateOptions
}

type NetworkCloudFinalizerPatchRequest struct {
	Name            string
	ResourceVersion string
	Finalizers      []string
	Options         metav1.PatchOptions
}

type NetworkCloudStatusUpdateRequest struct {
	Name    string
	Status  nsxv1alpha.NSXNetworkCloudStatus
	Options StatusUpdateOptions
}

type NetworkCloudCreateRequest struct {
	Object  *nsxv1alpha.NSXNetworkCloud
	Options metav1.CreateOptions
}

type NetworkCloudDeleteRequest struct {
	Name    string
	Options metav1.DeleteOptions
}

func (c *GroupClient) ApplyBatch(ctx context.Context, requests map[BatchKey]GroupApplyRequest) (map[BatchKey]*nsxv1alpha.NSXGroup, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[GroupApplyRequest, *nsxv1alpha.NSXGroup]{
		Operation: "apply",
		Resource:  groupResource,
		Execute: func(ctx context.Context, request GroupApplyRequest) (*nsxv1alpha.NSXGroup, error) {
			return c.resource.apply(ctx, request.Object, request.Options)
		},
	}, requests)
}

func (c *GroupClient) UpdateBatch(ctx context.Context, requests map[BatchKey]GroupUpdateRequest) (map[BatchKey]*nsxv1alpha.NSXGroup, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[GroupUpdateRequest, *nsxv1alpha.NSXGroup]{
		Operation: "update",
		Resource:  groupResource,
		Execute: func(ctx context.Context, request GroupUpdateRequest) (*nsxv1alpha.NSXGroup, error) {
			return c.resource.update(ctx, request.Object, request.Options)
		},
	}, requests)
}

func (c *GroupClient) PatchFinalizersBatch(ctx context.Context, requests map[BatchKey]GroupFinalizerPatchRequest) (map[BatchKey]*nsxv1alpha.NSXGroup, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[GroupFinalizerPatchRequest, *nsxv1alpha.NSXGroup]{
		Operation: "patchFinalizers",
		Resource:  groupResource,
		Execute: func(ctx context.Context, request GroupFinalizerPatchRequest) (*nsxv1alpha.NSXGroup, error) {
			return c.resource.patchFinalizers(ctx, request.Name, request.ResourceVersion, request.Finalizers, request.Options)
		},
	}, requests)
}

func (c *GroupClient) UpdateStatusBatch(ctx context.Context, requests map[BatchKey]GroupStatusUpdateRequest) (map[BatchKey]*nsxv1alpha.NSXGroup, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[GroupStatusUpdateRequest, *nsxv1alpha.NSXGroup]{
		Operation: "updateStatus",
		Resource:  groupResource,
		Execute: func(ctx context.Context, request GroupStatusUpdateRequest) (*nsxv1alpha.NSXGroup, error) {
			object := &nsxv1alpha.NSXGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:            request.Name,
					ResourceVersion: request.Options.ResourceVersion,
				},
				Status: request.Status,
			}
			return c.resource.updateStatus(ctx, object, request.Options.UpdateOptions)
		},
	}, requests)
}

func (c *GroupClient) CreateBatch(ctx context.Context, requests map[BatchKey]GroupCreateRequest) (map[BatchKey]*nsxv1alpha.NSXGroup, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[GroupCreateRequest, *nsxv1alpha.NSXGroup]{
		Operation: "create",
		Resource:  groupResource,
		Execute: func(ctx context.Context, request GroupCreateRequest) (*nsxv1alpha.NSXGroup, error) {
			return c.resource.create(ctx, request.Object, request.Options)
		},
	}, requests)
}

func (c *GroupClient) DeleteBatch(ctx context.Context, requests map[BatchKey]GroupDeleteRequest) (map[BatchKey]struct{}, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[GroupDeleteRequest, struct{}]{
		Operation: "delete",
		Resource:  groupResource,
		Execute: func(ctx context.Context, request GroupDeleteRequest) (struct{}, error) {
			if err := c.resource.delete(ctx, request.Name, request.Options); err != nil {
				return struct{}{}, err
			}
			return struct{}{}, nil
		},
	}, requests)
}

func (c *NetworkCloudClient) ApplyBatch(ctx context.Context, requests map[BatchKey]NetworkCloudApplyRequest) (map[BatchKey]*nsxv1alpha.NSXNetworkCloud, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[NetworkCloudApplyRequest, *nsxv1alpha.NSXNetworkCloud]{
		Operation: "apply",
		Resource:  networkCloudResource,
		Execute: func(ctx context.Context, request NetworkCloudApplyRequest) (*nsxv1alpha.NSXNetworkCloud, error) {
			return c.resource.apply(ctx, request.Object, request.Options)
		},
	}, requests)
}

func (c *NetworkCloudClient) UpdateBatch(ctx context.Context, requests map[BatchKey]NetworkCloudUpdateRequest) (map[BatchKey]*nsxv1alpha.NSXNetworkCloud, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[NetworkCloudUpdateRequest, *nsxv1alpha.NSXNetworkCloud]{
		Operation: "update",
		Resource:  networkCloudResource,
		Execute: func(ctx context.Context, request NetworkCloudUpdateRequest) (*nsxv1alpha.NSXNetworkCloud, error) {
			return c.resource.update(ctx, request.Object, request.Options)
		},
	}, requests)
}

func (c *NetworkCloudClient) PatchFinalizersBatch(ctx context.Context, requests map[BatchKey]NetworkCloudFinalizerPatchRequest) (map[BatchKey]*nsxv1alpha.NSXNetworkCloud, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[NetworkCloudFinalizerPatchRequest, *nsxv1alpha.NSXNetworkCloud]{
		Operation: "patchFinalizers",
		Resource:  networkCloudResource,
		Execute: func(ctx context.Context, request NetworkCloudFinalizerPatchRequest) (*nsxv1alpha.NSXNetworkCloud, error) {
			return c.resource.patchFinalizers(ctx, request.Name, request.ResourceVersion, request.Finalizers, request.Options)
		},
	}, requests)
}

func (c *NetworkCloudClient) UpdateStatusBatch(ctx context.Context, requests map[BatchKey]NetworkCloudStatusUpdateRequest) (map[BatchKey]*nsxv1alpha.NSXNetworkCloud, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[NetworkCloudStatusUpdateRequest, *nsxv1alpha.NSXNetworkCloud]{
		Operation: "updateStatus",
		Resource:  networkCloudResource,
		Execute: func(ctx context.Context, request NetworkCloudStatusUpdateRequest) (*nsxv1alpha.NSXNetworkCloud, error) {
			object := &nsxv1alpha.NSXNetworkCloud{
				ObjectMeta: metav1.ObjectMeta{
					Name:            request.Name,
					ResourceVersion: request.Options.ResourceVersion,
				},
				Status: request.Status,
			}
			return c.resource.updateStatus(ctx, object, request.Options.UpdateOptions)
		},
	}, requests)
}

func (c *NetworkCloudClient) CreateBatch(ctx context.Context, requests map[BatchKey]NetworkCloudCreateRequest) (map[BatchKey]*nsxv1alpha.NSXNetworkCloud, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[NetworkCloudCreateRequest, *nsxv1alpha.NSXNetworkCloud]{
		Operation: "create",
		Resource:  networkCloudResource,
		Execute: func(ctx context.Context, request NetworkCloudCreateRequest) (*nsxv1alpha.NSXNetworkCloud, error) {
			return c.resource.create(ctx, request.Object, request.Options)
		},
	}, requests)
}

func (c *NetworkCloudClient) DeleteBatch(ctx context.Context, requests map[BatchKey]NetworkCloudDeleteRequest) (map[BatchKey]struct{}, map[BatchKey]error, error) {
	return ExecuteBatch(ctx, c.resource.batchConfig, c.resource.log, BatchOperation[NetworkCloudDeleteRequest, struct{}]{
		Operation: "delete",
		Resource:  networkCloudResource,
		Execute: func(ctx context.Context, request NetworkCloudDeleteRequest) (struct{}, error) {
			if err := c.resource.delete(ctx, request.Name, request.Options); err != nil {
				return struct{}{}, err
			}
			return struct{}{}, nil
		},
	}, requests)
}

func (r typedResource[Object, List]) patchFinalizers(ctx context.Context, name string, resourceVersion string, finalizers []string, options metav1.PatchOptions) (Object, error) {
	operations := make([]JSONPatchOperation, 0, 2)
	if resourceVersion != "" {
		operations = append(operations, JSONPatchOperation{
			Op:    "test",
			Path:  "/metadata/resourceVersion",
			Value: resourceVersion,
		})
	}
	operations = append(operations, JSONPatchOperation{
		Op:    "add",
		Path:  "/metadata/finalizers",
		Value: finalizers,
	})
	body, err := json.Marshal(operations)
	if err != nil {
		var zero Object
		return zero, fmt.Errorf("marshal %s %q finalizer patch: %w", r.resource, name, err)
	}
	r.log.Info("patching typed kubernetes resource finalizers", zap.String("name", name), zap.String("resourceVersion", resourceVersion), zap.Int("finalizerCount", len(finalizers)))
	result := r.newObject()
	err = r.restClient.Patch(types.JSONPatchType).
		Resource(r.resource).
		Name(name).
		VersionedParams(&options, r.parameterCodec).
		Body(body).
		SetHeader("Content-Type", string(types.JSONPatchType)).
		Do(ctx).
		Into(result)
	if err != nil {
		var zero Object
		r.log.Debug("patch typed kubernetes resource finalizers failed", zap.String("name", name), zap.Error(err))
		return zero, fmt.Errorf("patch %s %q finalizers: %w", r.resource, name, err)
	}
	r.stampObject(result)
	r.log.Debug("patched typed kubernetes resource finalizers", zap.String("name", result.GetName()), zap.String("resourceVersion", result.GetResourceVersion()))
	return result, nil
}

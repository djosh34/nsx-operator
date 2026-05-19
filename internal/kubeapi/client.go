package kubeapi

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/operatormetrics"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

const (
	networkCloudResource = "nsxnetworkclouds"
	networkCloudKind     = "NSXNetworkCloud"
	groupResource        = "nsxgroups"
	groupKind            = "NSXGroup"
)

type Options struct {
	Config   *rest.Config
	Logger   *zap.Logger
	Recorder operatormetrics.Recorder
}

type Client struct {
	networkClouds *NetworkCloudClient
	groups        *GroupClient
}

func NewClient(options Options) (*Client, error) {
	if options.Config == nil {
		return nil, errors.New("kubernetes rest config is required")
	}
	log := options.Logger
	if log == nil {
		log = zap.NewNop()
	}
	recorder := options.Recorder
	if recorder == nil {
		recorder = operatormetrics.NopRecorder{}
	}
	scheme := runtime.NewScheme()
	if err := nsxv1alpha.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register nsx api scheme: %w", err)
	}
	config := rest.CopyConfig(options.Config)
	config.GroupVersion = &nsxv1alpha.SchemeGroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme).WithoutConversion()
	config.WrapTransport = wrapKubernetesMetricsTransport(config.WrapTransport, recorder, log)
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		return nil, fmt.Errorf("construct nsx kubernetes rest client: %w", err)
	}
	parameterCodec := runtime.NewParameterCodec(scheme)
	log.Info("constructed typed kubernetes crd client", zap.String("apiGroup", nsxv1alpha.GroupName), zap.String("apiVersion", nsxv1alpha.Version))

	return &Client{
		networkClouds: &NetworkCloudClient{
			resource: typedResource[*nsxv1alpha.NSXNetworkCloud, *nsxv1alpha.NSXNetworkCloudList]{
				restClient:     restClient,
				parameterCodec: parameterCodec,
				resource:       networkCloudResource,
				kind:           networkCloudKind,
				log:            log.With(zap.String("resource", networkCloudResource)),
				allowed: allowedFields(
					FieldNetworkCloudFQDN,
					FieldNetworkCloudID,
				),
				newObject: func() *nsxv1alpha.NSXNetworkCloud { return &nsxv1alpha.NSXNetworkCloud{} },
				newList:   func() *nsxv1alpha.NSXNetworkCloudList { return &nsxv1alpha.NSXNetworkCloudList{} },
			},
		},
		groups: &GroupClient{
			resource: typedResource[*nsxv1alpha.NSXGroup, *nsxv1alpha.NSXGroupList]{
				restClient:     restClient,
				parameterCodec: parameterCodec,
				resource:       groupResource,
				kind:           groupKind,
				log:            log.With(zap.String("resource", groupResource)),
				allowed: allowedFields(
					FieldNetworkCloudFQDN,
					FieldGroupID,
					FieldGroupMode,
				),
				newObject: func() *nsxv1alpha.NSXGroup { return &nsxv1alpha.NSXGroup{} },
				newList:   func() *nsxv1alpha.NSXGroupList { return &nsxv1alpha.NSXGroupList{} },
			},
		},
	}, nil
}

func (c *Client) NetworkClouds() *NetworkCloudClient {
	return c.networkClouds
}

func (c *Client) Groups() *GroupClient {
	return c.groups
}

type NetworkCloudClient struct {
	resource typedResource[*nsxv1alpha.NSXNetworkCloud, *nsxv1alpha.NSXNetworkCloudList]
}

func (c *NetworkCloudClient) List(ctx context.Context, options ListOptions) (*nsxv1alpha.NSXNetworkCloudList, error) {
	return c.resource.list(ctx, options)
}

func (c *NetworkCloudClient) Get(ctx context.Context, name string, options metav1.GetOptions) (*nsxv1alpha.NSXNetworkCloud, error) {
	return c.resource.get(ctx, name, options)
}

func (c *NetworkCloudClient) Create(ctx context.Context, object *nsxv1alpha.NSXNetworkCloud, options metav1.CreateOptions) (*nsxv1alpha.NSXNetworkCloud, error) {
	return c.resource.create(ctx, object, options)
}

func (c *NetworkCloudClient) Update(ctx context.Context, object *nsxv1alpha.NSXNetworkCloud, options metav1.UpdateOptions) (*nsxv1alpha.NSXNetworkCloud, error) {
	return c.resource.update(ctx, object, options)
}

func (c *NetworkCloudClient) Apply(ctx context.Context, object *nsxv1alpha.NSXNetworkCloud, options ApplyOptions) (*nsxv1alpha.NSXNetworkCloud, error) {
	return c.resource.apply(ctx, object, options)
}

func (c *NetworkCloudClient) UpdateStatus(ctx context.Context, name string, status nsxv1alpha.NSXNetworkCloudStatus, options StatusUpdateOptions) (*nsxv1alpha.NSXNetworkCloud, error) {
	object := &nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			ResourceVersion: options.ResourceVersion,
		},
		Status: status,
	}
	return c.resource.updateStatus(ctx, object, options.UpdateOptions)
}

func (c *NetworkCloudClient) Delete(ctx context.Context, name string, options metav1.DeleteOptions) error {
	return c.resource.delete(ctx, name, options)
}

func (c *NetworkCloudClient) Watch(ctx context.Context, options ListOptions) (watch.Interface, error) {
	return c.resource.watch(ctx, options)
}

type GroupClient struct {
	resource typedResource[*nsxv1alpha.NSXGroup, *nsxv1alpha.NSXGroupList]
}

func (c *GroupClient) List(ctx context.Context, options ListOptions) (*nsxv1alpha.NSXGroupList, error) {
	return c.resource.list(ctx, options)
}

func (c *GroupClient) Get(ctx context.Context, name string, options metav1.GetOptions) (*nsxv1alpha.NSXGroup, error) {
	return c.resource.get(ctx, name, options)
}

func (c *GroupClient) Create(ctx context.Context, object *nsxv1alpha.NSXGroup, options metav1.CreateOptions) (*nsxv1alpha.NSXGroup, error) {
	return c.resource.create(ctx, object, options)
}

func (c *GroupClient) Update(ctx context.Context, object *nsxv1alpha.NSXGroup, options metav1.UpdateOptions) (*nsxv1alpha.NSXGroup, error) {
	return c.resource.update(ctx, object, options)
}

func (c *GroupClient) Apply(ctx context.Context, object *nsxv1alpha.NSXGroup, options ApplyOptions) (*nsxv1alpha.NSXGroup, error) {
	return c.resource.apply(ctx, object, options)
}

func (c *GroupClient) UpdateStatus(ctx context.Context, name string, status nsxv1alpha.NSXGroupStatus, options StatusUpdateOptions) (*nsxv1alpha.NSXGroup, error) {
	object := &nsxv1alpha.NSXGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			ResourceVersion: options.ResourceVersion,
		},
		Status: status,
	}
	return c.resource.updateStatus(ctx, object, options.UpdateOptions)
}

func (c *GroupClient) Delete(ctx context.Context, name string, options metav1.DeleteOptions) error {
	return c.resource.delete(ctx, name, options)
}

func (c *GroupClient) Watch(ctx context.Context, options ListOptions) (watch.Interface, error) {
	return c.resource.watch(ctx, options)
}

type ListOptions struct {
	ResourceVersion string
	Filters         []FieldFilter
	Limit           int64
	Continue        string
}

type FieldSelectorField string

const (
	FieldNetworkCloudFQDN FieldSelectorField = "spec.networkCloudFQDN"
	FieldNetworkCloudID   FieldSelectorField = "spec.networkCloudId"
	FieldGroupID          FieldSelectorField = "spec.groupID"
	FieldGroupMode        FieldSelectorField = "spec.mode"
)

type FieldFilter struct {
	field FieldSelectorField
	value string
}

func FilterBy(field FieldSelectorField, value string) FieldFilter {
	return FieldFilter{field: field, value: value}
}

func (f FieldFilter) Field() FieldSelectorField {
	return f.field
}

func (f FieldFilter) Value() string {
	return f.value
}

type ApplyOptions struct {
	FieldManager string
	Force        bool
}

type StatusUpdateOptions struct {
	ResourceVersion string
	UpdateOptions   metav1.UpdateOptions
}

type clientObject interface {
	runtime.Object
	metav1.Object
}

type typedResource[Object clientObject, List runtime.Object] struct {
	restClient     rest.Interface
	parameterCodec runtime.ParameterCodec
	resource       string
	kind           string
	log            *zap.Logger
	allowed        map[FieldSelectorField]struct{}
	newObject      func() Object
	newList        func() List
}

func (r typedResource[Object, List]) list(ctx context.Context, options ListOptions) (List, error) {
	listOptions, err := r.listOptions(options)
	if err != nil {
		var zero List
		return zero, err
	}
	result := r.newList()
	r.log.Info("listing typed kubernetes resources", zap.String("fieldSelector", listOptions.FieldSelector), zap.String("resourceVersion", listOptions.ResourceVersion))
	err = r.restClient.Get().
		Resource(r.resource).
		VersionedParams(&listOptions, r.parameterCodec).
		Do(ctx).
		Into(result)
	if err != nil {
		var zero List
		r.log.Debug("list typed kubernetes resources failed", zap.Error(err))
		return zero, fmt.Errorf("list %s: %w", r.resource, err)
	}
	r.stampList(result)
	r.log.Debug("listed typed kubernetes resources", zap.String("fieldSelector", listOptions.FieldSelector))
	return result, nil
}

func (r typedResource[Object, List]) get(ctx context.Context, name string, options metav1.GetOptions) (Object, error) {
	result := r.newObject()
	r.log.Info("getting typed kubernetes resource", zap.String("name", name))
	err := r.restClient.Get().
		Resource(r.resource).
		Name(name).
		VersionedParams(&options, r.parameterCodec).
		Do(ctx).
		Into(result)
	if err != nil {
		var zero Object
		r.log.Debug("get typed kubernetes resource failed", zap.String("name", name), zap.Error(err))
		return zero, fmt.Errorf("get %s %q: %w", r.resource, name, err)
	}
	r.stampObject(result)
	r.log.Debug("got typed kubernetes resource", zap.String("name", name), zap.String("resourceVersion", result.GetResourceVersion()))
	return result, nil
}

func (r typedResource[Object, List]) create(ctx context.Context, object Object, options metav1.CreateOptions) (Object, error) {
	prepared, err := r.prepare(object)
	if err != nil {
		var zero Object
		return zero, err
	}
	r.log.Info("creating typed kubernetes resource", zap.String("name", prepared.GetName()))
	result := r.newObject()
	err = r.restClient.Post().
		Resource(r.resource).
		VersionedParams(&options, r.parameterCodec).
		Body(prepared).
		Do(ctx).
		Into(result)
	if err != nil {
		var zero Object
		r.log.Debug("create typed kubernetes resource failed", zap.String("name", prepared.GetName()), zap.Error(err))
		return zero, fmt.Errorf("create %s %q: %w", r.resource, prepared.GetName(), err)
	}
	r.stampObject(result)
	r.log.Debug("created typed kubernetes resource", zap.String("name", result.GetName()), zap.String("resourceVersion", result.GetResourceVersion()))
	return result, nil
}

func (r typedResource[Object, List]) update(ctx context.Context, object Object, options metav1.UpdateOptions) (Object, error) {
	prepared, err := r.prepare(object)
	if err != nil {
		var zero Object
		return zero, err
	}
	if prepared.GetResourceVersion() == "" {
		var zero Object
		return zero, fmt.Errorf("update %s %q: resourceVersion is required", r.resource, prepared.GetName())
	}
	r.log.Info("updating typed kubernetes resource", zap.String("name", prepared.GetName()), zap.String("resourceVersion", prepared.GetResourceVersion()))
	result := r.newObject()
	err = r.restClient.Put().
		Resource(r.resource).
		Name(prepared.GetName()).
		VersionedParams(&options, r.parameterCodec).
		Body(prepared).
		Do(ctx).
		Into(result)
	if err != nil {
		var zero Object
		r.log.Debug("update typed kubernetes resource failed", zap.String("name", prepared.GetName()), zap.Error(err))
		return zero, fmt.Errorf("update %s %q: %w", r.resource, prepared.GetName(), err)
	}
	r.stampObject(result)
	r.log.Debug("updated typed kubernetes resource", zap.String("name", result.GetName()), zap.String("resourceVersion", result.GetResourceVersion()))
	return result, nil
}

func (r typedResource[Object, List]) apply(ctx context.Context, object Object, options ApplyOptions) (Object, error) {
	if options.FieldManager == "" {
		var zero Object
		return zero, fmt.Errorf("apply %s: fieldManager is required", r.resource)
	}
	prepared, err := r.prepare(object)
	if err != nil {
		var zero Object
		return zero, err
	}
	prepared.SetResourceVersion("")
	prepared.SetManagedFields(nil)
	patchOptions := metav1.PatchOptions{
		FieldManager: options.FieldManager,
		Force:        &options.Force,
	}
	r.log.Info("applying typed kubernetes resource", zap.String("name", prepared.GetName()), zap.String("fieldManager", options.FieldManager), zap.Bool("force", options.Force))
	result := r.newObject()
	err = r.restClient.Patch(types.ApplyPatchType).
		Resource(r.resource).
		Name(prepared.GetName()).
		VersionedParams(&patchOptions, r.parameterCodec).
		Body(prepared).
		SetHeader("Content-Type", string(types.ApplyPatchType)).
		Do(ctx).
		Into(result)
	if err != nil {
		var zero Object
		r.log.Debug("apply typed kubernetes resource failed", zap.String("name", prepared.GetName()), zap.Error(err))
		return zero, fmt.Errorf("apply %s %q: %w", r.resource, prepared.GetName(), err)
	}
	r.stampObject(result)
	r.log.Debug("applied typed kubernetes resource", zap.String("name", result.GetName()), zap.String("resourceVersion", result.GetResourceVersion()))
	return result, nil
}

func (r typedResource[Object, List]) updateStatus(ctx context.Context, object Object, options metav1.UpdateOptions) (Object, error) {
	prepared, err := r.prepare(object)
	if err != nil {
		var zero Object
		return zero, err
	}
	r.log.Info("updating typed kubernetes resource status", zap.String("name", prepared.GetName()), zap.String("resourceVersion", prepared.GetResourceVersion()))
	result := r.newObject()
	err = r.restClient.Put().
		Resource(r.resource).
		Name(prepared.GetName()).
		SubResource("status").
		VersionedParams(&options, r.parameterCodec).
		Body(prepared).
		Do(ctx).
		Into(result)
	if err != nil {
		var zero Object
		r.log.Debug("update typed kubernetes resource status failed", zap.String("name", prepared.GetName()), zap.Error(err))
		return zero, fmt.Errorf("update %s %q status: %w", r.resource, prepared.GetName(), err)
	}
	r.stampObject(result)
	r.log.Debug("updated typed kubernetes resource status", zap.String("name", result.GetName()), zap.String("resourceVersion", result.GetResourceVersion()))
	return result, nil
}

func (r typedResource[Object, List]) delete(ctx context.Context, name string, options metav1.DeleteOptions) error {
	r.log.Info("deleting typed kubernetes resource", zap.String("name", name))
	err := r.restClient.Delete().
		Resource(r.resource).
		Name(name).
		VersionedParams(&options, r.parameterCodec).
		Do(ctx).
		Error()
	if err != nil {
		r.log.Debug("delete typed kubernetes resource failed", zap.String("name", name), zap.Error(err))
		return fmt.Errorf("delete %s %q: %w", r.resource, name, err)
	}
	r.log.Debug("deleted typed kubernetes resource", zap.String("name", name))
	return nil
}

func (r typedResource[Object, List]) watch(ctx context.Context, options ListOptions) (watch.Interface, error) {
	listOptions, err := r.listOptions(options)
	if err != nil {
		return nil, err
	}
	listOptions.Watch = true
	r.log.Info("watching typed kubernetes resources", zap.String("fieldSelector", listOptions.FieldSelector), zap.String("resourceVersion", listOptions.ResourceVersion))
	watcher, err := r.restClient.Get().
		Resource(r.resource).
		VersionedParams(&listOptions, r.parameterCodec).
		Watch(ctx)
	if err != nil {
		r.log.Debug("watch typed kubernetes resources failed", zap.Error(err))
		return nil, fmt.Errorf("watch %s: %w", r.resource, err)
	}
	r.log.Debug("started typed kubernetes resource watch", zap.String("fieldSelector", listOptions.FieldSelector))
	return watcher, nil
}

func (r typedResource[Object, List]) listOptions(options ListOptions) (metav1.ListOptions, error) {
	selector, err := r.fieldSelector(options.Filters)
	if err != nil {
		return metav1.ListOptions{}, err
	}
	return metav1.ListOptions{
		ResourceVersion: options.ResourceVersion,
		FieldSelector:   selector.String(),
		Limit:           options.Limit,
		Continue:        options.Continue,
	}, nil
}

func (r typedResource[Object, List]) fieldSelector(filters []FieldFilter) (fields.Selector, error) {
	if len(filters) == 0 {
		return fields.Everything(), nil
	}
	selectors := make([]fields.Selector, 0, len(filters))
	for _, filter := range filters {
		if _, ok := r.allowed[filter.field]; !ok {
			return nil, fmt.Errorf("field %q is not selectable for %s", filter.field, r.resource)
		}
		selectors = append(selectors, fields.OneTermEqualSelector(string(filter.field), filter.value))
	}
	return fields.AndSelectors(selectors...), nil
}

func (r typedResource[Object, List]) prepare(object Object) (Object, error) {
	if isNilObject(object) {
		var zero Object
		return zero, fmt.Errorf("%s object is required", r.resource)
	}
	copied, ok := object.DeepCopyObject().(Object)
	if !ok {
		var zero Object
		return zero, fmt.Errorf("copy %s object: unexpected object type %T", r.resource, object.DeepCopyObject())
	}
	gvk := schema.GroupVersionKind{
		Group:   nsxv1alpha.GroupName,
		Version: nsxv1alpha.Version,
		Kind:    r.kind,
	}
	copied.GetObjectKind().SetGroupVersionKind(gvk)
	return copied, nil
}

func (r typedResource[Object, List]) stampObject(object Object) {
	object.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
		Group:   nsxv1alpha.GroupName,
		Version: nsxv1alpha.Version,
		Kind:    r.kind,
	})
}

func (r typedResource[Object, List]) stampList(list List) {
	list.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{
		Group:   nsxv1alpha.GroupName,
		Version: nsxv1alpha.Version,
		Kind:    r.kind + "List",
	})
}

func allowedFields(fields ...FieldSelectorField) map[FieldSelectorField]struct{} {
	allowed := make(map[FieldSelectorField]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
	}
	return allowed
}

func isNilObject[Object clientObject](object Object) bool {
	value := reflect.ValueOf(object)
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

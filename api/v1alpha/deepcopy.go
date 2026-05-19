package v1alpha

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (in *NSXNetworkCloud) DeepCopyInto(out *NSXNetworkCloud) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *NSXNetworkCloud) DeepCopy() *NSXNetworkCloud {
	if in == nil {
		return nil
	}
	out := new(NSXNetworkCloud)
	in.DeepCopyInto(out)
	return out
}

func (in *NSXNetworkCloud) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *NSXNetworkCloudList) DeepCopyInto(out *NSXNetworkCloudList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NSXNetworkCloud, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *NSXNetworkCloudList) DeepCopy() *NSXNetworkCloudList {
	if in == nil {
		return nil
	}
	out := new(NSXNetworkCloudList)
	in.DeepCopyInto(out)
	return out
}

func (in *NSXNetworkCloudList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *NSXNetworkCloudStatus) DeepCopyInto(out *NSXNetworkCloudStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

func (in *NSXNetworkCloudSpec) DeepCopyInto(out *NSXNetworkCloudSpec) {
	*out = *in
	if in.WritesEnabled != nil {
		out.WritesEnabled = new(bool)
		*out.WritesEnabled = *in.WritesEnabled
	}
}

func (in *NSXGroup) DeepCopyInto(out *NSXGroup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *NSXGroup) DeepCopy() *NSXGroup {
	if in == nil {
		return nil
	}
	out := new(NSXGroup)
	in.DeepCopyInto(out)
	return out
}

func (in *NSXGroup) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *NSXGroupList) DeepCopyInto(out *NSXGroupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NSXGroup, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *NSXGroupList) DeepCopy() *NSXGroupList {
	if in == nil {
		return nil
	}
	out := new(NSXGroupList)
	in.DeepCopyInto(out)
	return out
}

func (in *NSXGroupList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

func (in *NSXGroupSpec) DeepCopyInto(out *NSXGroupSpec) {
	*out = *in
	if in.CIDRs != nil {
		out.CIDRs = make([]string, len(in.CIDRs))
		copy(out.CIDRs, in.CIDRs)
	}
	if in.SegmentPaths != nil {
		out.SegmentPaths = make([]string, len(in.SegmentPaths))
		copy(out.SegmentPaths, in.SegmentPaths)
	}
}

func (in *NSXGroupStatus) DeepCopyInto(out *NSXGroupStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

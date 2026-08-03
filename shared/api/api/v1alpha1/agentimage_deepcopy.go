// Hand-written DeepCopy methods (controller-gen not available in this repo).

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is a deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentImage) DeepCopyInto(out *AgentImage) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is a deepcopy function, copying the receiver, creating a new AgentImage.
func (in *AgentImage) DeepCopy() *AgentImage {
	if in == nil {
		return nil
	}
	out := new(AgentImage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is a deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AgentImage) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is a deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentImageList) DeepCopyInto(out *AgentImageList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]AgentImage, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is a deepcopy function, copying the receiver, creating a new AgentImageList.
func (in *AgentImageList) DeepCopy() *AgentImageList {
	if in == nil {
		return nil
	}
	out := new(AgentImageList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is a deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AgentImageList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is a deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentImageSpec) DeepCopyInto(out *AgentImageSpec) {
	*out = *in
	if in.Tools != nil {
		in, out := &in.Tools, &out.Tools
		*out = make([]AgentImageTool, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Env != nil {
		in, out := &in.Env, &out.Env
		*out = make([]AgentImageEnvVar, len(*in))
		copy(*out, *in)
	}
	if in.MCPServers != nil {
		in, out := &in.MCPServers, &out.MCPServers
		*out = make([]AgentImageMCPServer, len(*in))
		copy(*out, *in)
	}
	if in.Sidecars != nil {
		in, out := &in.Sidecars, &out.Sidecars
		*out = make([]AgentImageSidecar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.EnabledSkills != nil {
		in, out := &in.EnabledSkills, &out.EnabledSkills
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopyInto is a deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentImageEnvVar) DeepCopyInto(out *AgentImageEnvVar) {
	*out = *in
}

// DeepCopy is a deepcopy function, copying the receiver, creating a new AgentImageEnvVar.
func (in *AgentImageEnvVar) DeepCopy() *AgentImageEnvVar {
	if in == nil {
		return nil
	}
	out := new(AgentImageEnvVar)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is a deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentImageMCPServer) DeepCopyInto(out *AgentImageMCPServer) {
	*out = *in
}

// DeepCopy is a deepcopy function, copying the receiver, creating a new AgentImageMCPServer.
func (in *AgentImageMCPServer) DeepCopy() *AgentImageMCPServer {
	if in == nil {
		return nil
	}
	out := new(AgentImageMCPServer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is a deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentImageSidecar) DeepCopyInto(out *AgentImageSidecar) {
	*out = *in
	if in.Env != nil {
		in, out := &in.Env, &out.Env
		*out = make([]AgentImageEnvVar, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is a deepcopy function, copying the receiver, creating a new AgentImageSidecar.
func (in *AgentImageSidecar) DeepCopy() *AgentImageSidecar {
	if in == nil {
		return nil
	}
	out := new(AgentImageSidecar)
	in.DeepCopyInto(out)
	return out
}

// DeepCopy is a deepcopy function, copying the receiver, creating a new AgentImageSpec.
func (in *AgentImageSpec) DeepCopy() *AgentImageSpec {
	if in == nil {
		return nil
	}
	out := new(AgentImageSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is a deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentImageStatus) DeepCopyInto(out *AgentImageStatus) {
	*out = *in
}

// DeepCopy is a deepcopy function, copying the receiver, creating a new AgentImageStatus.
func (in *AgentImageStatus) DeepCopy() *AgentImageStatus {
	if in == nil {
		return nil
	}
	out := new(AgentImageStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is a deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentImageTool) DeepCopyInto(out *AgentImageTool) {
	*out = *in
	if in.Examples != nil {
		in, out := &in.Examples, &out.Examples
		*out = make([]AgentImageToolExample, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is a deepcopy function, copying the receiver, creating a new AgentImageTool.
func (in *AgentImageTool) DeepCopy() *AgentImageTool {
	if in == nil {
		return nil
	}
	out := new(AgentImageTool)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is a deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AgentImageToolExample) DeepCopyInto(out *AgentImageToolExample) {
	*out = *in
}

// DeepCopy is a deepcopy function, copying the receiver, creating a new AgentImageToolExample.
func (in *AgentImageToolExample) DeepCopy() *AgentImageToolExample {
	if in == nil {
		return nil
	}
	out := new(AgentImageToolExample)
	in.DeepCopyInto(out)
	return out
}

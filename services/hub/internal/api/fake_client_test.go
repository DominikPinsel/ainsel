package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	connectorv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// fakeClient is a minimal in-memory implementation of client.Client for tests.
type fakeClient struct {
	mu      sync.RWMutex
	objects map[string]map[types.NamespacedName]runtime.Object // kind -> nn -> obj
}

func newFakeClient(objs ...runtime.Object) *fakeClient {
	fc := &fakeClient{
		objects: make(map[string]map[types.NamespacedName]runtime.Object),
	}
	for _, obj := range objs {
		_ = fc.Create(context.Background(), obj.(client.Object))
	}
	return fc
}

func kindOf(obj runtime.Object) string {
	switch obj.(type) {
	case *corev1.Secret:
		return "Secret"
	case *corev1.SecretList:
		return "SecretList"
	case *corev1.Pod:
		return "Pod"
	case *corev1.PodList:
		return "PodList"
	case *batchv1.Job:
		return "Job"
	case *batchv1.JobList:
		return "JobList"
	case *connectorv1alpha1.WebhookConnector:
		return "WebhookConnector"
	case *connectorv1alpha1.WebhookConnectorList:
		return "WebhookConnectorList"
	case *connectorv1alpha1.Agent:
		return "Agent"
	case *connectorv1alpha1.AgentList:
		return "AgentList"
	case *connectorv1alpha1.AgentImage:
		return "AgentImage"
	case *connectorv1alpha1.AgentImageList:
		return "AgentImageList"
	default:
		return fmt.Sprintf("%T", obj)
	}
}

func (fc *fakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	kind := kindOf(obj)
	m, ok := fc.objects[kind]
	if !ok {
		return apierrors.NewNotFound(schema.GroupResource{Resource: kind}, key.Name)
	}
	stored, ok := m[key]
	if !ok {
		return apierrors.NewNotFound(schema.GroupResource{Resource: kind}, key.Name)
	}
	// Copy via JSON round-trip
	data, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, obj)
}

func (fc *fakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	// Build combined opts
	lo := &client.ListOptions{}
	for _, o := range opts {
		o.ApplyToList(lo)
	}

	switch l := list.(type) {
	case *corev1.SecretList:
		m := fc.objects["Secret"]
		var items []corev1.Secret
		for nn, obj := range m {
			if lo.Namespace != "" && nn.Namespace != lo.Namespace {
				continue
			}
			sec := obj.(*corev1.Secret)
			if lo.LabelSelector != nil && !lo.LabelSelector.Matches(labels.Set(sec.Labels)) {
				continue
			}
			items = append(items, *sec)
		}
		l.Items = items
	case *connectorv1alpha1.WebhookConnectorList:
		m := fc.objects["WebhookConnector"]
		var items []connectorv1alpha1.WebhookConnector
		for nn, obj := range m {
			if lo.Namespace != "" && nn.Namespace != lo.Namespace {
				continue
			}
			c := obj.(*connectorv1alpha1.WebhookConnector)
			if lo.LabelSelector != nil && !lo.LabelSelector.Matches(labels.Set(c.Labels)) {
				continue
			}
			items = append(items, *c)
		}
		l.Items = items
	case *connectorv1alpha1.AgentList:
		m := fc.objects["Agent"]
		var items []connectorv1alpha1.Agent
		for nn, obj := range m {
			if lo.Namespace != "" && nn.Namespace != lo.Namespace {
				continue
			}
			a := obj.(*connectorv1alpha1.Agent)
			if lo.LabelSelector != nil && !lo.LabelSelector.Matches(labels.Set(a.Labels)) {
				continue
			}
			items = append(items, *a)
		}
		l.Items = items
	case *connectorv1alpha1.AgentImageList:
		m := fc.objects["AgentImage"]
		var items []connectorv1alpha1.AgentImage
		for nn, obj := range m {
			if lo.Namespace != "" && nn.Namespace != lo.Namespace {
				continue
			}
			img := obj.(*connectorv1alpha1.AgentImage)
			if lo.LabelSelector != nil && !lo.LabelSelector.Matches(labels.Set(img.Labels)) {
				continue
			}
			items = append(items, *img)
		}
		l.Items = items
	case *batchv1.JobList:
		m := fc.objects["Job"]
		var items []batchv1.Job
		for nn, obj := range m {
			if lo.Namespace != "" && nn.Namespace != lo.Namespace {
				continue
			}
			j := obj.(*batchv1.Job)
			if lo.LabelSelector != nil && !lo.LabelSelector.Matches(labels.Set(j.Labels)) {
				continue
			}
			items = append(items, *j)
		}
		l.Items = items
	case *corev1.PodList:
		m := fc.objects["Pod"]
		var items []corev1.Pod
		for nn, obj := range m {
			if lo.Namespace != "" && nn.Namespace != lo.Namespace {
				continue
			}
			p := obj.(*corev1.Pod)
			if lo.LabelSelector != nil && !lo.LabelSelector.Matches(labels.Set(p.Labels)) {
				continue
			}
			items = append(items, *p)
		}
		l.Items = items
	}
	return nil
}

func (fc *fakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	kind := kindOf(obj)
	if fc.objects[kind] == nil {
		fc.objects[kind] = make(map[types.NamespacedName]runtime.Object)
	}
	// If the object uses GenerateName with no explicit Name, assign a unique name.
	name := obj.GetName()
	if name == "" && obj.GetGenerateName() != "" {
		name = fmt.Sprintf("%s%d", obj.GetGenerateName(), len(fc.objects[kind]))
		obj.SetName(name)
	}
	nn := types.NamespacedName{Name: name, Namespace: obj.GetNamespace()}
	if _, exists := fc.objects[kind][nn]; exists {
		return apierrors.NewAlreadyExists(schema.GroupResource{Resource: kind}, name)
	}
	// deep copy via JSON
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	copy := obj.DeepCopyObject()
	if err := json.Unmarshal(data, copy); err != nil {
		return err
	}
	fc.objects[kind][nn] = copy
	return nil
}

func (fc *fakeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	kind := kindOf(obj)
	if fc.objects[kind] == nil {
		return apierrors.NewNotFound(schema.GroupResource{Resource: kind}, obj.GetName())
	}
	nn := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	if _, exists := fc.objects[kind][nn]; !exists {
		return apierrors.NewNotFound(schema.GroupResource{Resource: kind}, obj.GetName())
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	copy := obj.DeepCopyObject()
	if err := json.Unmarshal(data, copy); err != nil {
		return err
	}
	fc.objects[kind][nn] = copy
	return nil
}

func (fc *fakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	kind := kindOf(obj)
	m, ok := fc.objects[kind]
	if !ok {
		return apierrors.NewNotFound(schema.GroupResource{Resource: kind}, obj.GetName())
	}
	nn := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	if _, exists := m[nn]; !exists {
		return apierrors.NewNotFound(schema.GroupResource{Resource: kind}, obj.GetName())
	}
	delete(m, nn)
	return nil
}

func (fc *fakeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return fmt.Errorf("not implemented")
}

func (fc *fakeClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	return fmt.Errorf("not implemented")
}

func (fc *fakeClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return fmt.Errorf("not implemented")
}

func (fc *fakeClient) Status() client.SubResourceWriter {
	return &fakeSubResource{parent: fc}
}

func (fc *fakeClient) SubResource(subResource string) client.SubResourceClient {
	return &fakeSubResource{parent: fc}
}

func (fc *fakeClient) Scheme() *runtime.Scheme {
	return runtime.NewScheme()
}

func (fc *fakeClient) RESTMapper() meta.RESTMapper {
	return nil
}

func (fc *fakeClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}

func (fc *fakeClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return true, nil
}

type fakeSubResource struct {
	parent *fakeClient
}

func (f *fakeSubResource) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return fmt.Errorf("not implemented")
}

func (f *fakeSubResource) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return fmt.Errorf("not implemented")
}

// Update delegates to the parent fakeClient so Status().Update() works in tests.
func (f *fakeSubResource) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if f.parent != nil {
		return f.parent.Update(ctx, obj)
	}
	return fmt.Errorf("not implemented")
}

func (f *fakeSubResource) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return fmt.Errorf("not implemented")
}

func (f *fakeSubResource) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
	return fmt.Errorf("not implemented")
}

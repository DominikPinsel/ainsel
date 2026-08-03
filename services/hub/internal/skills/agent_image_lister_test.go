package skills

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newUnstructuredAgentImage builds an unstructured AgentImage CR with the given
// name, namespace, and enabledSkills.
func newUnstructuredAgentImage(name, namespace string, enabledSkills []string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentImage",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetResourceVersion("1")
	if enabledSkills != nil {
		_ = unstructured.SetNestedStringSlice(obj.Object, enabledSkills, "spec", "enabledSkills")
	}
	return obj
}

func TestKubeAgentImageLister_AssignIdempotent(t *testing.T) {
	ctx := context.Background()
	ns := "test-ns"

	// Image already has "my-skill" in enabledSkills.
	existing := newUnstructuredAgentImage("img-already", ns, []string{"my-skill", "other-skill"})

	scheme := runtime.NewScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing)
	client := builder.Build()

	lister := &kubeAgentImageLister{client: client, namespace: ns}

	// Assign should be a no-op (skill already present).
	if err := lister.Assign(ctx, "my-skill", "img-already"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	// Verify the CR was NOT updated (resourceVersion unchanged).
	var after unstructured.Unstructured
	after.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentImage",
	})
	if err := client.Get(ctx, types.NamespacedName{Name: existing.GetName(), Namespace: existing.GetNamespace()}, &after); err != nil {
		t.Fatalf("Get after: %v", err)
	}
	if after.GetResourceVersion() != "1" {
		t.Errorf("expected resourceVersion unchanged (no Update call), got %q", after.GetResourceVersion())
	}

	// Verify skills are unchanged.
	skills, _, _ := unstructured.NestedStringSlice(after.Object, "spec", "enabledSkills")
	if len(skills) != 2 || skills[0] != "my-skill" || skills[1] != "other-skill" {
		t.Errorf("expected skills unchanged, got %v", skills)
	}
}

func TestKubeAgentImageLister_AssignAddsNewSkill(t *testing.T) {
	ctx := context.Background()
	ns := "test-ns"

	existing := newUnstructuredAgentImage("img-add", ns, []string{"other-skill"})

	scheme := runtime.NewScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing)
	client := builder.Build()

	lister := &kubeAgentImageLister{client: client, namespace: ns}

	if err := lister.Assign(ctx, "new-skill", "img-add"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	var after unstructured.Unstructured
	after.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentImage",
	})
	if err := client.Get(ctx, types.NamespacedName{Name: existing.GetName(), Namespace: existing.GetNamespace()}, &after); err != nil {
		t.Fatalf("Get after: %v", err)
	}

	skills, _, _ := unstructured.NestedStringSlice(after.Object, "spec", "enabledSkills")
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %v", skills)
	}
	found := false
	for _, s := range skills {
		if s == "new-skill" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'new-skill' in skills, got %v", skills)
	}
}

func TestKubeAgentImageLister_UnassignIdempotent(t *testing.T) {
	ctx := context.Background()
	ns := "test-ns"

	// Image does NOT have "absent-skill" in enabledSkills.
	existing := newUnstructuredAgentImage("img-no-skill", ns, []string{"other-skill"})

	scheme := runtime.NewScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing)
	client := builder.Build()

	lister := &kubeAgentImageLister{client: client, namespace: ns}

	// Unassign should be a no-op (skill not present).
	if err := lister.Unassign(ctx, "absent-skill", "img-no-skill"); err != nil {
		t.Fatalf("Unassign: %v", err)
	}

	// Verify the CR was NOT updated.
	var after unstructured.Unstructured
	after.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentImage",
	})
	if err := client.Get(ctx, types.NamespacedName{Name: existing.GetName(), Namespace: existing.GetNamespace()}, &after); err != nil {
		t.Fatalf("Get after: %v", err)
	}
	if after.GetResourceVersion() != "1" {
		t.Errorf("expected resourceVersion unchanged (no Update call), got %q", after.GetResourceVersion())
	}
}

func TestKubeAgentImageLister_UnassignRemovesSkill(t *testing.T) {
	ctx := context.Background()
	ns := "test-ns"

	existing := newUnstructuredAgentImage("img-rm", ns, []string{"my-skill", "other-skill"})

	scheme := runtime.NewScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing)
	client := builder.Build()

	lister := &kubeAgentImageLister{client: client, namespace: ns}

	if err := lister.Unassign(ctx, "my-skill", "img-rm"); err != nil {
		t.Fatalf("Unassign: %v", err)
	}

	var after unstructured.Unstructured
	after.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentImage",
	})
	if err := client.Get(ctx, types.NamespacedName{Name: existing.GetName(), Namespace: existing.GetNamespace()}, &after); err != nil {
		t.Fatalf("Get after: %v", err)
	}

	skills, _, _ := unstructured.NestedStringSlice(after.Object, "spec", "enabledSkills")
	if len(skills) != 1 || skills[0] != "other-skill" {
		t.Errorf("expected [other-skill], got %v", skills)
	}
}

func TestKubeAgentImageLister_AssignToEmptySkills(t *testing.T) {
	ctx := context.Background()
	ns := "test-ns"

	// Image has no enabledSkills field at all.
	existing := newUnstructuredAgentImage("img-empty", ns, nil)

	scheme := runtime.NewScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing)
	client := builder.Build()

	lister := &kubeAgentImageLister{client: client, namespace: ns}

	if err := lister.Assign(ctx, "new-skill", "img-empty"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	var after unstructured.Unstructured
	after.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentImage",
	})
	if err := client.Get(ctx, types.NamespacedName{Name: existing.GetName(), Namespace: existing.GetNamespace()}, &after); err != nil {
		t.Fatalf("Get after: %v", err)
	}

	skills, _, _ := unstructured.NestedStringSlice(after.Object, "spec", "enabledSkills")
	if len(skills) != 1 || skills[0] != "new-skill" {
		t.Errorf("expected [new-skill], got %v", skills)
	}
}

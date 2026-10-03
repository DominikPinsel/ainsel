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

// newUnstructuredAgent builds an unstructured Agent CR. A nil skillList
// leaves spec.skills unset, which means "inherit the image's enabledSkills".
func newUnstructuredAgent(name, namespace string, skillList []string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "ainsel.dev", Version: "v1alpha1", Kind: "Agent",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetResourceVersion("1")
	if skillList != nil {
		_ = unstructured.SetNestedStringSlice(obj.Object, skillList, "spec", "skills", "items")
	}
	return obj
}

// TestEnabledSkillIDsIncludesAgentLevelSelection pins the difference
// between the two tallies. An Agent's spec.skills.items replaces its
// image's enabledSkills for that agent, so a skill chosen only on the
// Agent is still mounted by a real pod. Driving the ConfigMap prune off
// UsageCounts alone would drop that key out from under the running agent.
func TestEnabledSkillIDsIncludesAgentLevelSelection(t *testing.T) {
	ctx := context.Background()
	ns := "test-ns"

	img := newUnstructuredAgentImage("img-1", ns, []string{"image-skill", "shared-skill"})
	// Narrows to its own list; "image-skill" is not in it.
	agentNarrow := newUnstructuredAgent("agent-narrow", ns, []string{"agent-only-skill", "shared-skill"})
	// No explicit selection: inherits img-1, already counted via the image.
	agentInherit := newUnstructuredAgent("agent-inherit", ns, nil)

	scheme := runtime.NewScheme()
	kc := fake.NewClientBuilder().WithScheme(scheme).
		WithRuntimeObjects(img, agentNarrow, agentInherit).Build()
	lister := &kubeAgentImageLister{client: kc, namespace: ns}

	usage, err := lister.UsageCounts(ctx)
	if err != nil {
		t.Fatalf("UsageCounts: %v", err)
	}
	if _, ok := usage["agent-only-skill"]; ok {
		t.Fatalf("UsageCounts should not see the agent-level skill; that is the point of the test: %v", usage)
	}

	enabled, err := lister.EnabledSkillIDs(ctx)
	if err != nil {
		t.Fatalf("EnabledSkillIDs: %v", err)
	}
	for _, want := range []string{"image-skill", "shared-skill", "agent-only-skill"} {
		if enabled[want] < 1 {
			t.Errorf("EnabledSkillIDs missing %q, got %v", want, enabled)
		}
	}
	// Each id is counted once per referencing object, not once per entry.
	if enabled["shared-skill"] != 2 {
		t.Errorf("shared-skill = %d, want 2 (image + agent)", enabled["shared-skill"])
	}
}

// TestEnabledSkillIDsDeduplicatesWithinAnObject checks the per-object
// dedupe: a CR that repeats an id must not inflate its count.
func TestEnabledSkillIDsDeduplicatesWithinAnObject(t *testing.T) {
	ctx := context.Background()
	ns := "test-ns"
	img := newUnstructuredAgentImage("img-dup", ns, []string{"s", "s", "s"})
	kc := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithRuntimeObjects(img).Build()
	lister := &kubeAgentImageLister{client: kc, namespace: ns}

	enabled, err := lister.EnabledSkillIDs(ctx)
	if err != nil {
		t.Fatalf("EnabledSkillIDs: %v", err)
	}
	if enabled["s"] != 1 {
		t.Errorf("s = %d, want 1 for a single image listing it repeatedly", enabled["s"])
	}
}

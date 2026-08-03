package personas_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
)

// agentGVK is the GVK of the Agent CR the hub already manages.
var agentGVK = schema.GroupVersionKind{Group: "ainsel.dev", Version: "v1alpha1", Kind: "Agent"}

// agentListGVK is the GVK of the list type for the fake client.
var agentListGVK = schema.GroupVersionKind{Group: "ainsel.dev", Version: "v1alpha1", Kind: "AgentList"}

func newAgentListerClient(t *testing.T, agents ...*unstructured.Unstructured) personas.AgentLister {
	t.Helper()
	scheme := runtime.NewScheme()
	// The fake client builder needs to know the kind -> list-kind mapping for
	// any unstructured List call.
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(nil)
	// Register list kind for unstructured Agent so List(...) works.
	for _, a := range agents {
		builder = builder.WithObjects(a)
	}
	kc := builder.Build()
	return personas.NewAgentLister(kc, "ainsel-test")
}

func TestAgentListerEmpty(t *testing.T) {
	lister := newAgentListerClient(t)
	refs, err := lister.ListReferrers(context.Background(), "01XPERSONA")
	if err != nil {
		t.Fatalf("ListReferrers: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected no referrers, got %+v", refs)
	}
}

func TestAgentListerMatchesByPersonaID(t *testing.T) {
	agent := &unstructured.Unstructured{}
	agent.SetGroupVersionKind(agentGVK)
	agent.SetName("code-reviewer-agent")
	agent.SetNamespace("ainsel-test")
	_ = unstructured.SetNestedField(agent.Object, "01XPERSONA", "spec", "persona", "id")

	other := &unstructured.Unstructured{}
	other.SetGroupVersionKind(agentGVK)
	other.SetName("other-agent")
	other.SetNamespace("ainsel-test")
	_ = unstructured.SetNestedField(other.Object, "DIFFERENT", "spec", "persona", "id")

	lister := newAgentListerClient(t, agent, other)
	refs, err := lister.ListReferrers(context.Background(), "01XPERSONA")
	if err != nil {
		t.Fatalf("ListReferrers: %v", err)
	}
	if len(refs) != 1 || refs[0].AgentName != "code-reviewer-agent" {
		t.Errorf("expected one referrer named code-reviewer-agent, got %+v", refs)
	}
}

// Sanity: keep the agentListGVK reference live in case a future refactor
// wants to assert the list kind name explicitly.
var _ = agentListGVK

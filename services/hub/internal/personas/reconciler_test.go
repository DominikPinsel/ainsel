package personas_test

import (
	"context"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
)

func newTestReconciler(t *testing.T) *personas.Reconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	kc := fake.NewClientBuilder().WithScheme(scheme).Build()
	return personas.NewReconciler(kc, "ainsel-test")
}

func TestReconcilerEnsureCreatesConfigMap(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)
	p := personas.Persona{
		ID:             "01HXP",
		Name:           "code-reviewer",
		CurrentVersion: 1,
		Text:           "You are a code reviewer.",
	}
	if err := r.Ensure(ctx, &p); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	var cm corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: "persona-01HXP", Namespace: "ainsel-test"}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if got := cm.Data["persona.md"]; got != "You are a code reviewer." {
		t.Errorf("persona.md content mismatch: %q", got)
	}
	if cm.Labels["ainsel.dev/managed-by"] != "hub" {
		t.Errorf("missing managed-by label, got %v", cm.Labels)
	}
	if cm.Labels["ainsel.dev/resource"] != "persona" {
		t.Errorf("missing resource label, got %v", cm.Labels)
	}
	if cm.Annotations["ainsel.dev/persona-name"] != "code-reviewer" {
		t.Errorf("missing persona-name annotation, got %v", cm.Annotations)
	}
	if cm.Annotations["ainsel.dev/persona-version"] != strconv.Itoa(1) {
		t.Errorf("persona-version annotation = %q", cm.Annotations["ainsel.dev/persona-version"])
	}
}

func TestReconcilerEnsureUpdatesConfigMap(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)
	if err := r.Ensure(ctx, &personas.Persona{ID: "01HXP", Name: "p", CurrentVersion: 1, Text: "v1"}); err != nil {
		t.Fatalf("Ensure v1: %v", err)
	}
	if err := r.Ensure(ctx, &personas.Persona{ID: "01HXP", Name: "p", CurrentVersion: 2, Text: "v2"}); err != nil {
		t.Fatalf("Ensure v2: %v", err)
	}
	var cm corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: "persona-01HXP", Namespace: "ainsel-test"}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if cm.Data["persona.md"] != "v2" {
		t.Errorf("expected v2, got %q", cm.Data["persona.md"])
	}
	if cm.Annotations["ainsel.dev/persona-version"] != "2" {
		t.Errorf("expected version annotation 2, got %q", cm.Annotations["ainsel.dev/persona-version"])
	}
}

func TestReconcilerDelete(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)
	if err := r.Ensure(ctx, &personas.Persona{ID: "01HXP", Name: "p", CurrentVersion: 1, Text: "x"}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := r.Delete(ctx, "01HXP"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var cm corev1.ConfigMap
	err := r.Client().Get(ctx, types.NamespacedName{Name: "persona-01HXP", Namespace: "ainsel-test"}, &cm)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestReconcilerDeleteMissingIsOK(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)
	if err := r.Delete(ctx, "01NOPE"); err != nil {
		t.Errorf("expected nil on missing, got %v", err)
	}
}

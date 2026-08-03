package skills_test

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sharedskills "github.com/DominikPinsel/ainsel/shared/api/skills"
	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
)

const testNamespace = "ainsel-test"

func newTestReconciler(t *testing.T, seedObjects ...runtime.Object) *skills.Reconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	builder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(seedObjects...)
	return skills.NewReconciler(builder.Build(), testNamespace)
}

func TestReconcilerEnsureCreatesSharedConfigMap(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)

	sk := &skills.Skill{ID: "code-review", Description: "Reviews PRs", Body: "Body."}
	if err := r.Ensure(ctx, sk); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	var cm corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if cm.Labels["ainsel.dev/managed-by"] != "hub" || cm.Labels["ainsel.dev/resource"] != "skills" {
		t.Errorf("labels missing: %+v", cm.Labels)
	}
	got, ok := cm.Data["code-review"]
	if !ok {
		t.Fatalf("expected key 'code-review' in data, got keys %v", keys(cm.Data))
	}
	if !strings.Contains(got, "name: code-review") || !strings.Contains(got, "description: Reviews PRs") || !strings.HasSuffix(got, "Body.") {
		t.Errorf("assembled SKILL.md mismatch: %q", got)
	}
}

func TestReconcilerEnsureAppendsToExistingConfigMap(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)

	if err := r.Ensure(ctx, &skills.Skill{ID: "a", Description: "da", Body: "ba"}); err != nil {
		t.Fatalf("Ensure a: %v", err)
	}
	if err := r.Ensure(ctx, &skills.Skill{ID: "b", Description: "db", Body: "bb"}); err != nil {
		t.Fatalf("Ensure b: %v", err)
	}

	var cm corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if _, ok := cm.Data["a"]; !ok {
		t.Errorf("missing key 'a'; data keys: %v", keys(cm.Data))
	}
	if _, ok := cm.Data["b"]; !ok {
		t.Errorf("missing key 'b'; data keys: %v", keys(cm.Data))
	}
}

func TestReconcilerEnsureOverwritesSameID(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)

	if err := r.Ensure(ctx, &skills.Skill{ID: "x", Description: "v1", Body: "old"}); err != nil {
		t.Fatalf("Ensure v1: %v", err)
	}
	if err := r.Ensure(ctx, &skills.Skill{ID: "x", Description: "v2", Body: "new"}); err != nil {
		t.Fatalf("Ensure v2: %v", err)
	}
	var cm corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if !strings.Contains(cm.Data["x"], "description: v2") || !strings.HasSuffix(cm.Data["x"], "new") {
		t.Errorf("expected v2 contents, got %q", cm.Data["x"])
	}
}

// TestReconcilerEnsureHandlesAlreadyExists exercises the race-handling path:
// when the shared ConfigMap exists before Ensure runs (simulating a concurrent
// create), Ensure should fall through to the update branch instead of failing.
func TestReconcilerEnsureHandlesAlreadyExists(t *testing.T) {
	ctx := context.Background()
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sharedskills.ConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{"pre-existing": "stale"},
	}
	r := newTestReconciler(t, existing)

	if err := r.Ensure(ctx, &skills.Skill{ID: "new-one", Description: "d", Body: "b"}); err != nil {
		t.Fatalf("Ensure on existing cm: %v", err)
	}
	var cm corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if _, ok := cm.Data["new-one"]; !ok {
		t.Errorf("expected new key added, got keys %v", keys(cm.Data))
	}
	if _, ok := cm.Data["pre-existing"]; !ok {
		t.Errorf("expected pre-existing key preserved, got keys %v", keys(cm.Data))
	}
}

func TestReconcilerDeleteRemovesKey(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)

	if err := r.Ensure(ctx, &skills.Skill{ID: "a", Body: "ba"}); err != nil {
		t.Fatalf("Ensure a: %v", err)
	}
	if err := r.Ensure(ctx, &skills.Skill{ID: "b", Body: "bb"}); err != nil {
		t.Fatalf("Ensure b: %v", err)
	}
	if err := r.Delete(ctx, "a"); err != nil {
		t.Fatalf("Delete a: %v", err)
	}

	var cm corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if _, ok := cm.Data["a"]; ok {
		t.Errorf("expected key 'a' removed, got keys %v", keys(cm.Data))
	}
	if _, ok := cm.Data["b"]; !ok {
		t.Errorf("expected key 'b' preserved, got keys %v", keys(cm.Data))
	}
}

func TestReconcilerDeleteMissingCMIsNoop(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)

	if err := r.Delete(ctx, "nope"); err != nil {
		t.Errorf("expected no error when CM is missing, got %v", err)
	}
	// And no ConfigMap was created as a side effect.
	var cm corev1.ConfigMap
	err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

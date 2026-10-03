package skills_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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
	if !strings.Contains(got, "name: code-review") || !strings.Contains(got, `description: "Reviews PRs"`) || !strings.HasSuffix(got, "Body.") {
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
	if !strings.Contains(cm.Data["x"], `description: "v2"`) || !strings.HasSuffix(cm.Data["x"], "new") {
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

// frontmatter parses the leading YAML block of a rendered SKILL.md the
// same way the agent runtime does, so these tests fail on the actual
// contract (valid, lossless metadata) rather than on a string shape.
func frontmatter(t *testing.T, skillMD string) map[string]any {
	t.Helper()
	const delim = "---\n"
	if !strings.HasPrefix(skillMD, delim) {
		t.Fatalf("SKILL.md does not start with a frontmatter delimiter: %q", skillMD)
	}
	rest := skillMD[len(delim):]
	idx := strings.Index(rest, "\n"+delim)
	if idx < 0 {
		t.Fatalf("SKILL.md has no closing frontmatter delimiter: %q", skillMD)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(rest[:idx]), &parsed); err != nil {
		t.Fatalf("frontmatter is not valid YAML: %v\n---\n%s", err, rest[:idx])
	}
	return parsed
}

func TestReconcilerEnsureRendersParsableFrontmatter(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name        string
		description string
	}{
		{"plain", "Reviews PRs"},
		// The regression this pins: a ": " in a description made the
		// plain scalar an invalid mapping, which cost the skill its
		// metadata. Most skill descriptions are written this way.
		{"colon space", "Read pull requests: metadata, commits, files, diffs"},
		{"leading dash", "- not a list item"},
		{"embedded quotes", `He said "ship it", then left`},
		{"backslash", `paths like C:\skills`},
		{"trailing colon", "Coordinates tools through a hierarchy:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestReconciler(t)
			sk := &skills.Skill{ID: "s", Description: tc.description, Body: "body"}
			if err := r.Ensure(ctx, sk); err != nil {
				t.Fatalf("Ensure: %v", err)
			}
			var cm corev1.ConfigMap
			if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
				t.Fatalf("get cm: %v", err)
			}
			fm := frontmatter(t, cm.Data["s"])
			if fm["name"] != "s" {
				t.Errorf("name = %v, want %q", fm["name"], "s")
			}
			if fm["description"] != tc.description {
				t.Errorf("description round-trip = %q, want %q", fm["description"], tc.description)
			}
		})
	}
}

func TestReconcilerConvergePrunesUnenabledAndDeliversEnabled(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)

	// Simulate a registry that mirrored everything: catalogue skills no
	// agent enables sit in the shared object alongside the hot set.
	for _, id := range []string{"hot-1", "hot-2", "catalogue-1", "catalogue-2"} {
		if err := r.Ensure(ctx, &skills.Skill{ID: id, Description: "d " + id, Body: "b " + id}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	keep := map[string]*skills.Skill{
		"hot-1": {ID: "hot-1", Description: "d hot-1", Body: "b hot-1"},
		"hot-2": {ID: "hot-2", Description: "rewritten body", Body: "b hot-2 updated"},
		"new-1": {ID: "new-1", Description: "just enabled", Body: "b new-1"},
	}
	delivered, undelivered, err := r.Converge(ctx, keep)
	if err != nil {
		t.Fatalf("Converge: %v", err)
	}
	if len(undelivered) != 0 {
		t.Fatalf("undelivered = %v, want none", undelivered)
	}

	var cm corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	got := map[string]bool{}
	for k := range cm.Data {
		got[k] = true
	}
	for want := range keep {
		if !got[want] {
			t.Errorf("expected key %q after converge, keys = %v", want, keys(cm.Data))
		}
	}
	for _, gone := range []string{"catalogue-1", "catalogue-2"} {
		if got[gone] {
			t.Errorf("unenabled key %q should have been pruned", gone)
		}
	}
	// An enabled skill whose body changed must be re-rendered, not left
	// at the stale value just because the key already existed.
	if !strings.Contains(cm.Data["hot-2"], "b hot-2 updated") {
		t.Errorf("changed skill not re-rendered: %q", cm.Data["hot-2"])
	}
	if len(delivered) != len(keep) {
		t.Errorf("delivered = %v, want %d ids", delivered, len(keep))
	}
}

func TestReconcilerConvergeIsIdempotentAndPrunesToEmpty(t *testing.T) {
	ctx := context.Background()
	r := newTestReconciler(t)
	if err := r.Ensure(ctx, &skills.Skill{ID: "only", Description: "d", Body: "b"}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	first, undelivered, err := r.Converge(ctx, map[string]*skills.Skill{
		"only": {ID: "only", Description: "d", Body: "b"},
	})
	if err != nil || len(undelivered) != 0 || len(first) != 1 {
		t.Fatalf("first converge: delivered=%v undelivered=%v err=%v", first, undelivered, err)
	}

	var before corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &before); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if _, again, err := r.Converge(ctx, map[string]*skills.Skill{
		"only": {ID: "only", Description: "d", Body: "b"},
	}); err != nil || len(again) != 0 {
		t.Fatalf("second converge should change nothing, got err=%v undelivered=%v", err, again)
	}
	var after corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &after); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if before.ResourceVersion != after.ResourceVersion {
		t.Errorf("no-op converge bumped the object (%s -> %s); agents would restart for nothing",
			before.ResourceVersion, after.ResourceVersion)
	}

	// Empty keep set: every managed key goes, the object itself stays.
	if _, _, err := r.Converge(ctx, nil); err != nil {
		t.Fatalf("converge to empty: %v", err)
	}
	var emptied corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &emptied); err != nil {
		t.Fatalf("get cm after emptying: %v", err)
	}
	if len(emptied.Data) != 0 {
		t.Errorf("expected no keys, got %v", keys(emptied.Data))
	}
}

// TestReconcilerConvergeReportsUndeliverableSkills exercises the ceiling
// itself: the fake apiserver rejects writes that would push the shared
// object past a cap, the way the real one rejects anything over 1 MiB.
func TestReconcilerConvergeReportsUndeliverableSkills(t *testing.T) {
	ctx := context.Background()
	const capBytes = 600 // total data the fake object will accept

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	// tooBig mimics the apiserver's ceiling check on the whole object.
	tooBig := func(cm *corev1.ConfigMap) error {
		total := 0
		for _, v := range cm.Data {
			total += len(v)
		}
		if total > capBytes {
			return fmt.Errorf(
				"ConfigMap %q is invalid: []: Too long: may not be more than %d bytes",
				sharedskills.ConfigMapName, capBytes)
		}
		return nil
	}
	kc := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if cm, ok := obj.(*corev1.ConfigMap); ok {
					if err := tooBig(cm); err != nil {
						return err
					}
				}
				return c.Create(ctx, obj, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if cm, ok := obj.(*corev1.ConfigMap); ok {
					if err := tooBig(cm); err != nil {
						return err
					}
				}
				return c.Update(ctx, obj, opts...)
			},
		}).Build()
	r := skills.NewReconciler(kc, testNamespace)

	filler := strings.Repeat("x", capBytes*2)
	keep := map[string]*skills.Skill{
		"small-1": {ID: "small-1", Description: "fits", Body: "a"},
		"small-2": {ID: "small-2", Description: "fits", Body: "b"},
		"huge":    {ID: "huge", Description: "does not fit", Body: filler},
	}
	delivered, undelivered, err := r.Converge(ctx, keep)
	if err != nil {
		t.Fatalf("Converge must not fail the pass over one undeliverable skill, got %v", err)
	}
	if _, bad := undelivered["huge"]; !bad {
		t.Errorf("expected 'huge' to be reported undelivered, got delivered=%v", delivered)
	}
	if len(undelivered) != 1 {
		t.Errorf("undelivered = %v, want exactly 'huge'", undelivered)
	}

	var cm corev1.ConfigMap
	if err := r.Client().Get(ctx, types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	// The skills that fit must have been delivered despite one failing.
	for _, id := range []string{"small-1", "small-2"} {
		if _, ok := cm.Data[id]; !ok {
			t.Errorf("skill %q that fits should have been delivered, keys = %v", id, keys(cm.Data))
		}
	}
	if _, ok := cm.Data["huge"]; ok {
		t.Error("oversized skill must not be present in the object")
	}
}

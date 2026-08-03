package skills_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sharedskills "github.com/DominikPinsel/ainsel/shared/api/skills"
	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
)

// stubImageLister returns configured referrers from ListReferrers calls.
type stubImageLister struct {
	refs   []skills.Referrer
	err    error
	counts map[string]int

	// Track Assign/Unassign calls for delegation tests.
	assignCalls   []assignCall
	unassignCalls []assignCall
	assignErr     error
	unassignErr   error
}

type assignCall struct {
	skillID        string
	agentImageName string
}

func (s *stubImageLister) ListReferrers(ctx context.Context, skillID string) ([]skills.Referrer, error) {
	return s.refs, s.err
}

func (s *stubImageLister) UsageCounts(ctx context.Context) (map[string]int, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.counts, nil
}

func (s *stubImageLister) Assign(_ context.Context, skillID, agentImageName string) error {
	s.assignCalls = append(s.assignCalls, assignCall{skillID: skillID, agentImageName: agentImageName})
	return s.assignErr
}

func (s *stubImageLister) Unassign(_ context.Context, skillID, agentImageName string) error {
	s.unassignCalls = append(s.unassignCalls, assignCall{skillID: skillID, agentImageName: agentImageName})
	return s.unassignErr
}

func newTestService(t *testing.T, lister skills.AgentImageLister) (*skills.Service, func()) {
	t.Helper()
	store, cleanup := newTestStore(t)

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	kc := fake.NewClientBuilder().WithScheme(scheme).Build()
	rec := skills.NewReconciler(kc, testNamespace)

	if lister == nil {
		lister = &stubImageLister{}
	}
	svc := skills.NewService(store, rec, lister)
	return svc, cleanup
}

func TestServiceCreateRendersConfigMapEntry(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, nil)
	defer cleanup()

	sk, err := svc.Create(ctx, skills.CreateRequest{
		ID:          "code-review",
		Name:        "Code Review",
		Description: "Reviews PRs",
		Body:        "Use when X.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sk.ID != "code-review" {
		t.Errorf("expected id preserved, got %q", sk.ID)
	}

	var cm corev1.ConfigMap
	if err := svc.Reconciler().Client().Get(ctx,
		types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("expected ConfigMap rendered: %v", err)
	}
	if !strings.Contains(cm.Data["code-review"], "Use when X.") {
		t.Errorf("body missing from rendered SKILL.md: %q", cm.Data["code-review"])
	}
}

func TestServiceCreateRejectsDuplicateID(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, nil)
	defer cleanup()

	if _, err := svc.Create(ctx, skills.CreateRequest{ID: "dup", Name: "first"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(ctx, skills.CreateRequest{ID: "dup", Name: "second"})
	if !errors.Is(err, skills.ErrIDTaken) {
		t.Errorf("expected ErrIDTaken, got %v", err)
	}
}

func TestServiceValidation(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, nil)
	defer cleanup()

	cases := []struct {
		name      string
		req       skills.CreateRequest
		wantField string
	}{
		{"empty id", skills.CreateRequest{ID: "", Name: "n"}, "id"},
		{"upper-case id", skills.CreateRequest{ID: "BadID", Name: "n"}, "id"},
		{"leading hyphen id", skills.CreateRequest{ID: "-foo", Name: "n"}, "id"},
		{"consecutive hyphens id", skills.CreateRequest{ID: "foo--bar", Name: "n"}, "id"},
		{"empty name", skills.CreateRequest{ID: "ok-id", Name: ""}, "name"},
		{"name too long", skills.CreateRequest{ID: "ok-id", Name: strings.Repeat("a", 201)}, "name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.Create(ctx, c.req)
			if err == nil {
				t.Fatal("expected validation error")
			}
			var verr *skills.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected *ValidationError, got %T: %v", err, err)
			}
			if verr.Field != c.wantField {
				t.Errorf("expected field %q, got %q", c.wantField, verr.Field)
			}
		})
	}
}

func TestServiceUpdateRerendersConfigMap(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, nil)
	defer cleanup()

	sk, err := svc.Create(ctx, skills.CreateRequest{ID: "u", Name: "u", Body: "v1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Update(ctx, sk.ID, skills.UpdateRequest{Body: ptr("v2")}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	var cm corev1.ConfigMap
	if err := svc.Reconciler().Client().Get(ctx,
		types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if !strings.HasSuffix(cm.Data["u"], "v2") {
		t.Errorf("expected v2 body in ConfigMap, got %q", cm.Data["u"])
	}
}

func TestServiceDeleteRefusedWhenReferenced(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, &stubImageLister{
		refs: []skills.Referrer{{AgentImageName: "img-1"}},
	})
	defer cleanup()

	sk, err := svc.Create(ctx, skills.CreateRequest{ID: "in-use", Name: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	err = svc.Delete(ctx, sk.ID)
	var conflictErr *skills.ErrInUse
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected ErrInUse, got %v", err)
	}
	if len(conflictErr.Referrers) != 1 || conflictErr.Referrers[0].AgentImageName != "img-1" {
		t.Errorf("expected referrer list, got %+v", conflictErr.Referrers)
	}
}

func TestServiceDeleteRemovesConfigMapEntry(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, nil)
	defer cleanup()

	sk, err := svc.Create(ctx, skills.CreateRequest{ID: "rm", Name: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(ctx, sk.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var cm corev1.ConfigMap
	if err := svc.Reconciler().Client().Get(ctx,
		types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err == nil {
		if _, exists := cm.Data["rm"]; exists {
			t.Errorf("expected key 'rm' removed from ConfigMap data: %+v", cm.Data)
		}
	}
}

func TestServiceConfigMapName(t *testing.T) {
	if skills.ConfigMapName() != sharedskills.ConfigMapName {
		t.Errorf("ConfigMapName() should mirror shared constant; got %q", skills.ConfigMapName())
	}
}

func TestServiceListEnrichesUsedBy(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, &stubImageLister{
		counts: map[string]int{"code-review": 3, "docs-helper": 1},
	})
	defer cleanup()

	for _, id := range []string{"code-review", "docs-helper", "unused"} {
		if _, err := svc.Create(ctx, skills.CreateRequest{ID: id, Name: id}); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

	summaries, err := svc.List(ctx, skills.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := make(map[string]int, len(summaries))
	for _, s := range summaries {
		got[s.ID] = s.UsedBy
	}
	want := map[string]int{"code-review": 3, "docs-helper": 1, "unused": 0}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("UsedBy[%s] = %d, want %d", id, got[id], w)
		}
	}
}

func TestServiceListNilListerLeavesUsedByZero(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	kc := fake.NewClientBuilder().WithScheme(scheme).Build()
	rec := skills.NewReconciler(kc, testNamespace)
	svc := skills.NewService(store, rec, nil)

	if _, err := svc.Create(ctx, skills.CreateRequest{ID: "solo", Name: "solo"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	summaries, err := svc.List(ctx, skills.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].UsedBy != 0 {
		t.Errorf("expected UsedBy 0 with nil lister, got %d", summaries[0].UsedBy)
	}
}

func TestServiceAssignDelegatesToLister(t *testing.T) {
	ctx := context.Background()
	lister := &stubImageLister{}
	svc, cleanup := newTestService(t, lister)
	defer cleanup()

	if _, err := svc.Create(ctx, skills.CreateRequest{ID: "my-skill", Name: "My Skill"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Assign(ctx, "my-skill", "img-abc"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if len(lister.assignCalls) != 1 {
		t.Fatalf("expected 1 assign call, got %d", len(lister.assignCalls))
	}
	if lister.assignCalls[0].skillID != "my-skill" || lister.assignCalls[0].agentImageName != "img-abc" {
		t.Errorf("unexpected assign call: %+v", lister.assignCalls[0])
	}
}

func TestServiceUnassignDelegatesToLister(t *testing.T) {
	ctx := context.Background()
	lister := &stubImageLister{}
	svc, cleanup := newTestService(t, lister)
	defer cleanup()

	if _, err := svc.Create(ctx, skills.CreateRequest{ID: "my-skill", Name: "My Skill"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Unassign(ctx, "my-skill", "img-abc"); err != nil {
		t.Fatalf("Unassign: %v", err)
	}
	if len(lister.unassignCalls) != 1 {
		t.Fatalf("expected 1 unassign call, got %d", len(lister.unassignCalls))
	}
	if lister.unassignCalls[0].skillID != "my-skill" || lister.unassignCalls[0].agentImageName != "img-abc" {
		t.Errorf("unexpected unassign call: %+v", lister.unassignCalls[0])
	}
}

func TestServiceAssignSkillNotFound(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, nil)
	defer cleanup()

	err := svc.Assign(ctx, "nonexistent", "img-abc")
	if !errors.Is(err, skills.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestServiceUnassignSkillNotFound(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, nil)
	defer cleanup()

	err := svc.Unassign(ctx, "nonexistent", "img-abc")
	if !errors.Is(err, skills.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestServiceAssignNilListerReturnsError(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	kc := fake.NewClientBuilder().WithScheme(scheme).Build()
	rec := skills.NewReconciler(kc, testNamespace)
	svc := skills.NewService(store, rec, nil)

	if _, err := svc.Create(ctx, skills.CreateRequest{ID: "my-skill", Name: "x"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	err := svc.Assign(ctx, "my-skill", "img-abc")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got %v", err)
	}
}

func TestServiceUnassignNilListerReturnsError(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	kc := fake.NewClientBuilder().WithScheme(scheme).Build()
	rec := skills.NewReconciler(kc, testNamespace)
	svc := skills.NewService(store, rec, nil)

	if _, err := svc.Create(ctx, skills.CreateRequest{ID: "my-skill", Name: "x"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	err := svc.Unassign(ctx, "my-skill", "img-abc")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got %v", err)
	}
}

func TestServiceListAssignmentsDelegatesToLister(t *testing.T) {
	ctx := context.Background()
	lister := &stubImageLister{
		refs: []skills.Referrer{{AgentImageName: "img-1"}, {AgentImageName: "img-2"}},
	}
	svc, cleanup := newTestService(t, lister)
	defer cleanup()

	refs, err := svc.ListAssignments(ctx, "some-skill")
	if err != nil {
		t.Fatalf("ListAssignments: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].AgentImageName != "img-1" || refs[1].AgentImageName != "img-2" {
		t.Errorf("unexpected refs: %+v", refs)
	}
}

func TestServiceListAssignmentsNilListerReturnsNil(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	kc := fake.NewClientBuilder().WithScheme(scheme).Build()
	rec := skills.NewReconciler(kc, testNamespace)
	svc := skills.NewService(store, rec, nil)

	refs, err := svc.ListAssignments(ctx, "some-skill")
	if err != nil {
		t.Fatalf("ListAssignments: %v", err)
	}
	if refs != nil {
		t.Errorf("expected nil refs with nil lister, got %+v", refs)
	}
}

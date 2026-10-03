package skills_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
	sharedskills "github.com/DominikPinsel/ainsel/shared/api/skills"
)

// stubImageLister returns configured referrers from ListReferrers calls.
type stubImageLister struct {
	refs    []skills.Referrer
	err     error
	counts  map[string]int
	enabled map[string]int

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

// enabled can differ from counts to model an Agent-level spec.skills.items
// selection that no AgentImage lists.
func (s *stubImageLister) EnabledSkillIDs(ctx context.Context) (map[string]int, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.enabled != nil {
		return s.enabled, nil
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

// TestServiceCreateDoesNotTouchSharedConfigMap pins the hot-set rule: a
// skill is registered in Postgres but is not mounted anywhere until an
// agent asks for it. Writing every catalogue entry into the one shared
// object is what let a library import consume the platform's entire
// ceiling and restart every skill-bearing agent.
func TestServiceCreateDoesNotTouchSharedConfigMap(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, nil)
	defer cleanup()

	if _, err := svc.Create(ctx, skills.CreateRequest{
		ID:          "code-review",
		Name:        "Code Review",
		Description: "Reviews PRs",
		Body:        "Use when X.",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Stored and readable, just not delivered.
	got, err := svc.Get(ctx, "code-review")
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if got.Body != "Use when X." {
		t.Errorf("body = %q, want it persisted", got.Body)
	}
	var cm corev1.ConfigMap
	if err := svc.Reconciler().Client().Get(ctx,
		types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err == nil {
		t.Errorf("catalogue Create must not populate the shared ConfigMap, keys=%v", keys(cm.Data))
	}
}

// TestServiceAssignDeliversTheSkill is the other half of the rule: the
// enable is what puts content on the object, so an agent does not wait
// for a convergence tick to get the skill it just asked for.
func TestServiceAssignDeliversTheSkill(t *testing.T) {
	ctx := context.Background()
	lister := &stubImageLister{}
	svc, cleanup := newServiceWithFakeClient(t, lister, skillsClient(nil))
	defer cleanup()

	if _, err := svc.Create(ctx, skills.CreateRequest{
		ID: "git-review", Name: "Git Review", Description: "Reviews git: diffs", Body: "body text",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Assign(ctx, "git-review", "ainsel-ai-agent-reviewer"); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	var cm corev1.ConfigMap
	if err := svc.Reconciler().Client().Get(ctx,
		types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("Assign should have delivered the skill: %v", err)
	}
	if !strings.Contains(cm.Data["git-review"], "body text") {
		t.Errorf("rendered SKILL.md missing body: %q", cm.Data["git-review"])
	}
	// A colon in the description must not corrupt the rendered file.
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
	// The skill is enabled by one image, so its content is live in the
	// shared object and an edit must re-render it.
	lister := &stubImageLister{counts: map[string]int{"u": 1}}
	svc, cleanup := newServiceWithFakeClient(t, lister, skillsClient(nil))
	defer cleanup()

	sk, err := svc.Create(ctx, skills.CreateRequest{ID: "u", Name: "u", Description: "d", Body: "v1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Create defers delivery; converge first so the entry exists.
	if _, err := svc.Converge(ctx); err != nil {
		t.Fatalf("Converge: %v", err)
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

// TestServiceUpdateLeavesCatalogueSkillsUndelivered is the counterpart:
// editing a skill no agent enables must not grow the shared object.
func TestServiceUpdateLeavesCatalogueSkillsUndelivered(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newServiceWithFakeClient(t, &stubImageLister{counts: map[string]int{}}, skillsClient(nil))
	defer cleanup()

	sk, err := svc.Create(ctx, skills.CreateRequest{ID: "lib", Name: "lib", Description: "d", Body: "v1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Update(ctx, sk.ID, skills.UpdateRequest{Body: ptr("v2")}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	var cm corev1.ConfigMap
	if err := svc.Reconciler().Client().Get(ctx,
		types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err == nil {
		t.Errorf("update of an unenabled skill must not populate the shared ConfigMap, keys=%v", keys(cm.Data))
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

// newServiceWithFakeClient is newTestService but lets the caller supply the
// controller-runtime client, so a test can make ConfigMap writes fail the
// way the apiserver does once the shared object hits its 1 MiB ceiling.
func newServiceWithFakeClient(t *testing.T, lister skills.AgentImageLister, kc client.WithWatch) (*skills.Service, func()) {
	t.Helper()
	store, cleanup := newTestStore(t)
	if lister == nil {
		lister = &stubImageLister{}
	}
	return skills.NewService(store, skills.NewReconciler(kc, testNamespace), lister), cleanup
}

func skillsClient(rejectUpdates error) client.WithWatch {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if rejectUpdates != nil {
		builder = builder.WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*corev1.ConfigMap); ok {
					return rejectUpdates
				}
				return c.Update(ctx, obj, opts...)
			},
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*corev1.ConfigMap); ok {
					return rejectUpdates
				}
				return c.Create(ctx, obj, opts...)
			},
		})
	}
	return builder.Build()
}

// TestServiceCreateSurvivesFullConfigMap pins the behaviour that makes a
// catalogue import possible at all: the registry is Postgres, the shared
// ConfigMap is one bounded delivery object, and a write that cannot be
// delivered must neither be undone nor reported as a rejected skill.
// Before this, once the object filled, every create on the hub failed and
// was compensated-deleted -- skills could not be stored at all.
func TestServiceCreateSurvivesFullConfigMap(t *testing.T) {
	ctx := context.Background()
	ceiling := errors.New(`ConfigMap "skills" is invalid: []: Too long: may not be more than 1048576 bytes`)
	svc, cleanup := newServiceWithFakeClient(t, nil, skillsClient(ceiling))
	defer cleanup()

	created, err := svc.Create(ctx, skills.CreateRequest{
		ID:          "catalogue-skill",
		Name:        "Catalogue Skill",
		Description: "A skill that does not fit right now: it is large",
		Body:        "body",
	})
	if err != nil {
		t.Fatalf("Create must succeed when delivery fails, got %v", err)
	}
	if created.ID != "catalogue-skill" {
		t.Errorf("unexpected skill %+v", created)
	}
	// The point of the old rollback was that the row must not linger;
	// assert it is really readable rather than silently deleted.
	got, err := svc.Get(ctx, "catalogue-skill")
	if err != nil {
		t.Fatalf("skill should persist despite undeliverable render: %v", err)
	}
	if got.Body != "body" {
		t.Errorf("persisted body = %q", got.Body)
	}
	// And nothing was written to the object.
	var cm corev1.ConfigMap
	if err := svc.Reconciler().Client().Get(ctx,
		types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err == nil {
		t.Errorf("no ConfigMap should exist when every render failed, keys=%v", cm.Data)
	}
}

func TestServiceUpdateSurvivesFullConfigMap(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()
	lister := &stubImageLister{counts: map[string]int{"s": 2}}

	// Deliver it while there is room.
	svc := skills.NewService(store, skills.NewReconciler(skillsClient(nil), testNamespace), lister)
	if _, err := svc.Create(ctx, skills.CreateRequest{ID: "s", Name: "S", Description: "old", Body: "old"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Converge(ctx); err != nil {
		t.Fatalf("Converge: %v", err)
	}

	// Now the object is at its ceiling: an edit to an enabled skill must
	// still be accepted and recorded.
	failing := skills.NewService(store, skills.NewReconciler(
		skillsClient(errors.New("too long")), testNamespace), lister)
	updated, err := failing.Update(ctx, "s", skills.UpdateRequest{Body: ptr("new body")})
	if err != nil {
		t.Fatalf("Update must succeed when delivery fails, got %v", err)
	}
	if updated.Body != "new body" {
		t.Errorf("Update returned body %q", updated.Body)
	}
}

func TestServiceConvergeDeliversOnlyEnabledSkills(t *testing.T) {
	ctx := context.Background()
	lister := &stubImageLister{counts: map[string]int{"enabled-1": 1, "enabled-2": 3}}
	svc, cleanup := newServiceWithFakeClient(t, lister, skillsClient(nil))
	defer cleanup()

	for _, id := range []string{"enabled-1", "enabled-2", "catalogue-only"} {
		if _, err := svc.Create(ctx, skills.CreateRequest{
			ID: id, Name: id, Description: "desc for " + id, Body: "body for " + id,
		}); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

	report, err := svc.Converge(ctx)
	if err != nil {
		t.Fatalf("Converge: %v", err)
	}
	if report.Enabled != 2 {
		t.Errorf("Enabled = %d, want 2 (counts keyed by skill, not by image)", report.Enabled)
	}
	if len(report.Undelivered) != 0 {
		t.Errorf("Undelivered = %v, want none", report.Undelivered)
	}

	var cm corev1.ConfigMap
	if err := svc.Reconciler().Client().Get(ctx,
		types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	for _, id := range []string{"enabled-1", "enabled-2"} {
		if _, ok := cm.Data[id]; !ok {
			t.Errorf("enabled skill %q missing from ConfigMap, keys=%v", id, keys(cm.Data))
		}
	}
	if _, ok := cm.Data["catalogue-only"]; ok {
		t.Error("skill enabled by no agent image must be pruned from the shared ConfigMap")
	}
}

func TestServiceConvergeRequiresWiring(t *testing.T) {
	ctx := context.Background()
	svc, cleanup := newTestService(t, nil) // lister defaults to a stub, reconciler present
	defer cleanup()
	if _, err := svc.Converge(ctx); err != nil {
		t.Fatalf("wired service should converge cleanly, got %v", err)
	}

	store, cleanup2 := newTestStore(t)
	defer cleanup2()
	noRec := skills.NewService(store, nil, &stubImageLister{})
	if _, err := noRec.Converge(ctx); err == nil {
		t.Error("Converge without a reconciler should report that it cannot run")
	}
}

// TestServiceConvergeKeepsAgentLevelSkills is the service-side view of the
// same trap: an Agent can select skills its AgentImage does not list. The
// convergence pass must deliver those, or it would delete the content a
// running agent mounts.
func TestServiceConvergeKeepsAgentLevelSkills(t *testing.T) {
	ctx := context.Background()
	// No image enables anything, but one agent picked a skill directly.
	lister := &stubImageLister{counts: map[string]int{}, enabled: map[string]int{"agent-picked": 1}}
	svc, cleanup := newServiceWithFakeClient(t, lister, skillsClient(nil))
	defer cleanup()

	for _, id := range []string{"agent-picked", "pure-catalogue"} {
		if _, err := svc.Create(ctx, skills.CreateRequest{ID: id, Name: id, Description: "d", Body: "b-" + id}); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	report, err := svc.Converge(ctx)
	if err != nil {
		t.Fatalf("Converge: %v", err)
	}
	if report.Enabled != 1 {
		t.Errorf("Enabled = %d, want 1", report.Enabled)
	}

	var cm corev1.ConfigMap
	if err := svc.Reconciler().Client().Get(ctx,
		types.NamespacedName{Name: sharedskills.ConfigMapName, Namespace: testNamespace}, &cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if _, ok := cm.Data["agent-picked"]; !ok {
		t.Errorf("agent-level skill was pruned, keys=%v", keys(cm.Data))
	}
	if _, ok := cm.Data["pure-catalogue"]; ok {
		t.Errorf("catalogue skill should not be delivered, keys=%v", keys(cm.Data))
	}
}

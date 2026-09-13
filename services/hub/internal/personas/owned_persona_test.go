package personas_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
)

// M3: agent-owned personas. A persona is either a shared template (visible in
// the library) or owned by exactly one Agent CR (created copy-on-write when
// that agent's persona is edited inline, hidden from the library).

func TestCreate_OwnerAgentIsPersisted(t *testing.T) {
	svc, cleanup := newTestService(t, nil)
	defer cleanup()
	ctx := context.Background()

	owned, err := svc.Create(ctx, personas.CreateRequest{
		Name: "Doc Writer", Text: "body", OwnerAgent: "a-1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if owned.OwnerAgent != "a-1" {
		t.Errorf("OwnerAgent = %q, want a-1", owned.OwnerAgent)
	}

	got, err := svc.Get(ctx, owned.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.OwnerAgent != "a-1" {
		t.Errorf("Get OwnerAgent = %q, want a-1", got.OwnerAgent)
	}
}

func TestList_HidesOwnedPersonas(t *testing.T) {
	svc, cleanup := newTestService(t, nil)
	defer cleanup()
	ctx := context.Background()

	tmpl, err := svc.Create(ctx, personas.CreateRequest{Name: "Template", Text: "shared"})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	// Same name as the template: allowed for owned personas (uniqueness is a
	// partial index over templates only).
	if _, err := svc.Create(ctx, personas.CreateRequest{
		Name: "Template", Text: "private", OwnerAgent: "a-1",
	}); err != nil {
		t.Fatalf("create owned: %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List returned %d personas, want 1 (templates only)", len(list))
	}
	if list[0].ID != tmpl.ID {
		t.Errorf("List[0].ID = %q, want the template %q", list[0].ID, tmpl.ID)
	}
}

func TestCreate_TemplateNameUniquenessStillEnforced(t *testing.T) {
	svc, cleanup := newTestService(t, nil)
	defer cleanup()
	ctx := context.Background()

	if _, err := svc.Create(ctx, personas.CreateRequest{Name: "Dup", Text: "a"}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	_, err := svc.Create(ctx, personas.CreateRequest{Name: "Dup", Text: "b"})
	if !errors.Is(err, personas.ErrNameTaken) {
		t.Errorf("err = %v, want ErrNameTaken for a duplicate template name", err)
	}
}

func TestOwnedBy(t *testing.T) {
	svc, cleanup := newTestService(t, nil)
	defer cleanup()
	ctx := context.Background()

	// No owned persona yet.
	got, err := svc.OwnedBy(ctx, "a-1")
	if err != nil {
		t.Fatalf("OwnedBy: %v", err)
	}
	if got != nil {
		t.Errorf("OwnedBy = %+v, want nil", got)
	}

	created, err := svc.Create(ctx, personas.CreateRequest{
		Name: "Owned", Text: "body", OwnerAgent: "a-1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err = svc.OwnedBy(ctx, "a-1")
	if err != nil {
		t.Fatalf("OwnedBy: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Errorf("OwnedBy = %+v, want persona %q", got, created.ID)
	}

	// A different agent owns nothing.
	other, err := svc.OwnedBy(ctx, "a-2")
	if err != nil {
		t.Fatalf("OwnedBy(a-2): %v", err)
	}
	if other != nil {
		t.Errorf("OwnedBy(a-2) = %+v, want nil", other)
	}
}

func TestEnsureOwned_CreatesThenUpdates(t *testing.T) {
	svc, cleanup := newTestService(t, nil)
	defer cleanup()
	ctx := context.Background()

	text := "first"
	created, err := svc.EnsureOwned(ctx, "a-1", "Agent One (own)", personas.UpdateRequest{Text: &text})
	if err != nil {
		t.Fatalf("EnsureOwned (create): %v", err)
	}
	if created.OwnerAgent != "a-1" {
		t.Errorf("OwnerAgent = %q, want a-1", created.OwnerAgent)
	}
	if created.Name != "Agent One (own)" {
		t.Errorf("Name = %q, want the default %q", created.Name, "Agent One (own)")
	}
	if created.CurrentVersion != 1 {
		t.Errorf("CurrentVersion = %d, want 1", created.CurrentVersion)
	}

	// Second call updates the same persona instead of forking another.
	name := "Renamed"
	text2 := "second"
	updated, err := svc.EnsureOwned(ctx, "a-1", "Agent One (own)", personas.UpdateRequest{
		Name: &name, Text: &text2,
	})
	if err != nil {
		t.Fatalf("EnsureOwned (update): %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("EnsureOwned forked a second persona: %q != %q", updated.ID, created.ID)
	}
	if updated.Name != "Renamed" {
		t.Errorf("Name = %q, want Renamed", updated.Name)
	}
	if updated.Text != "second" {
		t.Errorf("Text = %q, want second", updated.Text)
	}
	if updated.CurrentVersion != 2 {
		t.Errorf("CurrentVersion = %d, want 2", updated.CurrentVersion)
	}

	// Exactly one persona is owned by the agent.
	versions, err := svc.ListVersions(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("versions = %d, want 2", len(versions))
	}
}

func TestEnsureOwned_PrefersRequestNameOverDefault(t *testing.T) {
	svc, cleanup := newTestService(t, nil)
	defer cleanup()
	ctx := context.Background()

	name := "From Template"
	text := "body"
	p, err := svc.EnsureOwned(ctx, "a-1", "Agent One (own)", personas.UpdateRequest{
		Name: &name, Text: &text,
	})
	if err != nil {
		t.Fatalf("EnsureOwned: %v", err)
	}
	if p.Name != "From Template" {
		t.Errorf("Name = %q, want the request name", p.Name)
	}
}

func TestEnsureOwned_RequiresText(t *testing.T) {
	svc, cleanup := newTestService(t, nil)
	defer cleanup()
	ctx := context.Background()

	_, err := svc.EnsureOwned(ctx, "a-1", "Agent One (own)", personas.UpdateRequest{})
	var verr *personas.ValidationError
	if !errors.As(err, &verr) {
		t.Errorf("err = %v, want ValidationError", err)
	}
}

func TestDeleteOwned(t *testing.T) {
	svc, cleanup := newTestService(t, nil)
	defer cleanup()
	ctx := context.Background()

	tmpl, err := svc.Create(ctx, personas.CreateRequest{Name: "Template", Text: "shared"})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	owned, err := svc.Create(ctx, personas.CreateRequest{
		Name: "Owned", Text: "private", OwnerAgent: "a-1",
	})
	if err != nil {
		t.Fatalf("create owned: %v", err)
	}
	other, err := svc.Create(ctx, personas.CreateRequest{
		Name: "Other", Text: "private", OwnerAgent: "a-2",
	})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}

	// A template is never deletable through the ownership path.
	if err := svc.DeleteOwned(ctx, tmpl.ID, "a-1"); !errors.Is(err, personas.ErrNotOwned) {
		t.Errorf("DeleteOwned(template) = %v, want ErrNotOwned", err)
	}
	// Nor is another agent's persona.
	if err := svc.DeleteOwned(ctx, other.ID, "a-1"); !errors.Is(err, personas.ErrNotOwned) {
		t.Errorf("DeleteOwned(foreign) = %v, want ErrNotOwned", err)
	}

	if err := svc.DeleteOwned(ctx, owned.ID, "a-1"); err != nil {
		t.Fatalf("DeleteOwned(owned): %v", err)
	}
	if _, err := svc.Get(ctx, owned.ID); !errors.Is(err, personas.ErrNotFound) {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
	// Deleting again is a no-op (idempotent cleanup).
	if err := svc.DeleteOwned(ctx, owned.ID, "a-1"); err != nil {
		t.Errorf("second DeleteOwned = %v, want nil", err)
	}
	// The template and the other agent's persona survived.
	if _, err := svc.Get(ctx, tmpl.ID); err != nil {
		t.Errorf("template gone: %v", err)
	}
	if _, err := svc.Get(ctx, other.ID); err != nil {
		t.Errorf("foreign persona gone: %v", err)
	}
}

func TestDeleteOwned_BypassesReferrerCheck(t *testing.T) {
	// The owning agent is itself a referrer, so the library Delete would
	// refuse; DeleteOwned must not.
	lister := &stubAgentLister{refs: []personas.Referrer{{AgentName: "a-1"}}}
	svc, cleanup := newTestService(t, lister)
	defer cleanup()
	ctx := context.Background()

	owned, err := svc.Create(ctx, personas.CreateRequest{
		Name: "Owned", Text: "private", OwnerAgent: "a-1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(ctx, owned.ID); err == nil {
		t.Error("library Delete should refuse a referenced persona")
	}
	if err := svc.DeleteOwned(ctx, owned.ID, "a-1"); err != nil {
		t.Errorf("DeleteOwned = %v, want nil despite the owner referencing it", err)
	}
}

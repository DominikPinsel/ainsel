package personas_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
)

func TestStoreCreate(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	p := personas.Persona{
		ID:          "01HXTESTPERSONA00000000000",
		Name:        "code-reviewer",
		Description: "Reviews PRs",
		Text:        "You are a code reviewer.",
	}
	if err := store.Create(ctx, &p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.CurrentVersion != 1 {
		t.Errorf("expected CurrentVersion=1, got %d", p.CurrentVersion)
	}

	got, err := store.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Text != "You are a code reviewer." {
		t.Errorf("text mismatch: %q", got.Text)
	}
	if got.Name != "code-reviewer" {
		t.Errorf("name mismatch: %q", got.Name)
	}
}

func TestStoreCreateDuplicateName(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	if err := store.Create(ctx, &personas.Persona{ID: "01A", Name: "dup", Text: "a"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	err := store.Create(ctx, &personas.Persona{ID: "01B", Name: "dup", Text: "b"})
	if !errors.Is(err, personas.ErrNameTaken) {
		t.Errorf("expected ErrNameTaken, got %v", err)
	}
}

func TestStoreGetNotFound(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()
	_, err := store.Get(ctx, "01MISSING")
	if !errors.Is(err, personas.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreList(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(store.Create(ctx, &personas.Persona{ID: "01A", Name: "a", Text: "alpha"}))
	must(store.Create(ctx, &personas.Persona{ID: "01B", Name: "b", Text: "beta"}))

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 personas, got %d", len(got))
	}
	for _, s := range got {
		if s.CurrentVersion != 1 {
			t.Errorf("expected version=1, got %d for %s", s.CurrentVersion, s.Name)
		}
	}
}

func TestStoreUpdateBumpsVersionOnTextChange(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	p := personas.Persona{ID: "01C", Name: "c", Text: "v1 text"}
	if err := store.Create(ctx, &p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := store.Update(ctx, p.ID, personas.UpdateRequest{
		Description: ptr("updated desc"),
		Text:        ptr("v2 text"),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.CurrentVersion != 2 {
		t.Errorf("expected version 2, got %d", updated.CurrentVersion)
	}
	if updated.Text != "v2 text" {
		t.Errorf("expected new text, got %q", updated.Text)
	}
	if updated.Description != "updated desc" {
		t.Errorf("expected updated desc, got %q", updated.Description)
	}
}

func TestStoreUpdateNoOpWhenTextUnchanged(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	p := personas.Persona{ID: "01D", Name: "d", Text: "same"}
	if err := store.Create(ctx, &p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	updated, err := store.Update(ctx, p.ID, personas.UpdateRequest{
		Text: ptr("same"),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.CurrentVersion != 1 {
		t.Errorf("expected version still 1, got %d", updated.CurrentVersion)
	}
}

func TestStoreUpdateNotFound(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()
	_, err := store.Update(ctx, "01MISSING", personas.UpdateRequest{Text: ptr("x")})
	if !errors.Is(err, personas.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreListVersionsAndGetVersion(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	p := personas.Persona{ID: "01E", Name: "e", Text: "v1"}
	if err := store.Create(ctx, &p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := store.Update(ctx, p.ID, personas.UpdateRequest{Text: ptr("v2")}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := store.Update(ctx, p.ID, personas.UpdateRequest{Text: ptr("v3")}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	vs, err := store.ListVersions(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(vs) != 3 {
		t.Errorf("expected 3 versions, got %d", len(vs))
	}

	v2, err := store.GetVersion(ctx, p.ID, 2)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if v2.Text != "v2" {
		t.Errorf("expected v2 text, got %q", v2.Text)
	}
}

func TestStoreGetVersionNotFound(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	p := personas.Persona{ID: "01H", Name: "h", Text: "v1"}
	if err := store.Create(ctx, &p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := store.GetVersion(ctx, p.ID, 99)
	if !errors.Is(err, personas.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreRollback(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	p := personas.Persona{ID: "01F", Name: "f", Text: "v1"}
	if err := store.Create(ctx, &p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := store.Update(ctx, p.ID, personas.UpdateRequest{Text: ptr("v2")}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	rolled, err := store.Rollback(ctx, p.ID, 1)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if rolled.CurrentVersion != 3 {
		t.Errorf("expected version 3 (rollback creates new version), got %d", rolled.CurrentVersion)
	}
	if rolled.Text != "v1" {
		t.Errorf("expected rolled-back text, got %q", rolled.Text)
	}
}

func TestStoreDelete(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	p := personas.Persona{ID: "01G", Name: "g", Text: "x"}
	if err := store.Create(ctx, &p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := store.Get(ctx, p.ID)
	if !errors.Is(err, personas.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestStoreDeleteCascadesVersions pins the schema-level ON DELETE CASCADE
// behavior: when a persona is removed, every persona_versions row that
// pointed at it must be gone too. Without this test, a future migration that
// silently drops the CASCADE clause would leak version history rows.
func TestStoreDeleteCascadesVersions(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	p := personas.Persona{ID: "01CASCADE", Name: "cascade-target", Text: "v1"}
	if err := store.Create(ctx, &p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Second update so we have two persona_versions rows backing the persona.
	if _, err := store.Update(ctx, p.ID, personas.UpdateRequest{Text: ptr("v2")}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	versionsBefore, err := store.ListVersions(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListVersions before delete: %v", err)
	}
	if len(versionsBefore) != 2 {
		t.Fatalf("expected 2 versions before delete, got %d", len(versionsBefore))
	}

	if err := store.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// After the cascade, the persona_versions rows must be gone. ListVersions
	// is implemented as a plain SELECT against persona_versions, so an empty
	// slice with no error is the observable signal that the rows were
	// cascade-deleted (vs. left dangling).
	versionsAfter, err := store.ListVersions(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListVersions after delete: %v", err)
	}
	if len(versionsAfter) != 0 {
		t.Errorf("expected 0 versions after cascade delete, got %d", len(versionsAfter))
	}
}

func TestStoreDeleteNotFound(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()
	err := store.Delete(ctx, "01MISSING")
	if !errors.Is(err, personas.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

package skills_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
)

func TestStoreCreateAndGet(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	sk := skills.Skill{
		ID:          "code-review",
		Name:        "Code Review",
		Description: "Reviews PRs for bugs.",
		Body:        "When asked to review, do X.",
	}
	if err := store.Create(ctx, &sk); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sk.CreatedAt.IsZero() || sk.UpdatedAt.IsZero() {
		t.Errorf("expected timestamps to be set")
	}

	got, err := store.Get(ctx, "code-review")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Code Review" || got.Body != "When asked to review, do X." {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestStoreCreateDuplicateID(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	if err := store.Create(ctx, &skills.Skill{ID: "dup", Name: "first"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	err := store.Create(ctx, &skills.Skill{ID: "dup", Name: "second"})
	if !errors.Is(err, skills.ErrIDTaken) {
		t.Errorf("expected ErrIDTaken, got %v", err)
	}
}

func TestStoreGetNotFound(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, err := store.Get(ctx, "missing")
	if !errors.Is(err, skills.ErrNotFound) {
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
	must(store.Create(ctx, &skills.Skill{ID: "alpha", Name: "Alpha", Body: "long body alpha"}))
	must(store.Create(ctx, &skills.Skill{ID: "beta", Name: "Beta", Body: "long body beta"}))

	got, err := store.List(ctx, skills.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(got))
	}
	// List returns summaries — there's no Body field on SkillSummary, so the
	// projection itself prevents leaking large body strings. Names should be set.
	names := map[string]bool{got[0].Name: true, got[1].Name: true}
	if !names["Alpha"] || !names["Beta"] {
		t.Errorf("unexpected names: %+v", got)
	}
}

func TestStoreUpdatePartial(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	if err := store.Create(ctx, &skills.Skill{ID: "u", Name: "old", Description: "od", Body: "ob"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := store.Update(ctx, "u", skills.UpdateRequest{Name: ptr("new")})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "new" || updated.Description != "od" || updated.Body != "ob" {
		t.Errorf("partial update mismatch: %+v", updated)
	}
	if !updated.UpdatedAt.After(updated.CreatedAt) {
		t.Errorf("expected UpdatedAt to advance: %v vs %v", updated.UpdatedAt, updated.CreatedAt)
	}
}

func TestStoreUpdateNotFound(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, err := store.Update(ctx, "nope", skills.UpdateRequest{Name: ptr("x")})
	if !errors.Is(err, skills.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreDelete(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	if err := store.Create(ctx, &skills.Skill{ID: "del", Name: "x"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Delete(ctx, "del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := store.Get(ctx, "del")
	if !errors.Is(err, skills.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreDeleteNotFound(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	err := store.Delete(ctx, "nope")
	if !errors.Is(err, skills.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// seedSkills inserts a fixed set of skills used by the List filter tests.
func seedSkills(t *testing.T, store *skills.Store) {
	t.Helper()
	ctx := context.Background()
	rows := []skills.Skill{
		{ID: "code-review", Name: "Code Review", Description: "Reviews PRs for bugs", Tags: []string{"review", "go"}},
		{ID: "security-audit", Name: "Security Audit", Description: "Reviews code for vulnerabilities", Tags: []string{"security", "review"}},
		{ID: "docs-writer", Name: "Docs Writer", Description: "Writes documentation", Tags: []string{"docs"}},
	}
	for i := range rows {
		if err := store.Create(ctx, &rows[i]); err != nil {
			t.Fatalf("seed Create(%s): %v", rows[i].ID, err)
		}
	}
}

func ids(summaries []skills.SkillSummary) map[string]bool {
	out := make(map[string]bool, len(summaries))
	for _, s := range summaries {
		out[s.ID] = true
	}
	return out
}

func TestStoreListSearchByIDNameDescription(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	seedSkills(t, store)
	ctx := context.Background()

	cases := []struct {
		name   string
		search string
		want   []string
	}{
		{"match on id", "security-audit", []string{"security-audit"}},
		{"match on name case-insensitive", "code review", []string{"code-review"}},
		{"match on description", "vulnerabilities", []string{"security-audit"}},
		{"match multiple rows", "reviews", []string{"code-review", "security-audit"}},
		{"no match returns empty", "zzz-no-such-term", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := store.List(ctx, skills.ListFilter{Search: c.search})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			gotIDs := ids(got)
			if len(gotIDs) != len(c.want) {
				t.Fatalf("search %q: expected %v, got %v", c.search, c.want, gotIDs)
			}
			for _, w := range c.want {
				if !gotIDs[w] {
					t.Errorf("search %q: missing expected id %q in %v", c.search, w, gotIDs)
				}
			}
		})
	}
}

func TestStoreListSearchEscapesWildcards(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	seedSkills(t, store)
	ctx := context.Background()

	// A bare "%" must be treated literally, not as a match-everything wildcard.
	got, err := store.List(ctx, skills.ListFilter{Search: "%"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected literal %% to match nothing, got %d rows", len(got))
	}
}

func TestStoreListFilterByTags(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	seedSkills(t, store)
	ctx := context.Background()

	t.Run("single tag", func(t *testing.T) {
		got, err := store.List(ctx, skills.ListFilter{Tags: []string{"security"}})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		gotIDs := ids(got)
		if len(gotIDs) != 1 || !gotIDs["security-audit"] {
			t.Errorf("expected [security-audit], got %v", gotIDs)
		}
	})

	t.Run("multiple tags use overlap (OR)", func(t *testing.T) {
		got, err := store.List(ctx, skills.ListFilter{Tags: []string{"docs", "go"}})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		gotIDs := ids(got)
		if len(gotIDs) != 2 || !gotIDs["docs-writer"] || !gotIDs["code-review"] {
			t.Errorf("expected [docs-writer code-review], got %v", gotIDs)
		}
	})

	t.Run("unknown tag returns empty", func(t *testing.T) {
		got, err := store.List(ctx, skills.ListFilter{Tags: []string{"nope"}})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected no rows, got %d", len(got))
		}
	})
}

func TestStoreListCombinedSearchAndTags(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	seedSkills(t, store)
	ctx := context.Background()

	// Both code-review and security-audit carry the "review" tag, but only
	// security-audit matches the search term "vulnerabilities".
	got, err := store.List(ctx, skills.ListFilter{Search: "vulnerabilities", Tags: []string{"review"}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	gotIDs := ids(got)
	if len(gotIDs) != 1 || !gotIDs["security-audit"] {
		t.Errorf("expected [security-audit], got %v", gotIDs)
	}
}

func TestStoreListEmptyFilterReturnsAll(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	seedSkills(t, store)
	ctx := context.Background()

	got, err := store.List(ctx, skills.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 rows, got %d", len(got))
	}
	// Tags should round-trip as non-nil slices.
	for _, s := range got {
		if s.Tags == nil {
			t.Errorf("skill %s: expected non-nil tags", s.ID)
		}
	}
}

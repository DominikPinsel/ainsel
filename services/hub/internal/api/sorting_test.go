package api

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
)

func TestParseSortParams(t *testing.T) {
	allowed := []string{"name", "id", "usedBy", "updatedAt", "createdAt"}

	t.Run("empty query returns zero params", func(t *testing.T) {
		q := url.Values{}
		sp, err := ParseSortParams(q, allowed)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sp.OrderBy != "" || sp.OrderDir != "" {
			t.Errorf("expected zero SortParams, got %+v", sp)
		}
	})

	t.Run("valid orderBy without orderDir", func(t *testing.T) {
		q := url.Values{"orderBy": {"name"}}
		sp, err := ParseSortParams(q, allowed)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sp.OrderBy != "name" {
			t.Errorf("expected orderBy=name, got %q", sp.OrderBy)
		}
		if sp.OrderDir != "" {
			t.Errorf("expected empty orderDir, got %q", sp.OrderDir)
		}
	})

	t.Run("valid orderBy with orderDir", func(t *testing.T) {
		q := url.Values{"orderBy": {"usedBy"}, "orderDir": {"desc"}}
		sp, err := ParseSortParams(q, allowed)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sp.OrderBy != "usedby" {
			t.Errorf("expected orderBy=usedby, got %q", sp.OrderBy)
		}
		if sp.OrderDir != "desc" {
			t.Errorf("expected orderDir=desc, got %q", sp.OrderDir)
		}
	})

	t.Run("invalid orderBy returns error", func(t *testing.T) {
		q := url.Values{"orderBy": {"description"}}
		_, err := ParseSortParams(q, allowed)
		if err == nil {
			t.Fatal("expected error for invalid orderBy")
		}
		if !strings.Contains(err.Error(), "description") {
			t.Errorf("error should mention the invalid value, got: %v", err)
		}
	})

	t.Run("invalid orderDir returns error", func(t *testing.T) {
		q := url.Values{"orderBy": {"name"}, "orderDir": {"sideways"}}
		_, err := ParseSortParams(q, allowed)
		if err == nil {
			t.Fatal("expected error for invalid orderDir")
		}
		if !strings.Contains(err.Error(), "sideways") {
			t.Errorf("error should mention the invalid value, got: %v", err)
		}
	})

	t.Run("case-insensitive orderBy", func(t *testing.T) {
		q := url.Values{"orderBy": {"Name"}}
		sp, err := ParseSortParams(q, allowed)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sp.OrderBy != "name" {
			t.Errorf("expected orderBy=name (lowercase), got %q", sp.OrderBy)
		}
	})

	t.Run("orderDir without orderBy is ignored", func(t *testing.T) {
		q := url.Values{"orderDir": {"desc"}}
		sp, err := ParseSortParams(q, allowed)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sp.OrderBy != "" {
			t.Errorf("expected empty orderBy, got %q", sp.OrderBy)
		}
	})
}

func TestSortSkillSummaries(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	makeItems := func() []skills.SkillSummary {
		return []skills.SkillSummary{
			{ID: "c-skill", Name: "Charlie", UsedBy: 3, CreatedAt: base.Add(2 * time.Hour), UpdatedAt: base.Add(5 * time.Hour)},
			{ID: "a-skill", Name: "Alpha", UsedBy: 1, CreatedAt: base, UpdatedAt: base.Add(3 * time.Hour)},
			{ID: "b-skill", Name: "Bravo", UsedBy: 2, CreatedAt: base.Add(1 * time.Hour), UpdatedAt: base.Add(4 * time.Hour)},
		}
	}

	t.Run("sort by name asc", func(t *testing.T) {
		items := makeItems()
		sortSkillSummaries(items, SortParams{OrderBy: "name", OrderDir: "asc"})
		if items[0].Name != "Alpha" || items[1].Name != "Bravo" || items[2].Name != "Charlie" {
			t.Errorf("unexpected order: %s, %s, %s", items[0].Name, items[1].Name, items[2].Name)
		}
	})

	t.Run("sort by name desc", func(t *testing.T) {
		items := makeItems()
		sortSkillSummaries(items, SortParams{OrderBy: "name", OrderDir: "desc"})
		if items[0].Name != "Charlie" || items[1].Name != "Bravo" || items[2].Name != "Alpha" {
			t.Errorf("unexpected order: %s, %s, %s", items[0].Name, items[1].Name, items[2].Name)
		}
	})

	t.Run("sort by id asc", func(t *testing.T) {
		items := makeItems()
		sortSkillSummaries(items, SortParams{OrderBy: "id", OrderDir: "asc"})
		if items[0].ID != "a-skill" || items[1].ID != "b-skill" || items[2].ID != "c-skill" {
			t.Errorf("unexpected order: %s, %s, %s", items[0].ID, items[1].ID, items[2].ID)
		}
	})

	t.Run("sort by usedBy desc", func(t *testing.T) {
		items := makeItems()
		sortSkillSummaries(items, SortParams{OrderBy: "usedby", OrderDir: "desc"})
		if items[0].UsedBy != 3 || items[1].UsedBy != 2 || items[2].UsedBy != 1 {
			t.Errorf("unexpected order: %d, %d, %d", items[0].UsedBy, items[1].UsedBy, items[2].UsedBy)
		}
	})

	t.Run("sort by createdAt asc", func(t *testing.T) {
		items := makeItems()
		sortSkillSummaries(items, SortParams{OrderBy: "createdat", OrderDir: "asc"})
		if items[0].ID != "a-skill" || items[1].ID != "b-skill" || items[2].ID != "c-skill" {
			t.Errorf("unexpected order: %s, %s, %s", items[0].ID, items[1].ID, items[2].ID)
		}
	})

	t.Run("sort by updatedAt desc", func(t *testing.T) {
		items := makeItems()
		sortSkillSummaries(items, SortParams{OrderBy: "updatedat", OrderDir: "desc"})
		if items[0].ID != "c-skill" || items[1].ID != "b-skill" || items[2].ID != "a-skill" {
			t.Errorf("unexpected order: %s, %s, %s", items[0].ID, items[1].ID, items[2].ID)
		}
	})

	t.Run("default direction is asc when orderDir empty", func(t *testing.T) {
		items := makeItems()
		sortSkillSummaries(items, SortParams{OrderBy: "name"})
		if items[0].Name != "Alpha" || items[1].Name != "Bravo" || items[2].Name != "Charlie" {
			t.Errorf("expected asc default, got: %s, %s, %s", items[0].Name, items[1].Name, items[2].Name)
		}
	})

	t.Run("tiebreaker by id for equal names", func(t *testing.T) {
		items := []skills.SkillSummary{
			{ID: "z-skill", Name: "Same"},
			{ID: "a-skill", Name: "Same"},
			{ID: "m-skill", Name: "Same"},
		}
		sortSkillSummaries(items, SortParams{OrderBy: "name", OrderDir: "asc"})
		if items[0].ID != "a-skill" || items[1].ID != "m-skill" || items[2].ID != "z-skill" {
			t.Errorf("tiebreaker failed: %s, %s, %s", items[0].ID, items[1].ID, items[2].ID)
		}
	})

	t.Run("case-insensitive name sort", func(t *testing.T) {
		items := []skills.SkillSummary{
			{ID: "b", Name: "banana"},
			{ID: "a", Name: "Apple"},
			{ID: "c", Name: "cherry"},
		}
		sortSkillSummaries(items, SortParams{OrderBy: "name", OrderDir: "asc"})
		if items[0].Name != "Apple" || items[1].Name != "banana" || items[2].Name != "cherry" {
			t.Errorf("case-insensitive sort failed: %s, %s, %s", items[0].Name, items[1].Name, items[2].Name)
		}
	})

	t.Run("deterministic across pages", func(t *testing.T) {
		items1 := makeItems()
		items2 := makeItems()
		sp := SortParams{OrderBy: "usedby", OrderDir: "desc"}
		sortSkillSummaries(items1, sp)
		sortSkillSummaries(items2, sp)
		for i := range items1 {
			if items1[i].ID != items2[i].ID {
				t.Errorf("non-deterministic at index %d: %s vs %s", i, items1[i].ID, items2[i].ID)
			}
		}
	})

	t.Run("stable sort with tiebreaker for equal keys", func(t *testing.T) {
		items := []skills.SkillSummary{
			{ID: "c", Name: "C", UsedBy: 1},
			{ID: "a", Name: "A", UsedBy: 1},
			{ID: "b", Name: "B", UsedBy: 1},
		}
		sortSkillSummaries(items, SortParams{OrderBy: "usedby", OrderDir: "asc"})
		if items[0].ID != "a" || items[1].ID != "b" || items[2].ID != "c" {
			t.Errorf("tiebreaker failed: %s, %s, %s", items[0].ID, items[1].ID, items[2].ID)
		}
	})

	t.Run("id order conflicts with sort column", func(t *testing.T) {
		// ID order is opposite to name order — verifies the comparator
		// sorts by the primary column, not by id.
		items := []skills.SkillSummary{
			{ID: "z", Name: "Alpha"},
			{ID: "a", Name: "Zulu"},
		}
		sortSkillSummaries(items, SortParams{OrderBy: "name", OrderDir: "asc"})
		if items[0].Name != "Alpha" || items[1].Name != "Zulu" {
			t.Errorf("asc name sort wrong: %s, %s", items[0].Name, items[1].Name)
		}

		items2 := []skills.SkillSummary{
			{ID: "z", Name: "Alpha"},
			{ID: "a", Name: "Zulu"},
		}
		sortSkillSummaries(items2, SortParams{OrderBy: "name", OrderDir: "desc"})
		if items2[0].Name != "Zulu" || items2[1].Name != "Alpha" {
			t.Errorf("desc name sort wrong: %s, %s", items2[0].Name, items2[1].Name)
		}
	})
}

func TestSortSkillSummaries_EmptySlice(t *testing.T) {
	var items []skills.SkillSummary
	sortSkillSummaries(items, SortParams{OrderBy: "name", OrderDir: "asc"})
	if len(items) != 0 {
		t.Errorf("expected empty slice, got %d items", len(items))
	}
}



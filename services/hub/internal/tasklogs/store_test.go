package tasklogs

import (
	"context"
	"os"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("HUB_DB_URL")
	if dsn == "" {
		t.Skip("HUB_DB_URL not set, skipping integration test")
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewStore(pool)
}

func TestInsertAndList(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	e := &Entry{
		AgentName: "reviewer",
		Level:     LevelInfo,
		Message:   "started review",
		Fields:    map[string]any{"repo": "test/repo"},
	}
	if err := s.Insert(ctx, e); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if e.ID == 0 {
		t.Fatal("expected non-zero ID after insert")
	}
	if e.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt after insert")
	}

	entries, err := s.List(ctx, ListOptions{AgentName: "reviewer", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}
	found := false
	for _, entry := range entries {
		if entry.Message == "started review" {
			found = true
			if entry.Fields["repo"] != "test/repo" {
				t.Errorf("expected fields.repo='test/repo', got %v", entry.Fields["repo"])
			}
		}
	}
	if !found {
		t.Error("expected to find 'started review' entry")
	}
}

func TestListFiltersByLevel(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Insert(ctx, &Entry{AgentName: "level-test", Level: LevelInfo, Message: "info msg"})
	_ = s.Insert(ctx, &Entry{AgentName: "level-test", Level: LevelError, Message: "error msg"})

	entries, err := s.List(ctx, ListOptions{AgentName: "level-test", Level: LevelError})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if e.Level != LevelError {
			t.Errorf("expected only error entries, got level %q", e.Level)
		}
	}
}

func TestListDefaultLimit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// Insert with no explicit limit — should default to 500.
	entries, err := s.List(ctx, ListOptions{AgentName: "nonexistent-agent"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if entries == nil {
		// nil is fine for no results; just verify no error.
		return
	}
	if len(entries) > 500 {
		t.Errorf("expected at most 500 entries, got %d", len(entries))
	}
}

func TestPrune(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_ = s.Insert(ctx, &Entry{AgentName: "prune-test", Level: LevelInfo, Message: "to be pruned"})

	// Prune with zero retention deletes everything.
	n, err := s.Prune(ctx, 0)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n < 1 {
		t.Errorf("expected at least 1 pruned row, got %d", n)
	}
}

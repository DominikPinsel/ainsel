package invocations_test

import (
	"context"
	"testing"
	"time"

	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
)

// newTestStore boots a Postgres testcontainer, applies migrations via
// db.Migrate, and returns a PgStore wired to a fresh pool. Skips the test if
// Docker is unavailable, matching the existing test convention.
func newTestStore(t *testing.T) (*invocations.PgStore, func()) {
	t.Helper()
	ctx := context.Background()

	c, err := pgcontainer.Run(ctx, "postgres:17-alpine",
		pgcontainer.WithDatabase("ainsel_test"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		pgcontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("connection string: %v", err)
	}
	if err := db.Migrate(ctx, dsn); err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("open pool: %v", err)
	}

	cleanup := func() {
		pool.Close()
		_ = c.Terminate(context.Background())
	}
	return invocations.NewPgStore(pool), cleanup
}

func TestPgStoreRecordGetComplete(t *testing.T) {
	s, cleanup := newTestStore(t)
	defer cleanup()

	rec := s.Record(invocations.Invocation{
		AgentName:   "a-test",
		TriggerName: "t-test",
		EventID:     "evt-1",
		Connector:   "c-1",
	})
	if rec.ID == "" {
		t.Fatal("Record did not assign an ID")
	}
	if rec.Status != invocations.StatusRunning {
		t.Fatalf("Record status = %q, want running", rec.Status)
	}

	got, ok := s.Get(rec.ID)
	if !ok {
		t.Fatal("Get after Record missed the record")
	}
	if got.AgentName != "a-test" || got.EventID != "evt-1" || got.Status != invocations.StatusRunning {
		t.Fatalf("Get returned %+v", got)
	}
	if got.EndTime != nil || got.DurationMs != nil {
		t.Fatalf("running invocation has end time/duration: %+v", got)
	}

	if !s.Complete(rec.ID, invocations.StatusSuccess, "", time.Time{}) {
		t.Fatal("Complete returned false for an existing record")
	}
	got, ok = s.Get(rec.ID)
	if !ok {
		t.Fatal("Get after Complete missed the record")
	}
	if got.Status != invocations.StatusSuccess || got.EndTime == nil || got.DurationMs == nil {
		t.Fatalf("completed invocation = %+v", got)
	}

	if s.Complete("inv-doesnotexist", invocations.StatusSuccess, "", time.Time{}) {
		t.Fatal("Complete returned true for a missing record")
	}
}

func TestPgStoreListFiltersAndTotal(t *testing.T) {
	s, cleanup := newTestStore(t)
	defer cleanup()

	first := s.Record(invocations.Invocation{AgentName: "a-test", TriggerName: "t-a", EventID: "evt-1"})
	time.Sleep(5 * time.Millisecond) // ensure distinct start_time ordering
	second := s.Record(invocations.Invocation{AgentName: "a-other", TriggerName: "t-b", EventID: "evt-1"})
	_ = s.Complete(second.ID, invocations.StatusFailure, "boom", time.Time{})

	// No filters: newest first.
	all := s.List(invocations.ListOptions{})
	if len(all) != 2 || all[0].ID != second.ID || all[1].ID != first.ID {
		t.Fatalf("List = %+v", all)
	}

	// Filter by agent.
	only := s.List(invocations.ListOptions{AgentName: "a-test"})
	if len(only) != 1 || only[0].ID != first.ID {
		t.Fatalf("List(agent) = %+v", only)
	}

	// Filter by event ID.
	byEvent := s.List(invocations.ListOptions{EventID: "evt-1"})
	if len(byEvent) != 2 {
		t.Fatalf("List(event) = %+v", byEvent)
	}

	// Filter by status sees completed rows.
	failed := s.List(invocations.ListOptions{Status: invocations.StatusFailure})
	if len(failed) != 1 || failed[0].ID != second.ID || failed[0].Error != "boom" {
		t.Fatalf("List(status) = %+v", failed)
	}

	// Total ignores the limit.
	items, total := s.ListWithTotal(invocations.ListOptions{Limit: 1})
	if len(items) != 1 || total != 2 {
		t.Fatalf("ListWithTotal(limit=1) = %d items, total %d", len(items), total)
	}
}

func TestPgStorePrune(t *testing.T) {
	s, cleanup := newTestStore(t)
	defer cleanup()

	// Record with an explicit StartTime in the past (Record keeps a
	// non-zero StartTime), so the row is older than the prune retention.
	old := s.Record(invocations.Invocation{
		AgentName: "a-old",
		StartTime: time.Now().Add(-72 * time.Hour),
	})
	fresh := s.Record(invocations.Invocation{AgentName: "a-fresh"})

	n, err := s.Prune(context.Background(), 48*time.Hour)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("Prune removed %d rows, want 1", n)
	}
	if _, ok := s.Get(old.ID); ok {
		t.Fatal("old invocation survived pruning")
	}
	if _, ok := s.Get(fresh.ID); !ok {
		t.Fatal("fresh invocation was pruned")
	}
}

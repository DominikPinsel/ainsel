package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
)

// testStore boots a Postgres testcontainer, runs the hub migrations over it and
// returns a store backed by it. It skips when Docker is unavailable, matching
// the other integration suites in this module.
func testStore(t *testing.T) *Store {
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
	t.Cleanup(func() {
		pool.Close()
		_ = c.Terminate(context.Background())
	})
	return NewStore(pool)
}

// seedEvent inserts an event received at the given instant.
func seedEvent(t *testing.T, pool *pgxpool.Pool, id string, at time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO events (id, connector, headers, data, raw, received_at)
		 VALUES ($1, 'test-connector', '{}', '{}', '', $2)`, id, at)
	if err != nil {
		t.Fatalf("seed event %s: %v", id, err)
	}
}

// seedTask inserts an agent_tasks row for an existing event.
func seedTask(t *testing.T, pool *pgxpool.Pool, eventID, agent string, at time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO agent_tasks (event_id, agent_name, trigger_name, headers, payload, created_at)
		 VALUES ($1, $2, 'test-trigger', '{}', '{}', $3)`, eventID, agent, at)
	if err != nil {
		t.Fatalf("seed task for %s: %v", eventID, err)
	}
}

// seedErrorLog inserts an error-level task log at the given instant.
func seedErrorLog(t *testing.T, pool *pgxpool.Pool, agent string, at time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO task_logs (agent_name, level, message, created_at)
		 VALUES ($1, 'error', 'boom', $2)`, agent, at)
	if err != nil {
		t.Fatalf("seed error log: %v", err)
	}
}

func TestTotalsCountsEachMetricInItsOwnWindow(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// Window [12:00, 13:00). One event before it, one after it, two inside.
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	seedEvent(t, s.pool, "ev-before", start.Add(-time.Minute))
	seedEvent(t, s.pool, "ev-a", start.Add(time.Minute))
	seedEvent(t, s.pool, "ev-b", start.Add(30*time.Minute))
	seedEvent(t, s.pool, "ev-after", end)

	// Two tasks for ev-a (one outside the window) and one for ev-b, so the
	// routed count must be distinct events, not task rows.
	seedTask(t, s.pool, "ev-a", "agent-a", start.Add(2*time.Minute))
	seedTask(t, s.pool, "ev-a", "agent-b", start.Add(3*time.Minute))
	seedTask(t, s.pool, "ev-b", "agent-a", start.Add(40*time.Minute))
	seedTask(t, s.pool, "ev-after", "agent-a", end.Add(time.Minute))

	seedErrorLog(t, s.pool, "agent-a", start.Add(5*time.Minute))
	seedErrorLog(t, s.pool, "agent-a", end.Add(time.Minute))

	got, err := s.Totals(ctx, start, end)
	if err != nil {
		t.Fatalf("Totals: %v", err)
	}
	want := Totals{EventsConsumed: 2, TriggersMatched: 3, EventsRouted: 2, RoutingErrors: 1}
	if got != want {
		t.Errorf("Totals = %+v, want %+v", got, want)
	}
}

func TestSeriesBucketsByStepWidth(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Hour)
	step := time.Hour

	// Bucket 0 gets two events, bucket 1 none, bucket 2 one. The last event
	// sits exactly on `end` and must fall outside the window.
	seedEvent(t, s.pool, "s-a", start.Add(time.Minute))
	seedEvent(t, s.pool, "s-b", start.Add(59*time.Minute))
	seedEvent(t, s.pool, "s-c", start.Add(2*time.Hour+30*time.Minute))
	seedEvent(t, s.pool, "s-d", end)

	got, err := s.Series(ctx, MetricEventsConsumed, start, end, step)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	want := []Bucket{
		{Start: start, Count: 2},
		{Start: start.Add(2 * time.Hour), Count: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("Series returned %d buckets, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if !got[i].Start.Equal(w.Start) || got[i].Count != w.Count {
			t.Errorf("bucket %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestSeriesRoutingErrorsUsesErrorLogsOnly(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	step := 30 * time.Minute

	seedErrorLog(t, s.pool, "agent-a", start.Add(time.Minute))
	seedErrorLog(t, s.pool, "agent-a", start.Add(45*time.Minute))
	// A non-error row in the same window must not be counted.
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO task_logs (agent_name, level, message, created_at)
		 VALUES ('agent-a', 'info', 'fine', $1)`, start.Add(10*time.Minute)); err != nil {
		t.Fatalf("seed info log: %v", err)
	}

	got, err := s.Series(ctx, MetricRoutingErrors, start, end, step)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	want := []Bucket{
		{Start: start, Count: 1},
		{Start: start.Add(30 * time.Minute), Count: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("Series returned %d buckets, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if !got[i].Start.Equal(w.Start) || got[i].Count != w.Count {
			t.Errorf("bucket %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestSeriesRejectsUnknownMetric(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	if _, err := s.Series(ctx, "events_dropped", start, start.Add(time.Hour), time.Minute); err == nil {
		t.Fatal("expected an error for an unknown metric, got nil")
	}
}

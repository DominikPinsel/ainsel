package eventqueue

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool returns a pgxpool.Pool connected to the test database.
// Tests are skipped if TEST_DB_URL is not set.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		t.Skip("TEST_DB_URL not set, skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	// Clean up any leftover test data.
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM agent_tasks WHERE agent_name LIKE 'test-reap-%'")
		_, _ = pool.Exec(ctx, "DELETE FROM events WHERE id LIKE 'test-reap-%'")
	})
	return pool
}

// insertTestEvent inserts a minimal event row for use as a foreign key target.
func insertTestEvent(t *testing.T, pool *pgxpool.Pool, eventID string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx,
		`INSERT INTO events (id, connector, headers, data, raw)
		 VALUES ($1, 'test-connector', '{}', '{}', '')
		 ON CONFLICT (id) DO NOTHING`, eventID)
	if err != nil {
		t.Fatalf("insert test event: %v", err)
	}
}

// insertTestTask inserts an agent_tasks row and returns its ID.
func insertTestTask(t *testing.T, pool *pgxpool.Pool, eventID, agentName string) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO agent_tasks (event_id, agent_name, trigger_name, invocation_id, headers, payload)
		 VALUES ($1, $2, 'test-trigger', '', '{}', '{}')
		 RETURNING id`, eventID, agentName).Scan(&id)
	if err != nil {
		t.Fatalf("insert test task: %v", err)
	}
	return id
}

func TestReapStaleClaims_ReapsOldClaim(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()

	eventID := "test-reap-old-evt"
	agentName := "test-reap-agent-old"
	insertTestEvent(t, pool, eventID)
	taskID := insertTestTask(t, pool, eventID, agentName)

	// Simulate a claim that happened 600s ago.
	_, err := pool.Exec(ctx,
		`UPDATE agent_tasks
		 SET status = 'claimed', claimed_at = now() - interval '600 seconds', attempts = 1
		 WHERE id = $1`, taskID)
	if err != nil {
		t.Fatalf("set claimed: %v", err)
	}

	reaped, err := store.ReapStaleClaims(ctx, 300*time.Second)
	if err != nil {
		t.Fatalf("ReapStaleClaims: %v", err)
	}
	if len(reaped) != 1 {
		t.Fatalf("expected 1 reaped task, got %d", len(reaped))
	}
	if reaped[0].ID != taskID {
		t.Errorf("expected reaped task ID %d, got %d", taskID, reaped[0].ID)
	}
	if reaped[0].AgentName != agentName {
		t.Errorf("expected agent name %q, got %q", agentName, reaped[0].AgentName)
	}

	// Verify the task is now pending with error set.
	var status, errMsg string
	var retryAfter *time.Time
	err = pool.QueryRow(ctx,
		`SELECT status, error, retry_after FROM agent_tasks WHERE id = $1`, taskID,
	).Scan(&status, &errMsg, &retryAfter)
	if err != nil {
		t.Fatalf("query task: %v", err)
	}
	if status != "pending" {
		t.Errorf("expected status 'pending', got %q", status)
	}
	if errMsg != "claim timeout: agent did not ack/nak within deadline" {
		t.Errorf("unexpected error message: %q", errMsg)
	}
	if retryAfter == nil {
		t.Error("expected retry_after to be set")
	} else if time.Until(*retryAfter) > 35*time.Second {
		t.Errorf("retry_after too far in future: %v", time.Until(*retryAfter))
	}
}

func TestReapStaleClaims_MarksFailedWhenMaxAttemptsReached(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()

	eventID := "test-reap-max-evt"
	agentName := "test-reap-agent-max"
	insertTestEvent(t, pool, eventID)
	taskID := insertTestTask(t, pool, eventID, agentName)

	// Set attempts = max_attempts (default 10) and claim in the past.
	_, err := pool.Exec(ctx,
		`UPDATE agent_tasks
		 SET status = 'claimed', claimed_at = now() - interval '600 seconds',
		     attempts = 10, max_attempts = 10
		 WHERE id = $1`, taskID)
	if err != nil {
		t.Fatalf("set claimed: %v", err)
	}

	reaped, err := store.ReapStaleClaims(ctx, 300*time.Second)
	if err != nil {
		t.Fatalf("ReapStaleClaims: %v", err)
	}
	if len(reaped) != 1 {
		t.Fatalf("expected 1 reaped task, got %d", len(reaped))
	}

	var status string
	err = pool.QueryRow(ctx,
		`SELECT status FROM agent_tasks WHERE id = $1`, taskID).Scan(&status)
	if err != nil {
		t.Fatalf("query task: %v", err)
	}
	if status != "failed" {
		t.Errorf("expected status 'failed', got %q", status)
	}
}

func TestReapStaleClaims_NoOpWhenNoStaleClaims(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()

	reaped, err := store.ReapStaleClaims(ctx, 300*time.Second)
	if err != nil {
		t.Fatalf("ReapStaleClaims: %v", err)
	}
	if len(reaped) != 0 {
		t.Errorf("expected 0 reaped tasks, got %d", len(reaped))
	}
}

func TestReapStaleClaims_DoesNotReapFreshClaims(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()

	eventID := "test-reap-fresh-evt"
	agentName := "test-reap-agent-fresh"
	insertTestEvent(t, pool, eventID)
	taskID := insertTestTask(t, pool, eventID, agentName)

	// Claim 10s ago — well within the 300s timeout.
	_, err := pool.Exec(ctx,
		`UPDATE agent_tasks
		 SET status = 'claimed', claimed_at = now() - interval '10 seconds', attempts = 1
		 WHERE id = $1`, taskID)
	if err != nil {
		t.Fatalf("set claimed: %v", err)
	}

	reaped, err := store.ReapStaleClaims(ctx, 300*time.Second)
	if err != nil {
		t.Fatalf("ReapStaleClaims: %v", err)
	}
	if len(reaped) != 0 {
		t.Errorf("expected 0 reaped tasks (fresh claim), got %d", len(reaped))
	}

	// Verify task is still claimed.
	var status string
	err = pool.QueryRow(ctx,
		`SELECT status FROM agent_tasks WHERE id = $1`, taskID).Scan(&status)
	if err != nil {
		t.Fatalf("query task: %v", err)
	}
	if status != "claimed" {
		t.Errorf("expected status 'claimed', got %q", status)
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testQueueStore connects to the integration database (TEST_DB_URL) and
// returns an eventqueue.Store backed by it. Skipped when TEST_DB_URL is unset.
// Rows created by these tests are prefixed with "test-invtask-" and cleaned up.
func testQueueStore(t *testing.T) *eventqueue.Store {
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
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM agent_tasks WHERE event_id LIKE 'test-invtask-%'")
		_, _ = pool.Exec(ctx, "DELETE FROM events WHERE id LIKE 'test-invtask-%'")
	})
	return eventqueue.NewStore(pool)
}

// TestInvocations_EventEnrichesTasks verifies that GET /api/v1/invocations?event=…
// attaches the agent_tasks queue state to known invocations and synthesizes
// records for tasks missing from the in-memory store (#195).
func TestInvocations_EventEnrichesTasks(t *testing.T) {
	if os.Getenv("TEST_DB_URL") == "" {
		t.Skip("TEST_DB_URL not set, skipping integration test")
	}
	s := testServer(t)
	s.eventQueue = testQueueStore(t)
	s.mux.HandleFunc("/api/v1/invocations", s.handleInvocations)

	// Event with two tasks for two agents (agent_tasks deduplicates on
	// (event_id, agent_name)): one whose invocation is in memory, one that
	// is not (pre-restart history / still queued).
	eventID := "test-invtask-" + time.Now().UTC().Format("150405.000000000")
	ctx := context.Background()
	if _, err := s.eventQueue.Pool().Exec(ctx,
		`INSERT INTO events (id, connector, headers, data, raw)
		 VALUES ($1, 'test-connector', '{}', '{}', '')
		 ON CONFLICT (id) DO NOTHING`, eventID); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	// In-memory invocation for the completed task.
	mem := s.invocations.Record(invocations.Invocation{
		AgentName:   "agent-a",
		TriggerName: "trigger-a",
		StartTime:   time.Now().UTC().Add(-2 * time.Minute),
	})
	completedAt := time.Now().UTC().Add(-time.Minute)
	for _, tt := range []struct {
		invocationID string
		agent        string
		trigger      string
		status       string
		attempts     int
		errMsg       string
	}{
		{mem.ID, "agent-a", "trigger-a", "completed", 1, ""},
		{"inv-test-synthetic", "agent-b", "trigger-b", "pending", 0, ""},
	} {
		if _, err := s.eventQueue.Pool().Exec(ctx,
			`INSERT INTO agent_tasks (event_id, agent_name, trigger_name, invocation_id, headers, payload, status, attempts, error, completed_at)
			 VALUES ($1, $2, $3, $4, '{}', '{}', $5, $6, $7, CASE WHEN $5 = 'completed' THEN now() END)`,
			eventID, tt.agent, tt.trigger, tt.invocationID, tt.status, tt.attempts, tt.errMsg); err != nil {
			t.Fatalf("insert task: %v", err)
		}
	}
	_ = completedAt

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invocations?event="+eventID, nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Invocations []invocations.Invocation `json:"invocations"`
		Total       int                      `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Total != 2 {
		t.Fatalf("expected 2 invocations (1 in-memory + 1 synthetic), got %d: %s", resp.Total, rec.Body.String())
	}

	byID := map[string]invocations.Invocation{}
	for _, inv := range resp.Invocations {
		byID[inv.ID] = inv
	}

	// The in-memory record carries the queue state of its agent_tasks row.
	enriched, ok := byID[mem.ID]
	if !ok {
		t.Fatalf("in-memory invocation %s missing from response", mem.ID)
	}
	if enriched.Task == nil || enriched.Task.Status != "completed" || enriched.Task.Attempts != 1 {
		t.Errorf("expected task state completed/1 on %s, got %+v", mem.ID, enriched.Task)
	}

	// The queued task appears as a synthetic running invocation with its
	// queue state, so the event page can explain the empty transcript.
	synth, ok := byID["inv-test-synthetic"]
	if !ok {
		t.Fatalf("synthetic invocation for queued task missing from response: %s", rec.Body.String())
	}
	if synth.Status != invocations.StatusRunning {
		t.Errorf("expected synthetic status running, got %s", synth.Status)
	}
	if synth.Task == nil || synth.Task.Status != "pending" || synth.Task.Attempts != 0 {
		t.Errorf("expected task state pending/0, got %+v", synth.Task)
	}
	if synth.AgentName != "agent-b" {
		t.Errorf("expected agent-b, got %s", synth.AgentName)
	}
}

// TestInvocations_EventSynthesizesFailedTask checks that a failed task
// without a matching in-memory invocation surfaces as a failure entry with
// the task error attached.
func TestInvocations_EventSynthesizesFailedTask(t *testing.T) {
	if os.Getenv("TEST_DB_URL") == "" {
		t.Skip("TEST_DB_URL not set, skipping integration test")
	}
	s := testServer(t)
	s.eventQueue = testQueueStore(t)
	s.mux.HandleFunc("/api/v1/invocations", s.handleInvocations)

	eventID := "test-invtask-fail-" + time.Now().UTC().Format("150405.000000000")
	if _, err := s.eventQueue.Pool().Exec(context.Background(),
		`INSERT INTO events (id, connector, headers, data, raw)
		 VALUES ($1, 'test-connector', '{}', '{}', '')
		 ON CONFLICT (id) DO NOTHING`, eventID); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if _, err := s.eventQueue.Pool().Exec(context.Background(),
		`INSERT INTO agent_tasks (event_id, agent_name, trigger_name, invocation_id, headers, payload, status, attempts, error)
		 VALUES ($1, 'agent-b', 'trigger-b', 'inv-test-failed', '{}', '{}', 'failed', 3, 'Turn timed out after 600000ms')`,
		eventID); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invocations?event="+eventID, nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	var resp struct {
		Invocations []invocations.Invocation `json:"invocations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Invocations) != 1 {
		t.Fatalf("expected 1 synthetic invocation, got %d", len(resp.Invocations))
	}
	inv := resp.Invocations[0]
	if inv.Status != invocations.StatusFailure {
		t.Errorf("expected failure status, got %s", inv.Status)
	}
	if inv.Task == nil || inv.Task.Attempts != 3 || inv.Task.Error != "Turn timed out after 600000ms" {
		t.Errorf("unexpected task state: %+v", inv.Task)
	}
	if inv.Error != "Turn timed out after 600000ms" {
		t.Errorf("expected error on invocation, got %q", inv.Error)
	}
}

func TestInvocationStatusFromTask(t *testing.T) {
	cases := map[string]string{
		"pending":   invocations.StatusRunning,
		"claimed":   invocations.StatusRunning,
		"completed": invocations.StatusSuccess,
		"failed":    invocations.StatusFailure,
	}
	for taskStatus, want := range cases {
		if got := invocationStatusFromTask(eventqueue.Task{Status: taskStatus}); got != want {
			t.Errorf("invocationStatusFromTask(%q) = %q, want %q", taskStatus, got, want)
		}
	}
}
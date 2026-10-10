package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// newTestEventStore boots a Postgres testcontainer, applies migrations, and
// returns an eventqueue.Store wired to a fresh pool. Skips the test if Docker
// is unavailable.
func newTestEventStore(t *testing.T) (*eventqueue.Store, func()) {
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
	return eventqueue.NewStore(pool), cleanup
}

// eventsTestServer wires a Server with the given event store and registers the
// activity routes.
func eventsTestServer(t *testing.T, store *eventqueue.Store) *Server {
	t.Helper()
	s := testServer(t)
	s.SetEventQueue(store)
	s.mux.HandleFunc("/api/v1/events", s.handleEvents)
	s.mux.HandleFunc("/api/v1/events/", s.handleEvent)
	return s
}

// seedEvent inserts an event directly via the store.
func seedEvent(t *testing.T, store *eventqueue.Store, id, connector string, data string) {
	t.Helper()
	err := store.InsertEvent(context.Background(), eventqueue.Event{
		ID:        id,
		Connector: connector,
		Headers:   json.RawMessage(`{}`),
		Data:      json.RawMessage(data),
		Raw:       data,
	})
	if err != nil {
		t.Fatalf("InsertEvent(%s): %v", id, err)
	}
}

// seedTask inserts a task for an event with the given status by enqueueing and
// then updating the status directly.
func seedTask(t *testing.T, store *eventqueue.Store, eventID, agent, trigger, status string) {
	t.Helper()
	ctx := context.Background()
	err := store.EnqueueTask(ctx, eventqueue.Task{
		EventID:     eventID,
		AgentName:   agent,
		TriggerName: trigger,
		Headers:     json.RawMessage(`{}`),
		Payload:     json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("EnqueueTask(%s): %v", eventID, err)
	}
	if status != "pending" {
		if _, err := store.Pool().Exec(ctx,
			`UPDATE agent_tasks SET status = $1 WHERE event_id = $2 AND agent_name = $3`,
			status, eventID, agent,
		); err != nil {
			t.Fatalf("set task status: %v", err)
		}
	}
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) eventsEnvelope {
	t.Helper()
	var env eventsEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env
}

func TestListEvents_Empty(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=100", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if env.Events == nil {
		t.Fatalf("expected non-nil events slice, got nil")
	}
	if len(env.Events) != 0 || env.Total != 0 {
		t.Fatalf("expected empty envelope, got %+v", env)
	}
}

func TestListEvents_StatusDerivation(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	// unmatched: no tasks.
	seedEvent(t, store, "evt-unmatched", "github", `{"a":1}`)
	// matched: tasks exist, none failed.
	seedEvent(t, store, "evt-matched", "github", `{"b":2}`)
	seedTask(t, store, "evt-matched", "agent-a", "trig-1", "completed")
	// error: at least one failed task.
	seedEvent(t, store, "evt-error", "slack", `{"c":3}`)
	seedTask(t, store, "evt-error", "agent-b", "trig-2", "completed")
	seedTask(t, store, "evt-error", "agent-c", "trig-3", "failed")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=100", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if len(env.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(env.Events))
	}

	byID := map[string]activityEntry{}
	for _, e := range env.Events {
		byID[e.ID] = e
	}

	if got := byID["evt-unmatched"].Status; got != activityStatusUnmatched {
		t.Errorf("evt-unmatched: expected status %q, got %q", activityStatusUnmatched, got)
	}
	if got := byID["evt-matched"].Status; got != activityStatusMatched {
		t.Errorf("evt-matched: expected status %q, got %q", activityStatusMatched, got)
	}
	if got := byID["evt-error"].Status; got != activityStatusError {
		t.Errorf("evt-error: expected status %q, got %q", activityStatusError, got)
	}

	// Matches should be populated for routed events.
	if len(byID["evt-error"].Matches) != 2 {
		t.Errorf("evt-error: expected 2 matches, got %d", len(byID["evt-error"].Matches))
	}
	if len(byID["evt-unmatched"].Matches) != 0 {
		t.Errorf("evt-unmatched: expected 0 matches, got %d", len(byID["evt-unmatched"].Matches))
	}
	// Timestamp must be set.
	if byID["evt-matched"].Timestamp == "" {
		t.Errorf("evt-matched: expected non-empty timestamp")
	}
}

func TestListEvents_ConnectorFilter(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	seedEvent(t, store, "evt-gh", "github", `{"x":1}`)
	seedEvent(t, store, "evt-sl", "slack", `{"y":2}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?connector=slack", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)
	if len(env.Events) != 1 || env.Events[0].ID != "evt-sl" {
		t.Fatalf("expected only evt-sl, got %+v", env.Events)
	}
}

func TestListEvents_SinceFilter(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	seedEvent(t, store, "evt-1", "github", `{"x":1}`)

	// A since far in the future should exclude everything.
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?since="+future, nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if env := decodeEnvelope(t, rec); len(env.Events) != 0 {
		t.Fatalf("expected 0 events for future since, got %d", len(env.Events))
	}

	// A since far in the past should include the event.
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/events?since="+past, nil)
	rec = httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if env := decodeEnvelope(t, rec); len(env.Events) != 1 {
		t.Fatalf("expected 1 event for past since, got %d", len(env.Events))
	}
}

func TestListEvents_InvalidSince_400(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?since=not-a-time", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListEvents_InvalidLimit_400(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=abc", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// setEventTime overrides received_at so tests get a deterministic order.
func setEventTime(t *testing.T, store *eventqueue.Store, id string, ts time.Time) {
	t.Helper()
	if _, err := store.Pool().Exec(context.Background(),
		`UPDATE events SET received_at = $1 WHERE id = $2`, ts, id,
	); err != nil {
		t.Fatalf("set event time: %v", err)
	}
}

func TestListEvents_Pagination(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("evt-%d", i)
		seedEvent(t, store, id, "github", `{"i":1}`)
		// evt-4 is newest, evt-0 oldest.
		setEventTime(t, store, id, base.Add(time.Duration(i)*time.Minute))
	}

	for page, wantIDs := range [][]string{
		{"evt-4", "evt-3"},
		{"evt-2", "evt-1"},
		{"evt-0"},
		{},
	} {
		url := fmt.Sprintf("/api/v1/events?limit=2&offset=%d", page*2)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", url, rec.Code, rec.Body.String())
		}
		env := decodeEnvelope(t, rec)
		if env.Total != 5 {
			t.Fatalf("%s: expected total 5, got %d", url, env.Total)
		}
		if len(env.Events) != len(wantIDs) {
			t.Fatalf("%s: expected %d events, got %d", url, len(wantIDs), len(env.Events))
		}
		for i, want := range wantIDs {
			if env.Events[i].ID != want {
				t.Errorf("%s: event %d: expected %q, got %q", url, i, want, env.Events[i].ID)
			}
		}
	}
}

func TestListEvents_TotalReflectsFilters(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	seedEvent(t, store, "evt-gh-1", "github", `{"i":1}`)
	seedEvent(t, store, "evt-gh-2", "github", `{"i":2}`)
	seedEvent(t, store, "evt-sl-1", "slack", `{"i":3}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=1&connector=github", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	env := decodeEnvelope(t, rec)
	if len(env.Events) != 1 {
		t.Fatalf("expected 1 event on page, got %d", len(env.Events))
	}
	if env.Total != 2 {
		t.Fatalf("expected total 2 for connector=github, got %d", env.Total)
	}
}

func TestListEvents_StatusFilter(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	seedEvent(t, store, "evt-unmatched", "github", `{"a":1}`)
	seedEvent(t, store, "evt-matched", "github", `{"b":2}`)
	seedTask(t, store, "evt-matched", "agent-a", "trig-1", "completed")
	seedEvent(t, store, "evt-error", "github", `{"c":3}`)
	seedTask(t, store, "evt-error", "agent-b", "trig-2", "failed")

	cases := []struct {
		status string
		want   []string
	}{
		{activityStatusUnmatched, []string{"evt-unmatched"}},
		{activityStatusMatched, []string{"evt-matched"}},
		{activityStatusError, []string{"evt-error"}},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events?status="+tc.status, nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status=%s: expected 200, got %d: %s", tc.status, rec.Code, rec.Body.String())
		}
		env := decodeEnvelope(t, rec)
		if len(env.Events) != len(tc.want) || env.Total != len(tc.want) {
			t.Fatalf("status=%s: expected %d events (total %d), got %d events (total %d)",
				tc.status, len(tc.want), len(tc.want), len(env.Events), env.Total)
		}
		for i, want := range tc.want {
			if env.Events[i].ID != want {
				t.Errorf("status=%s: event %d: expected %q, got %q", tc.status, i, want, env.Events[i].ID)
			}
		}
	}
}

func TestListEvents_AgentFilter(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	seedEvent(t, store, "evt-a", "github", `{"a":1}`)
	seedTask(t, store, "evt-a", "agent-a", "trig-1", "completed")
	seedEvent(t, store, "evt-b", "github", `{"b":2}`)
	seedTask(t, store, "evt-b", "agent-b", "trig-2", "completed")
	seedEvent(t, store, "evt-ab", "github", `{"c":3}`)
	seedTask(t, store, "evt-ab", "agent-a", "trig-3", "completed")
	seedTask(t, store, "evt-ab", "agent-b", "trig-4", "completed")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?agent=agent-a", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	env := decodeEnvelope(t, rec)
	if env.Total != 2 {
		t.Fatalf("expected total 2 for agent=agent-a, got %d", env.Total)
	}
	got := map[string]bool{}
	for _, e := range env.Events {
		got[e.ID] = true
	}
	if !got["evt-a"] || !got["evt-ab"] || got["evt-b"] {
		t.Fatalf("unexpected events for agent filter: %+v", got)
	}
}

func TestListEvents_InvalidOffset_400(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	for _, v := range []string{"abc", "-1"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events?offset="+v, nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("offset=%s: expected 400, got %d", v, rec.Code)
		}
	}
}

func TestListEvents_InvalidStatus_400(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?status=bogus", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetEvent_Found(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	seedEvent(t, store, "evt-123", "github", `{"hello":"world"}`)
	seedTask(t, store, "evt-123", "agent-a", "trig-1", "completed")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/evt-123", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var entry activityEntry
	if err := json.NewDecoder(rec.Body).Decode(&entry); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if entry.ID != "evt-123" || entry.Connector != "github" || entry.Status != activityStatusMatched {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if len(entry.Matches) != 1 || entry.Matches[0].Agent != "agent-a" {
		t.Fatalf("unexpected matches: %+v", entry.Matches)
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/does-not-exist", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetEvent_RejectsMissingID(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/events/", s.handleEvent)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListEvents_NotConfigured_503(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/events", s.handleEvents)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when event queue not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}

// seedTaskWithInvocation inserts a task for an event and sets its invocation_id.
func seedTaskWithInvocation(t *testing.T, store *eventqueue.Store, eventID, agent, trigger, status, invocationID string) {
	t.Helper()
	ctx := context.Background()
	err := store.EnqueueTask(ctx, eventqueue.Task{
		EventID:      eventID,
		AgentName:    agent,
		TriggerName:  trigger,
		InvocationID: invocationID,
		Headers:      json.RawMessage(`{}`),
		Payload:      json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("EnqueueTask(%s): %v", eventID, err)
	}
	if status != "pending" {
		if _, err := store.Pool().Exec(ctx,
			`UPDATE agent_tasks SET status = $1 WHERE event_id = $2 AND agent_name = $3`,
			status, eventID, agent,
		); err != nil {
			t.Fatalf("set task status: %v", err)
		}
	}
}

func TestListEvents_RunStateEnrichment(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	// Event with a task linked to a completed invocation.
	seedEvent(t, store, "evt-run-ok", "github", `{"a":1}`)
	seedTaskWithInvocation(t, store, "evt-run-ok", "agent-a", "trig-1", "completed", "inv-aaaa")
	s.invocations.Record(invocations.Invocation{
		ID:          "inv-aaaa",
		AgentName:   "agent-a",
		TriggerName: "trig-1",
		EventID:     "evt-run-ok",
		Status:      invocations.StatusSuccess,
	})
	s.invocations.Complete("inv-aaaa", invocations.StatusSuccess, "", time.Time{})

	// Event with a task linked to a running invocation.
	seedEvent(t, store, "evt-run-running", "github", `{"b":2}`)
	seedTaskWithInvocation(t, store, "evt-run-running", "agent-b", "trig-2", "claimed", "inv-bbbb")
	s.invocations.Record(invocations.Invocation{
		ID:          "inv-bbbb",
		AgentName:   "agent-b",
		TriggerName: "trig-2",
		EventID:     "evt-run-running",
		Status:      invocations.StatusRunning,
	})

	// Event with a task whose invocation has been evicted (not in store).
	seedEvent(t, store, "evt-run-missing", "slack", `{"c":3}`)
	seedTaskWithInvocation(t, store, "evt-run-missing", "agent-c", "trig-3", "completed", "inv-missing")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=100", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	env := decodeEnvelope(t, rec)

	byID := map[string]activityEntry{}
	for _, e := range env.Events {
		byID[e.ID] = e
	}

	// Completed invocation: runStatus=success, durationMs set.
	okEntry := byID["evt-run-ok"]
	if len(okEntry.Matches) != 1 {
		t.Fatalf("expected 1 match for evt-run-ok, got %d", len(okEntry.Matches))
	}
	if okEntry.Matches[0].RunStatus != "success" {
		t.Errorf("expected runStatus success, got %q", okEntry.Matches[0].RunStatus)
	}
	if okEntry.Matches[0].DurationMs == nil {
		t.Error("expected durationMs to be set for completed invocation")
	}

	// Running invocation: runStatus=running, no duration.
	runEntry := byID["evt-run-running"]
	if len(runEntry.Matches) != 1 {
		t.Fatalf("expected 1 match for evt-run-running, got %d", len(runEntry.Matches))
	}
	if runEntry.Matches[0].RunStatus != "running" {
		t.Errorf("expected runStatus running, got %q", runEntry.Matches[0].RunStatus)
	}
	if runEntry.Matches[0].DurationMs != nil {
		t.Error("expected durationMs nil for running invocation")
	}

	// Missing invocation: runStatus empty (graceful degradation).
	missEntry := byID["evt-run-missing"]
	if len(missEntry.Matches) != 1 {
		t.Fatalf("expected 1 match for evt-run-missing, got %d", len(missEntry.Matches))
	}
	if missEntry.Matches[0].RunStatus != "" {
		t.Errorf("expected empty runStatus for missing invocation, got %q", missEntry.Matches[0].RunStatus)
	}
}

func TestGetEvent_RunStateEnrichment(t *testing.T) {
	store, cleanup := newTestEventStore(t)
	defer cleanup()
	s := eventsTestServer(t, store)

	seedEvent(t, store, "evt-detail", "github", `{"x":1}`)
	seedTaskWithInvocation(t, store, "evt-detail", "agent-a", "trig-1", "completed", "inv-detail")
	s.invocations.Record(invocations.Invocation{
		ID:        "inv-detail",
		AgentName: "agent-a",
		EventID:   "evt-detail",
	})
	s.invocations.Complete("inv-detail", invocations.StatusFailure, "something broke", time.Time{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/evt-detail", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var entry activityEntry
	if err := json.NewDecoder(rec.Body).Decode(&entry); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entry.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(entry.Matches))
	}
	if entry.Matches[0].RunStatus != "failure" {
		t.Errorf("expected runStatus failure, got %q", entry.Matches[0].RunStatus)
	}
	if entry.Matches[0].Error != "something broke" {
		t.Errorf("expected error 'something broke', got %q", entry.Matches[0].Error)
	}
	if entry.Matches[0].DurationMs == nil {
		t.Error("expected durationMs to be set for completed invocation")
	}
}

// outcomeFixture seeds events for the outcome-filter tests: every run status
// is represented, one event mixes outcomes across its tasks, and two events
// carry tasks without a usable invocation record (empty id, or a pruned one).
// Returns the server wired with a Postgres-backed invocation store so the
// SQL outcome filter and the runStatus enrichment read the same table — the
// production wiring.
func outcomeFixture(t *testing.T) (*Server, *eventqueue.Store, context.CancelFunc) {
	t.Helper()
	store, cleanup := newTestEventStore(t)
	s := eventsTestServer(t, store)
	s.invocations = invocations.NewPgStore(store.Pool())

	record := func(id, eventID, agent, status string) {
		s.invocations.Record(invocations.Invocation{
			ID: id, AgentName: agent, TriggerName: "trig-" + agent, EventID: eventID, Status: status,
		})
	}

	seedEvent(t, store, "evt-oc-ok", "github", `{"a":1}`)
	seedTaskWithInvocation(t, store, "evt-oc-ok", "agent-a", "trig-1", "completed", "inv-oc-ok")
	record("inv-oc-ok", "evt-oc-ok", "agent-a", invocations.StatusSuccess)

	seedEvent(t, store, "evt-oc-failed", "github", `{"b":2}`)
	seedTaskWithInvocation(t, store, "evt-oc-failed", "agent-b", "trig-2", "failed", "inv-oc-failed")
	record("inv-oc-failed", "evt-oc-failed", "agent-b", invocations.StatusFailure)

	// Mixed event: one success and one failure run — matches both filters.
	seedEvent(t, store, "evt-oc-mixed", "slack", `{"c":3}`)
	seedTaskWithInvocation(t, store, "evt-oc-mixed", "agent-c", "trig-3", "completed", "inv-oc-mixed-ok")
	record("inv-oc-mixed-ok", "evt-oc-mixed", "agent-c", invocations.StatusSuccess)
	seedTaskWithInvocation(t, store, "evt-oc-mixed", "agent-d", "trig-4", "failed", "inv-oc-mixed-fail")
	record("inv-oc-mixed-fail", "evt-oc-mixed", "agent-d", invocations.StatusFailure)

	seedEvent(t, store, "evt-oc-running", "github", `{"d":4}`)
	seedTaskWithInvocation(t, store, "evt-oc-running", "agent-e", "trig-5", "claimed", "inv-oc-run")
	record("inv-oc-run", "evt-oc-running", "agent-e", invocations.StatusRunning)

	// A task with no invocation record: matches the activity status filters
	// but no outcome, mirroring the UI which renders "—".
	seedEvent(t, store, "evt-oc-no-inv", "github", `{"e":5}`)
	seedTaskWithInvocation(t, store, "evt-oc-no-inv", "agent-f", "trig-6", "completed", "")

	// A task whose invocation id exists but whose record was pruned.
	seedEvent(t, store, "evt-oc-pruned", "github", `{"f":6}`)
	seedTaskWithInvocation(t, store, "evt-oc-pruned", "agent-g", "trig-7", "completed", "inv-oc-gone")

	return s, store, cleanup
}

// listEventsOutcome runs GET /api/v1/events with the given query and returns
// the decoded envelope.
func listEventsOutcome(t *testing.T, s *Server, query string) eventsEnvelope {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?"+query, nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("query %q: expected 200, got %d: %s", query, rec.Code, rec.Body.String())
	}
	return decodeEnvelope(t, rec)
}

// assertEventIDs checks the envelope holds exactly the wanted event ids.
func assertEventIDs(t *testing.T, env eventsEnvelope, want []string) {
	t.Helper()
	if env.Total != len(want) {
		t.Errorf("expected total %d, got %d", len(want), env.Total)
	}
	got := map[string]bool{}
	for _, e := range env.Events {
		got[e.ID] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("expected event %q in result, got %+v", id, got)
		}
		delete(got, id)
	}
	if len(got) != 0 {
		t.Errorf("unexpected events in result: %+v", got)
	}
}

func TestListEvents_OutcomeFilter(t *testing.T) {
	s, _, cleanup := outcomeFixture(t)
	defer cleanup()

	cases := []struct {
		outcome string
		want    []string
	}{
		{"success", []string{"evt-oc-ok", "evt-oc-mixed"}},
		{"failure", []string{"evt-oc-failed", "evt-oc-mixed"}},
		{"running", []string{"evt-oc-running"}},
		// No event has a timeout run; the events without invocation records
		// match no outcome at all.
		{"timeout", nil},
	}
	for _, tc := range cases {
		env := listEventsOutcome(t, s, "outcome="+tc.outcome)
		assertEventIDs(t, env, tc.want)
	}

	// AND with the activity status filter: evt-oc-mixed is the only event
	// whose runs include a success while its derived status is error (one
	// failed task).
	env := listEventsOutcome(t, s, "outcome=success&status=error")
	assertEventIDs(t, env, []string{"evt-oc-mixed"})

	env = listEventsOutcome(t, s, "outcome=failure&status=matched")
	assertEventIDs(t, env, nil)
}

// TestListEvents_OutcomeFilterPaginates is the regression the frontend bug
// was: a matching event deeper in history had to belong to a page the
// server-side filter can reach. With limit=1 the filtered set is walked one
// event at a time, oldest and newest failure both visible.
func TestListEvents_OutcomeFilterPaginates(t *testing.T) {
	s, store, cleanup := outcomeFixture(t)
	defer cleanup()

	// Pin received_at so the (received_at DESC, id DESC) page order is fully
	// deterministic: evt-oc-mixed is newer and must land on page one,
	// evt-oc-failed trails on page two.
	base := time.Now().UTC().Truncate(time.Second)
	pinTime := func(id string, at time.Time) {
		t.Helper()
		if _, err := store.Pool().Exec(context.Background(),
			`UPDATE events SET received_at = $1 WHERE id = $2`, at, id); err != nil {
			t.Fatalf("pin received_at(%s): %v", id, err)
		}
	}
	pinTime("evt-oc-failed", base.Add(-10*time.Second))
	pinTime("evt-oc-mixed", base.Add(-5*time.Second))

	page1 := listEventsOutcome(t, s, "outcome=failure&status=error&limit=1&offset=0")
	if len(page1.Events) != 1 || page1.Events[0].ID != "evt-oc-mixed" || page1.Total != 2 {
		t.Fatalf("first failure page: got %+v (total %d)", page1.Events, page1.Total)
	}

	page2 := listEventsOutcome(t, s, "outcome=failure&status=error&limit=1&offset=1")
	if len(page2.Events) != 1 || page2.Events[0].ID != "evt-oc-failed" || page2.Total != 2 {
		t.Fatalf("second failure page: got %+v (total %d)", page2.Events, page2.Total)
	}
}

func TestListEvents_InvalidOutcome_400(t *testing.T) {
	s, _, cleanup := outcomeFixture(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?outcome=bogus", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

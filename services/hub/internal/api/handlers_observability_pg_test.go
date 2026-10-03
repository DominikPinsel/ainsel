package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
	"github.com/DominikPinsel/ainsel/services/hub/internal/telemetry"
)

// obsDBServer boots a Postgres testcontainer, migrates it, and returns a server
// wired with the hub's own telemetry and task-log stores but **no Prometheus** —
// the shape a standalone install actually runs in. The pool is returned so a
// test can seed raw rows. Skips when Docker is unavailable, like the other
// integration suites here.
func obsDBServer(t *testing.T, paths ...string) (*Server, *pgxpool.Pool) {
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

	s := testServer(t)
	s.SetTelemetryStore(telemetry.NewStore(pool))
	s.SetTaskLogStore(tasklogs.NewStore(pool))
	for _, p := range paths {
		s.mux.HandleFunc(p, s.handleObservability)
	}
	return s, pool
}

func obsSeedEvent(t *testing.T, pool *pgxpool.Pool, id string, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO events (id, connector, headers, data, raw, received_at)
		 VALUES ($1, 'test-connector', '{}', '{}', '', $2)`, id, at); err != nil {
		t.Fatalf("seed event %s: %v", id, err)
	}
}

func obsSeedTask(t *testing.T, pool *pgxpool.Pool, eventID, agent string, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO agent_tasks (event_id, agent_name, trigger_name, headers, payload, created_at)
		 VALUES ($1, $2, 'test-trigger', '{}', '{}', $3)`, eventID, agent, at); err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

func obsSeedErrorLog(t *testing.T, pool *pgxpool.Pool, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO task_logs (agent_name, level, message, created_at)
		 VALUES ('agent-a', 'error', 'boom', $1)`, at); err != nil {
		t.Fatalf("seed error log: %v", err)
	}
}

// --- summary from the hub's own records ---

func TestObservability_SummaryReadsPostgresWhenPromMissing(t *testing.T) {
	s, pool := obsDBServer(t, "/api/v1/observability/metrics/summary")

	// The range-less call reports everything the hub still holds.
	obsSeedEvent(t, pool, "pg-sum-1", time.Now().UTC())
	obsSeedTask(t, pool, "pg-sum-1", "agent-a", time.Now().UTC())
	obsSeedErrorLog(t, pool, time.Now().UTC())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from the hub's own records, got %d: %s", rec.Code, rec.Body.String())
	}
	var body MetricsSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.EventsConsumed != 1 || body.TriggersMatched != 1 || body.EventsRouted != 1 {
		t.Errorf("unexpected summary from postgres: %+v", body)
	}
	if body.RoutingErrors != 1 {
		t.Errorf("expected 1 error from task_logs, got %v", body.RoutingErrors)
	}
	if body.Source != metricsSourcePostgres {
		t.Errorf("expected source %q, got %q", metricsSourcePostgres, body.Source)
	}
}

func TestObservability_SummaryWithRangeCountsWindow(t *testing.T) {
	s, pool := obsDBServer(t, "/api/v1/observability/metrics/summary")

	now := time.Now().UTC()
	// One event inside the 1h window, one two hours old.
	obsSeedEvent(t, pool, "pg-win-in", now.Add(-10*time.Minute))
	obsSeedEvent(t, pool, "pg-win-out", now.Add(-2*time.Hour))
	obsSeedTask(t, pool, "pg-win-in", "agent-a", now.Add(-9*time.Minute))
	obsSeedErrorLog(t, pool, now.Add(-5*time.Minute))
	obsSeedErrorLog(t, pool, now.Add(-30*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary?range=1h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body MetricsSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.EventsConsumed != 1 {
		t.Errorf("eventsConsumed = %v, want 1 (the 2h-old event must be outside the window)", body.EventsConsumed)
	}
	if body.TriggersMatched != 1 || body.EventsRouted != 1 {
		t.Errorf("unexpected task counts: %+v", body)
	}
	if body.RoutingErrors != 1 {
		t.Errorf("routingErrors = %v, want 1 (the 30h-old error is outside 1h)", body.RoutingErrors)
	}
}

func TestObservability_SummaryReturns503WithNoBackendAtAll(t *testing.T) {
	// Neither Prometheus nor the hub's own records. The console still needs a
	// 503 to label, which is the contract its unavailable state depends on.
	// testServer wires no stores, so that is exactly this state.
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/observability/metrics/summary", s.handleObservability)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != metricsRequiredMessage {
		t.Errorf("expected %q, got %q", metricsRequiredMessage, body.Error)
	}
}

// --- timeseries from the hub's own records ---

func TestObservability_TimeseriesFromPostgresIsZeroFilled(t *testing.T) {
	s, pool := obsDBServer(t, "/api/v1/observability/metrics/timeseries")

	now := time.Now().UTC().Truncate(time.Minute)
	// Three events at one instant. The 1h range steps at 30s, so the response
	// must be 120 points whose sum equals what was seeded.
	for _, id := range []string{"pg-ts-a", "pg-ts-b", "pg-ts-c"} {
		obsSeedEvent(t, pool, id, now)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/observability/metrics/timeseries?metric=events_consumed&range=1h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body MetricsTimeseries
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Source != metricsSourcePostgres {
		t.Errorf("expected source %q, got %q", metricsSourcePostgres, body.Source)
	}
	// Dense output is what the chart requires: bars are placed by index, so a
	// sparse response would draw three bars spread across the whole hour and
	// misreport when they happened.
	if len(body.Points) != 120 {
		t.Fatalf("expected 120 dense points for a 1h/30s window, got %d", len(body.Points))
	}
	var sum float64
	nonZero := []int{}
	for i, p := range body.Points {
		sum += p.Value
		if p.Value > 0 {
			nonZero = append(nonZero, i)
		}
	}
	if sum != 3 {
		t.Errorf("points sum = %v, want 3", sum)
	}
	// The seeded events are within the last minute of the window, and the
	// handler's window end moves a few milliseconds after `now` is taken, so
	// they land together in the final or penultimate 30s bucket.
	if len(nonZero) != 1 || nonZero[0] < 118 {
		t.Errorf("expected one populated bucket at the end of the window, got %v", nonZero)
	}
}

func TestObservability_TimeseriesErrorsFromPostgres(t *testing.T) {
	s, pool := obsDBServer(t, "/api/v1/observability/metrics/timeseries")

	now := time.Now().UTC().Truncate(time.Minute)
	obsSeedErrorLog(t, pool, now.Add(-time.Minute))
	obsSeedErrorLog(t, pool, now.Add(-2*time.Minute))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/observability/metrics/timeseries?metric=routing_errors&range=1h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body MetricsTimeseries
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var sum float64
	for _, p := range body.Points {
		sum += p.Value
	}
	if sum != 2 {
		t.Errorf("error points sum = %v, want 2", sum)
	}
}

func TestObservability_TimeseriesRejectsMetricWithNoRecords(t *testing.T) {
	// The metric registry is shared between backends: a name the hub's records
	// cannot answer must not quietly return an empty chart.
	s, _ := obsDBServer(t, "/api/v1/observability/metrics/timeseries")

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/observability/metrics/timeseries?metric=%s&range=1h", "hub_something_new"), nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown metric, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- panels that never needed Prometheus must stop asking for it ---

func TestObservability_TokensByEventServesWithoutProm(t *testing.T) {
	// This panel reads task_conversations through the hub's log store. It used to
	// sit behind the Prometheus gate, so it 503'd on every standalone install
	// for a dependency it does not have.
	s, _ := obsDBServer(t, "/api/v1/observability/metrics/tokens/by-event")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/by-event?range=24h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from the hub's own records, got %d: %s", rec.Code, rec.Body.String())
	}
	var body TokensByEvent
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Range != "24h" {
		t.Errorf("unexpected envelope: %+v", body)
	}
}

func TestObservability_TokenPanelsStillNamePrometheus(t *testing.T) {
	// Token metrics are published by the agent runtime to Prometheus only — the
	// hub's records have no cache-token columns yet (issue #281), so these
	// panels cannot be answered from Postgres. Their 503 must name the missing
	// backend instead of the generic text the console renders as
	// "Telemetry not configured".
	s, _ := obsDBServer(t, "/api/v1/observability/metrics/tokens/summary")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/summary?range=24h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != promRequiredMessage {
		t.Errorf("expected the prometheus-specific message, got %q", body.Error)
	}
}

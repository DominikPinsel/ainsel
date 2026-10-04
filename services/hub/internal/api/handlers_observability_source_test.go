package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
	"github.com/DominikPinsel/ainsel/services/hub/internal/telemetry"
)

// These tests pin *which* backend answers the event metric panels. The panel
// content itself is covered by handlers_observability_test.go (Prometheus) and
// handlers_observability_pg_test.go (the hub's records); what is decided here is
// the choice between them when a hub has both, because that choice is the
// difference an operator sees as numbers on the dashboard.

// recordsStoreWithoutDatabase returns a telemetry store that reports Ready() but
// cannot serve a query. pgx builds a pool lazily, so this needs no server and no
// Docker: the pool exists, which is all Ready() asks, and anything that tries to
// use it is refused on 127.0.0.1:1 inside the connect timeout.
//
// Tests that use it assert *which backend was chosen* — by watching whether the
// fake Prometheus was called at all, and by the error a failed database read
// produces — rather than asserting figures, which needs real rows and so lives
// in the container-backed suite.
func recordsStoreWithoutDatabase(t *testing.T) *telemetry.Store {
	t.Helper()
	pool, err := pgxpool.New(context.Background(),
		"postgres://ainsel-test:invalid@127.0.0.1:1/ainsel_test?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("build lazy pool: %v", err)
	}
	t.Cleanup(pool.Close)
	store := telemetry.NewStore(pool)
	if !store.Ready() {
		t.Fatal("lazy pool store reports not Ready; this helper's premise is wrong")
	}
	return store
}

// countingProm is a fake Prometheus that also records how many queries reached
// it, which is how these tests tell "answered from the records" apart from
// "answered from Prometheus, records never tried".
func countingProm(t *testing.T, value string) (*prometheus.Client, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(vectorResponse([]vectorSample{
			{Labels: map[string]string{"__name__": "hub_events_consumed_total"}, Value: value},
		}))
	}))
	t.Cleanup(srv.Close)
	return prometheus.NewClient(srv.URL, nil), &hits
}

// sourceTestServer builds a server with both backends configured, pinned to the
// given metricsSource, listening on the paths the test will call.
func sourceTestServer(t *testing.T, pin string, paths ...string) (*Server, *atomic.Int64) {
	t.Helper()
	prom, hits := countingProm(t, "7")
	s := testServer(t)
	s.prom = prom
	s.SetTelemetryStore(recordsStoreWithoutDatabase(t))
	s.SetMetricsSource(pin)
	for _, p := range paths {
		s.mux.HandleFunc(p, s.handleObservability)
	}
	return s, hits
}

func TestMetricsBackendChoosesRecordsUnlessPinned(t *testing.T) {
	// The decision table. records-beats-prometheus is the change: before it, an
	// install that had ever configured Prometheus could not get the hub's own
	// ledger onto the hub's own dashboard, and a hub pod restart reset the KPI
	// cards to the new pod's counters.
	cases := []struct {
		name       string
		pin        string
		hasProm    bool
		hasRecords bool
		wantSource string
	}{
		{name: "default with both", pin: "", hasProm: true, hasRecords: true, wantSource: metricsSourcePostgres},
		{name: "default records only", pin: "", hasProm: false, hasRecords: true, wantSource: metricsSourcePostgres},
		{name: "default prom only", pin: "", hasProm: true, hasRecords: false, wantSource: metricsSourcePrometheus},
		{name: "default neither", pin: "", hasProm: false, hasRecords: false},
		{name: "pin records with both", pin: MetricsSourceRecords, hasProm: true, hasRecords: true, wantSource: metricsSourcePostgres},
		{name: "pin records falls back to prom", pin: MetricsSourceRecords, hasProm: true, hasRecords: false, wantSource: metricsSourcePrometheus},
		{name: "pin prom with both", pin: MetricsSourcePrometheus, hasProm: true, hasRecords: true, wantSource: metricsSourcePrometheus},
		{name: "pin prom without client", pin: MetricsSourcePrometheus, hasProm: false, hasRecords: true},
		{name: "unrecognised pin uses the default", pin: "victoriametrics", hasProm: true, hasRecords: true, wantSource: metricsSourcePostgres},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(t)
			s.SetMetricsSource(tc.pin)
			if tc.hasProm {
				prom, _ := countingProm(t, "7")
				s.prom = prom
			}
			if tc.hasRecords {
				s.SetTelemetryStore(recordsStoreWithoutDatabase(t))
			}

			source, reason := s.metricsBackend()
			if source != tc.wantSource {
				t.Errorf("source = %q, want %q (reason %q)", source, tc.wantSource, reason)
			}
			if source == "" && reason == "" {
				t.Error("an unavailable backend must come with a reason to report")
			}
			// A pin that is honoured but cannot be served must name the dependency
			// the operator has to configure, not offer the other backend as advice.
			if tc.pin == MetricsSourcePrometheus && source == "" && reason != promRequiredMessage {
				t.Errorf("pinned Prometheus with no client: reason = %q, want %q", reason, promRequiredMessage)
			}
		})
	}
}

func TestObservability_SummaryDoesNotQueryPrometheusWhenRecordsAnswer(t *testing.T) {
	const path = "/api/v1/observability/metrics/summary"
	s, hits := sourceTestServer(t, "", path)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

	// The records store has no reachable database, so choosing it fails at the
	// query — a 502 that names the hub database. That is the proof of the choice:
	// a hub that asked Prometheus would have answered 200 with the fake's 7.
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected the records read to be attempted (502), got %d: %s", rec.Code, rec.Body.String())
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("Prometheus was queried %d times; the hub's records are the default backend", n)
	}
	if !strings.Contains(rec.Body.String(), "hub database") {
		t.Errorf("expected the failure to name the hub database, got %s", rec.Body.String())
	}
}

func TestObservability_SummaryServesPrometheusWhenPinned(t *testing.T) {
	const path = "/api/v1/observability/metrics/summary"
	// The escape hatch: an operator who wants counters gets counters even though
	// the hub could answer for itself.
	s, hits := sourceTestServer(t, MetricsSourcePrometheus, path)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from the pinned counters, got %d: %s", rec.Code, rec.Body.String())
	}
	if n := hits.Load(); n == 0 {
		t.Error("pinned Prometheus produced no queries")
	}
	var body MetricsSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Source != metricsSourcePrometheus {
		t.Errorf("source = %q, want %q", body.Source, metricsSourcePrometheus)
	}
	if body.EventsConsumed != 7 {
		t.Errorf("eventsConsumed = %v, want 7 from the counters", body.EventsConsumed)
	}
}

func TestObservability_SummaryPinnedPrometheusWithoutClientIs503(t *testing.T) {
	const path = "/api/v1/observability/metrics/summary"
	// The pin is not a preference to be quietly overridden. This hub has working
	// records, and answering from them would report rows under the "prometheus"
	// label the operator explicitly asked for; the 503 says what to fix instead.
	s, hits := sourceTestServer(t, MetricsSourcePrometheus, path)
	s.prom = nil

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("Prometheus was queried %d times with no client configured", n)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != promRequiredMessage {
		t.Errorf("error = %q, want %q", body.Error, promRequiredMessage)
	}
}

func TestObservability_TimeseriesUsesRecordsByDefault(t *testing.T) {
	s, hits := sourceTestServer(t, "", "/api/v1/observability/metrics/timeseries")
	path := "/api/v1/observability/metrics/timeseries?metric=events_consumed&range=1h"

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected the records read to be attempted (502), got %d: %s", rec.Code, rec.Body.String())
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("Prometheus was queried %d times; timeseries must follow the same default as the summary", n)
	}
}

func TestObservability_TokenPanelsIgnoreTheRecordsDefault(t *testing.T) {
	// The panels with no records behind them stay Prometheus-gated whichever way
	// the event-metric default points, so flipping that default cannot make them
	// answer from a table that has nothing to say.
	const path = "/api/v1/observability/metrics/tokens/summary"
	s, _ := sourceTestServer(t, "", path)

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path+"?range=1h", nil))

	if rec.Code == http.StatusBadGateway {
		t.Fatalf("token panel was routed to the records backend: %s", rec.Body.String())
	}
}

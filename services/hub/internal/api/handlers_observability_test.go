package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// fakePromServer returns an httptest server that pretends to be Prometheus.
// The handler is given the parsed query params and returns the JSON-encoded body.
func fakePromServer(t *testing.T, handler func(path string, params url.Values) interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := handler(r.URL.Path, r.URL.Query())
		_ = json.NewEncoder(w).Encode(body)
	}))
}

// vectorResponse builds a Prometheus instant-query response from {labels: value} pairs.
// Each entry in samples is (metric labels, value as string).
func vectorResponse(samples []vectorSample) interface{} {
	results := make([]map[string]interface{}, 0, len(samples))
	for _, s := range samples {
		results = append(results, map[string]interface{}{
			"metric": s.Labels,
			"value":  []interface{}{1715100000.0, s.Value},
		})
	}
	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"resultType": "vector",
			"result":     results,
		},
	}
}

type vectorSample struct {
	Labels map[string]string
	Value  string
}

// matrixResponse builds a Prometheus range-query response with one series.
func matrixResponse(samples [][2]float64) interface{} {
	values := make([][]interface{}, 0, len(samples))
	for _, s := range samples {
		values = append(values, []interface{}{s[0], fmt.Sprintf("%g", s[1])})
	}
	return map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"resultType": "matrix",
			"result": []map[string]interface{}{
				{
					"metric": map[string]string{},
					"values": values,
				},
			},
		},
	}
}

func newServerWithProm(t *testing.T, prom *prometheus.Client, objects ...runtime.Object) *Server {
	t.Helper()
	s := testServer(t, objects...)
	s.prom = prom
	s.mux.HandleFunc("/api/v1/observability/metrics/summary", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/timeseries", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/agents", s.handleObservability)
	// Deprecated aliases — same dispatcher, different path. Registered here so
	// the alias-specific tests can exercise the same fake Prometheus.
	s.mux.HandleFunc("/api/v1/metrics/summary", s.handleObservability)
	s.mux.HandleFunc("/api/v1/metrics/timeseries", s.handleObservability)
	s.mux.HandleFunc("/api/v1/metrics/agents", s.handleObservability)
	return s
}

// --- summary ---

func TestObservability_SummaryReturns503WhenPromNotConfigured(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/observability/metrics/summary", s.handleObservability)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestObservability_SummaryAggregatesHubCounters(t *testing.T) {
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		if path != "/api/v1/query" {
			t.Fatalf("unexpected path: %s", path)
		}
		q := params.Get("query")
		switch {
		case strings.Contains(q, "hub_events_consumed_total"):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "42"}})
		case strings.Contains(q, "hub_triggers_matched_total"):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "30"}})
		case strings.Contains(q, "hub_events_routed_total"):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "29"}})
		case strings.Contains(q, "hub_routing_errors_total"):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "1"}})
		}
		t.Fatalf("unexpected query: %s", q)
		return nil
	})
	defer srv.Close()

	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body MetricsSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.EventsConsumed != 42 || body.TriggersMatched != 30 || body.EventsRouted != 29 || body.RoutingErrors != 1 {
		t.Errorf("unexpected summary: %+v", body)
	}
	if body.UpdatedAt.IsZero() {
		t.Error("expected updatedAt to be populated")
	}
}

func TestObservability_SummaryHandlesEmptyResults(t *testing.T) {
	// Fresh hub: no samples scraped yet — should return zeros, not 5xx.
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		return vectorResponse(nil)
	})
	defer srv.Close()

	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body MetricsSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.EventsConsumed != 0 || body.RoutingErrors != 0 {
		t.Errorf("expected zeros, got %+v", body)
	}
}

func TestObservability_SummaryCaches(t *testing.T) {
	var calls atomic.Int32
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		calls.Add(1)
		return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "1"}})
	})
	defer srv.Close()

	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary", nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: expected 200, got %d", i, rec.Code)
		}
	}

	// 4 metrics in the registry => 4 prom calls on the first request, 0 on the others.
	if got := calls.Load(); got != 4 {
		t.Errorf("expected 4 prometheus calls (cached after first), got %d", got)
	}
}

// --- timeseries ---

func TestObservability_TimeseriesDefaultsToEventsConsumedWhenMetricOmitted(t *testing.T) {
	var capturedQuery string
	srv := fakePromServer(t, func(_ string, params url.Values) interface{} {
		capturedQuery = params.Get("query")
		return matrixResponse(nil)
	})
	defer srv.Close()
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/timeseries", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(capturedQuery, "hub_events_consumed_total") {
		t.Errorf("expected default metric to be events_consumed, got query %q", capturedQuery)
	}
	var body MetricsTimeseries
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Metric != "events_consumed" {
		t.Errorf("expected response metric=events_consumed, got %q", body.Metric)
	}
}

func TestObservability_TimeseriesRejectsUnknownMetric(t *testing.T) {
	srv := fakePromServer(t, func(string, url.Values) interface{} { return nil })
	defer srv.Close()
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/timeseries?metric=nonsense", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestObservability_TimeseriesRejectsUnknownRange(t *testing.T) {
	srv := fakePromServer(t, func(string, url.Values) interface{} { return nil })
	defer srv.Close()
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/timeseries?metric=events_consumed&range=42y", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestObservability_TimeseriesUsesRateQueryAndReturnsPoints(t *testing.T) {
	var capturedQuery string
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		if path != "/api/v1/query_range" {
			t.Fatalf("expected query_range, got %s", path)
		}
		capturedQuery = params.Get("query")
		return matrixResponse([][2]float64{
			{1715100000.0, 0.5},
			{1715100060.0, 0.7},
			{1715100120.0, 0.9},
		})
	})
	defer srv.Close()

	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/timeseries?metric=events_consumed&range=1h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(capturedQuery, "rate(hub_events_consumed_total[1m])") {
		t.Errorf("expected rate query for 1h range, got %q", capturedQuery)
	}
	var body MetricsTimeseries
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Metric != "events_consumed" || body.Range != "1h" {
		t.Errorf("unexpected envelope: %+v", body)
	}
	if len(body.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(body.Points))
	}
	if body.Points[2].Value != 0.9 {
		t.Errorf("expected last value 0.9, got %v", body.Points[2].Value)
	}
}

func TestObservability_TimeseriesDefaultRangeIs1h(t *testing.T) {
	var capturedQuery string
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		capturedQuery = params.Get("query")
		return matrixResponse(nil)
	})
	defer srv.Close()
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/timeseries?metric=routing_errors", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// 1h range => "1m" rate window.
	if !strings.Contains(capturedQuery, "[1m]") {
		t.Errorf("expected default range to use 1m rate window, got %q", capturedQuery)
	}
}

// --- agents ---

func TestObservability_AgentsAggregatesTokensAndInvocations(t *testing.T) {
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		q := params.Get("query")
		switch {
		case strings.Contains(q, "agent_tokens_used_total"):
			return vectorResponse([]vectorSample{
				{Labels: map[string]string{"agent": "dev", "token_type": "input"}, Value: "1500"},
				{Labels: map[string]string{"agent": "dev", "token_type": "output"}, Value: "300"},
				{Labels: map[string]string{"agent": "reviewer", "token_type": "input"}, Value: "800"},
				{Labels: map[string]string{"agent": "reviewer", "token_type": "output"}, Value: "200"},
			})
		case strings.Contains(q, "agent_invocations_total"):
			return vectorResponse([]vectorSample{
				{Labels: map[string]string{"agent": "dev"}, Value: "5"},
				{Labels: map[string]string{"agent": "reviewer"}, Value: "3"},
			})
		}
		t.Fatalf("unexpected query: %s", q)
		return nil
	})
	defer srv.Close()

	devAgent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "dev", Namespace: "test-ns"},
		Spec:       agentv1alpha1.AgentSpec{DisplayName: "Dev Bot"},
	}
	revAgent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "reviewer", Namespace: "test-ns"},
		Spec:       agentv1alpha1.AgentSpec{DisplayName: "Reviewer Bot"},
	}
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil), devAgent, revAgent)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/agents", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body AgentsMetricsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d (%+v)", len(body.Agents), body.Agents)
	}
	// Sorted alphabetically: dev, reviewer.
	dev := body.Agents[0]
	rev := body.Agents[1]
	if dev.Agent != "dev" || rev.Agent != "reviewer" {
		t.Errorf("unexpected ordering: %+v", body.Agents)
	}
	if dev.AgentName != "Dev Bot" {
		t.Errorf("dev agentName wrong: got %q", dev.AgentName)
	}
	if rev.AgentName != "Reviewer Bot" {
		t.Errorf("reviewer agentName wrong: got %q", rev.AgentName)
	}
	if dev.InputTokens != 1500 || dev.OutputTokens != 300 || dev.TotalTokens != 1800 {
		t.Errorf("dev tokens wrong: %+v", dev)
	}
	if dev.Invocations != 5 {
		t.Errorf("dev invocations: expected 5, got %v", dev.Invocations)
	}
}

func TestObservability_AgentsFallsBackToIDWhenAgentMissing(t *testing.T) {
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		q := params.Get("query")
		if strings.Contains(q, "agent_tokens_used_total") {
			return vectorResponse([]vectorSample{
				{Labels: map[string]string{"agent": "ghost", "token_type": "input"}, Value: "100"},
			})
		}
		return vectorResponse(nil)
	})
	defer srv.Close()
	// No agents seeded.
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/agents", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body AgentsMetricsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %+v", body.Agents)
	}
	if body.Agents[0].Agent != "ghost" {
		t.Errorf("expected agent=ghost, got %q", body.Agents[0].Agent)
	}
	if body.Agents[0].AgentName != "ghost" {
		t.Errorf("expected agentName fallback to ID, got %q", body.Agents[0].AgentName)
	}
}

func TestObservability_AgentsReturns503WhenPromNotConfigured(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/observability/metrics/agents", s.handleObservability)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/agents", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestObservability_RejectsNonGet(t *testing.T) {
	srv := fakePromServer(t, func(string, url.Values) interface{} { return nil })
	defer srv.Close()
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/observability/metrics/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// --- deprecated aliases ---
//
// The deployed ainsel-hub-frontend `main` bundle calls
// /api/v1/metrics/summary directly (see src/api/metrics/index.ts in that
// repo). We accept those legacy paths so the deployed dashboard widget stops
// 404'ing, but tag responses with RFC 8594 + RFC 8288 headers so callers can
// detect the alias and migrate to the canonical /api/v1/observability/...
// paths used by the dashboard rewrite.

func TestObservability_LegacySummaryAliasReturnsSameBodyWithDeprecationHeader(t *testing.T) {
	srv := fakePromServer(t, func(_ string, _ url.Values) interface{} {
		return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "7"}})
	})
	defer srv.Close()
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Deprecation"); got != "true" {
		t.Errorf(`expected Deprecation: "true", got %q`, got)
	}
	wantLink := `</api/v1/observability/metrics/summary>; rel="successor-version"`
	if got := rec.Header().Get("Link"); got != wantLink {
		t.Errorf("expected Link %q, got %q", wantLink, got)
	}
	var body MetricsSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// All four counters used the same 7-valued fake response.
	if body.EventsConsumed != 7 || body.TriggersMatched != 7 || body.EventsRouted != 7 || body.RoutingErrors != 7 {
		t.Errorf("expected aliased summary to mirror canonical body, got %+v", body)
	}
}

func TestObservability_LegacyTimeseriesAliasSetsDeprecationHeader(t *testing.T) {
	srv := fakePromServer(t, func(_ string, _ url.Values) interface{} {
		return matrixResponse([][2]float64{{1715100000.0, 1.0}})
	})
	defer srv.Close()
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/timeseries?metric=events_consumed", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Deprecation"); got != "true" {
		t.Errorf("expected Deprecation header, got %q", got)
	}
	wantLink := `</api/v1/observability/metrics/timeseries>; rel="successor-version"`
	if got := rec.Header().Get("Link"); got != wantLink {
		t.Errorf("expected Link %q, got %q", wantLink, got)
	}
}

func TestObservability_LegacyAgentsAliasSetsDeprecationHeader(t *testing.T) {
	srv := fakePromServer(t, func(_ string, _ url.Values) interface{} {
		return vectorResponse(nil)
	})
	defer srv.Close()
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/agents", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Deprecation"); got != "true" {
		t.Errorf("expected Deprecation header, got %q", got)
	}
	wantLink := `</api/v1/observability/metrics/agents>; rel="successor-version"`
	if got := rec.Header().Get("Link"); got != wantLink {
		t.Errorf("expected Link %q, got %q", wantLink, got)
	}
}

func TestObservability_LegacyAliasReturns503WhenPromNotConfigured(t *testing.T) {
	// The 503-when-unconfigured contract must hold on the alias path too — the
	// frontend explicitly handles 503 on these endpoints.
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/metrics/summary", s.handleObservability)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Deprecation"); got != "true" {
		t.Errorf("expected Deprecation header on 503 response too, got %q", got)
	}
}

func TestObservability_CanonicalPathHasNoDeprecationHeader(t *testing.T) {
	// Sanity check: the canonical /api/v1/observability/... path must not be
	// flagged as deprecated.
	srv := fakePromServer(t, func(_ string, _ url.Values) interface{} {
		return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "1"}})
	})
	defer srv.Close()
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Deprecation"); got != "" {
		t.Errorf("canonical path should not be marked deprecated, got Deprecation: %q", got)
	}
}

// --- range-aware summary ---

func TestObservability_SummaryWithRangeUsesIncreaseQueries(t *testing.T) {
	var capturedQueries []string
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		capturedQueries = append(capturedQueries, params.Get("query"))
		return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "5"}})
	})
	defer srv.Close()

	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary?range=6h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// All four queries should use increase() with the 6h range.
	for _, q := range capturedQueries {
		if !strings.Contains(q, "increase(") || !strings.Contains(q, "[6h]") {
			t.Errorf("expected increase query with [6h], got %q", q)
		}
	}
}

func TestObservability_SummaryWithRangeRoundsFractionalCounts(t *testing.T) {
	// increase() extrapolates fractional values (e.g. 64.7826); the summary
	// reports event counts, so values must be rounded to whole numbers before
	// reaching the dashboard KPI cards.
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		q := params.Get("query")
		switch {
		case strings.Contains(q, "hub_events_consumed_total"):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "64.7826"}})
		case strings.Contains(q, "hub_triggers_matched_total"):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "12.3"}})
		case strings.Contains(q, "hub_events_routed_total"):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "0.4"}})
		case strings.Contains(q, "hub_routing_errors_total"):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "1.5"}})
		}
		t.Fatalf("unexpected query: %s", q)
		return nil
	})
	defer srv.Close()

	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary?range=24h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body MetricsSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.EventsConsumed != 65 || body.TriggersMatched != 12 || body.EventsRouted != 0 || body.RoutingErrors != 2 {
		t.Errorf("expected rounded counts, got %+v", body)
	}
}

func TestObservability_SummaryRejectsInvalidRange(t *testing.T) {
	srv := fakePromServer(t, func(string, url.Values) interface{} { return nil })
	defer srv.Close()
	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary?range=42y", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestObservability_SummaryWithoutRangePreservesLegacyBehavior(t *testing.T) {
	// Without ?range=, the handler must use raw counter queries (no increase())
	// to preserve backward compatibility.
	var capturedQueries []string
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		capturedQueries = append(capturedQueries, params.Get("query"))
		return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "10"}})
	})
	defer srv.Close()

	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, q := range capturedQueries {
		if strings.Contains(q, "increase(") {
			t.Errorf("expected raw counter query without increase(), got %q", q)
		}
	}
}

func TestObservability_SummaryRangeCacheIsolation(t *testing.T) {
	// Different ranges must not share cache entries.
	var calls atomic.Int32
	srv := fakePromServer(t, func(_ string, _ url.Values) interface{} {
		calls.Add(1)
		return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "1"}})
	})
	defer srv.Close()

	s := newServerWithProm(t, prometheus.NewClient(srv.URL, nil))

	// First call with range=1h
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary?range=1h", nil)
	rec1 := httptest.NewRecorder()
	s.mux.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}
	firstCalls := calls.Load()

	// Second call with range=6h — must hit Prometheus again (different cache key).
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/summary?range=6h", nil)
	rec2 := httptest.NewRecorder()
	s.mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
	if calls.Load() == firstCalls {
		t.Error("expected additional Prometheus calls for different range, but cache was hit")
	}
}

// --- cache unit tests ---

func TestPromCache_TTLExpires(t *testing.T) {
	c := newPromCache(20 * time.Millisecond)
	c.set("k", "v")
	if v, ok := c.get("k"); !ok || v != "v" {
		t.Fatalf("expected fresh hit, got ok=%v v=%v", ok, v)
	}
	time.Sleep(50 * time.Millisecond)
	if _, ok := c.get("k"); ok {
		t.Fatal("expected entry to be expired")
	}
}

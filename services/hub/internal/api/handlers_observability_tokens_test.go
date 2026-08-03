package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// newServerWithTokensProm registers the canonical tokens routes onto a server
// wired with the given fake Prometheus client and optional seeded K8s objects.
func newServerWithTokensProm(t *testing.T, prom *prometheus.Client, objects ...runtime.Object) *Server {
	t.Helper()
	s := testServer(t, objects...)
	s.prom = prom
	s.mux.HandleFunc("/api/v1/observability/metrics/tokens/summary", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/tokens/timeseries", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/tokens/by-subject", s.handleObservability)
	s.mux.HandleFunc("/api/v1/observability/metrics/tokens/by-event", s.handleObservability)
	return s
}

// --- summary -----------------------------------------------------------------

func TestTokensSummary_ReturnsCurrentAndPriorWindowTotals(t *testing.T) {
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		q := params.Get("query")
		switch {
		case strings.Contains(q, `token_type="input"`):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "1500"}})
		case strings.Contains(q, `token_type="output"`):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "300"}})
		case strings.Contains(q, "offset 24h"):
			return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "1200"}})
		}
		t.Fatalf("unexpected query: %s", q)
		return nil
	})
	defer srv.Close()

	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body TokensSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.InputTokens != 1500 || body.OutputTokens != 300 || body.TotalTokens != 1800 {
		t.Errorf("token totals wrong: %+v", body)
	}
	if body.PreviousTotalTokens != 1200 {
		t.Errorf("previous total wrong: %v", body.PreviousTotalTokens)
	}
	if body.UpdatedAt.IsZero() {
		t.Error("expected updatedAt to be populated")
	}
}

func TestTokensSummary_HandlesEmptyResults(t *testing.T) {
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		return vectorResponse(nil)
	})
	defer srv.Close()

	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body TokensSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.InputTokens != 0 || body.OutputTokens != 0 || body.TotalTokens != 0 || body.PreviousTotalTokens != 0 {
		t.Errorf("expected zeros, got %+v", body)
	}
}

func TestTokensSummary_Returns503WhenPromNotConfigured(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/observability/metrics/tokens/summary", s.handleObservability)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestTokensSummary_AcceptsRangeParam(t *testing.T) {
	var capturedQueries []string
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		capturedQueries = append(capturedQueries, params.Get("query"))
		return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "100"}})
	})
	defer srv.Close()

	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/summary?range=1h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// All queries should use [1h] range.
	for _, q := range capturedQueries {
		if !strings.Contains(q, "[1h]") {
			t.Errorf("expected query with [1h] range, got %q", q)
		}
	}
}

func TestTokensSummary_RejectsInvalidRange(t *testing.T) {
	srv := fakePromServer(t, func(string, url.Values) interface{} { return nil })
	defer srv.Close()
	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/summary?range=42y", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTokensSummary_DefaultRangeIs24h(t *testing.T) {
	// Without ?range=, the handler should default to 24h and use [24h] in queries.
	var capturedQueries []string
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		capturedQueries = append(capturedQueries, params.Get("query"))
		return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "50"}})
	})
	defer srv.Close()

	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/summary", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, q := range capturedQueries {
		if !strings.Contains(q, "[24h]") {
			t.Errorf("expected default query with [24h] range, got %q", q)
		}
	}
}

// --- timeseries --------------------------------------------------------------

func TestTokensTimeseries_ReturnsPoints(t *testing.T) {
	var capturedQuery string
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		if path != "/api/v1/query_range" {
			t.Fatalf("expected query_range, got %s", path)
		}
		capturedQuery = params.Get("query")
		return matrixResponse([][2]float64{
			{1715100000.0, 100},
			{1715101800.0, 250},
			{1715103600.0, 75},
		})
	})
	defer srv.Close()

	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/timeseries?range=24h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(capturedQuery, "agent_tokens_used_total") {
		t.Errorf("expected agent_tokens_used_total in query, got %q", capturedQuery)
	}
	var body TokensTimeseries
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Range != "24h" {
		t.Errorf("expected range=24h, got %q", body.Range)
	}
	if len(body.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(body.Points))
	}
	if body.Points[1].Value != 250 {
		t.Errorf("expected mid-point value 250, got %v", body.Points[1].Value)
	}
}

func TestTokensTimeseries_DefaultRangeIs24h(t *testing.T) {
	srv := fakePromServer(t, func(string, url.Values) interface{} { return matrixResponse(nil) })
	defer srv.Close()
	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/timeseries", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body TokensTimeseries
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Range != "24h" {
		t.Errorf("expected default range=24h, got %q", body.Range)
	}
}

func TestTokensTimeseries_RejectsUnsupportedRange(t *testing.T) {
	srv := fakePromServer(t, func(string, url.Values) interface{} { return matrixResponse(nil) })
	defer srv.Close()
	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/timeseries?range=7d", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for 7d range, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- by-subject --------------------------------------------------------------

func TestTokensBySubject_AggregatesRows(t *testing.T) {
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		q := params.Get("query")
		switch {
		case strings.Contains(q, "agent_tokens_used_total"):
			return vectorResponse([]vectorSample{
				{Labels: map[string]string{"agent": "developer", "repo": "frontend", "event_type": "pull_request.opened", "model": "gpt-4", "token_type": "input"}, Value: "10000"},
				{Labels: map[string]string{"agent": "developer", "repo": "frontend", "event_type": "pull_request.opened", "model": "gpt-4", "token_type": "output"}, Value: "2000"},
				{Labels: map[string]string{"agent": "architect", "repo": "backend", "event_type": "issue.assigned", "model": "claude-3", "token_type": "input"}, Value: "5000"},
				{Labels: map[string]string{"agent": "architect", "repo": "backend", "event_type": "issue.assigned", "model": "claude-3", "token_type": "output"}, Value: "1500"},
			})
		}
		t.Fatalf("unexpected query: %s", q)
		return nil
	})
	defer srv.Close()

	devAgent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "developer", Namespace: "test-ns"},
		Spec:       agentv1alpha1.AgentSpec{DisplayName: "Developer Bot"},
	}
	archAgent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "architect", Namespace: "test-ns"},
		Spec:       agentv1alpha1.AgentSpec{DisplayName: "Architect Bot"},
	}
	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil), devAgent, archAgent)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/by-subject?range=24h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body TokensBySubject
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Range != "24h" {
		t.Errorf("expected range=24h, got %q", body.Range)
	}
	if len(body.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d (%+v)", len(body.Rows), body.Rows)
	}
	// Sorted by repo asc: backend then frontend.
	if body.Rows[0].Repo != "backend" || body.Rows[1].Repo != "frontend" {
		t.Errorf("rows out of order: %+v", body.Rows)
	}
	architect := body.Rows[0]
	developer := body.Rows[1]
	if architect.Agent != "architect" || architect.EventType != "issue.assigned" || architect.Model != "claude-3" {
		t.Errorf("architect row mislabeled: %+v", architect)
	}
	if architect.AgentName != "Architect Bot" {
		t.Errorf("architect agentName wrong: got %q", architect.AgentName)
	}
	if architect.InputTokens != 5000 || architect.OutputTokens != 1500 || architect.TotalTokens != 6500 {
		t.Errorf("architect tokens wrong: %+v", architect)
	}
	if developer.Agent != "developer" || developer.EventType != "pull_request.opened" || developer.Model != "gpt-4" {
		t.Errorf("developer row mislabeled: %+v", developer)
	}
	if developer.AgentName != "Developer Bot" {
		t.Errorf("developer agentName wrong: got %q", developer.AgentName)
	}
	if developer.InputTokens != 10000 || developer.OutputTokens != 2000 || developer.TotalTokens != 12000 {
		t.Errorf("developer tokens wrong: %+v", developer)
	}
}

func TestTokensBySubject_EmptyResultIs200(t *testing.T) {
	srv := fakePromServer(t, func(string, url.Values) interface{} { return vectorResponse(nil) })
	defer srv.Close()
	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/by-subject", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body TokensBySubject
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Rows) != 0 {
		t.Errorf("expected empty rows, got %+v", body.Rows)
	}
}

func TestTokensBySubject_SkipsRowsWithoutAgent(t *testing.T) {
	// A series without an agent label (stale exporter, misconfigured agent) must
	// not produce a row with a blank Agent field — it'd render as an empty row
	// on the dashboard.
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		q := params.Get("query")
		if strings.Contains(q, "agent_tokens_used_total") {
			return vectorResponse([]vectorSample{
				{Labels: map[string]string{"agent": "", "repo": "x", "event_type": "y", "token_type": "input"}, Value: "1"},
				{Labels: map[string]string{"agent": "dev", "repo": "x", "event_type": "y", "token_type": "input"}, Value: "2"},
			})
		}
		return vectorResponse(nil)
	})
	defer srv.Close()
	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/by-subject", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body TokensBySubject
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Rows) != 1 || body.Rows[0].Agent != "dev" {
		t.Errorf("expected one row for dev, got %+v", body.Rows)
	}
}

func TestTokensBySubject_FallsBackToIDWhenAgentMissing(t *testing.T) {
	srv := fakePromServer(t, func(path string, params url.Values) interface{} {
		q := params.Get("query")
		if strings.Contains(q, "agent_tokens_used_total") {
			return vectorResponse([]vectorSample{
				{Labels: map[string]string{"agent": "ghost", "repo": "x", "event_type": "y", "token_type": "input"}, Value: "5"},
			})
		}
		return vectorResponse(nil)
	})
	defer srv.Close()
	// No agents seeded — ghost is not in K8s.
	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/by-subject", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body TokensBySubject
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Rows) != 1 {
		t.Fatalf("expected 1 row, got %+v", body.Rows)
	}
	if body.Rows[0].Agent != "ghost" {
		t.Errorf("expected agent=ghost, got %q", body.Rows[0].Agent)
	}
	if body.Rows[0].AgentName != "ghost" {
		t.Errorf("expected agentName to fallback to ID when missing, got %q", body.Rows[0].AgentName)
	}
}

func TestTokensBySubject_RejectsUnknownRange(t *testing.T) {
	srv := fakePromServer(t, func(string, url.Values) interface{} { return vectorResponse(nil) })
	defer srv.Close()
	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/by-subject?range=42y", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// --- caching across token endpoints -----------------------------------------

func TestTokensSummary_Caches(t *testing.T) {
	var calls atomic.Int32
	srv := fakePromServer(t, func(_ string, _ url.Values) interface{} {
		calls.Add(1)
		return vectorResponse([]vectorSample{{Labels: map[string]string{}, Value: "1"}})
	})
	defer srv.Close()

	s := newServerWithTokensProm(t, prometheus.NewClient(srv.URL, nil))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/summary", nil)
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: expected 200, got %d", i, rec.Code)
		}
	}
	// 3 queries on the first call (input + output + prior); none thereafter.
	if got := calls.Load(); got != 3 {
		t.Errorf("expected 3 prometheus calls (cached after first), got %d", got)
	}
}

// --- by-event ----------------------------------------------------------------

func TestTokensByEvent_Returns503WhenTaskLogsNotConfigured(t *testing.T) {
	s := newServerWithTokensProm(t, prometheus.NewClient("http://localhost:0", nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/by-event?range=24h", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTokensByEvent_RejectsUnknownRange(t *testing.T) {
	s := newServerWithTokensProm(t, prometheus.NewClient("http://localhost:0", nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/by-event?range=42y", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTokensByEvent_DefaultRangeIs24h(t *testing.T) {
	// Without taskLogs configured, the handler returns 503 regardless of range.
	// This test verifies the route is reachable and the default range doesn't
	// cause a 400 (it would if the range validation ran before the taskLogs check).
	s := newServerWithTokensProm(t, prometheus.NewClient("http://localhost:0", nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/metrics/tokens/by-event", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	// 503 because taskLogs is nil, not 400 for range — confirms default range is accepted.
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- aggregateTokensByEvent (unit tests) ------------------------------------

// fakeInvocationGetter is a test double for invocationGetter.
type fakeInvocationGetter struct {
	data map[string]invocations.Invocation
}

func (f *fakeInvocationGetter) Get(id string) (invocations.Invocation, bool) {
	inv, ok := f.data[id]
	return inv, ok
}

func TestAggregateTokensByEvent_AggregatesAndSorts(t *testing.T) {
	// Two invocations map to event "ev-a", one to "ev-b". One invocation has
	// no matching event (evicted). One invocation maps to an invocation with
	// empty EventID (should be skipped).
	store := &fakeInvocationGetter{
		data: map[string]invocations.Invocation{
			"inv-1": {ID: "inv-1", EventID: "ev-a"},
			"inv-2": {ID: "inv-2", EventID: "ev-a"},
			"inv-3": {ID: "inv-3", EventID: "ev-b"},
			"inv-4": {ID: "inv-4", EventID: ""}, // empty EventID → skip
		},
	}

	rows := []tasklogs.InvocationTokenRow{
		{InvocationID: "inv-1", InputTokens: 100, OutputTokens: 50},
		{InvocationID: "inv-2", InputTokens: 200, OutputTokens: 80},
		{InvocationID: "inv-3", InputTokens: 500, OutputTokens: 150},
		{InvocationID: "inv-4", InputTokens: 999, OutputTokens: 999}, // skipped (empty EventID)
		{InvocationID: "inv-5", InputTokens: 10, OutputTokens: 20},   // skipped (not in store)
	}

	result := aggregateTokensByEvent(rows, store)

	// Should have 2 rows: ev-a and ev-b, sorted alphabetically.
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d: %+v", len(result), result)
	}
	if result[0].Event != "ev-a" || result[1].Event != "ev-b" {
		t.Errorf("rows not sorted by event ID: %+v", result)
	}

	// ev-a: inv-1 (100+50) + inv-2 (200+80) = input 300, output 130, total 430
	evA := result[0]
	if evA.InputTokens != 300 || evA.OutputTokens != 130 || evA.TotalTokens != 430 {
		t.Errorf("ev-a tokens wrong: %+v", evA)
	}

	// ev-b: inv-3 (500+150) = input 500, output 150, total 650
	evB := result[1]
	if evB.InputTokens != 500 || evB.OutputTokens != 150 || evB.TotalTokens != 650 {
		t.Errorf("ev-b tokens wrong: %+v", evB)
	}
}

func TestAggregateTokensByEvent_EmptyInputReturnsEmptySlice(t *testing.T) {
	store := &fakeInvocationGetter{data: map[string]invocations.Invocation{}}
	result := aggregateTokensByEvent(nil, store)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %+v", result)
	}
}

func TestAggregateTokensByEvent_SkipsInvocationWithNoEventID(t *testing.T) {
	store := &fakeInvocationGetter{
		data: map[string]invocations.Invocation{
			"inv-1": {ID: "inv-1", EventID: ""},
		},
	}
	rows := []tasklogs.InvocationTokenRow{
		{InvocationID: "inv-1", InputTokens: 100, OutputTokens: 50},
	}
	result := aggregateTokensByEvent(rows, store)
	if len(result) != 0 {
		t.Errorf("expected empty result (no valid event IDs), got %+v", result)
	}
}

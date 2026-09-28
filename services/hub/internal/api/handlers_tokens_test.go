package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
)

// newServerWithTokensRoute wires /api/v1/tokens (the endpoint the MCP
// get_token_usage tool calls) onto a server backed by the given fake
// Prometheus client.
func newServerWithTokensRoute(t *testing.T, prom *prometheus.Client) *Server {
	t.Helper()
	s := testServer(t)
	s.prom = prom
	s.mux.HandleFunc("/api/v1/tokens", s.handleTokens)
	return s
}

// tokensResponse mirrors the {"tokens": [...], "total": {...}} envelope.
type tokensResponse struct {
	Tokens []TokenEntry `json:"tokens"`
	Total  TokenTotals  `json:"total"`
}

func TestListTokens_IncludesCacheComponentsInTotals(t *testing.T) {
	srv := fakePromServer(t, func(_ string, params url.Values) interface{} {
		return vectorResponse([]vectorSample{
			{Labels: map[string]string{"agent": "dev", "repo": "AInsel/ainsel", "issue_id": "42", "model": "glm", "token_type": "input"}, Value: "2400"},
			{Labels: map[string]string{"agent": "dev", "repo": "AInsel/ainsel", "issue_id": "42", "model": "glm", "token_type": "output"}, Value: "900"},
			{Labels: map[string]string{"agent": "dev", "repo": "AInsel/ainsel", "issue_id": "42", "model": "glm", "token_type": "cache_read"}, Value: "30000"},
			{Labels: map[string]string{"agent": "dev", "repo": "AInsel/ainsel", "issue_id": "42", "model": "glm", "token_type": "cache_write"}, Value: "1200"},
		})
	})
	defer srv.Close()

	s := newServerWithTokensRoute(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body tokensResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Tokens) != 1 {
		t.Fatalf("expected the four token_type series merged into 1 entry, got %d (%+v)", len(body.Tokens), body.Tokens)
	}

	entry := body.Tokens[0]
	if entry.Agent != "dev" || entry.Repository != "AInsel/ainsel" || entry.IssueNumber != "42" || entry.Model != "glm" {
		t.Errorf("entry mislabeled: %+v", entry)
	}
	if entry.InputTokens != 2400 || entry.OutputTokens != 900 {
		t.Errorf("entry input/output wrong: %+v", entry)
	}
	if entry.CacheReadTokens != 30000 || entry.CacheWriteTokens != 1200 {
		t.Errorf("entry cache components wrong: %+v", entry)
	}
	// pi's usage.input excludes cache traffic, so a total built from
	// input+output alone would report 3300 instead of the 34500 actually used.
	if entry.TotalTokens != 34500 {
		t.Errorf("entry total must include cache components, got %v", entry.TotalTokens)
	}

	if body.Total.InputTokens != 2400 || body.Total.OutputTokens != 900 {
		t.Errorf("totals input/output wrong: %+v", body.Total)
	}
	if body.Total.CacheReadTokens != 30000 || body.Total.CacheWriteTokens != 1200 {
		t.Errorf("totals cache components wrong: %+v", body.Total)
	}
	if body.Total.TotalTokens != 34500 {
		t.Errorf("grand total must include cache components, got %v", body.Total.TotalTokens)
	}
}

func TestListTokens_MergesMultipleEntriesAndSumsTotals(t *testing.T) {
	srv := fakePromServer(t, func(_ string, params url.Values) interface{} {
		return vectorResponse([]vectorSample{
			{Labels: map[string]string{"agent": "dev", "repo": "a", "issue_id": "1", "model": "m", "token_type": "input"}, Value: "100"},
			{Labels: map[string]string{"agent": "dev", "repo": "a", "issue_id": "1", "model": "m", "token_type": "cache_read"}, Value: "900"},
			{Labels: map[string]string{"agent": "rev", "repo": "b", "issue_id": "2", "model": "m", "token_type": "output"}, Value: "50"},
		})
	})
	defer srv.Close()

	s := newServerWithTokensRoute(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body tokensResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Tokens) != 2 {
		t.Fatalf("expected 2 entries, got %d (%+v)", len(body.Tokens), body.Tokens)
	}
	// Grand total spans both entries: 100 + 900 + 50.
	if body.Total.TotalTokens != 1050 {
		t.Errorf("grand total = %v, want 1050", body.Total.TotalTokens)
	}
	// An entry with no cache series must report zero rather than a stale value.
	for _, e := range body.Tokens {
		if e.Agent == "rev" && (e.CacheReadTokens != 0 || e.CacheWriteTokens != 0 || e.TotalTokens != 50) {
			t.Errorf("rev entry should have no cache traffic: %+v", e)
		}
		if e.Agent == "dev" && e.TotalTokens != 1000 {
			t.Errorf("dev total = %v, want 1000", e.TotalTokens)
		}
	}
}

func TestListTokens_PassesFiltersToPromQL(t *testing.T) {
	var gotQuery string
	srv := fakePromServer(t, func(_ string, params url.Values) interface{} {
		gotQuery = params.Get("query")
		return vectorResponse(nil)
	})
	defer srv.Close()

	s := newServerWithTokensRoute(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tokens?agent=dev&repository=AInsel/ainsel&issueNumber=42", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`agent="dev"`, `repo="AInsel/ainsel"`, `issue_id="42"`} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query missing %s: %s", want, gotQuery)
		}
	}
}

func TestListTokens_Returns503WhenPromNotConfigured(t *testing.T) {
	s := newServerWithTokensRoute(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleTokens_RejectsNonGET(t *testing.T) {
	srv := fakePromServer(t, func(_ string, _ url.Values) interface{} {
		return vectorResponse(nil)
	})
	defer srv.Close()

	s := newServerWithTokensRoute(t, prometheus.NewClient(srv.URL, nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

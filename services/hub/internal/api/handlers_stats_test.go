package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
)

// TestGetStats_TokenTotalsIncludeCache covers the dashboard tile's token
// rollup. The runtime publishes prompt-cache traffic as its own token_type
// series and pi's usage.input excludes it, so the tile must sum all four
// components — input + output alone under-reports real consumption.
func TestGetStats_TokenTotalsIncludeCache(t *testing.T) {
	srv := fakePromServer(t, func(_ string, params url.Values) interface{} {
		return vectorResponse([]vectorSample{
			{Labels: map[string]string{"token_type": "input"}, Value: "1200"},
			{Labels: map[string]string{"token_type": "output"}, Value: "400"},
			{Labels: map[string]string{"token_type": "cache_read"}, Value: "20000"},
			{Labels: map[string]string{"token_type": "cache_write"}, Value: "900"},
		})
	})
	defer srv.Close()

	s := testServer(t)
	s.prom = prometheus.NewClient(srv.URL, nil)

	stats := s.GetStats(context.Background())

	if stats.Tokens.InputTokens != 1200 || stats.Tokens.OutputTokens != 400 {
		t.Errorf("input/output wrong: %+v", stats.Tokens)
	}
	if stats.Tokens.CacheReadTokens != 20000 || stats.Tokens.CacheWriteTokens != 900 {
		t.Errorf("cache components wrong: %+v", stats.Tokens)
	}
	if stats.Tokens.TotalTokens != 22500 {
		t.Errorf("total must include cache components, got %v", stats.Tokens.TotalTokens)
	}
}

// TestGetStats_TokensZeroWhenPromNotConfigured guards the documented behaviour
// that a missing metrics backend leaves the fields at zero instead of failing.
func TestGetStats_TokensZeroWhenPromNotConfigured(t *testing.T) {
	s := testServer(t)
	s.prom = nil

	stats := s.GetStats(context.Background())

	if stats.Tokens != (TokenTotals{}) {
		t.Errorf("expected zero token totals, got %+v", stats.Tokens)
	}
}

func TestHandleStats_RejectsNonGET(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/stats", s.handleStats)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/stats", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

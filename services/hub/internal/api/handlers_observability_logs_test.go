package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestObservabilityLogs_Returns503WhenNotConfigured(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/observability/logs", s.handleObservabilityLogs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/logs?app=hub-backend", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestObservabilityLogs_RejectsNonGet(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/observability/logs", s.handleObservabilityLogs)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/observability/logs?app=hub-backend", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestObservabilityLogs_RejectsUnknownRange(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/observability/logs", s.handleObservabilityLogs)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/logs?app=hub-backend&range=42y", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestObservabilityLogs_RejectsInvalidLimit(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/observability/logs", s.handleObservabilityLogs)

	for _, raw := range []string{"abc", "0", "-5"} {
		t.Run("limit="+raw, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/observability/logs?app=hub-backend&limit="+raw, nil)
			rec := httptest.NewRecorder()
			s.mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for limit=%q, got %d", raw, rec.Code)
			}
		})
	}
}

func TestParseLogsLimit(t *testing.T) {
	cases := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{raw: "", want: 500},
		{raw: "100", want: 100},
		{raw: "5000", want: 1000}, // clamped
		{raw: "abc", wantErr: true},
		{raw: "0", wantErr: true},
		{raw: "-5", wantErr: true},
	}
	for _, tc := range cases {
		t.Run("limit="+tc.raw, func(t *testing.T) {
			got, err := parseLogsLimit(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("want %d, got %d", tc.want, got)
			}
		})
	}
}

func TestParseLogsDuration(t *testing.T) {
	cases := []struct {
		since     string
		rangeTok  string
		wantHours float64
		wantErr   bool
	}{
		{since: "", rangeTok: "", wantHours: 1},
		{since: "", rangeTok: "1h", wantHours: 1},
		{since: "", rangeTok: "6h", wantHours: 6},
		{since: "", rangeTok: "24h", wantHours: 24},
		{since: "30m", rangeTok: "", wantHours: 0.5},
		{since: "2h", rangeTok: "1h", wantHours: 2}, // since takes precedence
		{since: "", rangeTok: "42y", wantErr: true},
		{since: "bad", rangeTok: "", wantErr: true},
	}
	for _, tc := range cases {
		name := "since=" + tc.since + ",range=" + tc.rangeTok
		t.Run(name, func(t *testing.T) {
			d, err := parseLogsDuration(tc.since, tc.rangeTok)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			hours := d.Hours()
			if hours < tc.wantHours-0.01 || hours > tc.wantHours+0.01 {
				t.Errorf("want ~%.1fh, got %.4fh", tc.wantHours, hours)
			}
		})
	}
}

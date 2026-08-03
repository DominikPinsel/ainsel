package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrors_Returns503WhenNotConfigured(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/errors", s.handleErrors)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/errors", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestErrors_RejectsNonGet(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/errors", s.handleErrors)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/errors", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

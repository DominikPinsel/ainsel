package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

// newInternalAuthServer builds a minimal Server wired like production for the
// internal-ingest paths: an OIDC-style auth middleware that rejects /api/v1/*
// requests without a bearer token, plus the handler-level X-Internal-Token
// check on the canonical /api/internal/* endpoints. The event queue is left nil
// so a request that reaches handleIngestEvent with a valid internal token
// deterministically returns 503 (proving it got past routing/auth).
func newInternalAuthServer() *Server {
	s := &Server{
		mux:                    http.NewServeMux(),
		internalValidateSecret: "test-secret",
	}
	// Only the canonical internal path is registered; the legacy
	// /api/v1/internal/* aliases are intentionally not supported.
	s.mux.HandleFunc("/api/internal/events", s.handleIngestEvent)

	// Mimic the OIDC middleware: reject anything without an authenticated user.
	s.SetAuthMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := oidc.FromContext(r.Context()); !ok {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	return s
}

func doInternal(s *Server, path, internalToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
	if internalToken != "" {
		req.Header.Set("X-Internal-Token", internalToken)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// TestInternalIngestBypassesOIDC verifies the canonical /api/internal/events
// endpoint (used by the webhook-receiver) is not routed through the OIDC auth
// middleware: it carries the /api/internal/ prefix, not /api/v1/, so an internal
// caller presenting only X-Internal-Token reaches the handler.
func TestInternalIngestBypassesOIDC(t *testing.T) {
	s := newInternalAuthServer()

	rec := doInternal(s, "/api/internal/events", "test-secret")
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("/api/internal/events was rejected by auth middleware (got 401); internal paths must bypass OIDC")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (reached handler, queue nil), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestInternalIngestStillRequiresToken verifies that bypassing the OIDC
// middleware does not leave the internal endpoint open: the handler-level
// X-Internal-Token check must still reject token-less callers.
func TestInternalIngestStillRequiresToken(t *testing.T) {
	s := newInternalAuthServer()

	rec := doInternal(s, "/api/internal/events", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without token: expected 401, got %d", rec.Code)
	}

	rec = doInternal(s, "/api/internal/events", "wrong-secret")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("with bad token: expected 401, got %d", rec.Code)
	}
}

// TestLegacyInternalPathNotSupported verifies the legacy /api/v1/internal/*
// aliases are no longer served: such a request is treated as an ordinary
// /api/v1/* path and rejected by the auth middleware rather than reaching the
// internal ingest handler.
func TestLegacyInternalPathNotSupported(t *testing.T) {
	s := newInternalAuthServer()

	rec := doInternal(s, "/api/v1/internal/events", "test-secret")
	if rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("legacy /api/v1/internal/events reached the internal handler; it must not be supported")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("legacy path: expected 401 from auth middleware, got %d", rec.Code)
	}
}

// TestNonInternalPathsStillRequireAuth verifies the OIDC middleware still
// applies to regular /api/v1/* endpoints.
func TestNonInternalPathsStillRequireAuth(t *testing.T) {
	s := newInternalAuthServer()
	// A non-internal route; the auth middleware should reject before dispatch.
	s.mux.HandleFunc("/api/v1/agents", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("non-internal /api/v1/agents without bearer: expected 401, got %d", rec.Code)
	}
}

// newAgentTaskAuthServer builds a Server with internal agent task routes
// registered alongside the OIDC-style auth middleware, mimicking production.
func newAgentTaskAuthServer() *Server {
	s := &Server{
		mux:                    http.NewServeMux(),
		internalValidateSecret: "test-secret",
	}
	s.mux.HandleFunc("/api/internal/agents/", s.handleInternalAgent)

	s.SetAuthMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := oidc.FromContext(r.Context()); !ok {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	return s
}

// TestInternalAgentNextTaskBypassesOIDC verifies that the agent poll
// endpoint under /api/internal/ is reachable with X-Internal-Token alone,
// without an OIDC bearer token.
func TestInternalAgentNextTaskBypassesOIDC(t *testing.T) {
	s := newAgentTaskAuthServer()

	req := httptest.NewRequest(http.MethodGet, "/api/internal/agents/a-refiner/next-task?timeout=1s", nil)
	req.Header.Set("X-Internal-Token", "test-secret")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("next-task with valid X-Internal-Token was rejected (401); internal paths must bypass OIDC")
	}
}

// TestInternalAgentAckBypassesOIDC verifies that the ack endpoint under
// /api/internal/ is reachable with X-Internal-Token alone.
func TestInternalAgentAckBypassesOIDC(t *testing.T) {
	s := newAgentTaskAuthServer()

	req := httptest.NewRequest(http.MethodPost, "/api/internal/agents/a-refiner/tasks/task-1/ack", strings.NewReader("{}"))
	req.Header.Set("X-Internal-Token", "test-secret")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("ack with valid X-Internal-Token was rejected (401); internal paths must bypass OIDC")
	}
}

// TestInternalAgentNextTaskRejectsBadToken verifies that bypassing OIDC
// does not skip the handler-level token validation: a wrong
// X-Internal-Token must still return 401.
func TestInternalAgentNextTaskRejectsBadToken(t *testing.T) {
	s := newAgentTaskAuthServer()

	req := httptest.NewRequest(http.MethodGet, "/api/internal/agents/a-refiner/next-task", nil)
	req.Header.Set("X-Internal-Token", "wrong-secret")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("next-task with wrong token: expected 401, got %d", rec.Code)
	}
}

// TestInternalAgentNextTaskRejectsNoToken verifies that a next-task
// request without X-Internal-Token is rejected by the handler.
func TestInternalAgentNextTaskRejectsNoToken(t *testing.T) {
	s := newAgentTaskAuthServer()

	req := httptest.NewRequest(http.MethodGet, "/api/internal/agents/a-refiner/next-task", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("next-task without token: expected 401, got %d", rec.Code)
	}
}

// TestInternalAgentUnknownPathReturns404 verifies that non-task paths
// under /api/internal/agents/ return 404 (CRUD is not exposed internally).
func TestInternalAgentUnknownPathReturns404(t *testing.T) {
	s := newAgentTaskAuthServer()

	req := httptest.NewRequest(http.MethodGet, "/api/internal/agents/a-refiner", nil)
	req.Header.Set("X-Internal-Token", "test-secret")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("internal agent CRUD: expected 404, got %d", rec.Code)
	}
}

func TestStripAgentsPrefix(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/internal/agents/a-refiner/next-task", "a-refiner/next-task"},
		{"/api/v1/agents/a-refiner/next-task", "a-refiner/next-task"},
		{"/api/internal/agents/a-refiner/tasks/1/ack", "a-refiner/tasks/1/ack"},
		{"/api/v1/agents/a-refiner/tasks/1/nack", "a-refiner/tasks/1/nack"},
		{"/other/path", "/other/path"},
	}
	for _, tt := range tests {
		if got := stripAgentsPrefix(tt.path); got != tt.want {
			t.Errorf("stripAgentsPrefix(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

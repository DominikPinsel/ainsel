package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/chat"
	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestChatSessions_NotConfigured(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSession)

	// With chat store nil, all endpoints should return 503.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/sessions", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatSessions_CreateMissingAgentName(t *testing.T) {
	s := testServer(t)
	s.chat = nil // explicitly nil; the handler returns 503 before parsing
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)

	// Even with a nil store, a POST with empty body should get 503,
	// not a 400 — the store check comes first.
	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/sessions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when chat not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatSession_MissingID(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSession)

	// /api/v1/chat/sessions/ with no ID should return 400.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/sessions/", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	// The trailing slash path maps to handleChatSession, which strips the
	// prefix and gets an empty id → 400.
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 400 or 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatSession_MethodNotAllowed(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/chat/sessions", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestChatSession_PatchNotConfigured(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSession)

	// With chat store nil, PATCH should return 503.
	body := []byte(`{"name":"New Name"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/chat/sessions/sess-test", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatSession_PatchNotConfiguredEmptyName(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSession)

	// With chat store nil, the handler returns 503 before parsing the body.
	// This test documents that the 503 check comes first, even when the
	// name field is empty.
	body := []byte(`{"name":""}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/chat/sessions/sess-test", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when chat not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}

// newTestChatStore boots a Postgres testcontainer, applies migrations, and
// returns a chat.Store wired to a fresh pool. Skips the test if Docker is
// unavailable.
func newTestChatStore(t *testing.T) (*chat.Store, func()) {
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
	return chat.NewStore(pool), cleanup
}

func TestChatSession_PatchSuccess(t *testing.T) {
	store, cleanup := newTestChatStore(t)
	defer cleanup()

	s := testServer(t)
	s.chat = store
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSession)

	// Create a session via the store.
	sess, err := store.CreateSession(context.Background(), "developer", "user-1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// PATCH the session name.
	body := []byte(`{"name":"My Renamed Chat"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/chat/sessions/"+sess.ID, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var updated chat.Session
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if updated.Name != "My Renamed Chat" {
		t.Fatalf("expected name %q, got %q", "My Renamed Chat", updated.Name)
	}
	if updated.ID != sess.ID {
		t.Fatalf("expected id %q, got %q", sess.ID, updated.ID)
	}
}

func TestChatSession_PatchEmptyName_400(t *testing.T) {
	store, cleanup := newTestChatStore(t)
	defer cleanup()

	s := testServer(t)
	s.chat = store
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSession)

	// Create a session via the store.
	sess, err := store.CreateSession(context.Background(), "developer", "user-1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// PATCH with an empty name → 400.
	body := []byte(`{"name":""}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/chat/sessions/"+sess.ID, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty name, got %d: %s", rec.Code, rec.Body.String())
	}

	// PATCH with whitespace-only name → 400 (trimmed to empty).
	body = []byte(`{"name":"   "}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/chat/sessions/"+sess.ID, bytes.NewReader(body))
	rec = httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for whitespace-only name, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatSession_PatchNotFound(t *testing.T) {
	store, cleanup := newTestChatStore(t)
	defer cleanup()

	s := testServer(t)
	s.chat = store
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSession)

	// PATCH a non-existent session → 404.
	body := []byte(`{"name":"New Name"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/chat/sessions/sess-missing", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
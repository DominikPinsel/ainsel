package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newInternalChatServer builds a Server wired like production for the
// internal chat paths: an auth middleware that rejects every /api/v1/*
// request without an authenticated user (mimicking the OIDC middleware),
// while /api/internal/chat/* is protected handler-side by X-Internal-Token.
func newInternalChatServer(t *testing.T) *Server {
	t.Helper()
	s := testServer(t)
	s.internalValidateSecret = "test-secret"
	s.mux.HandleFunc("/api/internal/chat/sessions", s.handleInternalChatSessions)
	s.mux.HandleFunc("/api/internal/chat/sessions/", s.handleInternalChatSession)
	// Reject everything that reaches the auth middleware, like OIDC does
	// for requests without a bearer token.
	s.SetAuthMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
		})
	})
	return s
}

func doInternalChat(s *Server, method, path, token, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// TestInternalChatBypassesOIDC verifies the internal chat endpoints are not
// routed through the OIDC auth middleware (they carry the /api/internal/
// prefix), so a sidecar presenting only X-Internal-Token reaches them.
func TestInternalChatBypassesOIDC(t *testing.T) {
	s := newInternalChatServer(t)

	rec := doInternalChat(s, http.MethodGet, "/api/internal/chat/sessions", "test-secret", "")
	// Chat store is nil, so past auth the handler answers 503 — anything
	// but 401 proves the request bypassed the OIDC middleware.
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("internal chat endpoint was rejected by auth middleware (got 401); internal paths must bypass OIDC")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (chat not configured) past auth, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestInternalChatRequiresToken verifies missing or wrong tokens are
// rejected with 401.
func TestInternalChatRequiresToken(t *testing.T) {
	s := newInternalChatServer(t)

	for _, tc := range []struct {
		name  string
		token string
	}{
		{"missing token", ""},
		{"wrong token", "not-the-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doInternalChat(s, http.MethodGet, "/api/internal/chat/sessions", tc.token, "")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestInternalChatListSessions verifies the sidecar can list sessions for
// its agent.
func TestInternalChatListSessions(t *testing.T) {
	store, cleanup := newTestChatStore(t)
	defer cleanup()

	s := newInternalChatServer(t)
	s.chat = store

	if _, err := store.CreateSession(context.Background(), "a-test", "user-1"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := store.CreateSession(context.Background(), "a-other", "user-1"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	rec := doInternalChat(s, http.MethodGet, "/api/internal/chat/sessions?agent=a-test", "test-secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []struct {
			AgentName string `json:"agentName"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("expected 1 session for agent a-test, got %d", resp.Total)
	}
}

// TestInternalChatGetSessionAndPostMessage verifies the sidecar can fetch a
// session and post an assistant reply.
func TestInternalChatGetSessionAndPostMessage(t *testing.T) {
	store, cleanup := newTestChatStore(t)
	defer cleanup()

	s := newInternalChatServer(t)
	s.chat = store

	sess, err := store.CreateSession(context.Background(), "a-test", "user-1")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Get the session.
	rec := doInternalChat(s, http.MethodGet, "/api/internal/chat/sessions/"+sess.ID, "test-secret", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get session: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Post an assistant reply.
	rec = doInternalChat(s, http.MethodPost, "/api/internal/chat/sessions/"+sess.ID+"/messages", "test-secret",
		`{"role":"assistant","content":"hello back"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("post message: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// The message must be part of the session history now.
	got, err := store.GetSession(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("get session after reply: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "hello back" {
		t.Fatalf("expected stored assistant reply, got %+v", got.Messages)
	}
}

// TestInternalChatGetSessionNotFound verifies unknown sessions return 404.
func TestInternalChatGetSessionNotFound(t *testing.T) {
	store, cleanup := newTestChatStore(t)
	defer cleanup()

	s := newInternalChatServer(t)
	s.chat = store

	rec := doInternalChat(s, http.MethodGet, "/api/internal/chat/sessions/sess-nope", "test-secret", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestInternalChatMethodNotAllowed verifies the sidecar-facing surface only
// exposes the operations the chat MCP tools need.
func TestInternalChatMethodNotAllowed(t *testing.T) {
	s := newInternalChatServer(t)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/internal/chat/sessions"},
		{http.MethodPatch, "/api/internal/chat/sessions/sess-1"},
		{http.MethodDelete, "/api/internal/chat/sessions/sess-1"},
		{http.MethodGet, "/api/internal/chat/sessions/sess-1/messages"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := doInternalChat(s, tc.method, tc.path, "test-secret", "{}")
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

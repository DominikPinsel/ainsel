package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// newTestServer starts an httptest.Server that records the last request and
// returns a canned response. Returns the server and a pointer to the recorded
// request path.
func newTestServer(status int, response any) (*httptest.Server, *string) {
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		if r.URL.RawQuery != "" {
			lastPath += "?" + r.URL.RawQuery
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	}))
	return srv, &lastPath
}

func reqWithArgs(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestListSessions(t *testing.T) {
	srv, lastPath := newTestServer(http.StatusOK, []map[string]any{
		{"id": "sess-1", "agent_name": "developer"},
	})
	defer srv.Close()

	ct := NewChatTools(srv.URL, "test-token", "developer")
	result, err := ct.ListSessions(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if *lastPath != "/api/internal/chat/sessions?agent=developer" {
		t.Fatalf("unexpected path: %s", *lastPath)
	}
}

func TestGetHistory(t *testing.T) {
	srv, lastPath := newTestServer(http.StatusOK, map[string]any{
		"id":       "sess-1",
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})
	defer srv.Close()

	ct := NewChatTools(srv.URL, "test-token", "developer")
	result, err := ct.GetHistory(context.Background(), reqWithArgs(map[string]any{
		"session_id": "sess-1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if *lastPath != "/api/internal/chat/sessions/sess-1" {
		t.Fatalf("unexpected path: %s", *lastPath)
	}
}

func TestGetHistoryWithLimit(t *testing.T) {
	srv, lastPath := newTestServer(http.StatusOK, map[string]any{})
	defer srv.Close()

	ct := NewChatTools(srv.URL, "test-token", "developer")
	_, err := ct.GetHistory(context.Background(), reqWithArgs(map[string]any{
		"session_id": "sess-1",
		"limit":      float64(10),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *lastPath != "/api/internal/chat/sessions/sess-1?limit=10" {
		t.Fatalf("unexpected path: %s", *lastPath)
	}
}

func TestGetHistoryMissingSessionID(t *testing.T) {
	srv, _ := newTestServer(http.StatusOK, map[string]any{})
	defer srv.Close()

	ct := NewChatTools(srv.URL, "test-token", "developer")
	result, err := ct.GetHistory(context.Background(), reqWithArgs(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing session_id")
	}
}

func TestSendReply(t *testing.T) {
	srv, lastPath := newTestServer(http.StatusCreated, map[string]any{
		"id":      "msg-1",
		"role":    "assistant",
		"content": "hello there",
	})
	defer srv.Close()

	ct := NewChatTools(srv.URL, "test-token", "developer")
	result, err := ct.SendReply(context.Background(), reqWithArgs(map[string]any{
		"session_id": "sess-1",
		"content":     "hello there",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if *lastPath != "/api/internal/chat/sessions/sess-1/messages" {
		t.Fatalf("unexpected path: %s", *lastPath)
	}
}

func TestSendReplyMissingContent(t *testing.T) {
	srv, _ := newTestServer(http.StatusCreated, map[string]any{})
	defer srv.Close()

	ct := NewChatTools(srv.URL, "test-token", "developer")
	result, err := ct.SendReply(context.Background(), reqWithArgs(map[string]any{
		"session_id": "sess-1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing content")
	}
}

func TestSendStatus(t *testing.T) {
	srv, lastPath := newTestServer(http.StatusCreated, map[string]any{
		"id": "msg-2",
	})
	defer srv.Close()

	ct := NewChatTools(srv.URL, "test-token", "developer")
	result, err := ct.SendStatus(context.Background(), reqWithArgs(map[string]any{
		"session_id": "sess-1",
		"content":    "Looking at the repo...",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if *lastPath != "/api/internal/chat/sessions/sess-1/messages" {
		t.Fatalf("unexpected path: %s", *lastPath)
	}
}

func TestSendStatusMissingSessionID(t *testing.T) {
	srv, _ := newTestServer(http.StatusCreated, map[string]any{})
	defer srv.Close()

	ct := NewChatTools(srv.URL, "test-token", "developer")
	result, err := ct.SendStatus(context.Background(), reqWithArgs(map[string]any{
		"content": "working...",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing session_id")
	}
}

func TestSendReplyHubError(t *testing.T) {
	srv, _ := newTestServer(http.StatusInternalServerError, map[string]any{
		"error": "session not found",
	})
	defer srv.Close()

	ct := NewChatTools(srv.URL, "test-token", "developer")
	result, err := ct.SendReply(context.Background(), reqWithArgs(map[string]any{
		"session_id": "bad-session",
		"content":    "hello",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error from hub 500")
	}
}
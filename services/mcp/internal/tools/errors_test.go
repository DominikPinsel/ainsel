package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestGetRecentErrors(t *testing.T) {
	var capturedQuery string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/errors" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		capturedQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]any{
				{"id": "err-1", "severity": "error", "message": "forgejo 403"},
			},
			"total": 1,
		})
	}))
	defer hub.Close()

	et := &ErrorTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"agent": "code-reviewer",
		"since": "2026-05-20T00:00:00Z",
		"limit": float64(25),
	}
	result, err := et.GetRecentErrors(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "err-1") {
		t.Errorf("expected err-1 in response; got: %s", text)
	}
	if !strings.Contains(capturedQuery, "agent=code-reviewer") {
		t.Errorf("expected agent filter forwarded; query was: %s", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "since=") {
		t.Errorf("expected since filter forwarded; query was: %s", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "limit=25") {
		t.Errorf("expected limit forwarded; query was: %s", capturedQuery)
	}
}

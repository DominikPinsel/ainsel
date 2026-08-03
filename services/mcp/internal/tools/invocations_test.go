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

func TestListInvocations(t *testing.T) {
	var capturedQuery string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/invocations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		capturedQuery = r.URL.RawQuery
		// The real hub wraps results: {"invocations":[...], "total":...}. The
		// tool relays the body verbatim, so we mirror that shape here.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"invocations": []map[string]any{
				{"id": "inv-1", "agent": "code-reviewer", "status": "succeeded"},
				{"id": "inv-2", "agent": "code-reviewer", "status": "failed"},
			},
			"total": 2,
		})
	}))
	defer hub.Close()

	it := &InvocationTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"agent": "code-reviewer", "limit": float64(50)}
	result, err := it.ListInvocations(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "inv-1") || !strings.Contains(text, "inv-2") {
		t.Errorf("expected both invocations in output; got: %s", text)
	}
	if !strings.Contains(capturedQuery, "agent=code-reviewer") {
		t.Errorf("expected agent filter forwarded; query was: %s", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "limit=50") {
		t.Errorf("expected limit forwarded; query was: %s", capturedQuery)
	}
}

func TestGetInvocation(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/invocations/inv-abc123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "inv-abc123",
			"agent":  "code-reviewer",
			"prompt": "review the diff",
		})
	}))
	defer hub.Close()

	it := &InvocationTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"id": "inv-abc123"}
	result, err := it.GetInvocation(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "inv-abc123") {
		t.Errorf("expected invocation id; got: %s", text)
	}
}

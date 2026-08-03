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

// TestSummarizeAgentActivity exercises the per-agent rollup. The hub wraps
// list responses in {"invocations":[...], "total":...}; the mock mirrors that
// so the tool exercises its real parsing path.
func TestSummarizeAgentActivity(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/invocations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"invocations": []map[string]any{
				{"id": "inv-1", "agent": "code-reviewer", "status": "succeeded", "tokensInput": 1000, "tokensOutput": 200, "costUSD": 0.02},
				{"id": "inv-2", "agent": "code-reviewer", "status": "succeeded", "tokensInput": 800, "tokensOutput": 150, "costUSD": 0.015},
				{"id": "inv-3", "agent": "code-reviewer", "status": "failed", "tokensInput": 500, "tokensOutput": 0, "costUSD": 0.005, "error": "forgejo 403"},
				{"id": "inv-4", "agent": "issue-triager", "status": "succeeded", "tokensInput": 300, "tokensOutput": 80, "costUSD": 0.007},
			},
			"total": 4,
		})
	}))
	defer hub.Close()

	at := &ActivityTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	result, err := at.SummarizeAgentActivity(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "code-reviewer") || !strings.Contains(text, "issue-triager") {
		t.Errorf("expected both agents in summary; got: %s", text)
	}
	// code-reviewer should have: total=3, succeeded=2, failed=1. Asserting
	// each breakdown value pins the status switch in the implementation so a
	// regression there is caught immediately.
	if !strings.Contains(text, `"total": 3`) {
		t.Errorf("expected total=3 for code-reviewer; got: %s", text)
	}
	if !strings.Contains(text, `"succeeded": 2`) {
		t.Errorf("expected succeeded=2 for code-reviewer; got: %s", text)
	}
	if !strings.Contains(text, `"failed": 1`) {
		t.Errorf("expected failed=1 for code-reviewer; got: %s", text)
	}
	// Latency aggregation is intentionally omitted (the hub does not expose
	// p50/p95 on invocation aggregates yet); the tool surfaces a _note about
	// that limitation in its response.
	if !strings.Contains(text, "_note") {
		t.Errorf("expected _note about latency aggregation; got: %s", text)
	}
}

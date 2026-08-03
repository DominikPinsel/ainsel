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

func TestGetTokenUsage(t *testing.T) {
	var capturedQuery string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tokens" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		capturedQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tokens": []map[string]any{
				{"agent": "code-reviewer", "model": "claude-opus", "inputTokens": 1234, "outputTokens": 56},
			},
			"total": map[string]any{"inputTokens": 1234, "outputTokens": 56},
		})
	}))
	defer hub.Close()

	ut := &UsageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"agent":      "code-reviewer",
		"repository": "ainsel",
	}
	result, err := ut.GetTokenUsage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "code-reviewer") {
		t.Errorf("expected code-reviewer in output; got: %s", text)
	}
	if !strings.Contains(capturedQuery, "agent=code-reviewer") {
		t.Errorf("expected agent filter forwarded; query was: %s", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "repository=ainsel") {
		t.Errorf("expected repository filter forwarded; query was: %s", capturedQuery)
	}
}

func TestGetStats(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/stats" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"agents":   map[string]any{"total": 3, "healthy": 3},
			"triggers": map[string]any{"total": 5, "healthy": 5},
		})
	}))
	defer hub.Close()

	ut := &UsageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	result, err := ut.GetStats(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "agents") || !strings.Contains(text, "triggers") {
		t.Errorf("expected agents/triggers blocks in stats; got: %s", text)
	}
}

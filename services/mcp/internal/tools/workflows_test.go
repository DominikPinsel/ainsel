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

// TestSummarizeWorkflows mocks the hub responses in the same shape the real
// hub returns: a paginated wrapper {"items":[...], "total":..., ...} where each
// item is the SimpleAgentResponse / SimpleTriggerResponse projection, not a raw
// Kubernetes CRD object.
func TestSummarizeWorkflows(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":           "code-reviewer",
						"name":         "Indiana Codes",
						"imageRef":     map[string]any{"name": "ainsel-ai-agent-default"},
						"llm":          map[string]any{"model": "claude-opus-4-7"},
						"persona":      map[string]any{"configMapRef": "code-reviewer-persona"},
						"enabledTools": []string{"forgejo", "git", "code-review"},
						"scaling":      map[string]any{"replicas": 3},
					},
					{
						"id":       "orphan-agent",
						"name":     "Orphan",
						"imageRef": map[string]any{"name": "ainsel-ai-agent-default"},
						"llm":      map[string]any{"model": "claude-opus-4-7"},
					},
				},
				"total":      2,
				"page":       1,
				"pageSize":   2,
				"totalPages": 1,
			})
		case "/api/v1/triggers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":           "ai-reviewer-on-prs",
						"name":         "AI review on PRs",
						"agentRef":     "code-reviewer",
						"connectorRef": "forgejo-main",
						"filters": []map[string]any{
							{"field": "headers.X-Gitea-Event", "op": "eq", "value": "pull_request"},
							{"field": "repository.name", "op": "in", "value": "aic,ainsel"},
						},
					},
					{
						"id":           "dangling-trigger",
						"name":         "Points at deleted agent",
						"agentRef":     "deleted-agent",
						"connectorRef": "forgejo-main",
					},
				},
				"total":      2,
				"page":       1,
				"pageSize":   2,
				"totalPages": 1,
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer hub.Close()

	wt := &WorkflowTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	result, err := wt.SummarizeWorkflows(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text

	// Both agents appear
	if !strings.Contains(text, "code-reviewer") {
		t.Error("expected code-reviewer agent in output")
	}
	if !strings.Contains(text, "orphan-agent") {
		t.Error("expected orphan-agent in output (even with no triggers)")
	}
	// Trigger nested under its agent
	if !strings.Contains(text, "ai-reviewer-on-prs") {
		t.Error("expected trigger nested under code-reviewer")
	}
	// Orphaned trigger surfaces
	if !strings.Contains(text, "orphanedTriggers") {
		t.Error("expected orphanedTriggers key for trigger pointing at missing agent")
	}
	if !strings.Contains(text, "dangling-trigger") {
		t.Error("expected dangling-trigger to appear in orphanedTriggers")
	}
}

// TestDecodeWrapped covers the edge cases that motivated the helper's
// rewrite: the hub may legitimately return wrapper objects where the items
// key is missing entirely or explicitly null (empty result set). Both should
// resolve to an empty slice, not an error, so callers don't have to special-
// case empty pages.
func TestDecodeWrapped(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		key     string
		wantLen int
		wantErr bool
	}{
		{name: "wrapper with items", body: `{"items":[{"id":"a"}],"total":1}`, key: "items", wantLen: 1},
		{name: "wrapper with empty items array", body: `{"items":[],"total":0}`, key: "items", wantLen: 0},
		{name: "wrapper with null items", body: `{"items":null,"total":0}`, key: "items", wantLen: 0},
		{name: "wrapper without items key", body: `{"total":0}`, key: "items", wantLen: 0},
		{name: "bare array", body: `[{"id":"a"},{"id":"b"}]`, key: "items", wantLen: 2},
		{name: "invocations key", body: `{"invocations":[{"id":"x"}],"total":1}`, key: "invocations", wantLen: 1},
		{name: "invocations null", body: `{"invocations":null}`, key: "invocations", wantLen: 0},
		{name: "garbage", body: `not json at all`, key: "items", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeWrapped([]byte(tc.body), tc.key)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil; result=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantLen {
				t.Errorf("expected len=%d, got %d (%v)", tc.wantLen, len(got), got)
			}
		})
	}
}

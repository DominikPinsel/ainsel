package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestGetAgentLogs(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/observability/logs" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"logs":  []map[string]any{{"timestamp": "2024-01-01T00:00:00Z", "message": "log line 1"}},
			"total": 1,
			"query": r.URL.Query().Get("query"),
		})
	}))
	defer hub.Close()

	lt := NewLogTools(hub.URL)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"agent": "olli"}
	result, err := lt.GetAgentLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
}

func TestQueryLogs(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"logs":  []any{},
			"total": 0,
			"query": r.URL.Query().Get("query"),
		})
	}))
	defer hub.Close()

	lt := NewLogTools(hub.URL)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"query": `{namespace="ainsel"} |= "error"`}
	result, err := lt.QueryLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
}

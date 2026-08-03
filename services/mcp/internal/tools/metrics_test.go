package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestGetAgentMetrics(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/observability/metrics/query" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result": []map[string]any{
					{"metric": map[string]any{"agent": "olli"}, "value": []any{1700000000, "42"}},
				},
			},
		})
	}))
	defer hub.Close()

	mt := NewMetricsTools(hub.URL)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"agent": "olli"}
	result, err := mt.GetAgentMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
}

func TestQueryMetrics(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": []any{}},
		})
	}))
	defer hub.Close()

	mt := NewMetricsTools(hub.URL)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"query": `up{namespace="ainsel"}`}
	result, err := mt.QueryMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
}

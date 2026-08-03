package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestGetPlatformHealth(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/platform/health" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"name":       "hub-abc123",
				"phase":      "Running",
				"ready":      "1/1",
				"restarts":   0,
				"containers": []map[string]any{{"name": "hub", "ready": true, "state": "running", "restarts": 0}},
			},
		})
	}))
	defer hub.Close()

	ht := NewHealthTools(hub.URL)
	result, err := ht.GetPlatformHealth(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
}

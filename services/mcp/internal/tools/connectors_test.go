package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestListConnectors(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/connectors" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"metadata": map[string]any{"name": "forgejo-connector"}, "spec": map[string]any{"type": "forgejo"}},
			{"metadata": map[string]any{"name": "slack-connector"}, "spec": map[string]any{"type": "slack"}},
		})
	}))
	defer hub.Close()

	ct := &ConnectorTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	result, err := ct.ListConnectors(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if len(text) == 0 {
		t.Error("expected non-empty response")
	}
}

func TestGetConnector(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/connectors/forgejo-connector" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]any{"name": "forgejo-connector"},
			"spec":     map[string]any{"type": "forgejo"},
		})
	}))
	defer hub.Close()

	ct := &ConnectorTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "forgejo-connector"}
	result, err := ct.GetConnector(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if len(text) == 0 {
		t.Error("expected non-empty response")
	}
}

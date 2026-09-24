package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestUpdateConnector_DisplayName(t *testing.T) {
	var capturedMethod, capturedPath, capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "c-0c4b01e3",
			"name": "connector-forgejo-ainsel",
		})
	}))
	defer hub.Close()

	ct := &ConnectorTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":         "c-0c4b01e3",
		"display_name": "connector-forgejo-ainsel",
	}
	result, err := ct.UpdateConnector(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/connectors/c-0c4b01e3" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(capturedBody, `"name":"connector-forgejo-ainsel"`) {
		t.Errorf("expected display name in request body; got: %s", capturedBody)
	}
	// disabled was not provided — it must not be sent.
	if strings.Contains(capturedBody, "disabled") {
		t.Errorf("expected disabled to be omitted; got: %s", capturedBody)
	}
}

func TestUpdateConnector_Disabled(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "c-x", "name": "c", "disabled": true})
	}))
	defer hub.Close()

	ct := &ConnectorTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "c-x", "disabled": true}
	result, err := ct.UpdateConnector(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(capturedBody, `"disabled":true`) {
		t.Errorf("expected disabled in body; got: %s", capturedBody)
	}
}

func TestUpdateConnector_NoFields(t *testing.T) {
	ct := &ConnectorTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "c-x"}
	result, _ := ct.UpdateConnector(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when no optional fields provided")
	}
	if !strings.Contains(result.Content[0].(mcp.TextContent).Text, "at least one") {
		t.Errorf("expected descriptive error; got: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestUpdateConnector_MissingName(t *testing.T) {
	ct := &ConnectorTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"display_name": "connector-new"}
	result, _ := ct.UpdateConnector(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when name is missing")
	}
}

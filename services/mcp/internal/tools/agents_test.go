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

func TestListAgents(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"metadata": map[string]any{"name": "olli"}, "spec": map[string]any{"displayName": "Olli"}},
			{"metadata": map[string]any{"name": "difi"}, "spec": map[string]any{"displayName": "Difi"}},
		})
	}))
	defer hub.Close()

	at := &AgentTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	result, err := at.ListAgents(context.Background(), mcp.CallToolRequest{})
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

func TestGetAgent(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents/olli" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]any{"name": "olli"},
			"spec":     map[string]any{"displayName": "Olli"},
		})
	}))
	defer hub.Close()

	at := &AgentTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "olli"}
	result, err := at.GetAgent(context.Background(), req)
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

func TestUpdateAgent_ModelOnly(t *testing.T) {
	var capturedMethod, capturedPath, capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "a-director",
			"name": "Director",
			"llm": map[string]any{
				"model":    "glm-5.1",
				"maxTurns": 10,
			},
		})
	}))
	defer hub.Close()

	at := &AgentTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "a-director",
		"model": "glm-5.1",
	}
	result, err := at.UpdateAgent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/agents/a-director" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(capturedBody, `"model":"glm-5.1"`) {
		t.Errorf("expected model in request body; got: %s", capturedBody)
	}
	// Should not include maxTurns since it was not provided
	if strings.Contains(capturedBody, "maxTurns") {
		t.Errorf("expected maxTurns to be omitted; got: %s", capturedBody)
	}
}

func TestUpdateAgent_TemperatureOnly(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "a-director",
			"name": "Director",
			"llm": map[string]any{
				"model":       "glm-5.1",
				"temperature": 0.7,
			},
		})
	}))
	defer hub.Close()

	at := &AgentTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":        "a-director",
		"temperature": 0.7,
	}
	result, err := at.UpdateAgent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(capturedBody, `"temperature":0.7`) {
		t.Errorf("expected temperature in request body; got: %s", capturedBody)
	}
}

func TestUpdateAgent_MultipleFields(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "a-director",
			"name": "Director",
			"llm": map[string]any{
				"model":       "qwen3.5:cloud",
				"maxTurns":    5,
				"temperature": 0.3,
			},
		})
	}))
	defer hub.Close()

	at := &AgentTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":        "a-director",
		"model":       "qwen3.5:cloud",
		"max_turns":   5.0,
		"temperature": 0.3,
	}
	result, err := at.UpdateAgent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(capturedBody, `"model":"qwen3.5:cloud"`) {
		t.Errorf("expected model in body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, `"maxTurns":5`) {
		t.Errorf("expected maxTurns in body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, `"temperature":0.3`) {
		t.Errorf("expected temperature in body; got: %s", capturedBody)
	}
}

func TestUpdateAgent_NoOptionalFields(t *testing.T) {
	at := &AgentTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "a-director"}
	result, _ := at.UpdateAgent(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when no optional fields provided")
	}
	if !strings.Contains(result.Content[0].(mcp.TextContent).Text, "at least one") {
		t.Errorf("expected descriptive error; got: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestUpdateAgent_MissingName(t *testing.T) {
	at := &AgentTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"model": "glm-5.1"}
	result, _ := at.UpdateAgent(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when name is missing")
	}
}
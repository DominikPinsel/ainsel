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

func TestListAgentImages(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent-images" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "ainsel-ai-agent-default", "name": "Default agent runtime"},
				{"id": "ainsel-ai-agent-codex", "name": "Codex runtime"},
			},
		})
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	result, err := it.ListAgentImages(context.Background(), mcp.CallToolRequest{})
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

func TestGetAgentImage(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent-images/ainsel-ai-agent-default" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "ainsel-ai-agent-default",
			"name": "Default agent runtime",
		})
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "ainsel-ai-agent-default"}
	result, err := it.GetAgentImage(context.Background(), req)
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

func TestCreateAgentImage(t *testing.T) {
	var capturedMethod, capturedPath, capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "ainsel-ai-agent-custom",
			"displayName": "Custom runtime",
			"imageUrl":    "registry.example.com/agent:latest",
		})
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"display_name": "Custom runtime",
		"image_url":    "registry.example.com/agent:latest",
		"description":  "A custom agent image",
		"env":          `[{"name":"LOG_LEVEL","value":"debug"},{"name":"PORT","value":"8080"}]`,
	}
	result, err := it.CreateAgentImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/agent-images" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(capturedBody, "Custom runtime") {
		t.Errorf("expected displayName in request body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "LOG_LEVEL") {
		t.Errorf("expected env vars in request body; got: %s", capturedBody)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "ainsel-ai-agent-custom") {
		t.Errorf("expected created image id in response; got: %s", text)
	}
}

func TestCreateAgentImage_Minimal(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "ainsel-ai-agent-minimal",
			"displayName": "Minimal",
			"imageUrl":    "registry.example.com/min:1.0",
		})
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"display_name": "Minimal",
		"image_url":    "registry.example.com/min:1.0",
	}
	result, err := it.CreateAgentImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestCreateAgentImage_MissingRequired(t *testing.T) {
	it := &AgentImageTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}

	// Missing display_name
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"image_url": "registry.example.com/img"}
	result, _ := it.CreateAgentImage(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when display_name is missing")
	}

	// Missing image_url
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]any{"display_name": "My Image"}
	result2, _ := it.CreateAgentImage(context.Background(), req2)
	if !result2.IsError {
		t.Error("expected error when image_url is missing")
	}
}

func TestCreateAgentImage_InvalidEnv(t *testing.T) {
	it := &AgentImageTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"display_name": "Bad",
		"image_url":    "registry.example.com/bad",
		"env":          "not-json",
	}
	result, _ := it.CreateAgentImage(context.Background(), req)
	if !result.IsError {
		t.Error("expected error for invalid env JSON")
	}
}

func TestUpdateAgentImage(t *testing.T) {
	var capturedMethod, capturedPath, capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "ainsel-ai-agent-default",
			"displayName": "Updated runtime",
			"imageUrl":    "registry.example.com/agent:v2",
		})
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":        "ainsel-ai-agent-default",
		"image_url":   "registry.example.com/agent:v2",
		"description": "Updated description",
		"env":         `[{"name":"LOG_LEVEL","value":"info"}]`,
	}
	result, err := it.UpdateAgentImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/agent-images/ainsel-ai-agent-default" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(capturedBody, "agent:v2") {
		t.Errorf("expected imageUrl in request body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "LOG_LEVEL") {
		t.Errorf("expected env vars in request body; got: %s", capturedBody)
	}
}

func TestUpdateAgentImage_ClearEnv(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "ainsel-ai-agent-default",
			"displayName": "Default agent runtime",
		})
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":        "ainsel-ai-agent-default",
		"display_name": "Default agent runtime",
		"env":         "[]",
	}
	result, err := it.UpdateAgentImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(capturedBody, `"env":[]`) {
		t.Errorf("expected env to be cleared; got: %s", capturedBody)
	}
}

func TestUpdateAgentImage_PreserveEnv(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "ainsel-ai-agent-default",
			"displayName": "Default agent runtime",
		})
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":         "ainsel-ai-agent-default",
		"display_name": "Default agent runtime",
	}
	result, err := it.UpdateAgentImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if strings.Contains(capturedBody, "env") {
		t.Errorf("expected env to be omitted (preserved); got: %s", capturedBody)
	}
}

func TestUpdateAgentImage_MissingName(t *testing.T) {
	it := &AgentImageTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"display_name": "something"}
	result, _ := it.UpdateAgentImage(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when name is missing")
	}
}

func TestUpdateAgentImage_NoFields(t *testing.T) {
	it := &AgentImageTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "ainsel-ai-agent-default"}
	result, _ := it.UpdateAgentImage(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when no fields to update")
	}
	if !strings.Contains(result.Content[0].(mcp.TextContent).Text, "at least one") {
		t.Errorf("expected descriptive error; got: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestUpdateAgentImage_Tools(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "ainsel-ai-agent-default",
			"displayName": "Default agent runtime",
		})
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name": "ainsel-ai-agent-default",
		"tools": `[{"name":"run_shell","kind":"shell","description":"Run a shell command","examples":[{"title":"List files","snippet":"run_shell: ls -la"}]}]`,
	}
	result, err := it.UpdateAgentImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(capturedBody, "run_shell") {
		t.Errorf("expected tool name in request body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "shell") {
		t.Errorf("expected tool kind in request body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "List files") {
		t.Errorf("expected example title in request body; got: %s", capturedBody)
	}
}

func TestUpdateAgentImage_ClearTools(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "ainsel-ai-agent-default",
			"displayName": "Default agent runtime",
		})
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "ainsel-ai-agent-default",
		"tools": "[]",
	}
	result, err := it.UpdateAgentImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(capturedBody, `"tools":[]`) {
		t.Errorf("expected tools to be cleared; got: %s", capturedBody)
	}
}

func TestUpdateAgentImage_PreserveTools(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "ainsel-ai-agent-default",
			"displayName": "Default agent runtime",
		})
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":         "ainsel-ai-agent-default",
		"display_name": "Default agent runtime",
	}
	result, err := it.UpdateAgentImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if strings.Contains(capturedBody, "tools") {
		t.Errorf("expected tools to be omitted (preserved); got: %s", capturedBody)
	}
}

func TestUpdateAgentImage_InvalidTools(t *testing.T) {
	it := &AgentImageTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "ainsel-ai-agent-default",
		"tools": "not-json",
	}
	result, _ := it.UpdateAgentImage(context.Background(), req)
	if !result.IsError {
		t.Error("expected error for invalid tools JSON")
	}
}

func TestDeleteAgentImage(t *testing.T) {
	var capturedMethod, capturedPath string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "ainsel-ai-agent-default"}
	result, err := it.DeleteAgentImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", capturedMethod)
	}
	if capturedPath != "/api/v1/agent-images/ainsel-ai-agent-default" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "ainsel-ai-agent-default") {
		t.Errorf("expected deleted image name in response; got: %s", text)
	}
}

func TestDeleteAgentImage_MissingName(t *testing.T) {
	it := &AgentImageTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, _ := it.DeleteAgentImage(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when name is missing")
	}
}

func TestDeleteAgentImage_Conflict(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"agent image in use","affectedAgents":["a-3f9a2b"]}`))
	}))
	defer hub.Close()

	it := &AgentImageTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "ainsel-ai-agent-default"}
	result, err := it.DeleteAgentImage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error when delete returns conflict")
	}
}
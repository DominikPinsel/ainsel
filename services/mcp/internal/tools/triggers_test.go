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

func TestListTriggers(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/triggers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"metadata": map[string]any{"name": "push-trigger"}, "spec": map[string]any{"connectorRef": "github-main"}},
			{"metadata": map[string]any{"name": "pr-trigger"}, "spec": map[string]any{"connectorRef": "github-main"}},
		})
	}))
	defer hub.Close()

	tt := &TriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	result, err := tt.ListTriggers(context.Background(), mcp.CallToolRequest{})
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

func TestGetTrigger(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/triggers/push-trigger" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]any{"name": "push-trigger"},
			"spec":     map[string]any{"connectorRef": "github-main"},
		})
	}))
	defer hub.Close()

	tt := &TriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "push-trigger"}
	result, err := tt.GetTrigger(context.Background(), req)
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

func TestDeleteTrigger(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/triggers/bad-trigger" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hub.Close()

	tt := &TriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "bad-trigger"}
	result, err := tt.DeleteTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if text != "trigger bad-trigger deleted" {
		t.Errorf("unexpected result text: %s", text)
	}
}

func TestDeleteTrigger_NotFound(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"trigger not found"}`))
	}))
	defer hub.Close()

	tt := &TriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "nonexistent"}
	result, err := tt.DeleteTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for 404")
	}
}

func TestDeleteTrigger_MissingName(t *testing.T) {
	tt := &TriggerTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": ""}
	result, err := tt.DeleteTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
}

func TestCreateTrigger(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/triggers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if payload["name"] != "my-trigger" {
			t.Errorf("unexpected name: %v", payload["name"])
		}
		if payload["agentRef"] != "agent-1" {
			t.Errorf("unexpected agentRef: %v", payload["agentRef"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "t-new",
			"name": payload["name"],
		})
	}))
	defer hub.Close()

	tt := &TriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":         "my-trigger",
		"agentRef":     "agent-1",
		"connectorRef": "connector-1",
		"filters":      `[{"field":"headers.X-Github-Event","op":"eq","value":"issue_comment"}]`,
	}
	result, err := tt.CreateTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"id":"t-new"`) {
		t.Errorf("expected response to contain id; got %s", text)
	}
}

func TestCreateTrigger_MissingRequired(t *testing.T) {
	tt := &TriggerTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "my-trigger"}
	result, err := tt.CreateTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing required fields")
	}
}

func TestCreateTrigger_GroupIDForwarded(t *testing.T) {
	var captured map[string]any
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "t-new"})
	}))
	defer hub.Close()

	tt := &TriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":         "my-trigger",
		"agentRef":     "agent-1",
		"connectorRef": "connector-1",
		"groupId":      "team-a",
	}
	result, err := tt.CreateTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if captured["groupId"] != "team-a" {
		t.Errorf("expected groupId team-a in payload; got %v", captured["groupId"])
	}

	// Without groupId the field must be omitted entirely so hubs without
	// access control keep working.
	captured = nil
	req.Params.Arguments = map[string]any{
		"name":         "my-trigger",
		"agentRef":     "agent-1",
		"connectorRef": "connector-1",
	}
	if _, err := tt.CreateTrigger(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := captured["groupId"]; ok {
		t.Errorf("expected no groupId in payload when omitted; got %v", captured["groupId"])
	}
}

func TestCreateTrigger_GroupIDRequiredHint(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"groupId is required"}`))
	}))
	defer hub.Close()

	tt := &TriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":         "my-trigger",
		"agentRef":     "agent-1",
		"connectorRef": "connector-1",
	}
	result, err := tt.CreateTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "groupId is required") {
		t.Errorf("expected original hub error in message; got %s", text)
	}
	if !strings.Contains(text, "retry with a groupId") {
		t.Errorf("expected groupId hint in message; got %s", text)
	}
}

func TestUpdateTrigger(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/triggers/t-123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if payload["connectorRef"] != "new-connector" {
			t.Errorf("unexpected connectorRef: %v", payload["connectorRef"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "t-123",
			"connectorRef": payload["connectorRef"],
		})
	}))
	defer hub.Close()

	tt := &TriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":         "t-123",
		"connectorRef": "new-connector",
	}
	result, err := tt.UpdateTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"id":"t-123"`) {
		t.Errorf("expected response to contain id; got %s", text)
	}
}

func TestUpdateTrigger_MissingName(t *testing.T) {
	tt := &TriggerTools{HubURL: "http://unused", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"connectorRef": "new-connector"}
	result, err := tt.UpdateTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
}
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

func TestListCronTriggers(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/cron-triggers" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "nightly-build", "name": "Nightly Build", "schedule": "0 2 * * *", "agentRef": "builder-agent", "enabled": true},
			},
			"total": 1,
		})
	}))
	defer hub.Close()

	st := &CronTriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := st.ListCronTriggers(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "nightly-build") {
		t.Errorf("expected response text to include cron trigger id; got: %s", text)
	}
}

func TestGetCronTrigger(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/cron-triggers/nightly-build" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "nightly-build", "name": "Nightly Build", "schedule": "0 2 * * *",
			"agentRef": "builder-agent", "prompt": "Run the nightly build", "enabled": true,
		})
	}))
	defer hub.Close()

	st := &CronTriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "nightly-build"}
	result, err := st.GetCronTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "nightly build") {
		t.Errorf("expected prompt in response; got: %s", text)
	}
}

func TestGetCronTriggerMissingName(t *testing.T) {
	st := &CronTriggerTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := st.GetCronTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing name")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "name is required") {
		t.Errorf("expected error to mention name is required; got: %s", text)
	}
}

func TestCreateCronTrigger(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/cron-triggers" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "nightly-build", "name": "Nightly Build", "schedule": "0 2 * * *",
			"agentRef": "builder-agent", "prompt": "Run the nightly build", "enabled": true,
		})
	}))
	defer hub.Close()

	st := &CronTriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":     "Nightly Build",
		"agentRef": "builder-agent",
		"schedule": "0 2 * * *",
		"prompt":   "Run the nightly build",
		"enabled":  true,
	}
	result, err := st.CreateCronTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "nightly-build") {
		t.Errorf("expected created cron trigger id in response; got: %s", text)
	}
	if !strings.Contains(capturedBody, "Nightly Build") {
		t.Errorf("expected name in request body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "builder-agent") {
		t.Errorf("expected agentRef in request body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "0 2 * * *") {
		t.Errorf("expected schedule in request body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "Run the nightly build") {
		t.Errorf("expected prompt in request body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "enabled") {
		t.Errorf("expected enabled in request body; got: %s", capturedBody)
	}
}

func TestCreateCronTriggerMissingFields(t *testing.T) {
	st := &CronTriggerTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}

	tests := []struct {
		name string
		args map[string]any
	}{
		{"missing name", map[string]any{"agentRef": "a", "schedule": "0 2 * * *", "prompt": "p"}},
		{"missing agentRef", map[string]any{"name": "n", "schedule": "0 2 * * *", "prompt": "p"}},
		{"missing schedule", map[string]any{"name": "n", "agentRef": "a", "prompt": "p"}},
		{"missing prompt", map[string]any{"name": "n", "agentRef": "a", "schedule": "0 2 * * *"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = tt.args
			result, _ := st.CreateCronTrigger(context.Background(), req)
			if !result.IsError {
				t.Errorf("expected error when %s", tt.name)
			}
		})
	}
}

func TestCreateCronTriggerOptionalFields(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "minimal", "name": "Minimal",
		})
	}))
	defer hub.Close()

	st := &CronTriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":     "Minimal",
		"agentRef": "agent-1",
		"schedule": "0 2 * * *",
		"prompt":   "do thing",
	}
	result, err := st.CreateCronTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	// enabled should not be in the payload when omitted
	if strings.Contains(capturedBody, "enabled") {
		t.Errorf("expected enabled to be omitted; got: %s", capturedBody)
	}
}

func TestUpdateCronTrigger(t *testing.T) {
	var capturedPath, capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected method: %s", r.Method)
		}
		capturedPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "nightly-build", "name": "Nightly Build", "schedule": "0 3 * * *",
			"agentRef": "builder-agent", "prompt": "Run the nightly build", "enabled": true,
		})
	}))
	defer hub.Close()

	st := &CronTriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":     "nightly-build",
		"schedule": "0 3 * * *",
	}
	result, err := st.UpdateCronTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedPath != "/api/v1/cron-triggers/nightly-build" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(capturedBody, "0 3 * * *") {
		t.Errorf("expected schedule in request body; got: %s", capturedBody)
	}
	// agentRef and prompt should be omitted since they were not provided
	if strings.Contains(capturedBody, "agentRef") {
		t.Errorf("expected agentRef to be omitted from partial update; got: %s", capturedBody)
	}
	if strings.Contains(capturedBody, "prompt") {
		t.Errorf("expected prompt to be omitted from partial update; got: %s", capturedBody)
	}
}

func TestUpdateCronTriggerMissingName(t *testing.T) {
	st := &CronTriggerTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"schedule": "0 3 * * *"}
	result, _ := st.UpdateCronTrigger(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when name is missing")
	}
}

func TestUpdateCronTriggerClearPrompt(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "nightly-build", "name": "Nightly Build", "schedule": "0 2 * * *",
			"agentRef": "builder-agent", "prompt": "", "enabled": true,
		})
	}))
	defer hub.Close()

	st := &CronTriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":   "nightly-build",
		"prompt": "",
	}
	result, err := st.UpdateCronTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	// prompt should be present in the body with an empty value, not omitted
	if !strings.Contains(capturedBody, `"prompt":""`) {
		t.Errorf("expected prompt to be present as empty string in request body; got: %s", capturedBody)
	}
}

func TestUpdateCronTriggerClearAgentRef(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "nightly-build", "name": "Nightly Build", "schedule": "0 2 * * *",
			"agentRef": "", "prompt": "Run the nightly build", "enabled": true,
		})
	}))
	defer hub.Close()

	st := &CronTriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":     "nightly-build",
		"agentRef": "",
	}
	result, err := st.UpdateCronTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	// agentRef should be present in the body with an empty value, not omitted
	if !strings.Contains(capturedBody, `"agentRef":""`) {
		t.Errorf("expected agentRef to be present as empty string in request body; got: %s", capturedBody)
	}
}

func TestUpdateCronTriggerEnabledFalse(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "nightly-build", "name": "Nightly Build", "enabled": false,
		})
	}))
	defer hub.Close()

	st := &CronTriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":    "nightly-build",
		"enabled": false,
	}
	result, err := st.UpdateCronTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	// enabled:false must be present — the ok-only check preserves false updates
	if !strings.Contains(capturedBody, `"enabled":false`) {
		t.Errorf("expected enabled:false in request body; got: %s", capturedBody)
	}
}

func TestDeleteCronTrigger(t *testing.T) {
	var capturedPath string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("unexpected method: %s", r.Method)
		}
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hub.Close()

	st := &CronTriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "nightly-build"}
	result, err := st.DeleteCronTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedPath != "/api/v1/cron-triggers/nightly-build" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(result.Content[0].(mcp.TextContent).Text, "nightly-build") {
		t.Errorf("expected name in success message; got: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestDeleteCronTriggerMissingName(t *testing.T) {
	st := &CronTriggerTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, _ := st.DeleteCronTrigger(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when name is missing")
	}
}

func TestGetCronTriggerNotFound(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer hub.Close()

	st := &CronTriggerTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "nonexistent"}
	result, err := st.GetCronTrigger(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for not found")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "404") {
		t.Errorf("expected 404 status in error; got: %s", text)
	}
}
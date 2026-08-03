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

func TestListPersonas(t *testing.T) {
	var capturedQuery string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/personas" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		capturedQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "01HX1", "name": "code-reviewer", "description": "reviews PRs", "currentVersion": 2},
			},
			"total": 1, "page": 1, "pageSize": 20, "totalPages": 1,
		})
	}))
	defer hub.Close()

	pt := &PersonaTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"page": float64(2), "pageSize": float64(10)}
	result, err := pt.ListPersonas(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "code-reviewer") {
		t.Errorf("expected response text to include persona name; got: %s", text)
	}
	if !strings.Contains(capturedQuery, "page=2") || !strings.Contains(capturedQuery, "pageSize=10") {
		t.Errorf("expected page+pageSize query forwarded; got: %s", capturedQuery)
	}
}

func TestGetPersona(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/personas/01HX1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "01HX1", "name": "code-reviewer", "description": "reviews PRs",
			"currentVersion": 2, "text": "You are a thorough code reviewer.",
		})
	}))
	defer hub.Close()

	pt := &PersonaTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"id": "01HX1"}
	result, err := pt.GetPersona(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "thorough code reviewer") {
		t.Errorf("expected persona text in response; got: %s", text)
	}
}

func TestGetPersonaMissingID(t *testing.T) {
	pt := &PersonaTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := pt.GetPersona(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing id")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "id is required") {
		t.Errorf("expected error to mention id is required; got: %s", text)
	}
}

func TestListPersonaVersions(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/personas/01HX1/versions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"personaId": "01HX1", "versionNumber": 2, "createdAt": "2026-05-10T00:00:00Z"},
				{"personaId": "01HX1", "versionNumber": 1, "createdAt": "2026-05-01T00:00:00Z"},
			},
			"total": 2, "page": 1, "pageSize": 20, "totalPages": 1,
		})
	}))
	defer hub.Close()

	pt := &PersonaTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"persona_id": "01HX1"}
	result, err := pt.ListPersonaVersions(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "versionNumber") {
		t.Errorf("expected versions in response; got: %s", text)
	}
}

func TestListPersonaVersionsMissingID(t *testing.T) {
	pt := &PersonaTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := pt.ListPersonaVersions(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing persona_id")
	}
}

func TestGetPersonaVersion(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/personas/01HX1/versions/3" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"personaId": "01HX1", "versionNumber": 3,
			"text":      "Older code reviewer text.",
			"createdAt": "2026-05-15T00:00:00Z",
		})
	}))
	defer hub.Close()

	pt := &PersonaTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"persona_id":     "01HX1",
		"version_number": float64(3),
	}
	result, err := pt.GetPersonaVersion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Older code reviewer text") {
		t.Errorf("expected versioned text in response; got: %s", text)
	}
}

func TestGetPersonaVersionMissingArgs(t *testing.T) {
	pt := &PersonaTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}

	// Missing persona_id
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"version_number": float64(1)}
	result, _ := pt.GetPersonaVersion(context.Background(), req)
	if !result.IsError {
		t.Error("expected error result when persona_id is missing")
	}

	// Missing version_number
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]any{"persona_id": "01HX1"}
	result2, _ := pt.GetPersonaVersion(context.Background(), req2)
	if !result2.IsError {
		t.Error("expected error result when version_number is missing")
	}

	// version_number <= 0
	req3 := mcp.CallToolRequest{}
	req3.Params.Arguments = map[string]any{"persona_id": "01HX1", "version_number": float64(0)}
	result3, _ := pt.GetPersonaVersion(context.Background(), req3)
	if !result3.IsError {
		t.Error("expected error result when version_number is 0 or negative")
	}
}

func TestCreatePersona(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/personas" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "01HX2", "name": "planner", "description": "plans things",
			"currentVersion": 1, "text": "You are a planner.",
		})
	}))
	defer hub.Close()

	pt := &PersonaTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":        "planner",
		"text":        "You are a planner.",
		"description": "plans things",
	}
	result, err := pt.CreatePersona(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "01HX2") {
		t.Errorf("expected created persona id in response; got: %s", text)
	}
	if !strings.Contains(capturedBody, "planner") {
		t.Errorf("expected name in request body; got: %s", capturedBody)
	}
}

func TestCreatePersonaMissingFields(t *testing.T) {
	pt := &PersonaTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}

	// Missing name
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"text": "some text"}
	result, _ := pt.CreatePersona(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when name is missing")
	}

	// Missing text
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]any{"name": "planner"}
	result2, _ := pt.CreatePersona(context.Background(), req2)
	if !result2.IsError {
		t.Error("expected error when text is missing")
	}
}

func TestUpdatePersona(t *testing.T) {
	var capturedPath, capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected method: %s", r.Method)
		}
		capturedPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "01HX1", "name": "planner", "description": "updated desc",
			"currentVersion": 2, "text": "Updated text.",
		})
	}))
	defer hub.Close()

	pt := &PersonaTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"id":   "01HX1",
		"text": "Updated text.",
	}
	result, err := pt.UpdatePersona(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedPath != "/api/v1/personas/01HX1" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(capturedBody, "Updated text") {
		t.Errorf("expected text in request body; got: %s", capturedBody)
	}
}

func TestUpdatePersonaMissingArgs(t *testing.T) {
	pt := &PersonaTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}

	// Missing id
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"text": "something"}
	result, _ := pt.UpdatePersona(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when id is missing")
	}

	// No fields to update
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]any{"id": "01HX1"}
	result2, _ := pt.UpdatePersona(context.Background(), req2)
	if !result2.IsError {
		t.Error("expected error when no fields provided")
	}
	if !strings.Contains(result2.Content[0].(mcp.TextContent).Text, "at least one") {
		t.Errorf("expected descriptive error; got: %s", result2.Content[0].(mcp.TextContent).Text)
	}
}

func TestDeletePersona(t *testing.T) {
	var capturedPath string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("unexpected method: %s", r.Method)
		}
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hub.Close()

	pt := &PersonaTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"id": "01HX1"}
	result, err := pt.DeletePersona(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedPath != "/api/v1/personas/01HX1" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(result.Content[0].(mcp.TextContent).Text, "01HX1") {
		t.Errorf("expected id in success message; got: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestDeletePersonaMissingID(t *testing.T) {
	pt := &PersonaTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, _ := pt.DeletePersona(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when id is missing")
	}
}

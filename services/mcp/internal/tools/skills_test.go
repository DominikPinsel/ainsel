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

func TestListSkills(t *testing.T) {
	var capturedQuery string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/skills" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		capturedQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "code-review", "name": "Code Review", "description": "Reviews PRs"},
			},
			"total": 1, "page": 1, "pageSize": 20, "totalPages": 1,
		})
	}))
	defer hub.Close()

	st := &SkillTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"page": float64(2), "pageSize": float64(10)}
	result, err := st.ListSkills(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "code-review") {
		t.Errorf("expected response text to include skill id; got: %s", text)
	}
	if !strings.Contains(capturedQuery, "page=2") || !strings.Contains(capturedQuery, "pageSize=10") {
		t.Errorf("expected page+pageSize query forwarded; got: %s", capturedQuery)
	}
}

func TestListSkillsNoPagination(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "page=1") || !strings.Contains(r.URL.RawQuery, "pageSize=50") {
			t.Errorf("expected default pagination params; got: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{}, "total": 0, "page": 1, "pageSize": 20, "totalPages": 0,
		})
	}))
	defer hub.Close()

	st := &SkillTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := st.ListSkills(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestGetSkill(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/skills/code-review" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "code-review", "name": "Code Review", "description": "Reviews PRs",
			"body": "You are a thorough code reviewer.",
		})
	}))
	defer hub.Close()

	st := &SkillTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"id": "code-review"}
	result, err := st.GetSkill(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "thorough code reviewer") {
		t.Errorf("expected skill body in response; got: %s", text)
	}
}

func TestGetSkillMissingID(t *testing.T) {
	st := &SkillTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, err := st.GetSkill(context.Background(), req)
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

func TestCreateSkill(t *testing.T) {
	var capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/skills" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "code-review", "name": "Code Review", "description": "Reviews PRs",
			"body": "You are a thorough code reviewer.",
		})
	}))
	defer hub.Close()

	st := &SkillTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"id":          "code-review",
		"name":        "Code Review",
		"description": "Reviews PRs",
		"body":        "You are a thorough code reviewer.",
	}
	result, err := st.CreateSkill(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "code-review") {
		t.Errorf("expected created skill id in response; got: %s", text)
	}
	if !strings.Contains(capturedBody, "code-review") {
		t.Errorf("expected id in request body; got: %s", capturedBody)
	}
	if !strings.Contains(capturedBody, "Code Review") {
		t.Errorf("expected name in request body; got: %s", capturedBody)
	}
}

func TestCreateSkillMissingFields(t *testing.T) {
	st := &SkillTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}

	// Missing id
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "Code Review"}
	result, _ := st.CreateSkill(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when id is missing")
	}

	// Missing name
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]any{"id": "code-review"}
	result2, _ := st.CreateSkill(context.Background(), req2)
	if !result2.IsError {
		t.Error("expected error when name is missing")
	}
}

func TestCreateSkillOptionalFields(t *testing.T) {
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

	st := &SkillTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"id":   "minimal",
		"name": "Minimal",
	}
	result, err := st.CreateSkill(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	// description and body should not be in the payload when omitted
	if strings.Contains(capturedBody, "description") {
		t.Errorf("expected description to be omitted; got: %s", capturedBody)
	}
	if strings.Contains(capturedBody, "body") {
		t.Errorf("expected body to be omitted; got: %s", capturedBody)
	}
}

func TestUpdateSkill(t *testing.T) {
	var capturedPath, capturedBody string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected method: %s", r.Method)
		}
		capturedPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		capturedBody = string(b)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "code-review", "name": "Code Review", "description": "Updated desc",
			"body": "You are a thorough code reviewer.",
		})
	}))
	defer hub.Close()

	st := &SkillTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"id":          "code-review",
		"description": "Updated desc",
	}
	result, err := st.UpdateSkill(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedPath != "/api/v1/skills/code-review" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(capturedBody, "Updated desc") {
		t.Errorf("expected description in request body; got: %s", capturedBody)
	}
	// name and body should be omitted since they were not provided
	if strings.Contains(capturedBody, "name") {
		t.Errorf("expected name to be omitted from partial update; got: %s", capturedBody)
	}
}

func TestUpdateSkillMissingArgs(t *testing.T) {
	st := &SkillTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}

	// Missing id
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"name": "something"}
	result, _ := st.UpdateSkill(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when id is missing")
	}

	// No fields to update
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]any{"id": "code-review"}
	result2, _ := st.UpdateSkill(context.Background(), req2)
	if !result2.IsError {
		t.Error("expected error when no fields provided")
	}
	if !strings.Contains(result2.Content[0].(mcp.TextContent).Text, "at least one") {
		t.Errorf("expected descriptive error; got: %s", result2.Content[0].(mcp.TextContent).Text)
	}
}

func TestDeleteSkill(t *testing.T) {
	var capturedPath string
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("unexpected method: %s", r.Method)
		}
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hub.Close()

	st := &SkillTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"id": "code-review"}
	result, err := st.DeleteSkill(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if capturedPath != "/api/v1/skills/code-review" {
		t.Errorf("unexpected path: %s", capturedPath)
	}
	if !strings.Contains(result.Content[0].(mcp.TextContent).Text, "code-review") {
		t.Errorf("expected id in success message; got: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestDeleteSkillMissingID(t *testing.T) {
	st := &SkillTools{HubURL: "http://nope", HTTPClient: http.DefaultClient}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}
	result, _ := st.DeleteSkill(context.Background(), req)
	if !result.IsError {
		t.Error("expected error when id is missing")
	}
}

func TestDeleteSkillInUse(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "skill in use",
			"referrers": []map[string]any{
				{"agentImageName": "reviewer-agent"},
			},
		})
	}))
	defer hub.Close()

	st := &SkillTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"id": "code-review"}
	result, err := st.DeleteSkill(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for skill in use")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "409") {
		t.Errorf("expected 409 status in error; got: %s", text)
	}
	if !strings.Contains(text, "skill in use") {
		t.Errorf("expected 'skill in use' in error; got: %s", text)
	}
}

func TestGetSkillNotFound(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer hub.Close()

	st := &SkillTools{HubURL: hub.URL, HTTPClient: hub.Client()}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"id": "nonexistent"}
	result, err := st.GetSkill(context.Background(), req)
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
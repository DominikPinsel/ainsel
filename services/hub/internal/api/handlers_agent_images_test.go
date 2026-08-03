package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/mcpservers"
	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// registerAgentImageRoutes wires the agent-images routes onto the test server.
func registerAgentImageRoutes(s *Server) {
	s.mux.HandleFunc("/api/v1/agent-images", s.handleAgentImages)
	s.mux.HandleFunc("/api/v1/agent-images/", s.handleAgentImagePath)
}

// TestAgentImages_EmptyList verifies that a GET on an empty store returns 200
// with an empty items array in the paginated envelope.
func TestAgentImages_EmptyList(t *testing.T) {
	s := testServer(t)
	registerAgentImageRoutes(s)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-images", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var page struct {
		Items      []SimpleAgentImageResponse `json:"items"`
		Total      int                        `json:"total"`
		Page       int                        `json:"page"`
		PageSize   int                        `json:"pageSize"`
		TotalPages int                        `json:"totalPages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if page.Items == nil {
		t.Errorf("expected non-nil items array, got nil")
	}
	if len(page.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(page.Items))
	}
	if page.Total != 0 {
		t.Errorf("expected total=0, got %d", page.Total)
	}
	if page.Page != 1 {
		t.Errorf("expected page=1, got %d", page.Page)
	}
}

// TestAgentImages_CreateValid verifies that POST with a valid body returns 201
// with a generated img-... ID and Status.Phase == "Pending".
func TestAgentImages_CreateValid(t *testing.T) {
	s := testServer(t)
	registerAgentImageRoutes(s)

	body, _ := json.Marshal(SimpleAgentImageCreateRequest{
		DisplayName: "My Image",
		ImageURL:    "registry.example.com/my-image:v1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !strings.HasPrefix(resp.ID, "img-") {
		t.Errorf("expected ID to start with 'img-', got %q", resp.ID)
	}
	// "img-" (4) + 8 hex chars = 12 chars total
	if len(resp.ID) != 12 {
		t.Errorf("expected ID length 12 (img-XXXXXXXX), got %d: %q", len(resp.ID), resp.ID)
	}
	if resp.DisplayName != "My Image" {
		t.Errorf("expected displayName 'My Image', got %q", resp.DisplayName)
	}
	if resp.ImageURL != "registry.example.com/my-image:v1" {
		t.Errorf("expected imageURL 'registry.example.com/my-image:v1', got %q", resp.ImageURL)
	}
	if resp.Status.Phase != string(agentv1alpha1.AgentImagePhaseReady) {
		t.Errorf("expected status.phase 'Ready', got %q", resp.Status.Phase)
	}
	if resp.Tools == nil {
		t.Errorf("expected non-nil tools array, got nil")
	}
}

// TestAgentImages_CreateMissingImageURL verifies that POST without imageURL returns 400.
func TestAgentImages_CreateMissingImageURL(t *testing.T) {
	s := testServer(t)
	registerAgentImageRoutes(s)

	body, _ := json.Marshal(SimpleAgentImageCreateRequest{
		DisplayName: "No URL Image",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAgentImages_CreateMissingDisplayName verifies that POST without displayName returns 400.
func TestAgentImages_CreateMissingDisplayName(t *testing.T) {
	s := testServer(t)
	registerAgentImageRoutes(s)

	body, _ := json.Marshal(SimpleAgentImageCreateRequest{
		ImageURL: "registry.example.com/my-image:v1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAgentImages_ToolCount verifies that GET returns the correct toolCount.
func TestAgentImages_ToolCount(t *testing.T) {
	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "img-tools001",
			Namespace: "test-ns",
		},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Image With Tools",
			ImageURL:    "registry.example.com/tools:v1",
			Tools: []agentv1alpha1.AgentImageTool{
				{Name: "git", Kind: agentv1alpha1.AgentImageToolKindShell},
				{Name: "mcp__example-mcp__list", Kind: agentv1alpha1.AgentImageToolKindMCP, McpSource: "example-mcp"},
				{Name: "ls", Kind: agentv1alpha1.AgentImageToolKindShell, Disabled: true},
			},
		},
		Status: agentv1alpha1.AgentImageStatus{
			Phase: agentv1alpha1.AgentImagePhaseReady,
		},
	}

	s := testServer(t, existing)
	registerAgentImageRoutes(s)

	// Verify GET returns correct toolCount.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-images/img-tools001", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ToolCount != 3 {
		t.Errorf("expected toolCount 3, got %d", resp.ToolCount)
	}
	if len(resp.Tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(resp.Tools))
	}

	// Verify LIST also returns correct toolCount.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/agent-images", nil)
	listRec := httptest.NewRecorder()
	s.mux.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var page struct {
		Items []SimpleAgentImageResponse `json:"items"`
		Total int                        `json:"total"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&page); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected total=1, got %d", page.Total)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	if page.Items[0].ToolCount != 3 {
		t.Errorf("expected list toolCount 3, got %d", page.Items[0].ToolCount)
	}
}

// TestAgentImages_GetNotFound verifies that GET on a non-existent image returns 404.
func TestAgentImages_GetNotFound(t *testing.T) {
	s := testServer(t)
	registerAgentImageRoutes(s)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-images/does-not-exist", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAgentImages_GetExisting verifies that GET on an existing image returns 200 with the right body.
func TestAgentImages_GetExisting(t *testing.T) {
	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "img-abc12345",
			Namespace: "test-ns",
		},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Existing Image",
			ImageURL:    "registry.example.com/existing:v2",
		},
		Status: agentv1alpha1.AgentImageStatus{
			Phase: agentv1alpha1.AgentImagePhaseReady,
		},
	}

	s := testServer(t, existing)
	registerAgentImageRoutes(s)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-images/img-abc12345", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID != "img-abc12345" {
		t.Errorf("expected ID 'img-abc12345', got %q", resp.ID)
	}
	if resp.DisplayName != "Existing Image" {
		t.Errorf("expected displayName 'Existing Image', got %q", resp.DisplayName)
	}
	if resp.ImageURL != "registry.example.com/existing:v2" {
		t.Errorf("expected imageURL 'registry.example.com/existing:v2', got %q", resp.ImageURL)
	}
	if resp.Status.Phase != "Ready" {
		t.Errorf("expected phase 'Ready', got %q", resp.Status.Phase)
	}
}

// TestUpdateAgentImage_AddDescription verifies that PUT with description only
// returns 200 and updates the description without touching other fields.
func TestUpdateAgentImage_AddDescription(t *testing.T) {
	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "img-upd00001",
			Namespace: "test-ns",
		},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Update Test Image",
			ImageURL:    "registry.example.com/update-test:v1",
		},
		Status: agentv1alpha1.AgentImageStatus{
			Phase: agentv1alpha1.AgentImagePhaseReady,
		},
	}

	s := testServer(t, existing)
	registerAgentImageRoutes(s)

	desc := "A new description"
	body, _ := json.Marshal(SimpleAgentImageUpdateRequest{
		Description: &desc,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-images/img-upd00001", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Description != "A new description" {
		t.Errorf("expected description 'A new description', got %q", resp.Description)
	}
	// Original fields should be unchanged.
	if resp.DisplayName != "Update Test Image" {
		t.Errorf("expected displayName unchanged 'Update Test Image', got %q", resp.DisplayName)
	}
	if resp.ImageURL != "registry.example.com/update-test:v1" {
		t.Errorf("expected imageURL unchanged, got %q", resp.ImageURL)
	}
}

// TestUpdateAgentImage_RemoveToolReferencedByAgent verifies that PUT with an
// empty tools list returns 409 when an Agent references the removed tool.
func TestUpdateAgentImage_RemoveToolReferencedByAgent(t *testing.T) {
	img := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "img-tooltest1",
			Namespace: "test-ns",
		},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Tool Test Image",
			ImageURL:    "registry.example.com/tool-test:v1",
			Tools: []agentv1alpha1.AgentImageTool{
				{Name: "git", Kind: agentv1alpha1.AgentImageToolKindShell},
			},
		},
		Status: agentv1alpha1.AgentImageStatus{
			Phase: agentv1alpha1.AgentImagePhaseReady,
		},
	}

	agent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-agent",
			Namespace: "test-ns",
		},
		Spec: agentv1alpha1.AgentSpec{
			DisplayName:  "My Agent",
			ImageRef:     agentv1alpha1.AgentImageRef{Name: "img-tooltest1"},
			Runtime:      agentv1alpha1.AgentRuntime{},
			LLM:          agentv1alpha1.AgentLLM{Model: "glm-5.1:cloud"},
			EnabledTools: []string{"git"},
		},
	}

	s := testServer(t, img, agent)
	registerAgentImageRoutes(s)

	emptyTools := []AgentImageToolInfo{}
	body, _ := json.Marshal(SimpleAgentImageUpdateRequest{
		Tools: &emptyTools,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-images/img-tooltest1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp struct {
		Error          string   `json:"error"`
		AffectedAgents []string `json:"affectedAgents"`
		RemovedTools   []string `json:"removedTools"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error != "tool referenced by agent(s)" {
		t.Errorf("expected error 'tool referenced by agent(s)', got %q", errResp.Error)
	}
	if len(errResp.AffectedAgents) == 0 {
		t.Errorf("expected at least one affected agent, got none")
	}
	found := false
	for _, a := range errResp.AffectedAgents {
		if a == "my-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'my-agent' in affectedAgents, got %v", errResp.AffectedAgents)
	}
}

// TestDeleteAgentImage_OK verifies that DELETE on an image with no referencing
// agents returns 204.
func TestDeleteAgentImage_OK(t *testing.T) {
	img := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "img-del00001",
			Namespace: "test-ns",
		},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Deletable Image",
			ImageURL:    "registry.example.com/deletable:v1",
		},
	}

	s := testServer(t, img)
	registerAgentImageRoutes(s)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agent-images/img-del00001", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify the image is gone.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/agent-images/img-del00001", nil)
	getRec := httptest.NewRecorder()
	s.mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getRec.Code)
	}
}

// TestDeleteAgentImage_RejectsWhenAgentReferences verifies that DELETE returns
// 409 listing affected agents when an Agent references the image.
func TestDeleteAgentImage_RejectsWhenAgentReferences(t *testing.T) {
	img := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "img-nodlt001",
			Namespace: "test-ns",
		},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Referenced Image",
			ImageURL:    "registry.example.com/referenced:v1",
		},
	}

	agent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ref-agent",
			Namespace: "test-ns",
		},
		Spec: agentv1alpha1.AgentSpec{
			DisplayName: "Referencing Agent",
			ImageRef:    agentv1alpha1.AgentImageRef{Name: "img-nodlt001"},
			Runtime:     agentv1alpha1.AgentRuntime{},
			LLM:         agentv1alpha1.AgentLLM{Model: "glm-5.1:cloud"},
		},
	}

	s := testServer(t, img, agent)
	registerAgentImageRoutes(s)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agent-images/img-nodlt001", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp struct {
		Error          string   `json:"error"`
		AffectedAgents []string `json:"affectedAgents"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if len(errResp.AffectedAgents) == 0 {
		t.Errorf("expected at least one affected agent, got none")
	}
	found := false
	for _, a := range errResp.AffectedAgents {
		if a == "ref-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'ref-agent' in affectedAgents, got %v", errResp.AffectedAgents)
	}
}

// TestDeleteAgentImage_NotFound verifies that DELETE on an image that does not
// exist returns 404 with a clear error body.
func TestDeleteAgentImage_NotFound(t *testing.T) {
	s := testServer(t)
	registerAgentImageRoutes(s)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agent-images/img-missing1", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error != "agent image not found" {
		t.Errorf("expected error 'agent image not found', got %q", errResp.Error)
	}
}

func TestAgentImages_CreateWithEnv(t *testing.T) {
	s := testServer(t)
	registerAgentImageRoutes(s)

	body, _ := json.Marshal(SimpleAgentImageCreateRequest{
		DisplayName: "Test Image",
		ImageURL:    "ghcr.io/test/image:latest",
		Env: []AgentImageEnvVarInfo{
			{Name: "FORGEJO_URL", Value: "https://git.example.com"},
			{Name: "OTHER_VAR", Value: "hello"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(resp.Env))
	}
	found := map[string]string{}
	for _, e := range resp.Env {
		found[e.Name] = e.Value
	}
	if found["FORGEJO_URL"] != "https://git.example.com" {
		t.Errorf("unexpected FORGEJO_URL: %q", found["FORGEJO_URL"])
	}
	if found["OTHER_VAR"] != "hello" {
		t.Errorf("unexpected OTHER_VAR: %q", found["OTHER_VAR"])
	}
}

func TestAgentImages_GetReturnsEnv(t *testing.T) {
	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: "img-envtest1", Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Env Test Image",
			ImageURL:    "ghcr.io/test/image:latest",
			Env: []agentv1alpha1.AgentImageEnvVar{
				{Name: "FORGEJO_URL", Value: "https://git.example.com"},
			},
		},
		Status: agentv1alpha1.AgentImageStatus{Phase: agentv1alpha1.AgentImagePhaseReady},
	}

	s := testServer(t, existing)
	registerAgentImageRoutes(s)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-images/img-envtest1", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Env) != 1 || resp.Env[0].Name != "FORGEJO_URL" || resp.Env[0].Value != "https://git.example.com" {
		t.Errorf("unexpected env: %+v", resp.Env)
	}
	if resp.Env[0].Secret {
		t.Errorf("non-secret env var should have Secret=false")
	}
}

func TestAgentImages_UpdateEnv(t *testing.T) {
	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: "img-envupd01", Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Env Update Image",
			ImageURL:    "ghcr.io/test/image:latest",
		},
		Status: agentv1alpha1.AgentImageStatus{Phase: agentv1alpha1.AgentImagePhaseReady},
	}

	s := testServer(t, existing)
	registerAgentImageRoutes(s)

	newEnv := []AgentImageEnvVarInfo{{Name: "FORGEJO_URL", Value: "https://git.example.com"}}
	body, _ := json.Marshal(SimpleAgentImageUpdateRequest{Env: &newEnv})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-images/img-envupd01", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Env) != 1 || resp.Env[0].Name != "FORGEJO_URL" {
		t.Errorf("unexpected env: %+v", resp.Env)
	}
}

func TestAgentImages_UpdateNilEnvPreservesExisting(t *testing.T) {
	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: "img-envprs01", Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Env Preserve Image",
			ImageURL:    "ghcr.io/test/image:latest",
			Env:         []agentv1alpha1.AgentImageEnvVar{{Name: "KEEP_ME", Value: "yes"}},
		},
		Status: agentv1alpha1.AgentImageStatus{Phase: agentv1alpha1.AgentImagePhaseReady},
	}

	s := testServer(t, existing)
	registerAgentImageRoutes(s)

	newName := "Updated Name"
	body, _ := json.Marshal(SimpleAgentImageUpdateRequest{DisplayName: &newName})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-images/img-envprs01", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Env) != 1 || resp.Env[0].Name != "KEEP_ME" {
		t.Errorf("env should be preserved when Env is nil in request: %+v", resp.Env)
	}
	if resp.DisplayName != "Updated Name" {
		t.Errorf("displayName should be updated: %s", resp.DisplayName)
	}
}

// TestAgentImages_UpdateToolEnabled verifies that PUT persists Enabled on tools and
// always clears IsNew regardless of what the client sends.
func TestAgentImages_UpdateToolEnabled(t *testing.T) {
	// Seed an image with an MCP tool that is new and disabled.
	img := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: "img-test", Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Test",
			ImageURL:    "registry.example.com/test:v1",
			Tools: []agentv1alpha1.AgentImageTool{
				{
					Name:      "mcp__example-mcp__list",
					Kind:      agentv1alpha1.AgentImageToolKindMCP,
					McpSource: "example-mcp",
					Disabled:  true,
					IsNew:     true,
				},
			},
		},
	}
	img.Status.Phase = agentv1alpha1.AgentImagePhaseReady

	s := testServer(t, img)
	registerAgentImageRoutes(s)

	// Client sends Enabled=true (enabling the tool) and IsNew=true (client still sees badge).
	// Backend should: set Disabled=false AND clear IsNew regardless.
	body, _ := json.Marshal(SimpleAgentImageUpdateRequest{
		Tools: &[]AgentImageToolInfo{
			{
				Name:      "mcp__example-mcp__list",
				Kind:      "mcp",
				McpSource: "example-mcp",
				Enabled:   true,
				IsNew:     true, // client still sees isNew — backend must clear it
			},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-images/img-test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(resp.Tools))
	}
	if !resp.Tools[0].Enabled {
		t.Error("tool should now be Enabled=true")
	}
	if resp.Tools[0].IsNew {
		t.Error("IsNew should be cleared on PUT")
	}
}

// TestAgentImages_UpdateToolDisabled verifies that PUT with Enabled=false persists
// Disabled=true in the stored CRD — catching the zero-value false bypass.
func TestAgentImages_UpdateToolDisabled(t *testing.T) {
	// Seed an enabled (Disabled=false) tool.
	img := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: "img-dis", Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Test",
			ImageURL:    "registry.example.com/test:v1",
			Tools: []agentv1alpha1.AgentImageTool{
				{
					Name:      "mcp__example-mcp__list",
					Kind:      agentv1alpha1.AgentImageToolKindMCP,
					McpSource: "example-mcp",
					Disabled:  false, // currently enabled
				},
			},
		},
	}
	img.Status.Phase = agentv1alpha1.AgentImagePhaseReady

	s := testServer(t, img)
	registerAgentImageRoutes(s)

	// Client sends Enabled=false — handler must persist Disabled=true.
	body, _ := json.Marshal(SimpleAgentImageUpdateRequest{
		Tools: &[]AgentImageToolInfo{
			{
				Name:      "mcp__example-mcp__list",
				Kind:      "mcp",
				McpSource: "example-mcp",
				Enabled:   false, // disable it
			},
		},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-images/img-dis", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(resp.Tools))
	}
	if resp.Tools[0].Enabled {
		t.Error("tool should now be Enabled=false (was disabled by client)")
	}
}

// Ensure AgentImage implements runtime.Object (compile-time check).
var _ runtime.Object = (*agentv1alpha1.AgentImage)(nil)

// --- Secret env var tests ---

// TestAgentImages_CreateWithSecretEnv verifies that creating an image with a
// secret env var stores the value but the API response redacts it.
func TestAgentImages_CreateWithSecretEnv(t *testing.T) {
	s := testServer(t)
	registerAgentImageRoutes(s)

	body, _ := json.Marshal(SimpleAgentImageCreateRequest{
		DisplayName: "Secret Test Image",
		ImageURL:    "ghcr.io/test/image:latest",
		Env: []AgentImageEnvVarInfo{
			{Name: "FORGEJO_TOKEN", Value: "s3cr3t", Secret: true},
			{Name: "LOG_LEVEL", Value: "debug", Secret: false},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/agent-images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(resp.Env))
	}

	// Secret env var should have value redacted.
	var secretVar, plainVar AgentImageEnvVarInfo
	for _, e := range resp.Env {
		if e.Name == "FORGEJO_TOKEN" {
			secretVar = e
		}
		if e.Name == "LOG_LEVEL" {
			plainVar = e
		}
	}
	if secretVar.Value != "" {
		t.Errorf("secret env var value should be redacted, got %q", secretVar.Value)
	}
	if !secretVar.Secret {
		t.Errorf("FORGEJO_TOKEN should have Secret=true")
	}
	if plainVar.Value != "debug" {
		t.Errorf("plain env var value should be returned, got %q", plainVar.Value)
	}
	if plainVar.Secret {
		t.Errorf("LOG_LEVEL should have Secret=false")
	}
}

// TestAgentImages_GetSecretEnvRedacted verifies that GET on an image with
// secret env vars returns redacted values.
func TestAgentImages_GetSecretEnvRedacted(t *testing.T) {
	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: "img-sec01", Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Secret Image",
			ImageURL:    "ghcr.io/test/image:latest",
			Env: []agentv1alpha1.AgentImageEnvVar{
				{Name: "API_KEY", Value: "super-secret", Secret: true},
				{Name: "LOG_LEVEL", Value: "debug"},
			},
		},
		Status: agentv1alpha1.AgentImageStatus{Phase: agentv1alpha1.AgentImagePhaseReady},
	}

	s := testServer(t, existing)
	registerAgentImageRoutes(s)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-images/img-sec01", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(resp.Env))
	}

	for _, e := range resp.Env {
		if e.Name == "API_KEY" {
			if e.Value != "" {
				t.Errorf("secret env var value should be redacted in GET, got %q", e.Value)
			}
			if !e.Secret {
				t.Errorf("API_KEY should have Secret=true")
			}
		}
		if e.Name == "LOG_LEVEL" {
			if e.Value != "debug" {
				t.Errorf("plain env var value should be returned, got %q", e.Value)
			}
		}
	}
}

// TestAgentImages_UpdateSecretEnvPreservesValue verifies that updating a
// secret env var with an empty value preserves the existing value in the CRD.
func TestAgentImages_UpdateSecretEnvPreservesValue(t *testing.T) {
	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: "img-secupd01", Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Secret Update Image",
			ImageURL:    "ghcr.io/test/image:latest",
			Env: []agentv1alpha1.AgentImageEnvVar{
				{Name: "API_KEY", Value: "existing-secret", Secret: true},
				{Name: "LOG_LEVEL", Value: "debug"},
			},
		},
		Status: agentv1alpha1.AgentImageStatus{Phase: agentv1alpha1.AgentImagePhaseReady},
	}

	s := testServer(t, existing)
	registerAgentImageRoutes(s)

	// Send update with empty value for the secret env var – should preserve the existing value.
	newEnv := []AgentImageEnvVarInfo{
		{Name: "API_KEY", Value: "", Secret: true},
		{Name: "LOG_LEVEL", Value: "info"},
	}
	body, _ := json.Marshal(SimpleAgentImageUpdateRequest{Env: &newEnv})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-images/img-secupd01", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Read back the CRD object from the fake client to verify the actual stored value.
	var img agentv1alpha1.AgentImage
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: "img-secupd01", Namespace: "test-ns"}, &img); err != nil {
		t.Fatalf("get crd: %v", err)
	}
	for _, e := range img.Spec.Env {
		if e.Name == "API_KEY" {
			if e.Value != "existing-secret" {
				t.Errorf("secret env var value should be preserved, got %q", e.Value)
			}
			if !e.Secret {
				t.Errorf("API_KEY should have Secret=true")
			}
		}
		if e.Name == "LOG_LEVEL" {
			if e.Value != "info" {
				t.Errorf("plain env var should be updated, got %q", e.Value)
			}
		}
	}
}

// TestAgentImages_UpdateSecretEnvOverwritesValue verifies that updating a
// secret env var with a non-empty value replaces the stored value.
func TestAgentImages_UpdateSecretEnvOverwritesValue(t *testing.T) {
	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: "img-secow01", Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Secret Overwrite Image",
			ImageURL:    "ghcr.io/test/image:latest",
			Env: []agentv1alpha1.AgentImageEnvVar{
				{Name: "API_KEY", Value: "old-secret", Secret: true},
			},
		},
		Status: agentv1alpha1.AgentImageStatus{Phase: agentv1alpha1.AgentImagePhaseReady},
	}

	s := testServer(t, existing)
	registerAgentImageRoutes(s)

	newEnv := []AgentImageEnvVarInfo{
		{Name: "API_KEY", Value: "new-secret", Secret: true},
	}
	body, _ := json.Marshal(SimpleAgentImageUpdateRequest{Env: &newEnv})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-images/img-secow01", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var img agentv1alpha1.AgentImage
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: "img-secow01", Namespace: "test-ns"}, &img); err != nil {
		t.Fatalf("get crd: %v", err)
	}
	for _, e := range img.Spec.Env {
		if e.Name == "API_KEY" {
			if e.Value != "new-secret" {
				t.Errorf("secret env var value should be overwritten, got %q", e.Value)
			}
		}
	}
}

func agentImageTestServerWithMCP(t *testing.T, objects ...runtime.Object) *Server {
	t.Helper()
	s := testServer(t, objects...)
	s.mcp = mcpServiceForTest(t)
	registerAgentImageRoutes(s)
	return s
}

func TestAgentImages_CreateWithMCPServerReference(t *testing.T) {
	s := agentImageTestServerWithMCP(t)
	ctx := t.Context()
	_ = s.mcp.Create(ctx, &mcpservers.MCPServer{Name: "github", DisplayName: "GitHub", URL: "https://mcp.github.com/sse", TokenFromEnv: "GITHUB_TOKEN"}) // #nosec G101 -- test data, env var name not a credential

	body, _ := json.Marshal(SimpleAgentImageCreateRequest{
		DisplayName: "Test Image",
		ImageURL:    "ghcr.io/test/image:latest",
		MCPServers:  []string{"github"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.MCPServers) != 1 {
		t.Fatalf("expected 1 mcp server, got %d", len(resp.MCPServers))
	}
	if resp.MCPServers[0].Name != "github" {
		t.Errorf("expected name 'github', got %q", resp.MCPServers[0].Name)
	}
	if resp.MCPServers[0].URL != "https://mcp.github.com/sse" {
		t.Errorf("expected url 'https://mcp.github.com/sse', got %q", resp.MCPServers[0].URL)
	}
	if resp.MCPServers[0].TokenFromEnv != "GITHUB_TOKEN" {
		t.Errorf("tokenFromEnv should be carried through: %q", resp.MCPServers[0].TokenFromEnv)
	}

	// Verify the CRD stores the env-var reference.
	var img agentv1alpha1.AgentImage
	if err := s.client.Get(ctx, types.NamespacedName{Name: resp.ID, Namespace: "test-ns"}, &img); err != nil {
		t.Fatalf("get crd: %v", err)
	}
	if len(img.Spec.MCPServers) != 1 || img.Spec.MCPServers[0].TokenFromEnv != "GITHUB_TOKEN" {
		t.Errorf("crd should store tokenFromEnv: %+v", img.Spec.MCPServers)
	}
}

func TestAgentImages_CreateRejectsMissingMCPServer(t *testing.T) {
	s := agentImageTestServerWithMCP(t)

	body, _ := json.Marshal(SimpleAgentImageCreateRequest{
		DisplayName: "Test Image",
		ImageURL:    "ghcr.io/test/image:latest",
		MCPServers:  []string{"missing"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentImages_UpdateWithMCPServerReference(t *testing.T) {
	s := agentImageTestServerWithMCP(t)
	ctx := t.Context()
	_ = s.mcp.Create(ctx, &mcpservers.MCPServer{Name: "github", DisplayName: "GitHub", URL: "https://mcp.github.com/sse", TokenFromEnv: "GITHUB_TOKEN"}) // #nosec G101 -- test data, env var name not a credential

	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: "img-mcpupd01", Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "MCP Update Image",
			ImageURL:    "ghcr.io/test/image:latest",
		},
		Status: agentv1alpha1.AgentImageStatus{Phase: agentv1alpha1.AgentImagePhaseReady},
	}
	if err := s.client.Create(ctx, existing); err != nil {
		t.Fatalf("create existing: %v", err)
	}

	newMCPServers := []string{"github"}
	body, _ := json.Marshal(SimpleAgentImageUpdateRequest{MCPServers: &newMCPServers})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-images/img-mcpupd01", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.MCPServers) != 1 || resp.MCPServers[0].Name != "github" {
		t.Errorf("unexpected mcp servers: %+v", resp.MCPServers)
	}
}

// TestAgentImages_CreateHasNoDefaultMCPServers verifies that newly created
// AgentImages have no MCP servers unless the request explicitly names them.
func TestAgentImages_CreateHasNoDefaultMCPServers(t *testing.T) {
	s := testServer(t)
	registerAgentImageRoutes(s)

	body, _ := json.Marshal(SimpleAgentImageCreateRequest{
		DisplayName: "Plain Image",
		ImageURL:    "registry.example.com/plain:v1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.MCPServers) != 0 {
		t.Errorf("expected no default MCP servers in new image, got %d: %+v", len(resp.MCPServers), resp.MCPServers)
	}
}

func skillsServiceForTest(t *testing.T) *skills.Service {
	t.Helper()
	ctx := t.Context()
	c, err := pgcontainer.Run(ctx, "postgres:17-alpine",
		pgcontainer.WithDatabase("test"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		pgcontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	dsn, _ := c.ConnectionString(ctx, "sslmode=disable")
	if err := db.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	return skills.NewService(skills.NewStore(pool), nil, nil)
}

func agentImageTestServerWithSkills(t *testing.T, objects ...runtime.Object) *Server {
	t.Helper()
	s := testServer(t, objects...)
	s.skills = skillsServiceForTest(t)
	registerAgentImageRoutes(s)
	return s
}

func TestAgentImages_CreateWithInvalidEnabledSkills(t *testing.T) {
	s := agentImageTestServerWithSkills(t)
	body, _ := json.Marshal(SimpleAgentImageCreateRequest{
		DisplayName:   "Test Image",
		ImageURL:      "ghcr.io/test/image:latest",
		EnabledSkills: []string{"nonexistent-skill"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "nonexistent-skill") {
		t.Errorf("expected error body to mention the bad skill id, got: %s", rec.Body.String())
	}
}

func TestAgentImages_UpdateWithInvalidEnabledSkills(t *testing.T) {
	s := agentImageTestServerWithSkills(t)
	ctx := t.Context()
	existing := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: "img-skills01", Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: "Skill Test Image",
			ImageURL:    "ghcr.io/test/image:latest",
		},
		Status: agentv1alpha1.AgentImageStatus{Phase: agentv1alpha1.AgentImagePhaseReady},
	}
	if err := s.client.Create(ctx, existing); err != nil {
		t.Fatalf("create existing: %v", err)
	}

	badSkills := []string{"nonexistent-skill"}
	body, _ := json.Marshal(SimpleAgentImageUpdateRequest{EnabledSkills: &badSkills})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agent-images/img-skills01", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "nonexistent-skill") {
		t.Errorf("expected error body to mention the bad skill id, got: %s", rec.Body.String())
	}
}

func TestAgentImages_CreateWithValidEnabledSkills(t *testing.T) {
	s := agentImageTestServerWithSkills(t)
	ctx := t.Context()
	_, err := s.skills.Create(ctx, skills.CreateRequest{
		ID:          "git-review",
		Name:        "Git Review",
		Description: "Review PRs",
		Body:        "Review body",
	})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}

	body, _ := json.Marshal(SimpleAgentImageCreateRequest{
		DisplayName:   "Test Image",
		ImageURL:      "ghcr.io/test/image:latest",
		EnabledSkills: []string{"git-review"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-images", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentImageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.EnabledSkills) != 1 || resp.EnabledSkills[0] != "git-review" {
		t.Errorf("unexpected enabled skills: %+v", resp.EnabledSkills)
	}
}

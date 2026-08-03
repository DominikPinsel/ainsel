package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// testAgentImage builds a minimal AgentImage with the given name and tools for use
// as a test fixture. The display name defaults to the resource name.
func testAgentImage(name string, tools ...string) *agentv1alpha1.AgentImage {
	return testAgentImageWithDisplayName(name, name, tools...)
}

// testAgentImageWithDisplayName builds an AgentImage with an explicit display name.
func testAgentImageWithDisplayName(name, displayName string, tools ...string) *agentv1alpha1.AgentImage {
	img := &agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test-ns"},
		Spec: agentv1alpha1.AgentImageSpec{
			DisplayName: displayName,
			ImageURL:    "registry/" + name + ":latest",
		},
	}
	for _, t := range tools {
		img.Spec.Tools = append(img.Spec.Tools, agentv1alpha1.AgentImageTool{
			Name: t,
			Kind: agentv1alpha1.AgentImageToolKindContainer,
		})
	}
	return img
}

func TestAgents_CreateAndList(t *testing.T) {
	img := testAgentImage("img-1", "git", "bash")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	createReq := SimpleAgentCreateRequest{
		Name:        "My Test Agent",
		Description: "A test agent",
		ImageRef:    AgentImageRefInfo{Name: "img-1"},
		LLM: AgentLLMInfo{
			Model: "glm-5.1:cloud",
		},
		EnabledTools: []string{"git"},
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !strings.HasPrefix(created.ID, "a-") {
		t.Errorf("expected ID to start with 'a-', got %s", created.ID)
	}
	if len(created.ID) != 10 {
		t.Errorf("expected ID length 10 (a-XXXXXXXX), got %d: %s", len(created.ID), created.ID)
	}
	if created.Name != "My Test Agent" {
		t.Errorf("expected name 'My Test Agent', got %s", created.Name)
	}
	if created.Description != "A test agent" {
		t.Errorf("expected description 'A test agent', got %s", created.Description)
	}
	if created.ImageRef.Name != "img-1" {
		t.Errorf("expected imageRef.name 'img-1', got %s", created.ImageRef.Name)
	}
	if len(created.EnabledTools) != 1 || created.EnabledTools[0] != "git" {
		t.Errorf("expected enabledTools [git], got %v", created.EnabledTools)
	}
	if created.LLM.Model != "glm-5.1:cloud" {
		t.Errorf("expected llm.model 'glm-5.1:cloud', got %s", created.LLM.Model)
	}

	// List agents
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	listRec := httptest.NewRecorder()
	s.mux.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	var page struct {
		Items      []SimpleAgentResponse `json:"items"`
		Total      int                   `json:"total"`
		Page       int                   `json:"page"`
		PageSize   int                   `json:"pageSize"`
		TotalPages int                   `json:"totalPages"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&page); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected 1 agent, got total=%d items=%d", page.Total, len(page.Items))
	}
	if page.Page != 1 || page.PageSize != 50 || page.TotalPages != 1 {
		t.Errorf("unexpected pagination fields: page=%d pageSize=%d totalPages=%d", page.Page, page.PageSize, page.TotalPages)
	}
	if page.Items[0].ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, page.Items[0].ID)
	}
	if page.Items[0].Name != "My Test Agent" {
		t.Errorf("expected name 'My Test Agent', got %s", page.Items[0].Name)
	}
}

func TestAgents_UpdateRename(t *testing.T) {
	img := testAgentImage("img-1", "git")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	createReq := SimpleAgentCreateRequest{
		Name:     "Original Name",
		ImageRef: AgentImageRefInfo{Name: "img-1"},
		LLM:      AgentLLMInfo{Model: "glm-5.1:cloud"},
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	originalID := created.ID

	newName := "Renamed Agent"
	updateReq := SimpleAgentUpdateRequest{Name: &newName}
	updateBody, _ := json.Marshal(updateReq)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+originalID, bytes.NewReader(updateBody))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	s.mux.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	var updated SimpleAgentResponse
	if err := json.NewDecoder(putRec.Body).Decode(&updated); err != nil {
		t.Fatalf("failed to decode update response: %v", err)
	}
	if updated.ID != originalID {
		t.Errorf("expected ID %s unchanged, got %s", originalID, updated.ID)
	}
	if updated.Name != "Renamed Agent" {
		t.Errorf("expected name 'Renamed Agent', got %s", updated.Name)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+originalID, nil)
	getRec := httptest.NewRecorder()
	s.mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on get, got %d: %s", getRec.Code, getRec.Body.String())
	}
	var fetched SimpleAgentResponse
	if err := json.NewDecoder(getRec.Body).Decode(&fetched); err != nil {
		t.Fatalf("failed to decode get response: %v", err)
	}
	if fetched.Name != "Renamed Agent" {
		t.Errorf("expected persisted name 'Renamed Agent', got %s", fetched.Name)
	}
}

func TestCreateAgent_RejectsMissingImageRef(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)

	body := SimpleAgentCreateRequest{Name: "agent1"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "imageRef.name is required") {
		t.Errorf("expected error about imageRef.name, got: %s", rec.Body.String())
	}
}

func TestCreateAgent_RejectsUnknownImageRef(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)

	body := SimpleAgentCreateRequest{
		Name:     "agent1",
		ImageRef: AgentImageRefInfo{Name: "nope"},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "does not match any AgentImage") {
		t.Errorf("expected error about unknown AgentImage, got: %s", rec.Body.String())
	}
}

func TestCreateAgent_RejectsEnabledToolNotInImage(t *testing.T) {
	img := testAgentImage("img-1", "git")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)

	body := SimpleAgentCreateRequest{
		Name:         "agent1",
		ImageRef:     AgentImageRefInfo{Name: "img-1"},
		EnabledTools: []string{"git", "not-a-real-tool"},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not-a-real-tool") {
		t.Errorf("expected error mentioning 'not-a-real-tool', got: %s", rec.Body.String())
	}
}

func TestCreateAgent_OKWithValidImageRef(t *testing.T) {
	img := testAgentImage("img-1", "git")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)

	body := SimpleAgentCreateRequest{
		Name:         "agent1",
		ImageRef:     AgentImageRefInfo{Name: "img-1"},
		LLM:          AgentLLMInfo{Model: "glm-5.1:cloud"},
		EnabledTools: []string{"git"},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ImageRef.Name != "img-1" {
		t.Errorf("expected imageRef.name 'img-1', got %q", resp.ImageRef.Name)
	}
	if len(resp.EnabledTools) != 1 || resp.EnabledTools[0] != "git" {
		t.Errorf("expected enabledTools [git], got %v", resp.EnabledTools)
	}
}

func TestUpdateAgent_Replicas(t *testing.T) {
	img := testAgentImage("img-1", "git")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	three := int32(3)
	createReq := SimpleAgentCreateRequest{
		Name:     "Replica Agent",
		ImageRef: AgentImageRefInfo{Name: "img-1"},
		LLM:      AgentLLMInfo{Model: "glm-5.1:cloud"},
		Replicas: &three,
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Replicas == nil || *created.Replicas != 3 {
		t.Fatalf("expected replicas=3 after create, got %v", created.Replicas)
	}

	five := int32(5)
	updateReq := SimpleAgentUpdateRequest{Replicas: &five}
	ubody, _ := json.Marshal(updateReq)
	ureq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+created.ID, bytes.NewReader(ubody))
	ureq.Header.Set("Content-Type", "application/json")
	urec := httptest.NewRecorder()
	s.mux.ServeHTTP(urec, ureq)
	if urec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", urec.Code, urec.Body.String())
	}
	var updated SimpleAgentResponse
	if err := json.NewDecoder(urec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.Replicas == nil || *updated.Replicas != 5 {
		t.Errorf("expected replicas=5 after update, got %v", updated.Replicas)
	}
}

func TestUpdateAgent_PartialLLMUpdate(t *testing.T) {
	img := testAgentImage("img-1", "git")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	// Create an agent with model=glm-5.1 and maxTurns=10
	ten := 10
	createReq := SimpleAgentCreateRequest{
		Name:     "LLM Agent",
		ImageRef: AgentImageRefInfo{Name: "img-1"},
		LLM:      AgentLLMInfo{Model: "glm-5.1", MaxTurns: ten},
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.LLM.Model != "glm-5.1" {
		t.Fatalf("expected model 'glm-5.1' after create, got %s", created.LLM.Model)
	}
	if created.LLM.MaxTurns != 10 {
		t.Fatalf("expected maxTurns=10 after create, got %d", created.LLM.MaxTurns)
	}

	// Update only maxTurns to 5 — model should be preserved
	five := 5
	updateReq := SimpleAgentUpdateRequest{
		LLM: &AgentLLMInfo{MaxTurns: five},
	}
	ubody, _ := json.Marshal(updateReq)
	ureq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+created.ID, bytes.NewReader(ubody))
	ureq.Header.Set("Content-Type", "application/json")
	urec := httptest.NewRecorder()
	s.mux.ServeHTTP(urec, ureq)
	if urec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", urec.Code, urec.Body.String())
	}
	var updated SimpleAgentResponse
	if err := json.NewDecoder(urec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.LLM.Model != "glm-5.1" {
		t.Errorf("expected model preserved as 'glm-5.1', got %s", updated.LLM.Model)
	}
	if updated.LLM.MaxTurns != 5 {
		t.Errorf("expected maxTurns updated to 5, got %d", updated.LLM.MaxTurns)
	}
}

func TestUpdateAgent_PartialLLMUpdate_Temperature(t *testing.T) {
	img := testAgentImage("img-1", "git")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	// Create an agent with model=glm-5.1
	createReq := SimpleAgentCreateRequest{
		Name:     "Temp Agent",
		ImageRef: AgentImageRefInfo{Name: "img-1"},
		LLM:      AgentLLMInfo{Model: "glm-5.1"},
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Update only temperature — model should be preserved
	temp := 0.7
	updateReq := SimpleAgentUpdateRequest{
		LLM: &AgentLLMInfo{Temperature: &temp},
	}
	ubody, _ := json.Marshal(updateReq)
	ureq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+created.ID, bytes.NewReader(ubody))
	ureq.Header.Set("Content-Type", "application/json")
	urec := httptest.NewRecorder()
	s.mux.ServeHTTP(urec, ureq)
	if urec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", urec.Code, urec.Body.String())
	}
	var updated SimpleAgentResponse
	if err := json.NewDecoder(urec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.LLM.Model != "glm-5.1" {
		t.Errorf("expected model preserved as 'glm-5.1', got %s", updated.LLM.Model)
	}
	if updated.LLM.Temperature == nil || *updated.LLM.Temperature != 0.7 {
		t.Errorf("expected temperature updated to 0.7, got %v", updated.LLM.Temperature)
	}
}

// --- Custom provider regression tests ---

func TestCreateAgent_CustomProvider_PersistsURLAndCreatesSecret(t *testing.T) {
	img := testAgentImage("img-1")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	req := SimpleAgentCreateRequest{
		Name:     "Custom Agent",
		ImageRef: AgentImageRefInfo{Name: "img-1"},
		LLM: AgentLLMInfo{
			Model:    "any-model",
			Provider: "custom",
		},
		CustomProvider: &AgentCustomProviderInfo{
			URL:    "https://api.example.com/v1",
			APIKey: "sk-abc-123",
		},
	}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Fetch the persisted Agent from the fake client and check the spec
	var ag agentv1alpha1.Agent
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: created.ID, Namespace: "test-ns"}, &ag); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if ag.Spec.CustomProvider == nil {
		t.Fatalf("expected spec.customProvider to be set, got nil; spec=%+v", ag.Spec)
	}
	if ag.Spec.CustomProvider.URL != "https://api.example.com/v1" {
		t.Errorf("expected URL persisted, got %q", ag.Spec.CustomProvider.URL)
	}
	if ag.Spec.CustomProvider.APIKeySecretRef == nil {
		t.Errorf("expected APIKeySecretRef to be set")
	}

	// Verify the secret was created with the key
	var sec corev1.Secret
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: created.ID + "-custom-provider-key", Namespace: "test-ns"}, &sec); err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if got := string(sec.Data["api-key"]); got != "sk-abc-123" {
		// fake client stores both Data and StringData; Data may be nil if only StringData set
		if got := sec.StringData["api-key"]; got != "sk-abc-123" {
			t.Errorf("expected api-key 'sk-abc-123', got Data=%q StringData=%q", string(sec.Data["api-key"]), sec.StringData["api-key"])
		}
	}
}

func TestUpdateAgent_CustomProvider_FromNoneToCustom(t *testing.T) {
	img := testAgentImage("img-1")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	// Create an agent without a custom provider
	createReq := SimpleAgentCreateRequest{
		Name:     "agent",
		ImageRef: AgentImageRefInfo{Name: "img-1"},
		LLM:      AgentLLMInfo{Model: "m", Provider: "ollama-cloud"},
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created SimpleAgentResponse
	_ = json.NewDecoder(rec.Body).Decode(&created)

	// Now PUT an update switching to custom with URL+APIKey, simulating the form save
	updateReq := SimpleAgentUpdateRequest{
		LLM: &AgentLLMInfo{Model: "m", Provider: "custom"},
		CustomProvider: &AgentCustomProviderInfo{
			URL:    "https://my-custom.example/v1",
			APIKey: "sk-new-key", // #nosec G101 -- test data, placeholder API key
		},
	}
	ubody, _ := json.Marshal(updateReq)
	ureq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+created.ID, bytes.NewReader(ubody))
	ureq.Header.Set("Content-Type", "application/json")
	urec := httptest.NewRecorder()
	s.mux.ServeHTTP(urec, ureq)
	if urec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", urec.Code, urec.Body.String())
	}

	// Fetch persisted agent
	var ag agentv1alpha1.Agent
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: created.ID, Namespace: "test-ns"}, &ag); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if ag.Spec.LLM.Provider != "custom" {
		t.Errorf("expected provider 'custom', got %q", ag.Spec.LLM.Provider)
	}
	if ag.Spec.CustomProvider == nil {
		t.Fatalf("EXPECTED spec.customProvider populated after update, got nil — THIS IS THE BUG")
	}
	if ag.Spec.CustomProvider.URL != "https://my-custom.example/v1" {
		t.Errorf("expected URL persisted, got %q", ag.Spec.CustomProvider.URL)
	}
	if ag.Spec.CustomProvider.APIKeySecretRef == nil {
		t.Errorf("expected APIKeySecretRef to be set")
	}
}

// Regression: PUT /agents/{id} used to nil-deref when the body had an empty
// URL and a non-empty APIKey on an agent that did not yet have a custom
// provider. The handler created the secret, then crashed dereferencing
// existing.Spec.CustomProvider (still nil because URL was empty). The fix
// rejects this payload with 400 before any secret is written.
func TestUpdateAgent_CustomProvider_EmptyURLNonEmptyKey_Rejects400(t *testing.T) {
	img := testAgentImage("img-1")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	createReq := SimpleAgentCreateRequest{
		Name:     "agent",
		ImageRef: AgentImageRefInfo{Name: "img-1"},
		LLM:      AgentLLMInfo{Model: "m", Provider: "ollama-cloud"},
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	var created SimpleAgentResponse
	_ = json.NewDecoder(rec.Body).Decode(&created)

	updateReq := SimpleAgentUpdateRequest{
		LLM:            &AgentLLMInfo{Provider: "custom"},
		CustomProvider: &AgentCustomProviderInfo{URL: "", APIKey: "sk-orphan"},
	}
	ubody, _ := json.Marshal(updateReq)
	ureq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+created.ID, bytes.NewReader(ubody))
	ureq.Header.Set("Content-Type", "application/json")
	urec := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("backend panicked on URL=empty APIKey=non-empty payload: %v", r)
		}
	}()
	s.mux.ServeHTTP(urec, ureq)

	if urec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", urec.Code, urec.Body.String())
	}
	if !strings.Contains(urec.Body.String(), "URL is required") {
		t.Errorf("expected error about URL, got: %s", urec.Body.String())
	}

	// No orphan secret should have been written before we returned.
	var sec corev1.Secret
	err := s.client.Get(context.Background(), types.NamespacedName{Name: created.ID + "-custom-provider-key", Namespace: "test-ns"}, &sec)
	if err == nil {
		t.Errorf("orphan secret should not exist after rejected request, but found %q", sec.Name)
	}
}

// When the agent already has a custom provider, updating only the API key
// (with no URL change) is a valid partial update and should rotate the secret.
func TestUpdateAgent_CustomProvider_RotateAPIKeyOnly(t *testing.T) {
	img := testAgentImage("img-1")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	// Create agent with custom provider already configured.
	createReq := SimpleAgentCreateRequest{
		Name:     "agent",
		ImageRef: AgentImageRefInfo{Name: "img-1"},
		LLM:      AgentLLMInfo{Model: "m", Provider: "custom"},
		CustomProvider: &AgentCustomProviderInfo{
			URL:    "https://api.example.com/v1",
			APIKey: "sk-original",
		},
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created SimpleAgentResponse
	_ = json.NewDecoder(rec.Body).Decode(&created)

	// Rotate only the API key — URL field left empty on the update.
	updateReq := SimpleAgentUpdateRequest{
		CustomProvider: &AgentCustomProviderInfo{URL: "", APIKey: "sk-rotated"},
	}
	ubody, _ := json.Marshal(updateReq)
	ureq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+created.ID, bytes.NewReader(ubody))
	ureq.Header.Set("Content-Type", "application/json")
	urec := httptest.NewRecorder()
	s.mux.ServeHTTP(urec, ureq)
	if urec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", urec.Code, urec.Body.String())
	}

	var ag agentv1alpha1.Agent
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: created.ID, Namespace: "test-ns"}, &ag); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if ag.Spec.CustomProvider == nil {
		t.Fatalf("expected existing custom provider to be preserved")
	}
	if ag.Spec.CustomProvider.URL != "https://api.example.com/v1" {
		t.Errorf("expected URL preserved, got %q", ag.Spec.CustomProvider.URL)
	}
	if ag.Spec.CustomProvider.APIKeySecretRef == nil {
		t.Errorf("expected APIKeySecretRef preserved")
	}

	// Secret should now hold the rotated key.
	var sec corev1.Secret
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: created.ID + "-custom-provider-key", Namespace: "test-ns"}, &sec); err != nil {
		t.Fatalf("get secret: %v", err)
	}
	got := sec.StringData["api-key"]
	if got == "" {
		got = string(sec.Data["api-key"])
	}
	if got != "sk-rotated" {
		t.Errorf("expected rotated api-key, got %q", got)
	}
}

// --- Image display name enrichment tests ---

// helper: create an agent via the API and return the decoded response.
func createTestAgent(t *testing.T, s *Server, req SimpleAgentCreateRequest) SimpleAgentResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create agent: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return resp
}

// helper: get an agent via the API and return the decoded response.
func getTestAgent(t *testing.T, s *Server, id string) SimpleAgentResponse {
	t.Helper()
	httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+id, nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("get agent: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	return resp
}

// helper: list agents via the API and return the decoded items.
func listTestAgents(t *testing.T, s *Server) []SimpleAgentResponse {
	t.Helper()
	httpReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("list agents: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []SimpleAgentResponse `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return page.Items
}

func TestAgents_DisplayName_OnCreateGetList(t *testing.T) {
	// AgentImage with a display name distinct from its resource name.
	img := testAgentImageWithDisplayName("img-abc", "My Friendly Image", "git")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	created := createTestAgent(t, s, SimpleAgentCreateRequest{
		Name:     "Display Name Agent",
		ImageRef: AgentImageRefInfo{Name: "img-abc"},
		LLM:      AgentLLMInfo{Model: "glm-5.1:cloud"},
	})

	// Create response should carry the display name.
	if created.ImageRef.Name != "img-abc" {
		t.Errorf("expected imageRef.name 'img-abc', got %q", created.ImageRef.Name)
	}
	if created.ImageRef.DisplayName != "My Friendly Image" {
		t.Errorf("expected imageRef.displayName 'My Friendly Image', got %q", created.ImageRef.DisplayName)
	}

	// Get response should carry the display name.
	fetched := getTestAgent(t, s, created.ID)
	if fetched.ImageRef.DisplayName != "My Friendly Image" {
		t.Errorf("get: expected displayName 'My Friendly Image', got %q", fetched.ImageRef.DisplayName)
	}
	if fetched.ImageRef.Name != "img-abc" {
		t.Errorf("get: expected name 'img-abc', got %q", fetched.ImageRef.Name)
	}

	// List response should carry the display name.
	items := listTestAgents(t, s)
	if len(items) != 1 {
		t.Fatalf("expected 1 agent in list, got %d", len(items))
	}
	if items[0].ImageRef.DisplayName != "My Friendly Image" {
		t.Errorf("list: expected displayName 'My Friendly Image', got %q", items[0].ImageRef.DisplayName)
	}
	if items[0].ImageRef.Name != "img-abc" {
		t.Errorf("list: expected name 'img-abc', got %q", items[0].ImageRef.Name)
	}
}

func TestAgents_DisplayName_FallbackWhenImageDeleted(t *testing.T) {
	img := testAgentImageWithDisplayName("img-del", "Will Be Deleted", "git")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	created := createTestAgent(t, s, SimpleAgentCreateRequest{
		Name:     "Orphan Agent",
		ImageRef: AgentImageRefInfo{Name: "img-del"},
		LLM:      AgentLLMInfo{Model: "glm-5.1:cloud"},
	})
	if created.ImageRef.DisplayName != "Will Be Deleted" {
		t.Fatalf("expected displayName before delete, got %q", created.ImageRef.DisplayName)
	}

	// Delete the AgentImage from the fake client.
	if err := s.client.Delete(context.Background(), img); err != nil {
		t.Fatalf("delete image: %v", err)
	}

	// Get should still succeed with empty displayName (fallback).
	fetched := getTestAgent(t, s, created.ID)
	if fetched.ImageRef.Name != "img-del" {
		t.Errorf("expected imageRef.name 'img-del', got %q", fetched.ImageRef.Name)
	}
	if fetched.ImageRef.DisplayName != "" {
		t.Errorf("expected empty displayName after image deletion, got %q", fetched.ImageRef.DisplayName)
	}

	// List should also succeed with empty displayName.
	items := listTestAgents(t, s)
	if len(items) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(items))
	}
	if items[0].ImageRef.DisplayName != "" {
		t.Errorf("list: expected empty displayName after image deletion, got %q", items[0].ImageRef.DisplayName)
	}
}

func TestAgents_DisplayName_UpdateWithImageRef_ReusesValidatedName(t *testing.T) {
	img1 := testAgentImageWithDisplayName("img-old", "Old Image", "git")
	img2 := testAgentImageWithDisplayName("img-new", "New Image", "bash")
	s := testServer(t, img1, img2)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	created := createTestAgent(t, s, SimpleAgentCreateRequest{
		Name:         "Update Agent",
		ImageRef:     AgentImageRefInfo{Name: "img-old"},
		LLM:          AgentLLMInfo{Model: "glm-5.1:cloud"},
		EnabledTools: []string{"git"},
	})
	if created.ImageRef.DisplayName != "Old Image" {
		t.Fatalf("expected initial displayName 'Old Image', got %q", created.ImageRef.DisplayName)
	}

	// Update with a new imageRef — this exercises the "validated name reused" branch.
	newImageRef := AgentImageRefInfo{Name: "img-new"}
	newTools := []string{"bash"}
	updateReq := SimpleAgentUpdateRequest{
		ImageRef:     &newImageRef,
		EnabledTools: &newTools,
	}
	ubody, _ := json.Marshal(updateReq)
	ureq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+created.ID, bytes.NewReader(ubody))
	ureq.Header.Set("Content-Type", "application/json")
	urec := httptest.NewRecorder()
	s.mux.ServeHTTP(urec, ureq)
	if urec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", urec.Code, urec.Body.String())
	}
	var updated SimpleAgentResponse
	if err := json.NewDecoder(urec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.ImageRef.Name != "img-new" {
		t.Errorf("expected imageRef.name 'img-new', got %q", updated.ImageRef.Name)
	}
	if updated.ImageRef.DisplayName != "New Image" {
		t.Errorf("expected displayName 'New Image', got %q", updated.ImageRef.DisplayName)
	}
}

func TestAgents_DisplayName_UpdateWithoutImageRef_ResolvesSeparately(t *testing.T) {
	img := testAgentImageWithDisplayName("img-resolve", "Resolved Image", "git")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	created := createTestAgent(t, s, SimpleAgentCreateRequest{
		Name:     "Resolve Agent",
		ImageRef: AgentImageRefInfo{Name: "img-resolve"},
		LLM:      AgentLLMInfo{Model: "glm-5.1:cloud"},
	})

	// Update only the name — no imageRef in the request.
	// This exercises the "resolved separately" branch.
	newName := "Renamed Resolve Agent"
	updateReq := SimpleAgentUpdateRequest{Name: &newName}
	ubody, _ := json.Marshal(updateReq)
	ureq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+created.ID, bytes.NewReader(ubody))
	ureq.Header.Set("Content-Type", "application/json")
	urec := httptest.NewRecorder()
	s.mux.ServeHTTP(urec, ureq)
	if urec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", urec.Code, urec.Body.String())
	}
	var updated SimpleAgentResponse
	if err := json.NewDecoder(urec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Name != "Renamed Resolve Agent" {
		t.Errorf("expected name 'Renamed Resolve Agent', got %q", updated.Name)
	}
	if updated.ImageRef.Name != "img-resolve" {
		t.Errorf("expected imageRef.name 'img-resolve', got %q", updated.ImageRef.Name)
	}
	if updated.ImageRef.DisplayName != "Resolved Image" {
		t.Errorf("expected displayName 'Resolved Image', got %q", updated.ImageRef.DisplayName)
	}
}

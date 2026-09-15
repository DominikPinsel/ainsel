package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

// envAgentServer wires an agent-handling test server around a single image.
func envAgentServer(t *testing.T) *Server {
	t.Helper()
	s := testServer(t, testAgentImage("img-1", "git", "bash"))
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)
	return s
}

// createAgentWithEnv POSTs an agent carrying the given env overrides and
// returns the decoded response.
func createAgentWithEnv(t *testing.T, s *Server, env []AgentImageEnvVarInfo) SimpleAgentResponse {
	t.Helper()
	body, err := json.Marshal(SimpleAgentCreateRequest{
		Name:         "env agent",
		ImageRef:     AgentImageRefInfo{Name: "img-1"},
		LLM:          AgentLLMInfo{Model: "glm-5.1:cloud"},
		EnabledTools: []string{"git"},
		Env:          env,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	return created
}

// storedAgent reads the Agent CR back from the fake client so tests can assert
// on values the API deliberately masks.
func storedAgent(t *testing.T, s *Server, id string) *agentv1alpha1.Agent {
	t.Helper()
	var a agentv1alpha1.Agent
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: id, Namespace: s.ns}, &a); err != nil {
		t.Fatalf("get stored agent %s: %v", id, err)
	}
	return &a
}

// putAgentEnv sends an update carrying only the env field.
func putAgentEnv(t *testing.T, s *Server, id string, env []AgentImageEnvVarInfo) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(SimpleAgentUpdateRequest{Env: &env})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+id, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.mux.ServeHTTP(rec, req)
	return rec
}

func envValue(in []agentv1alpha1.AgentEnvVar, name string) (string, bool) {
	for _, e := range in {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

func TestAgentEnv_CreateStoresValuesAndMasksSecrets(t *testing.T) {
	s := envAgentServer(t)
	created := createAgentWithEnv(t, s, []AgentImageEnvVarInfo{
		{Name: "LOG_LEVEL", Value: "info"},
		{Name: "API_TOKEN", Value: "s3cret", Secret: true},
	})

	if len(created.Env) != 2 {
		t.Fatalf("expected 2 env entries in the response, got %d", len(created.Env))
	}
	byName := map[string]AgentImageEnvVarInfo{}
	for _, e := range created.Env {
		byName[e.Name] = e
	}
	if byName["LOG_LEVEL"].Value != "info" {
		t.Errorf("plain value should be returned, got %q", byName["LOG_LEVEL"].Value)
	}
	if !byName["API_TOKEN"].Secret {
		t.Error("the secret flag should be returned")
	}
	if byName["API_TOKEN"].Value != "" {
		t.Errorf("secret value must be masked, got %q", byName["API_TOKEN"].Value)
	}

	// The stored CR keeps the real value even though the API masks it.
	stored := storedAgent(t, s, created.ID)
	if v, ok := envValue(stored.Spec.Env, "API_TOKEN"); !ok || v != "s3cret" {
		t.Errorf("stored secret value = %q (found=%v), want %q", v, ok, "s3cret")
	}
	if v, _ := envValue(stored.Spec.Env, "LOG_LEVEL"); v != "info" {
		t.Errorf("stored plain value = %q, want %q", v, "info")
	}
}

func TestAgentEnv_UpdateReplacesOverrides(t *testing.T) {
	s := envAgentServer(t)
	created := createAgentWithEnv(t, s, []AgentImageEnvVarInfo{
		{Name: "LOG_LEVEL", Value: "info"},
		{Name: "DROPPED", Value: "gone"},
	})

	rec := putAgentEnv(t, s, created.ID, []AgentImageEnvVarInfo{
		{Name: "LOG_LEVEL", Value: "debug"},
		{Name: "ADDED", Value: "new"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	stored := storedAgent(t, s, created.ID)
	if len(stored.Spec.Env) != 2 {
		t.Fatalf("expected the override list to be replaced, got %d entries", len(stored.Spec.Env))
	}
	if v, _ := envValue(stored.Spec.Env, "LOG_LEVEL"); v != "debug" {
		t.Errorf("LOG_LEVEL = %q, want %q", v, "debug")
	}
	if _, ok := envValue(stored.Spec.Env, "DROPPED"); ok {
		t.Error("DROPPED should no longer be present after a replacement")
	}
	if v, ok := envValue(stored.Spec.Env, "ADDED"); !ok || v != "new" {
		t.Errorf("ADDED = %q (found=%v), want %q", v, ok, "new")
	}
}

func TestAgentEnv_UpdateKeepsExistingSecretValue(t *testing.T) {
	s := envAgentServer(t)
	created := createAgentWithEnv(t, s, []AgentImageEnvVarInfo{
		{Name: "API_TOKEN", Value: "s3cret", Secret: true},
	})

	// The UI submits secret entries with an empty value to mean "unchanged",
	// because it never received the real one.
	rec := putAgentEnv(t, s, created.ID, []AgentImageEnvVarInfo{
		{Name: "API_TOKEN", Value: "", Secret: true},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	stored := storedAgent(t, s, created.ID)
	if v, ok := envValue(stored.Spec.Env, "API_TOKEN"); !ok || v != "s3cret" {
		t.Errorf("secret value = %q (found=%v), want the stored %q to be kept", v, ok, "s3cret")
	}
}

func TestAgentEnv_UpdateWithEmptyListClearsOverrides(t *testing.T) {
	s := envAgentServer(t)
	created := createAgentWithEnv(t, s, []AgentImageEnvVarInfo{
		{Name: "LOG_LEVEL", Value: "info"},
	})

	rec := putAgentEnv(t, s, created.ID, []AgentImageEnvVarInfo{})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var updated SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(updated.Env) != 0 {
		t.Errorf("expected no env in the response, got %v", updated.Env)
	}
	if stored := storedAgent(t, s, created.ID); len(stored.Spec.Env) != 0 {
		t.Errorf("expected the CR env to be cleared, got %v", stored.Spec.Env)
	}
}

func TestAgentEnv_UpdateWithoutEnvLeavesOverridesUntouched(t *testing.T) {
	s := envAgentServer(t)
	created := createAgentWithEnv(t, s, []AgentImageEnvVarInfo{
		{Name: "LOG_LEVEL", Value: "info"},
	})

	name := "renamed"
	body, _ := json.Marshal(SimpleAgentUpdateRequest{Name: &name})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	stored := storedAgent(t, s, created.ID)
	if v, ok := envValue(stored.Spec.Env, "LOG_LEVEL"); !ok || v != "info" {
		t.Errorf("LOG_LEVEL = %q (found=%v); an update without env must leave overrides alone", v, ok)
	}
}

func TestAgentEnv_RejectsInvalidEntries(t *testing.T) {
	cases := []struct {
		name string
		env  []AgentImageEnvVarInfo
	}{
		{"empty name", []AgentImageEnvVarInfo{{Name: "  ", Value: "x"}}},
		{"name with a space", []AgentImageEnvVarInfo{{Name: "LOG LEVEL", Value: "x"}}},
		{"name starting with a digit", []AgentImageEnvVarInfo{{Name: "1BAD", Value: "x"}}},
		{"duplicate names", []AgentImageEnvVarInfo{
			{Name: "LOG_LEVEL", Value: "a"},
			{Name: "LOG_LEVEL", Value: "b"},
		}},
	}

	for _, tc := range cases {
		t.Run("create/"+tc.name, func(t *testing.T) {
			s := envAgentServer(t)
			body, _ := json.Marshal(SimpleAgentCreateRequest{
				Name:         "env agent",
				ImageRef:     AgentImageRefInfo{Name: "img-1"},
				LLM:          AgentLLMInfo{Model: "glm-5.1:cloud"},
				EnabledTools: []string{"git"},
				Env:          tc.env,
			})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			s.mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})

		t.Run("update/"+tc.name, func(t *testing.T) {
			s := envAgentServer(t)
			created := createAgentWithEnv(t, s, nil)
			if rec := putAgentEnv(t, s, created.ID, tc.env); rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// newAgentPersonaServer wires a Server with the agent routes and a persona
// stub, creating one agent through the API and returning it.
// personaStub implements PersonaService for the agent-scoped persona tests.
// The library methods are inert: these tests only exercise Get, OwnedBy,
// EnsureOwned and DeleteOwned. (The api_test package has its own stub for the
// library endpoints; test helpers are not visible across packages.)
type personaStub struct {
	getFn         func(ctx context.Context, id string) (*personas.Persona, error)
	ownedByFn     func(ctx context.Context, agentName string) (*personas.Persona, error)
	ensureOwnedFn func(ctx context.Context, agentName, defaultName string, req personas.UpdateRequest) (*personas.Persona, error)
	deleteOwnedFn func(ctx context.Context, personaID, agentName string) error
}

func (p *personaStub) Create(ctx context.Context, req personas.CreateRequest) (*personas.Persona, error) {
	return nil, nil
}
func (p *personaStub) Get(ctx context.Context, id string) (*personas.Persona, error) {
	if p.getFn == nil {
		return nil, personas.ErrNotFound
	}
	return p.getFn(ctx, id)
}
func (p *personaStub) List(ctx context.Context) ([]personas.PersonaSummary, error) { return nil, nil }
func (p *personaStub) Update(ctx context.Context, id string, req personas.UpdateRequest) (*personas.Persona, error) {
	return nil, nil
}
func (p *personaStub) Delete(ctx context.Context, id string) error { return nil }
func (p *personaStub) ListVersions(ctx context.Context, id string) ([]personas.VersionSummary, error) {
	return nil, nil
}
func (p *personaStub) GetVersion(ctx context.Context, id string, n int) (*personas.Version, error) {
	return nil, nil
}
func (p *personaStub) Rollback(ctx context.Context, id string, n int) (*personas.Persona, error) {
	return nil, nil
}
func (p *personaStub) OwnedBy(ctx context.Context, agentName string) (*personas.Persona, error) {
	if p.ownedByFn == nil {
		return nil, nil
	}
	return p.ownedByFn(ctx, agentName)
}
func (p *personaStub) EnsureOwned(ctx context.Context, agentName, defaultName string, req personas.UpdateRequest) (*personas.Persona, error) {
	if p.ensureOwnedFn == nil {
		return nil, personas.ErrNotFound
	}
	return p.ensureOwnedFn(ctx, agentName, defaultName, req)
}
func (p *personaStub) DeleteOwned(ctx context.Context, personaID, agentName string) error {
	if p.deleteOwnedFn == nil {
		return nil
	}
	return p.deleteOwnedFn(ctx, personaID, agentName)
}

func newAgentPersonaServer(t *testing.T, svc PersonaService, displayName string) (*Server, string) {
	t.Helper()
	img := testAgentImage("img-1", "git")
	s := testServer(t, img)
	s.personas = svc
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	body, _ := json.Marshal(SimpleAgentCreateRequest{
		Name:     displayName,
		ImageRef: AgentImageRefInfo{Name: "img-1"},
		LLM:      AgentLLMInfo{Model: "glm-5.1:cloud"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: create agent: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created SimpleAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("setup: decode: %v", err)
	}
	return s, created.ID
}

// pointAgentAtPersona sets spec.persona on the agent CR directly.
func pointAgentAtPersona(t *testing.T, s *Server, agentName, personaID string) {
	t.Helper()
	agent := &agentv1alpha1.Agent{}
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: agentName, Namespace: s.ns}, agent); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	agent.Spec.Persona = agentv1alpha1.AgentPersona{ID: personaID}
	if err := s.client.Update(context.Background(), agent); err != nil {
		t.Fatalf("update agent: %v", err)
	}
}

func doAgentPersona(t *testing.T, s *Server, method, agentName string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/api/v1/agents/"+agentName+"/persona", rdr)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func TestAgentPersona_GET_Unset(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")

	rec := doAgentPersona(t, s, http.MethodGet, id, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got AgentPersonaResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Owned || got.Persona != nil || got.Ref != "" {
		t.Errorf("expected empty persona view, got %+v", got)
	}
}

func TestAgentPersona_GET_TemplateIsNotOwned(t *testing.T) {
	template := &personas.Persona{ID: "p-tmpl", Name: "Doc Writer", Text: "shared", CurrentVersion: 3}
	s, id := newAgentPersonaServer(t, &personaStub{
		getFn: func(ctx context.Context, pid string) (*personas.Persona, error) {
			if pid != "p-tmpl" {
				return nil, personas.ErrNotFound
			}
			return template, nil
		},
	}, "Doc Writer")
	pointAgentAtPersona(t, s, id, "p-tmpl")

	rec := doAgentPersona(t, s, http.MethodGet, id, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got AgentPersonaResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Owned {
		t.Error("a shared template must not be reported as owned")
	}
	if got.Ref != "p-tmpl" || got.Persona == nil || got.Persona.Text != "shared" {
		t.Errorf("unexpected view: %+v", got)
	}
}

func TestAgentPersona_GET_Owned(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")
	owned := &personas.Persona{ID: "p-own", Name: "Doc Writer (own)", Text: "mine", OwnerAgent: id}
	s.personas = &personaStub{
		getFn: func(ctx context.Context, pid string) (*personas.Persona, error) {
			if pid != "p-own" {
				return nil, personas.ErrNotFound
			}
			return owned, nil
		},
	}
	pointAgentAtPersona(t, s, id, "p-own")

	rec := doAgentPersona(t, s, http.MethodGet, id, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got AgentPersonaResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Owned {
		t.Error("expected owned=true for the agent's own persona")
	}
	if got.Persona == nil || got.Persona.Text != "mine" {
		t.Errorf("unexpected persona: %+v", got.Persona)
	}
}

func TestAgentPersona_GET_DanglingRefReportsRef(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{
		getFn: func(ctx context.Context, pid string) (*personas.Persona, error) {
			return nil, personas.ErrNotFound
		},
	}, "Doc Writer")
	pointAgentAtPersona(t, s, id, "p-gone")

	rec := doAgentPersona(t, s, http.MethodGet, id, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a dangling reference, got %d: %s", rec.Code, rec.Body.String())
	}
	var got AgentPersonaResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Persona != nil || got.Ref != "p-gone" {
		t.Errorf("expected ref-only view, got %+v", got)
	}
}

func TestAgentPersona_GET_UnknownAgent(t *testing.T) {
	s, _ := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")

	rec := doAgentPersona(t, s, http.MethodGet, "a-doesnotexist", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestAgentPersona_PUT_CopyOnWriteRepointsAgent(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")
	pointAgentAtPersona(t, s, id, "p-tmpl")

	var gotAgent, gotDefault string
	forked := &personas.Persona{ID: "p-own", Name: "Doc Writer", Text: "edited", OwnerAgent: id, CurrentVersion: 1}
	s.personas = &personaStub{
		ensureOwnedFn: func(ctx context.Context, agentName, defaultName string, req personas.UpdateRequest) (*personas.Persona, error) {
			gotAgent, gotDefault = agentName, defaultName
			return forked, nil
		},
	}

	rec := doAgentPersona(t, s, http.MethodPut, id, AgentPersonaRequest{Text: "edited"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got AgentPersonaResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Owned || got.Ref != "p-own" {
		t.Errorf("expected owned persona p-own, got %+v", got)
	}
	if gotAgent != id {
		t.Errorf("EnsureOwned agent = %q, want %q", gotAgent, id)
	}
	if gotDefault != "Doc Writer (own)" {
		t.Errorf("default name = %q, want %q", gotDefault, "Doc Writer (own)")
	}

	// The CR must now reference the fork, and updatedAt must be stamped.
	agent := &agentv1alpha1.Agent{}
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: id, Namespace: s.ns}, agent); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Spec.Persona.ID != "p-own" {
		t.Errorf("spec.persona.id = %q, want p-own", agent.Spec.Persona.ID)
	}
	stamp := agent.Annotations[AgentUpdatedAtAnnotation]
	if stamp == "" {
		t.Fatal("expected updatedAt stamp after re-pointing")
	}
	if _, err := time.Parse(time.RFC3339, stamp); err != nil {
		t.Errorf("stamp %q is not RFC3339: %v", stamp, err)
	}
}

func TestAgentPersona_PUT_ExistingOwnedDoesNotTouchCR(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")
	pointAgentAtPersona(t, s, id, "p-own")
	// Remove any stamp so we can prove the handler didn't write.
	agent := &agentv1alpha1.Agent{}
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: id, Namespace: s.ns}, agent); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	delete(agent.Annotations, AgentUpdatedAtAnnotation)
	if err := s.client.Update(context.Background(), agent); err != nil {
		t.Fatalf("update agent: %v", err)
	}

	s.personas = &personaStub{
		ensureOwnedFn: func(ctx context.Context, agentName, defaultName string, req personas.UpdateRequest) (*personas.Persona, error) {
			return &personas.Persona{ID: "p-own", Name: "Doc Writer (own)", Text: *req.Text, OwnerAgent: agentName, CurrentVersion: 2}, nil
		},
	}

	rec := doAgentPersona(t, s, http.MethodPut, id, AgentPersonaRequest{Name: "Doc Writer (own)", Text: "v2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := s.client.Get(context.Background(), types.NamespacedName{Name: id, Namespace: s.ns}, agent); err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Spec.Persona.ID != "p-own" {
		t.Errorf("spec.persona.id = %q, want p-own", agent.Spec.Persona.ID)
	}
	if got := agent.Annotations[AgentUpdatedAtAnnotation]; got != "" {
		t.Errorf("CR should not be rewritten when the reference is unchanged, stamp = %q", got)
	}
}

func TestAgentPersona_PUT_PassesNameAndDescription(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")

	var gotReq personas.UpdateRequest
	s.personas = &personaStub{
		ensureOwnedFn: func(ctx context.Context, agentName, defaultName string, req personas.UpdateRequest) (*personas.Persona, error) {
			gotReq = req
			return &personas.Persona{ID: "p-own", Name: "Renamed", Text: *req.Text, OwnerAgent: agentName}, nil
		},
	}

	rec := doAgentPersona(t, s, http.MethodPut, id, AgentPersonaRequest{
		Name: "Renamed", Description: "desc", Text: "body",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotReq.Name == nil || *gotReq.Name != "Renamed" {
		t.Errorf("name = %v, want Renamed", gotReq.Name)
	}
	if gotReq.Description == nil || *gotReq.Description != "desc" {
		t.Errorf("description = %v, want desc", gotReq.Description)
	}
	if gotReq.Text == nil || *gotReq.Text != "body" {
		t.Errorf("text = %v, want body", gotReq.Text)
	}
}

func TestAgentPersona_PUT_ValidationErrorIs400(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")
	s.personas = &personaStub{
		ensureOwnedFn: func(ctx context.Context, agentName, defaultName string, req personas.UpdateRequest) (*personas.Persona, error) {
			return nil, &personas.ValidationError{Field: "text", Message: "is required"}
		},
	}

	rec := doAgentPersona(t, s, http.MethodPut, id, AgentPersonaRequest{Text: ""})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentPersona_PUT_UnknownAgentIs404(t *testing.T) {
	s, _ := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")

	rec := doAgentPersona(t, s, http.MethodPut, "a-nope", AgentPersonaRequest{Text: "x"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestAgentPersona_NoServiceIs503(t *testing.T) {
	img := testAgentImage("img-1", "git")
	s := testServer(t, img)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	rec := doAgentPersona(t, s, http.MethodGet, "a-whatever", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 without a persona service, got %d", rec.Code)
	}
}

func TestAgentPersona_PUT_MethodNotAllowed(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")

	rec := doAgentPersona(t, s, http.MethodDelete, id, nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestUpdateAgent_RepointAwayReclaimsOwnedPersona(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")
	pointAgentAtPersona(t, s, id, "p-own")

	var deletedID, deletedAgent string
	s.personas = &personaStub{
		ownedByFn: func(ctx context.Context, agentName string) (*personas.Persona, error) {
			if agentName != id {
				t.Errorf("OwnedBy agent = %q, want %q", agentName, id)
			}
			return &personas.Persona{ID: "p-own", OwnerAgent: agentName}, nil
		},
		deleteOwnedFn: func(ctx context.Context, personaID, agentName string) error {
			deletedID, deletedAgent = personaID, agentName
			return nil
		},
	}

	body, _ := json.Marshal(SimpleAgentUpdateRequest{Persona: &AgentPersonaInfo{ID: "p-tmpl"}})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+id, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if deletedID != "p-own" || deletedAgent != id {
		t.Errorf("DeleteOwned(%q, %q), want (p-own, %q)", deletedID, deletedAgent, id)
	}
}

func TestUpdateAgent_SamePersonaKeepsOwned(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")
	pointAgentAtPersona(t, s, id, "p-own")

	deleted := false
	s.personas = &personaStub{
		ownedByFn: func(ctx context.Context, agentName string) (*personas.Persona, error) {
			return &personas.Persona{ID: "p-own", OwnerAgent: agentName}, nil
		},
		deleteOwnedFn: func(ctx context.Context, personaID, agentName string) error {
			deleted = true
			return nil
		},
	}

	body, _ := json.Marshal(SimpleAgentUpdateRequest{Persona: &AgentPersonaInfo{ID: "p-own"}})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+id, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if deleted {
		t.Error("re-selecting the same persona must not delete it")
	}
}

func TestDeleteAgent_ReclaimsOwnedPersona(t *testing.T) {
	s, id := newAgentPersonaServer(t, &personaStub{}, "Doc Writer")
	pointAgentAtPersona(t, s, id, "p-own")

	var deletedID, deletedAgent string
	s.personas = &personaStub{
		ownedByFn: func(ctx context.Context, agentName string) (*personas.Persona, error) {
			return &personas.Persona{ID: "p-own", OwnerAgent: agentName}, nil
		},
		deleteOwnedFn: func(ctx context.Context, personaID, agentName string) error {
			deletedID, deletedAgent = personaID, agentName
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/"+id, nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("expected 200/204, got %d: %s", rec.Code, rec.Body.String())
	}
	if deletedID != "p-own" || deletedAgent != id {
		t.Errorf("DeleteOwned(%q, %q), want (p-own, %q)", deletedID, deletedAgent, id)
	}
}

func TestDefaultOwnedPersonaName(t *testing.T) {
	t.Run("uses display name", func(t *testing.T) {
		a := &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{Name: "a-1"},
			Spec:       agentv1alpha1.AgentSpec{DisplayName: "Doc Writer"},
		}
		if got := defaultOwnedPersonaName(a); got != "Doc Writer (own)" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("falls back to CR name", func(t *testing.T) {
		a := &agentv1alpha1.Agent{ObjectMeta: metav1.ObjectMeta{Name: "a-1"}}
		if got := defaultOwnedPersonaName(a); got != "a-1 (own)" {
			t.Errorf("got %q", got)
		}
	})
}

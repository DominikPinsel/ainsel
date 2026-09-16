package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

// Agent-scoped persona endpoints.
//
// Personas used to be purely shared library objects: an agent referenced one
// by id, and editing "the agent's persona" edited that shared object for every
// agent using it. These endpoints make the agent the owner of its persona
// content — the hub forks a private, agent-owned persona on the first inline
// edit (copy-on-write) and updates that copy afterwards. Shared templates stay
// in the library and are never modified through this path.
//
// Access is gated by the agent (requireRead/requireWrite on "agent"), so an
// owned persona needs no separate authz resource record.

// AgentPersonaResponse is the agent-centric view of its persona: the content,
// whether this agent owns it (inline edits are private) or references a shared
// template (the next edit forks a private copy), and the id the Agent CR
// currently references — set even when the persona no longer exists, so the UI
// can distinguish "no persona" from "dangling reference".
type AgentPersonaResponse struct {
	Owned   bool              `json:"owned"`
	Ref     string            `json:"ref,omitempty"`
	Persona *personas.Persona `json:"persona,omitempty"`
}

// AgentPersonaRequest is the body of PUT /api/v1/agents/{id}/persona.
// Text is required; Name falls back to a name derived from the agent when
// empty, and an empty Description clears it.
type AgentPersonaRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Text        string `json:"text"`
}

func (s *Server) handleAgentPersona(w http.ResponseWriter, r *http.Request, agentName string) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		if !s.requireRead(w, r, "agent", agentName) {
			return
		}
		s.getAgentPersona(ctx, w, agentName)
	case http.MethodPut:
		if !s.requireWrite(w, r, "agent", agentName) {
			return
		}
		s.putAgentPersona(ctx, w, r, agentName)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) getAgentPersona(ctx context.Context, w http.ResponseWriter, agentName string) {
	if s.personas == nil {
		writeError(w, http.StatusServiceUnavailable, "persona service not configured")
		return
	}
	agent := &agentv1alpha1.Agent{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: agentName, Namespace: s.ns}, agent); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	ref := agent.Spec.Persona.ID
	if ref == "" {
		writeJSON(w, http.StatusOK, AgentPersonaResponse{})
		return
	}
	p, err := s.personas.Get(ctx, ref)
	if err != nil {
		// A dangling reference is a normal state to report, not a server
		// error: the persona was deleted out from under the agent.
		writeJSON(w, http.StatusOK, AgentPersonaResponse{Ref: ref})
		return
	}
	writeJSON(w, http.StatusOK, AgentPersonaResponse{
		Owned:   p.OwnerAgent == agentName,
		Ref:     ref,
		Persona: p,
	})
}

func (s *Server) putAgentPersona(ctx context.Context, w http.ResponseWriter, r *http.Request, agentName string) {
	if s.personas == nil {
		writeError(w, http.StatusServiceUnavailable, "persona service not configured")
		return
	}
	var req AgentPersonaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	agent := &agentv1alpha1.Agent{}
	if err := s.client.Get(ctx, types.NamespacedName{Name: agentName, Namespace: s.ns}, agent); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	name := strings.TrimSpace(req.Name)
	description := req.Description
	text := req.Text
	p, err := s.personas.EnsureOwned(ctx, agentName, defaultOwnedPersonaName(agent), personas.UpdateRequest{
		Name:        &name,
		Description: &description,
		Text:        &text,
	})
	if err != nil {
		writePersonaServiceError(w, err)
		return
	}

	// Copy-on-write forked a new persona: re-point the agent at it. If this
	// write fails the persona is orphaned but reused by the next call, so
	// EnsureOwned stays idempotent.
	if agent.Spec.Persona.ID != p.ID {
		agent.Spec.Persona = agentv1alpha1.AgentPersona{ID: p.ID}
		if agent.Annotations == nil {
			agent.Annotations = map[string]string{}
		}
		agent.Annotations[AgentUpdatedAtAnnotation] = stampAgentUpdatedAt()
		if err := s.client.Update(ctx, agent); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to link persona: "+err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, AgentPersonaResponse{Owned: true, Ref: p.ID, Persona: p})
}

// defaultOwnedPersonaName derives the name for a freshly forked persona when
// the request carries none. The agent CR name is unique, so the result never
// collides with a template or another agent's persona — and template names
// are unique only among templates anyway (partial index), so a fork may also
// keep the name of the template it was forked from.
func defaultOwnedPersonaName(agent *agentv1alpha1.Agent) string {
	display := strings.TrimSpace(agent.Spec.DisplayName)
	if display == "" {
		display = agent.Name
	}
	return fmt.Sprintf("%s (own)", display)
}

// cleanupOwnedPersona deletes the persona owned by agentName, if any. Used
// when the agent is re-pointed at another persona or deleted: owned personas
// are invisible to the library, so nothing else would ever reclaim them.
// Best-effort — failures are logged, never surfaced to the caller.
func (s *Server) cleanupOwnedPersona(ctx context.Context, agentName, keepPersonaID string) {
	if s.personas == nil {
		return
	}
	owned, err := s.personas.OwnedBy(ctx, agentName)
	if err != nil {
		slog.Error("look up agent-owned persona", "error", err, "agent", agentName)
		return
	}
	if owned == nil || owned.ID == keepPersonaID {
		return
	}
	if err := s.personas.DeleteOwned(ctx, owned.ID, agentName); err != nil {
		slog.Error("delete orphaned agent-owned persona",
			"error", err, "agent", agentName, "persona", owned.ID)
	}
}

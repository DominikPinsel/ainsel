package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SimpleTriggerResponse is the simplified API representation of a Trigger.
type SimpleTriggerResponse struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	AgentRef     string               `json:"agentRef"`
	ConnectorRef string               `json:"connectorRef"`
	Filters      []TriggerFilterInfo  `json:"filters,omitempty"`
	Status       *SimpleTriggerStatus `json:"status,omitempty"`
}

// TriggerFilterInfo is a simplified representation of a filter condition.
type TriggerFilterInfo struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// SimpleTriggerStatus summarises the validation conditions of a Trigger.
type SimpleTriggerStatus struct {
	AgentValid     bool `json:"agentValid"`
	ConnectorValid bool `json:"connectorValid"`
}

// SimpleTriggerCreateRequest is used to create a new Trigger.
type SimpleTriggerCreateRequest struct {
	Name         string              `json:"name"`
	GroupID      string              `json:"groupId"`
	AgentRef     string              `json:"agentRef"`
	ConnectorRef string              `json:"connectorRef"`
	Filters      []TriggerFilterInfo `json:"filters,omitempty"`
}

// SimpleTriggerUpdateRequest is used to update an existing Trigger. All fields are optional.
type SimpleTriggerUpdateRequest struct {
	Name         string              `json:"name,omitempty"`
	AgentRef     string              `json:"agentRef,omitempty"`
	ConnectorRef string              `json:"connectorRef,omitempty"`
	Filters      []TriggerFilterInfo `json:"filters,omitempty"`
}

func toSimpleTriggerResponse(t *triggers.Trigger) SimpleTriggerResponse {
	resp := SimpleTriggerResponse{
		ID:           t.ID,
		Name:         t.DisplayName,
		AgentRef:     t.AgentRef,
		ConnectorRef: t.ConnectorRef,
	}

	for _, f := range t.Filters {
		resp.Filters = append(resp.Filters, TriggerFilterInfo{
			Field: f.Field,
			Op:    f.Op,
			Value: f.Value,
		})
	}

	resp.Status = &SimpleTriggerStatus{
		AgentValid:     t.AgentValid,
		ConnectorValid: t.ConnectorValid,
	}

	return resp
}

// agentExists checks whether the referenced Agent CRD exists in the hub's namespace.
func (s *Server) agentExists(ctx context.Context, name string) bool {
	var agent ainselv1alpha1.Agent
	if err := s.client.Get(ctx, client.ObjectKey{Namespace: s.ns, Name: name}, &agent); err != nil {
		slog.Warn("agent ref validation failed", "agent", name, "error", err)
		return false
	}
	return true
}

// connectorExists checks whether the referenced WebhookConnector CRD exists in the hub's namespace.
func (s *Server) connectorExists(ctx context.Context, name string) bool {
	var wc ainselv1alpha1.WebhookConnector
	if err := s.client.Get(ctx, client.ObjectKey{Namespace: s.ns, Name: name}, &wc); err != nil {
		slog.Warn("connector ref validation failed", "connector", name, "error", err)
		return false
	}
	return true
}

// validateTriggerRefs checks that the referenced Agent and WebhookConnector exist
// and returns the corresponding validity flags.
func (s *Server) validateTriggerRefs(ctx context.Context, agentRef, connectorRef string) (agentValid, connectorValid bool) {
	return s.agentExists(ctx, agentRef), s.connectorExists(ctx, connectorRef)
}

func (s *Server) handleTriggers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		s.listTriggers(ctx, w, r)
	case http.MethodPost:
		s.createTrigger(ctx, w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := extractName(r.URL.Path, "/api/v1/triggers/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing trigger id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.requireRead(w, r, "trigger", id) {
			return
		}
		s.getTrigger(ctx, w, id)
	case http.MethodPut:
		if !s.requireWrite(w, r, "trigger", id) {
			return
		}
		s.updateTrigger(ctx, w, r, id)
	case http.MethodDelete:
		if !s.requireWrite(w, r, "trigger", id) {
			return
		}
		s.deleteTrigger(ctx, w, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listTriggers(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, err := ParsePageParams(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	agentRef := q.Get("agent")
	connectorRef := q.Get("connector")

	all, err := s.triggerStore.ListTriggers(ctx, agentRef, connectorRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Sort by id for stable pagination.
	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})

	// Filter to resources the caller can access.
	if s.authzStore != nil && !s.callerIsAdmin(r) {
		names := make([]string, len(all))
		for i := range all {
			names[i] = all[i].ID
		}
		set := toAccessSet(s.filterByAccess(r, "trigger", names))
		filtered := all[:0]
		for i := range all {
			if set[all[i].ID] {
				filtered = append(filtered, all[i])
			}
		}
		all = filtered
	}

	total := len(all)
	lo, hi := page.Slice(total)
	items := make([]SimpleTriggerResponse, 0, hi-lo)
	for i := lo; i < hi; i++ {
		items = append(items, toSimpleTriggerResponse(&all[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":      items,
		"total":      total,
		"page":       page.Page,
		"pageSize":   page.PageSize,
		"totalPages": page.TotalPages(total),
	})
}

func (s *Server) getTrigger(ctx context.Context, w http.ResponseWriter, id string) {
	t, err := s.triggerStore.GetTrigger(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "trigger not found")
		return
	}
	writeJSON(w, http.StatusOK, toSimpleTriggerResponse(t))
}

func (s *Server) createTrigger(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var req SimpleTriggerCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate group membership for resource creation.
	if s.authzChecker != nil {
		if req.GroupID == "" {
			writeError(w, http.StatusBadRequest, "groupId is required")
			return
		}
		if !s.requireGroupWrite(w, r, req.GroupID) {
			return
		}
	}

	id := generateID("t")

	filters := make([]ainselapishared.Filter, 0, len(req.Filters))
	for _, f := range req.Filters {
		filters = append(filters, ainselapishared.Filter{
			Field: f.Field,
			Op:    f.Op,
			Value: f.Value,
		})
	}

	// Validate references at write time — replaces controller reconciliation
	// with at-write-time validation.
	agentValid, connectorValid := s.validateTriggerRefs(ctx, req.AgentRef, req.ConnectorRef)

	t := &triggers.Trigger{
		ID:             id,
		DisplayName:    req.Name,
		AgentRef:       req.AgentRef,
		ConnectorRef:   req.ConnectorRef,
		Filters:        filters,
		AgentValid:     agentValid,
		ConnectorValid: connectorValid,
	}

	if err := s.triggerStore.CreateTrigger(ctx, t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Assign the trigger to the creator's chosen group.
	if s.authzStore != nil {
		groupID := req.GroupID
		if groupID == "" {
			groupID = "legacy"
		}
		if err := s.authzStore.SetResourceGroup(r.Context(), "trigger", id, groupID, false); err != nil {
			slog.Error("set resource group on create", "error", err, "resource", id)
		}
	}

	writeJSON(w, http.StatusCreated, toSimpleTriggerResponse(t))
}

func (s *Server) updateTrigger(ctx context.Context, w http.ResponseWriter, r *http.Request, id string) {
	var req SimpleTriggerUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	var displayName, agentRef, connectorRef *string
	var filters *[]ainselapishared.Filter

	if req.Name != "" {
		displayName = &req.Name
	}
	if req.AgentRef != "" {
		agentRef = &req.AgentRef
	}
	if req.ConnectorRef != "" {
		connectorRef = &req.ConnectorRef
	}
	if req.Filters != nil {
		fl := make([]ainselapishared.Filter, 0, len(req.Filters))
		for _, f := range req.Filters {
			fl = append(fl, ainselapishared.Filter{
				Field: f.Field,
				Op:    f.Op,
				Value: f.Value,
			})
		}
		filters = &fl
	}

	t, err := s.triggerStore.UpdateTrigger(ctx, id, displayName, agentRef, connectorRef, filters)
	if err != nil {
		writeError(w, http.StatusNotFound, "trigger not found")
		return
	}

	// Re-validate references after update. The store resets validity flags
	// to false when refs change; we re-validate all refs to catch both
	// changed refs and refs whose target was deleted/created since last check.
	agentValid, connectorValid := s.validateTriggerRefs(ctx, t.AgentRef, t.ConnectorRef)
	if err := s.triggerStore.SetTriggerValidity(ctx, id, agentValid, connectorValid); err != nil {
		slog.Error("update trigger validity after update", "error", err, "trigger", id)
	}
	t.AgentValid = agentValid
	t.ConnectorValid = connectorValid

	writeJSON(w, http.StatusOK, toSimpleTriggerResponse(t))
}

func (s *Server) deleteTrigger(ctx context.Context, w http.ResponseWriter, id string) {
	if err := s.triggerStore.DeleteTrigger(ctx, id); err != nil {
		writeError(w, http.StatusNotFound, "trigger not found")
		return
	}

	// Clean up ownership record.
	if s.authzStore != nil {
		if err := s.authzStore.DeleteResourceGroup(ctx, "trigger", id); err != nil {
			slog.Error("delete ownership", "error", err, "resource", id)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

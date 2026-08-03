package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
	cronparse "github.com/DominikPinsel/ainsel/shared/api/cron"
)

// CronTriggerResponse is the simplified API representation of a CronTrigger.
type CronTriggerResponse struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	AgentRef string             `json:"agentRef"`
	Schedule string             `json:"schedule"`
	Prompt   string             `json:"prompt"`
	Enabled  bool               `json:"enabled"`
	Status   *CronTriggerStatus `json:"status,omitempty"`
}

// CronTriggerStatus summarises the validation conditions and last/next run.
type CronTriggerStatus struct {
	AgentValid    bool   `json:"agentValid"`
	ScheduleValid bool   `json:"scheduleValid"`
	Ready         bool   `json:"ready"`
	LastRun       string `json:"lastRun,omitempty"`
	NextRun       string `json:"nextRun,omitempty"`
}

// CronTriggerCreateRequest creates a new CronTrigger.
type CronTriggerCreateRequest struct {
	Name     string `json:"name"`
	GroupID  string `json:"groupId"`
	AgentRef string `json:"agentRef"`
	Schedule string `json:"schedule"`
	Prompt   string `json:"prompt"`
	Enabled  *bool  `json:"enabled,omitempty"`
}

// CronTriggerUpdateRequest updates an existing CronTrigger. All fields optional.
type CronTriggerUpdateRequest struct {
	Name     string `json:"name,omitempty"`
	AgentRef string `json:"agentRef,omitempty"`
	Schedule string `json:"schedule,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
}

// cronTriggerResourceType is the authz resource type used for ownership and
// access grants on cron triggers.
const cronTriggerResourceType = "cron-trigger"

func toCronTriggerResponse(ct *triggers.CronTrigger) CronTriggerResponse {
	resp := CronTriggerResponse{
		ID:       ct.ID,
		Name:     ct.DisplayName,
		AgentRef: ct.AgentRef,
		Schedule: ct.Schedule,
		Prompt:   ct.Prompt,
		Enabled:  ct.Enabled,
	}

	st := &CronTriggerStatus{
		AgentValid:    ct.AgentValid,
		ScheduleValid: ct.ScheduleValid,
		Ready:         ct.AgentValid && ct.ScheduleValid,
	}
	if ct.LastRun != nil {
		st.LastRun = ct.LastRun.Format(time.RFC3339)
	}
	if ct.NextRun != nil {
		st.NextRun = ct.NextRun.Format(time.RFC3339)
	}
	resp.Status = st
	return resp
}

func (s *Server) handleCronTriggers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCronTriggers(r.Context(), w, r)
	case http.MethodPost:
		s.createCronTrigger(r.Context(), w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleCronTrigger(w http.ResponseWriter, r *http.Request) {
	id := extractName(r.URL.Path, "/api/v1/cron-triggers/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing cron trigger id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.requireRead(w, r, cronTriggerResourceType, id) {
			return
		}
		s.getCronTrigger(r.Context(), w, id)
	case http.MethodPut:
		if !s.requireWrite(w, r, cronTriggerResourceType, id) {
			return
		}
		s.updateCronTrigger(r.Context(), w, r, id)
	case http.MethodDelete:
		if !s.requireWrite(w, r, cronTriggerResourceType, id) {
			return
		}
		s.deleteCronTrigger(r.Context(), w, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listCronTriggers(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := ParsePageParams(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	agentRef := q.Get("agent")
	all, err := s.triggerStore.ListCronTriggers(ctx, agentRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})

	// Filter to resources the caller can access.
	if s.authzStore != nil && !s.callerIsAdmin(r) {
		names := make([]string, len(all))
		for i := range all {
			names[i] = all[i].ID
		}
		set := toAccessSet(s.filterByAccess(r, cronTriggerResourceType, names))
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
	items := make([]CronTriggerResponse, 0, hi-lo)
	for i := lo; i < hi; i++ {
		items = append(items, toCronTriggerResponse(&all[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":      items,
		"total":      total,
		"page":       page.Page,
		"pageSize":   page.PageSize,
		"totalPages": page.TotalPages(total),
	})
}

func (s *Server) getCronTrigger(ctx context.Context, w http.ResponseWriter, id string) {
	ct, err := s.triggerStore.GetCronTrigger(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "cron trigger not found")
		return
	}
	writeJSON(w, http.StatusOK, toCronTriggerResponse(ct))
}

func (s *Server) createCronTrigger(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var req CronTriggerCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.AgentRef == "" || req.Schedule == "" || req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "agentRef, schedule, and prompt are required")
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

	// Validate schedule at create time.
	if _, err := cronparse.Parse(req.Schedule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid schedule: "+err.Error())
		return
	}

	id := generateID("c")

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Validate agent ref at write time.
	agentValid := s.agentExists(ctx, req.AgentRef)

	ct := &triggers.CronTrigger{
		ID:            id,
		DisplayName:   req.Name,
		AgentRef:      req.AgentRef,
		Schedule:      req.Schedule,
		Prompt:        req.Prompt,
		Enabled:       enabled,
		AgentValid:    agentValid,
		ScheduleValid: true, // validated above
	}

	if err := s.triggerStore.CreateCronTrigger(ctx, ct); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.authzStore != nil {
		groupID := req.GroupID
		if groupID == "" {
			groupID = "legacy"
		}
		if err := s.authzStore.SetResourceGroup(r.Context(), cronTriggerResourceType, id, groupID, false); err != nil {
			slog.Error("set resource group on create", "error", err, "resource", id)
		}
	}

	writeJSON(w, http.StatusCreated, toCronTriggerResponse(ct))
}

func (s *Server) updateCronTrigger(ctx context.Context, w http.ResponseWriter, r *http.Request, id string) {
	var req CronTriggerUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate schedule at update time if provided.
	if req.Schedule != "" {
		if _, err := cronparse.Parse(req.Schedule); err != nil {
			writeError(w, http.StatusBadRequest, "invalid schedule: "+err.Error())
			return
		}
	}

	var displayName, agentRef, schedule, prompt *string
	var enabled *bool

	if req.Name != "" {
		displayName = &req.Name
	}
	if req.AgentRef != "" {
		agentRef = &req.AgentRef
	}
	if req.Schedule != "" {
		schedule = &req.Schedule
	}
	if req.Prompt != "" {
		prompt = &req.Prompt
	}
	if req.Enabled != nil {
		enabled = req.Enabled
	}

	ct, err := s.triggerStore.UpdateCronTrigger(ctx, id, displayName, agentRef, schedule, prompt, enabled)
	if err != nil {
		writeError(w, http.StatusNotFound, "cron trigger not found")
		return
	}

	// Re-validate agent ref and schedule after update. The store resets
	// validity flags to false when refs change; we re-validate to set them
	// correctly.
	agentValid := s.agentExists(ctx, ct.AgentRef)
	scheduleValid := true // schedule was validated above if provided; if not provided, existing schedule was already valid
	if err := s.triggerStore.SetCronTriggerValidity(ctx, id, agentValid, scheduleValid); err != nil {
		slog.Error("update cron trigger validity after update", "error", err, "trigger", id)
	}
	ct.AgentValid = agentValid
	ct.ScheduleValid = scheduleValid

	writeJSON(w, http.StatusOK, toCronTriggerResponse(ct))
}

func (s *Server) deleteCronTrigger(ctx context.Context, w http.ResponseWriter, id string) {
	if err := s.triggerStore.DeleteCronTrigger(ctx, id); err != nil {
		writeError(w, http.StatusNotFound, "cron trigger not found")
		return
	}
	if s.authzStore != nil {
		if err := s.authzStore.DeleteResourceGroup(ctx, cronTriggerResourceType, id); err != nil {
			slog.Error("delete ownership", "error", err, "resource", id)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

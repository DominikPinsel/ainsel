package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
)

// Activity status values. These mirror the ActivityStatus union in the
// frontend (frontend/src/api/events.ts).
const (
	activityStatusUnmatched = "unmatched"
	activityStatusMatched   = "matched"
	activityStatusError     = "error"
)

// Default and maximum page sizes for the activity list endpoint. The
// frontend requests one page at a time via limit/offset; the cap prevents
// unbounded queries.
const (
	defaultEventLimit = 100
	maxEventLimit     = 500
)

// validActivityStatuses are the status values accepted by the list filter.
var validActivityStatuses = map[string]bool{
	activityStatusUnmatched: true,
	activityStatusMatched:   true,
	activityStatusError:     true,
}

// activityMatch is a single agent/trigger that an event was routed to.
type activityMatch struct {
	Trigger    string `json:"trigger"`
	Agent      string `json:"agent"`
	RunStatus  string `json:"runStatus,omitempty"`
	DurationMs *int64 `json:"durationMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

// activityEntry is the per-event shape consumed by the frontend Activity page.
type activityEntry struct {
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	Connector string          `json:"connector,omitempty"`
	Status    string          `json:"status"`
	Matches   []activityMatch `json:"matches,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// eventsEnvelope wraps one page of the activity list. Events holds the
// requested page; Total is the number of all events matching the filters,
// which the frontend uses to render the pager.
type eventsEnvelope struct {
	Events []activityEntry `json:"events"`
	Total  int             `json:"total"`
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listEvents(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getEvent(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) getEvent(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/events/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing event id")
		return
	}
	if s.eventQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "event queue not configured")
		return
	}

	evt, err := s.eventQueue.GetEvent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get event")
		return
	}
	if evt == nil {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}

	tasks, err := s.eventQueue.TasksForEvents(r.Context(), []string{id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get event tasks")
		return
	}

	writeJSON(w, http.StatusOK, buildActivityEntry(*evt, tasks, s.invocations))
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	if s.eventQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "event queue not configured")
		return
	}

	q := r.URL.Query()

	limit := defaultEventLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = n
	}
	if limit > maxEventLimit {
		limit = maxEventLimit
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid offset")
			return
		}
		offset = n
	}

	filter := eventqueue.EventFilter{Connector: q.Get("connector"), Agent: q.Get("agent")}

	if v := q.Get("status"); v != "" {
		if !validActivityStatuses[v] {
			writeError(w, http.StatusBadRequest, "invalid status: expected matched, unmatched or error")
			return
		}
		filter.Status = v
	}

	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since: expected RFC 3339 timestamp")
			return
		}
		filter.Since = t
	}

	events, err := s.eventQueue.QueryEvents(r.Context(), filter, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}

	total, err := s.eventQueue.CountEvents(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count events")
		return
	}

	entries := make([]activityEntry, 0, len(events))
	if len(events) > 0 {
		ids := make([]string, len(events))
		for i, e := range events {
			ids[i] = e.ID
		}
		tasks, err := s.eventQueue.TasksForEvents(r.Context(), ids)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list event tasks")
			return
		}
		tasksByEvent := make(map[string][]eventqueue.Task, len(events))
		for _, t := range tasks {
			tasksByEvent[t.EventID] = append(tasksByEvent[t.EventID], t)
		}
		for _, e := range events {
			entries = append(entries, buildActivityEntry(e, tasksByEvent[e.ID], s.invocations))
		}
	}

	writeJSON(w, http.StatusOK, eventsEnvelope{Events: entries, Total: total})
}

// buildActivityEntry maps a stored event and its tasks into the frontend
// activity shape. Status is derived from the tasks: no tasks → unmatched,
// any failed task → error, otherwise → matched.
//
// When invStore is non-nil, each match is enriched with run-state information
// (runStatus, durationMs, error) from the corresponding invocation. The
// invocation store is an in-memory ring buffer, so lookups may miss for
// evicted or pre-restart records; in that case the match fields are left
// empty and the UI renders "—".
func buildActivityEntry(evt eventqueue.Event, tasks []eventqueue.Task, invStore *invocations.Store) activityEntry {
	entry := activityEntry{
		ID:        evt.ID,
		Timestamp: evt.ReceivedAt.UTC().Format(time.RFC3339),
		Connector: evt.Connector,
		Status:    activityStatusUnmatched,
	}

	if len(tasks) > 0 {
		entry.Status = activityStatusMatched
		matches := make([]activityMatch, 0, len(tasks))
		for _, t := range tasks {
			if t.Status == "failed" {
				entry.Status = activityStatusError
			}
			m := activityMatch{Trigger: t.TriggerName, Agent: t.AgentName}
			if invStore != nil && t.InvocationID != "" {
				if inv, ok := invStore.Get(t.InvocationID); ok {
					m.RunStatus = inv.Status
					m.DurationMs = inv.DurationMs
					m.Error = inv.Error
				}
			}
			matches = append(matches, m)
		}
		entry.Matches = matches
	}

	if payload := activityPayload(evt.Data); payload != nil {
		entry.Payload = payload
	}

	return entry
}

// activityPayload returns the event data as a raw JSON payload, or nil when
// the data is empty or an empty object so the field is omitted from the
// response.
func activityPayload(data json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return nil
	}
	return data
}

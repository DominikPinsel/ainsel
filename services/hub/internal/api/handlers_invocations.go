package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
)

// invocationStatusFromTask maps an agent_tasks status onto the invocation
// lifecycle vocabulary. A pending or claimed task means the agent has not
// finished (or not even started) the turn, so both map to running; the
// precise queue state is carried alongside in Invocation.Task.
func invocationStatusFromTask(task eventqueue.Task) string {
	switch task.Status {
	case "completed":
		return invocations.StatusSuccess
	case "failed":
		return invocations.StatusFailure
	default:
		return invocations.StatusRunning
	}
}

// taskStateFromTask projects the agent_tasks row onto the API's TaskState.
func taskStateFromTask(task eventqueue.Task) *invocations.TaskState {
	return &invocations.TaskState{
		Status:   task.Status,
		Attempts: task.Attempts,
		Error:    task.Error,
	}
}

// enrichWithTasks attaches the queue state of the agent_tasks row matching
// each invocation by ID, and appends synthetic invocation records for tasks
// whose invocation is missing from the in-memory store (pre-restart history
// or tasks still waiting in the queue).
func (s *Server) enrichWithTasks(r *http.Request, scope telemetryScope, list []invocations.Invocation, total int, eventID string) ([]invocations.Invocation, int) {
	tasks, err := s.eventQueue.TasksForEvents(r.Context(), []string{eventID})
	if err != nil {
		// Queue state is best-effort enrichment; never fail the endpoint
		// because of it.
		return list, total
	}

	byInvocation := make(map[string]eventqueue.Task, len(tasks))
	for _, t := range tasks {
		if t.InvocationID != "" {
			byInvocation[t.InvocationID] = t
		}
	}

	known := make(map[string]bool, len(list))
	for i := range list {
		if t, ok := byInvocation[list[i].ID]; ok {
			list[i].Task = taskStateFromTask(t)
		}
		known[list[i].ID] = true
	}

	// Tasks newest-first (agent_tasks IDs are monotonic), skipping those
	// already represented above.
	for i := len(tasks) - 1; i >= 0; i-- {
		t := tasks[i]
		if t.InvocationID == "" || known[t.InvocationID] {
			continue
		}
		// Synthesized entries come straight from agent_tasks and never passed
		// through the scoped store query, so they need the same check here —
		// otherwise ?event=<id> would reintroduce every agent the caller
		// cannot read.
		if !scope.allowsAgent(t.AgentName) {
			continue
		}
		known[t.InvocationID] = true
		inv := invocations.Invocation{
			ID:          t.InvocationID,
			AgentName:   t.AgentName,
			TriggerName: t.TriggerName,
			EventID:     t.EventID,
			Status:      invocationStatusFromTask(t),
			StartTime:   t.CreatedAt,
			EndTime:     t.CompletedAt,
			Task:        taskStateFromTask(t),
		}
		if inv.EndTime != nil && t.CreatedAt != (time.Time{}) {
			d := inv.EndTime.Sub(inv.StartTime).Milliseconds()
			inv.DurationMs = &d
		}
		if t.Status == "failed" {
			inv.Error = t.Error
		}
		list = append(list, inv)
	}
	return list, len(list)
}

// handleInvocations handles the /api/v1/invocations collection endpoint.
func (s *Server) handleInvocations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listInvocations(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleInvocation handles /api/v1/invocations/:id (single invocation).
func (s *Server) handleInvocation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/invocations/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing invocation id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getInvocation(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listInvocations(w http.ResponseWriter, r *http.Request) {
	if s.invocations == nil {
		writeError(w, http.StatusServiceUnavailable, "invocation history not configured")
		return
	}
	q := r.URL.Query()

	page, err := ParsePageParams(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	opts := invocations.ListOptions{
		AgentName:   q.Get("agent"),
		Status:      q.Get("status"),
		TriggerName: q.Get("trigger"),
		EventID:     q.Get("event"),
	}

	// An invocation belongs to the agent that ran it. Naming one requires read
	// access to that agent; an unfiltered query is restricted to the agents the
	// caller may read. The other filters narrow within that scope.
	scope := s.telemetryScopeFor(r)
	if opts.AgentName != "" {
		if !s.requireRead(w, r, "agent", opts.AgentName) {
			return
		}
	} else if !scope.unrestricted {
		if len(scope.agents) == 0 {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"invocations": []invocations.Invocation{},
				"total":       0,
				"capacity":    s.invocations.Capacity(),
				"page":        page.Page,
				"pageSize":    page.PageSize,
				"totalPages":  0,
			})
			return
		}
		opts.AgentNames = scope.agents
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.Since = t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.Until = t
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			opts.Limit = n
		}
	}

	// total is the number of invocations matching the filters before
	// opts.Limit is applied, so clients can detect truncation even when the
	// result set is capped.
	list, total := s.invocations.ListWithTotal(opts)

	// Enrich with queue state from agent_tasks and synthesize entries for
	// tasks whose invocation is no longer (or never was) in the in-memory
	// store — the ring buffer is wiped on hub restarts and evicts old
	// records, but agent_tasks persists. This keeps the event detail page
	// meaningful for older events and for tasks that are still queued.
	if opts.EventID != "" && s.eventQueue != nil {
		list, total = s.enrichWithTasks(r, scope, list, total, opts.EventID)
	}
	lo, hi := page.Slice(len(list))
	pageItems := list[lo:hi]
	if pageItems == nil {
		pageItems = []invocations.Invocation{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"invocations": pageItems,
		"total":       total,
		"capacity":    s.invocations.Capacity(),
		"page":        page.Page,
		"pageSize":    page.PageSize,
		"totalPages":  page.TotalPages(len(list)),
	})
}

func (s *Server) getInvocation(w http.ResponseWriter, r *http.Request, id string) {
	if s.invocations == nil {
		writeError(w, http.StatusServiceUnavailable, "invocation history not configured")
		return
	}
	inv, ok := s.invocations.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "invocation not found")
		return
	}
	// A record with no agent cannot be attributed to any resource, so only an
	// unrestricted caller may see it.
	if inv.AgentName == "" {
		if scope := s.telemetryScopeFor(r); !scope.unrestricted {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	} else if !s.requireRead(w, r, "agent", inv.AgentName) {
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

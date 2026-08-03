package api

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
	"github.com/DominikPinsel/ainsel/services/hub/internal/types"
)

// SetEventQueue wires the event queue store for the ingest and agent task endpoints.
func (s *Server) SetEventQueue(eq *eventqueue.Store) {
	s.eventQueue = eq
}

// requireInternalToken checks the X-Internal-Token header against the
// configured shared secret. Returns true if the request is authorized.
func (s *Server) requireInternalToken(w http.ResponseWriter, r *http.Request) bool {
	if s.internalValidateSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "internal endpoints disabled")
		return false
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Internal-Token")), []byte(s.internalValidateSecret)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid internal token")
		return false
	}
	return true
}

// handleIngestEvent accepts POST /api/internal/events from connectors
// (webhook-receiver). Protected by X-Internal-Token.
func (s *Server) handleIngestEvent(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalToken(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.eventQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "event queue not configured")
		return
	}

	var evt eventqueue.Event
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if evt.ID == "" || evt.Connector == "" {
		writeError(w, http.StatusBadRequest, "id and connector are required")
		return
	}

	if err := s.eventQueue.InsertEvent(r.Context(), evt); err != nil {
		slog.Error("ingest event failed", "event_id", evt.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store event")
		return
	}

	slog.Info("event ingested", "event_id", evt.ID, "connector", evt.Connector)
	w.WriteHeader(http.StatusAccepted)
}

// handleAgentNextTask serves GET /api/internal/agents/{name}/next-task?timeout=30s.
// Long-polls for the next pending task for the named agent.
// Returns 200 with the task JSON, or 204 No Content on timeout.
func (s *Server) handleAgentNextTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireInternalToken(w, r) {
		return
	}
	if s.eventQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "event queue not configured")
		return
	}

	// Extract agent name from path: /api/internal/agents/{name}/next-task
	// or legacy /api/v1/agents/{name}/next-task.
	path := stripAgentsPrefix(r.URL.Path)
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] != "next-task" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	agentName := parts[0]

	// Parse timeout (default 30s, max 60s).
	timeout := 30 * time.Second
	if v := r.URL.Query().Get("timeout"); v != "" {
		if secs, err := strconv.Atoi(strings.TrimSuffix(v, "s")); err == nil && secs > 0 {
			timeout = time.Duration(secs) * time.Second
			if timeout > 60*time.Second {
				timeout = 60 * time.Second
			}
		}
	}

	task, err := s.eventQueue.WaitForTask(r.Context(), agentName, timeout)
	if err != nil {
		slog.Error("wait for task failed", "agent", agentName, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to poll task")
		return
	}
	if task == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	writeJSON(w, http.StatusOK, task)
}

// handleAgentTaskAck serves POST /api/internal/agents/{name}/tasks/{id}/ack.
func (s *Server) handleAgentTaskAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.eventQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "event queue not configured")
		return
	}

	agentName, taskID, ok := parseAgentTaskPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// Verify the task belongs to this agent.
	task, err := s.eventQueue.GetTask(r.Context(), taskID, agentName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get task")
		return
	}
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	if err := s.eventQueue.AckTask(r.Context(), taskID); err != nil {
		slog.Error("ack task failed", "task_id", taskID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to ack task")
		return
	}

	// Update invocation store if we have one.
	if s.invocations != nil && task.InvocationID != "" {
		s.invocations.Complete(task.InvocationID, "success", "", time.Time{})
	}

	// Broadcast stats to WebSocket clients.
	if s.wsHub != nil {
		s.broadcastQueueStats(r)
	}

	slog.Info("task acked", "task_id", taskID, "agent", agentName, "invocation_id", task.InvocationID)
	w.WriteHeader(http.StatusNoContent)
}

// handleAgentTaskNack serves POST /api/internal/agents/{name}/tasks/{id}/nack.
func (s *Server) handleAgentTaskNack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.eventQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "event queue not configured")
		return
	}

	agentName, taskID, ok := parseAgentTaskPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// Verify the task belongs to this agent.
	task, err := s.eventQueue.GetTask(r.Context(), taskID, agentName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get task")
		return
	}
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	var body struct {
		Error   string `json:"error"`
		DelayMs int    `json:"delay_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	delay := 60 * time.Second
	if body.DelayMs > 0 {
		delay = time.Duration(body.DelayMs) * time.Millisecond
	}

	if err := s.eventQueue.NakTask(r.Context(), taskID, delay, body.Error); err != nil {
		slog.Error("nack task failed", "task_id", taskID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to nack task")
		return
	}

	// Update invocation store.
	if s.invocations != nil && task.InvocationID != "" {
		s.invocations.Complete(task.InvocationID, "failure", body.Error, time.Time{})
	}

	// Log error event for observability.
	slog.Info("error_event",
		"log_type", "error_event",
		"severity", "error",
		"source", "agent",
		"agent", agentName,
		"error_message", body.Error,
		"task_id", taskID,
		"invocation_id", task.InvocationID,
	)

	// Broadcast error to WebSocket clients.
	if s.wsHub != nil {
		s.broadcastAgentError(agentName, body.Error, task.InvocationID)
	}

	slog.Info("task nacked", "task_id", taskID, "agent", agentName, "delay_ms", body.DelayMs)
	w.WriteHeader(http.StatusNoContent)
}

// handleQueueInfo serves GET /api/v1/queue/info — queue statistics endpoint.
func (s *Server) handleQueueInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.eventQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "event queue not configured")
		return
	}

	info, err := s.eventQueue.GetStreamInfo(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get queue info")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleQueueRecent serves GET /api/v1/queue/recent?count=20&connector= — recent events endpoint.
func (s *Server) handleQueueRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.eventQueue == nil {
		writeError(w, http.StatusServiceUnavailable, "event queue not configured")
		return
	}

	count := 20
	if v := r.URL.Query().Get("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			count = n
			if count > 100 {
				count = 100
			}
		}
	}
	connector := r.URL.Query().Get("connector")

	events, err := s.eventQueue.RecentEvents(r.Context(), count, connector, time.Time{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get recent events")
		return
	}
	if events == nil {
		events = []eventqueue.Event{}
	}
	writeJSON(w, http.StatusOK, events)
}

// stripAgentsPrefix removes the /api/internal/agents/ or /api/v1/agents/
// prefix from a path, returning the remainder (e.g. "{name}/next-task").
func stripAgentsPrefix(path string) string {
	if s := strings.TrimPrefix(path, "/api/internal/agents/"); s != path {
		return s
	}
	return strings.TrimPrefix(path, "/api/v1/agents/")
}

// parseAgentTaskPath extracts agent name and task ID from paths like
// /api/internal/agents/{name}/tasks/{id}/ack or /api/v1/agents/{name}/tasks/{id}/nack.
func parseAgentTaskPath(path string) (agentName string, taskID int64, ok bool) {
	trimmed := stripAgentsPrefix(path)
	// Expected: {name}/tasks/{id}/ack or {name}/tasks/{id}/nack
	parts := strings.Split(trimmed, "/")
	if len(parts) != 4 || parts[1] != "tasks" {
		return "", 0, false
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return parts[0], id, true
}

// broadcastStats broadcasts current stats to WebSocket clients.
func (s *Server) broadcastQueueStats(r *http.Request) {
	if s.wsHub == nil {
		return
	}
	stats := s.GetStats(r.Context())
	s.wsHub.broadcast(wsMessage{Type: "stats", Data: stats})
}

// broadcastAgentError broadcasts an agent error to WebSocket clients.
func (s *Server) broadcastAgentError(agentName, errMsg, invocationID string) {
	if s.wsHub == nil {
		return
	}
	s.BroadcastError(types.ErrorEntry{
		Severity: "error",
		Source:   "agent",
		Message:  errMsg,
		Details:  map[string]interface{}{"agent": agentName, "invocation_id": invocationID},
	})
}

// handleIngestTaskLog accepts POST /api/internal/task-logs from agent runtimes.
func (s *Server) handleIngestTaskLog(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalToken(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.taskLogs == nil {
		writeError(w, http.StatusServiceUnavailable, "task log store not configured")
		return
	}

	var entry tasklogs.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if err := s.taskLogs.Insert(r.Context(), &entry); err != nil {
		slog.Error("task log insert failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store task log")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleIngestTaskMessage accepts POST /api/internal/task-messages from agent runtimes.
func (s *Server) handleIngestTaskMessage(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalToken(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.taskLogs == nil {
		writeError(w, http.StatusServiceUnavailable, "task log store not configured")
		return
	}

	var msg tasklogs.ConversationMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	if err := s.taskLogs.InsertConversation(r.Context(), &msg); err != nil {
		slog.Error("task message insert failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store task message")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

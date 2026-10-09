package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
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

	// Stamp the channel the event is born in. The connector label decides it —
	// never whatever the publisher put in the body, since a stream you did not
	// publish to must not be writable by naming it. Provisioning happens here on
	// first sight, so an event can never be stored without a home even if the
	// reconciler has not run yet.
	if s.channelSvc != nil {
		channelID, err := s.channelSvc.ConnectorChannelFor(r.Context(), evt.Connector)
		if err != nil {
			// A missing channel is a broken registry, not a bad event: store it
			// unstamped rather than rejecting a webhook a connector already
			// accepted and would otherwise retry forever.
			slog.Warn("could not resolve birth channel", "event_id", evt.ID, "connector", evt.Connector, "error", err)
		}
		evt.ChannelID = channelID
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

	// A re-claimed task (attempts >= 2) is a retry of an attempt that already
	// finished — its original invocation row was closed 'failure' by the nack
	// handler when that attempt gave up. Retrying under the dead id would run
	// invisibly: nothing in 'running' for the Activity feed, logs and
	// transcript filed under a completed record. Record a fresh invocation and
	// repoint the task at it so the attempt is observable end to end.
	if err := recordRetryInvocation(r.Context(), s.invocations, s.eventQueue, s.eventQueue, task); err != nil {
		slog.Error("retry invocation not recorded", "task_id", task.ID, "agent", agentName, "error", err)
	}

	writeJSON(w, http.StatusOK, task)
}

// headerInvocationID names the task header carrying the invocation id. The
// same constant lives in the router and cron packages; the api package needs
// its own copy because it must rewrite the header when re-pointing a retry at
// a fresh invocation row.
const headerInvocationID = "X-Invocation-ID"

// taskInvocationUpdater and eventConnectorReader narrow the *eventqueue.Store
// surface recordRetryInvocation depends on, so tests can use fakes instead of
// a live PostgreSQL pool.
type taskInvocationUpdater interface {
	UpdateTaskInvocation(ctx context.Context, taskID int64, invocationID string, headers json.RawMessage) error
}

type eventConnectorReader interface {
	EventConnector(ctx context.Context, eventID string) (string, bool, error)
}

// recordRetryInvocation re-points a re-claimed (retry) task at a freshly
// recorded invocation row, mutating task in place so the long-poll response
// hands the runner the new id.
//
// A task is claimed once per attempt, and attempts only grows on claims, so
// Attempts >= 2 reliably identifies a retry. Only a finished ('terminal')
// invocation is superseded: a 'running' row or one that was pruned (unknown)
// means there is nothing to supersede and the task keeps its current id.
// The closed rows of earlier attempts are left untouched as history, which
// gives the Activity feed one row per attempt instead of a single row whose
// status flip-flops under concurrent attempts.
func recordRetryInvocation(ctx context.Context, rec invocations.Store, upd taskInvocationUpdater, events eventConnectorReader, task *eventqueue.Task) error {
	if task == nil || task.Attempts < 2 || task.AgentName == "" {
		return nil
	}
	if task.InvocationID != "" {
		// Only supersede a finished invocation. Get returning false means the
		// row was pruned (e.g. capacity eviction) — then keep the old id rather
		// than orphan the task on a row that was never persisted.
		if orig, ok := rec.Get(task.InvocationID); !ok || !orig.IsTerminal() {
			return nil
		}
	} // else: tasks routed without an invocation (e.g. channel transfers) also
	// get one from this retry on — until now they had none at any attempt.

	connector := ""
	if events != nil {
		c, known, err := events.EventConnector(ctx, task.EventID)
		if err != nil {
			// Best effort: an unknown connector does not justify hiding the
			// attempt — record without the label rather than fail the hook.
			slog.Warn("retry invocation: connector lookup failed", "task_id", task.ID, "event_id", task.EventID, "error", err)
		} else if known {
			connector = c
		}
	}

	recorded := rec.Record(invocations.Invocation{
		AgentName:   task.AgentName,
		TriggerName: task.TriggerName,
		EventID:     task.EventID,
		Connector:   connector,
	})

	headers := rewriteInvocationHeader(task.Headers, recorded.ID)
	if err := upd.UpdateTaskInvocation(ctx, task.ID, recorded.ID, headers); err != nil {
		return fmt.Errorf("repoint task %d to invocation %s: %w", task.ID, recorded.ID, err)
	}

	previous := task.InvocationID
	task.InvocationID = recorded.ID
	task.Headers = headers
	slog.Info("retry invocation recorded",
		"task_id", task.ID,
		"attempt", task.Attempts,
		"agent", task.AgentName,
		"trigger", task.TriggerName,
		"invocation_id", recorded.ID,
		"previous_invocation_id", previous,
	)
	return nil
}

// rewriteInvocationHeader replaces the X-Invocation-ID entry in a task's
// cached headers. Headers without one (or unparsable JSON) are returned
// unchanged — the runner reads the id from the task's invocation_id field,
// the header is only kept for context parity with the first attempt.
func rewriteInvocationHeader(headers json.RawMessage, invocationID string) json.RawMessage {
	if len(headers) == 0 {
		return headers
	}
	var m map[string]any
	if err := json.Unmarshal(headers, &m); err != nil {
		return headers
	}
	m[headerInvocationID] = invocationID
	out, err := json.Marshal(m)
	if err != nil {
		return headers
	}
	return out
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

// handleQueueRecent serves GET /api/v1/queue/recent?count=20&connector=&filter= — recent events endpoint.
// filter is a subject pattern of the form "<connector>.<eventType>" with
// "*"/">" wildcards (see eventqueue.ParseSubjectFilter).
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
	filter := r.URL.Query().Get("filter")

	events, err := s.eventQueue.RecentEvents(r.Context(), count, connector, filter, time.Time{})
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

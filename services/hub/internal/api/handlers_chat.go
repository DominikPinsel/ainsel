package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/DominikPinsel/ainsel/services/hub/internal/channels"
	"github.com/DominikPinsel/ainsel/services/hub/internal/chat"
	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// resolveAgentNameForRouting returns the Kubernetes CR name for an agent.
// If the stored name already looks like a CR name (starts with "a-"), it
// is returned as-is. Otherwise we look up agents by display name so legacy
// sessions and future sessions both route to the correct agent.
func (s *Server) resolveAgentNameForRouting(ctx context.Context, name string) (string, error) {
	if strings.HasPrefix(name, "a-") {
		return name, nil
	}
	var list agentv1alpha1.AgentList
	if err := s.client.List(ctx, &list, client.InNamespace(s.ns)); err != nil {
		return "", fmt.Errorf("list agents: %w", err)
	}
	for _, a := range list.Items {
		if a.Spec.DisplayName == name {
			return a.Name, nil
		}
	}
	return "", fmt.Errorf("no agent with display name %q", name)
}

// enrichSessionDisplayName looks up the agent by CR name and, if found,
// replaces the stored AgentName with the agent's DisplayName so the UI
// stays human-readable. Falls back to the stored value on any error.
func (s *Server) enrichSessionDisplayName(ctx context.Context, sess *chat.Session) {
	var agent agentv1alpha1.Agent
	if err := s.client.Get(ctx, client.ObjectKey{Name: sess.AgentName, Namespace: s.ns}, &agent); err != nil {
		return
	}
	if agent.Spec.DisplayName != "" {
		sess.AgentName = agent.Spec.DisplayName
	}
}
func (s *Server) handleChatSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createChatSession(w, r)
	case http.MethodGet:
		s.listChatSessions(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleChatSession handles /api/v1/chat/sessions/{id} (single session).
func (s *Server) handleChatSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/chat/sessions/")
	// Strip any trailing path segments (e.g. /messages).
	slashIdx := strings.Index(id, "/")
	if slashIdx >= 0 {
		id = id[:slashIdx]
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	// Check if this is a /messages sub-path.
	if strings.Contains(r.URL.Path, "/messages") {
		switch r.Method {
		case http.MethodPost:
			s.addChatMessage(w, r, id)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getChatSession(w, r, id)
	case http.MethodPatch:
		s.updateChatSession(w, r, id)
	case http.MethodDelete:
		s.deleteChatSession(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) createChatSession(w http.ResponseWriter, r *http.Request) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not configured")
		return
	}
	var req chat.CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.AgentName == "" {
		writeError(w, http.StatusBadRequest, "agentName is required")
		return
	}

	userID := ""
	if u, ok := oidc.FromContext(r.Context()); ok {
		userID = u.Sub
	}
	if userID == "" {
		userID = "anonymous"
	}

	sess, err := s.chat.CreateSession(r.Context(), req.AgentName, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

func (s *Server) listChatSessions(w http.ResponseWriter, r *http.Request) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not configured")
		return
	}
	page, err := ParsePageParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	opts := chat.ListSessionsOptions{
		AgentName: q.Get("agent"),
		UserID:    q.Get("user"),
		Limit:     page.PageSize * page.Page, // fetch enough rows for the requested page
	}

	all, err := s.chat.ListSessions(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions: "+err.Error())
		return
	}
	total := len(all)
	lo, hi := page.Slice(total)
	items := all[lo:hi]
	if items == nil {
		items = []chat.Session{}
	}
	for i := range items {
		s.enrichSessionDisplayName(r.Context(), &items[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"total":      total,
		"page":       page.Page,
		"pageSize":   page.PageSize,
		"totalPages": page.TotalPages(total),
	})
}

func (s *Server) getChatSession(w http.ResponseWriter, r *http.Request, id string) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not configured")
		return
	}
	sess, err := s.chat.GetSession(r.Context(), id)
	if errors.Is(err, chat.ErrSessionNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get session: "+err.Error())
		return
	}
	s.enrichSessionDisplayName(r.Context(), sess)
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) updateChatSession(w http.ResponseWriter, r *http.Request, id string) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not configured")
		return
	}
	var req chat.UpdateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	// Trim and validate the name.
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required and must not be empty")
		return
	}

	if err := s.chat.UpdateSessionName(r.Context(), id, name); err != nil {
		if errors.Is(err, chat.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update session: "+err.Error())
		return
	}

	// Return the updated session so the caller sees the new name.
	sess, err := s.chat.GetSession(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch updated session: "+err.Error())
		return
	}
	s.enrichSessionDisplayName(r.Context(), sess)
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) deleteChatSession(w http.ResponseWriter, r *http.Request, id string) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not configured")
		return
	}
	if err := s.chat.DeleteSession(r.Context(), id); errors.Is(err, chat.ErrSessionNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete session: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addChatMessage(w http.ResponseWriter, r *http.Request, sessionID string) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable, "chat not configured")
		return
	}
	var req chat.CreateMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Role == "" {
		writeError(w, http.StatusBadRequest, "role is required")
		return
	}
	// Validate role against the allowed set to prevent spoofing.
	switch req.Role {
	case chat.RoleUser, chat.RoleAssistant, chat.RoleStatus:
		// ok
	default:
		writeError(w, http.StatusBadRequest, "invalid role: must be user, assistant, or status")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	msg, err := s.chat.AddMessage(r.Context(), sessionID, req.Role, req.Content, req.Tokens)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add message: "+err.Error())
		return
	}

	// Broadcast the new message to connected WebSocket clients so the
	// frontend can update in real-time.
	if s.wsHub != nil {
		s.wsHub.broadcast(wsMessage{
			Type: "chat.message",
			Data: map[string]any{
				"sessionId": sessionID,
				"message":   msg,
			},
		})
	}

	// When a user sends a message, enqueue a chat.message event for the
	// agent via the event queue. The agent responds via the
	// mcp__chat__send_reply MCP tool.
	if req.Role == chat.RoleUser && s.eventQueue != nil {
		sess, err := s.chat.GetSession(r.Context(), sessionID)
		if err == nil {
			agentName, resolveErr := s.resolveAgentNameForRouting(r.Context(), sess.AgentName)
			if resolveErr != nil {
				slog.Warn("chat publish: could not resolve agent name, falling back to stored value", "agentName", sess.AgentName, "error", resolveErr)
				agentName = sess.AgentName
			}
			eventID := fmt.Sprintf("chat-%s-%d", sessionID, msg.ID)
			dataJSON, _ := json.Marshal(map[string]any{
				"session_id": sessionID,
				"message":    req.Content,
			})
			headers := map[string]string{"type": "chat.message"}
			headersJSON, _ := json.Marshal(headers)

			// A chat message is born directly in the agent's inbox: no connector
			// published it, the user typed it into that conversation.
			var birthChannel string
			if s.channelSvc != nil {
				id, err := s.channelSvc.InboxChannelFor(r.Context(), agentName)
				if err != nil {
					slog.Warn("chat publish: could not resolve inbox channel", "agent", agentName, "error", err)
				} else {
					birthChannel = id
				}
			}

			if err := s.eventQueue.InsertEvent(r.Context(), eventqueue.Event{
				ID:        eventID,
				Connector: ainselapishared.SourceChat,
				ChannelID: birthChannel,
				Headers:   headersJSON,
				Data:      dataJSON,
				Raw:       string(dataJSON),
			}); err != nil {
				writeError(w, http.StatusBadGateway, "failed to store chat event: "+err.Error())
				return
			}
			_ = s.eventQueue.MarkRouted(r.Context(), eventID)
			taskHeaders := map[string]string{"type": "chat.message", "X-Trigger-Name": ainselapishared.SourceChat}
			taskHeadersJSON, _ := json.Marshal(taskHeaders)
			if err := s.eventQueue.EnqueueTask(r.Context(), eventqueue.Task{
				EventID:     eventID,
				AgentName:   agentName,
				TriggerName: ainselapishared.SourceChat,
				Headers:     taskHeadersJSON,
				Payload:     dataJSON,
			}); err != nil {
				writeError(w, http.StatusBadGateway, "failed to enqueue chat event for agent: "+err.Error())
				return
			}
			// Anything attached to that inbox now sees the message too.
			s.transferChatMessage(r.Context(), eventID, birthChannel, agentName, headers, dataJSON)
		}
	}

	writeJSON(w, http.StatusCreated, msg)
}

// transferChatMessage performs the bridge subscriptions out of an inbox a chat
// message was just delivered to. Failures are logged: the user's message has
// already been stored and queued for their own agent.
func (s *Server) transferChatMessage(ctx context.Context, eventID, birthChannel, agentName string, headers map[string]string, payload json.RawMessage) {
	if s.channelTransfer == nil || birthChannel == "" {
		return
	}
	if _, err := s.channelTransfer.Apply(ctx, channels.TransferInput{
		EventID:      eventID,
		BirthChannel: birthChannel,
		SourceLabel:  ainselapishared.SourceChat,
		Headers:      headers,
		Payload:      payload,
		Delivered:    []string{agentName},
	}); err != nil {
		slog.Error("chat transfer failed", "event_id", eventID, "channel", birthChannel, "error", err)
	}
}

// --- Internal (agent-sidecar) chat endpoints ---
//
// The chat sidecar running in each agent pod talks to the hub using the
// shared X-Internal-Token secret. Because it is not an OIDC user it cannot
// authenticate against /api/v1/chat/*, which the OIDC auth middleware
// protects. The routes below expose the subset of the chat API the sidecar
// needs under /api/internal/, bypassing OIDC and authenticating via
// X-Internal-Token at the handler level — the same pattern used by the
// event-queue and task endpoints.

const internalChatSessionsPrefix = "/api/internal/chat/sessions/"

// handleInternalChatSessions handles /api/internal/chat/sessions.
// Only GET (list sessions) is exposed to the sidecar.
func (s *Server) handleInternalChatSessions(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalToken(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.listChatSessions(w, r)
}

// handleInternalChatSession handles /api/internal/chat/sessions/{id} and
// /api/internal/chat/sessions/{id}/messages. Only GET (session + history)
// and POST messages (send reply/status) are exposed to the sidecar.
func (s *Server) handleInternalChatSession(w http.ResponseWriter, r *http.Request) {
	if !s.requireInternalToken(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, internalChatSessionsPrefix)
	// Strip any trailing path segments (e.g. /messages).
	if slashIdx := strings.Index(id, "/"); slashIdx >= 0 {
		id = id[:slashIdx]
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	if strings.Contains(r.URL.Path, "/messages") {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.addChatMessage(w, r, id)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.getChatSession(w, r, id)
}

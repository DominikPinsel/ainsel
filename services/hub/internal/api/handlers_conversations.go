package api

import (
	"net/http"
	"strconv"

	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
)

// handleConversations serves GET /api/v1/observability/conversations.
//
// Returns conversation messages captured from agent turns, stored in the
// task_conversations table.
//
// Query parameters:
//
//	?agent=<name>          Filter by agent name.
//	?invocation=<id>       Filter by invocation ID.
//	?correlation=<id>      Filter by correlation ID.
//	?limit=N               Max messages (default 100).
func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.taskLogs == nil {
		writeError(w, http.StatusServiceUnavailable, "log backend not configured")
		return
	}

	q := r.URL.Query()
	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
			if limit > 500 {
				limit = 500
			}
		}
	}

	messages, err := s.taskLogs.ListConversations(
		r.Context(),
		q.Get("agent"),
		q.Get("invocation"),
		q.Get("correlation"),
		limit,
	)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query conversations: "+err.Error())
		return
	}

	if messages == nil {
		messages = []tasklogs.ConversationMessage{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"messages": messages,
		"total":    len(messages),
	})
}

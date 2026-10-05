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
		writeError(w, http.StatusServiceUnavailable, logStoreRequiredMessage)
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

	opts := tasklogs.ConversationListOptions{
		AgentName:     q.Get("agent"),
		InvocationID:  q.Get("invocation"),
		CorrelationID: q.Get("correlation"),
		Limit:         limit,
	}

	// Conversations are the agent/user dialogue verbatim, so they get the same
	// treatment as task logs: naming an agent requires read access to it, and
	// an unfiltered query is restricted to the agents the caller may read. The
	// invocation and correlation filters narrow within that scope but cannot
	// widen it.
	if opts.AgentName != "" {
		if !s.requireRead(w, r, "agent", opts.AgentName) {
			return
		}
	} else if scope := s.telemetryScopeFor(r); !scope.unrestricted {
		if len(scope.agents) == 0 {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"messages": []tasklogs.ConversationMessage{},
				"total":    0,
			})
			return
		}
		opts.AgentNames = scope.agents
	}

	messages, err := s.taskLogs.ListConversations(r.Context(), opts)
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

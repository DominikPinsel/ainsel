package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
)

func (s *Server) handleErrors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listErrors(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// listErrors serves error entries from the hub's own task_logs table.
func (s *Server) listErrors(w http.ResponseWriter, r *http.Request) {
	if s.taskLogs == nil {
		writeError(w, http.StatusServiceUnavailable, "log backend not configured")
		return
	}

	q := r.URL.Query()

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	// Time range: default last 24h.
	start := time.Now().Add(-24 * time.Hour)
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = t
		}
	}

	opts := tasklogs.ListOptions{
		Level: tasklogs.LevelError,
		Since: start,
		Limit: limit,
	}
	if v := q.Get("agent"); v != "" {
		opts.AgentName = v
	}

	entries, err := s.taskLogs.List(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query errors: "+err.Error())
		return
	}

	errors := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		entry := map[string]interface{}{
			"id":        fmt.Sprintf("err-%d", e.ID),
			"timestamp": e.CreatedAt.Format(time.RFC3339Nano),
			"severity":  e.Level,
			"source":    "agent",
			"agent":     e.AgentName,
			"message":   e.Message,
		}
		if len(e.Fields) > 0 {
			entry["details"] = e.Fields
		}
		errors = append(errors, entry)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"errors": errors,
		"total":  len(errors),
	})
}

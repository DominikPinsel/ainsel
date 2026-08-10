package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
)

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
		s.getInvocation(w, id)
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

func (s *Server) getInvocation(w http.ResponseWriter, id string) {
	if s.invocations == nil {
		writeError(w, http.StatusServiceUnavailable, "invocation history not configured")
		return
	}
	inv, ok := s.invocations.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "invocation not found")
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

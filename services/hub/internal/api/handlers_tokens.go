// internal/api/handlers_tokens.go
package api

import (
	"fmt"
	"net/http"

	"github.com/DominikPinsel/ainsel/services/hub/internal/prometheus"
)

// TokenEntry represents token consumption for one agent + repo + issue + model combination.
type TokenEntry struct {
	Agent        string  `json:"agent"`
	Repository   string  `json:"repository"`
	IssueNumber  string  `json:"issueNumber"`
	Model        string  `json:"model"`
	InputTokens  float64 `json:"inputTokens"`
	OutputTokens float64 `json:"outputTokens"`
}

// TokenTotals holds aggregate token counts.
type TokenTotals struct {
	InputTokens  float64 `json:"inputTokens"`
	OutputTokens float64 `json:"outputTokens"`
}

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTokens(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	if s.prom == nil {
		writeError(w, http.StatusServiceUnavailable, "metrics backend not configured")
		return
	}

	q := r.URL.Query()

	// Build PromQL query with optional label filters.
	// The agent emits agent_tokens_used_total with labels
	// {agent, repo, org, event_type, issue_id, model, token_type}.
	// The JSON API still exposes "repository"/"issueNumber" so the frontend
	// doesn't have to change.
	filters := ""
	var parts []string
	if v := q.Get("agent"); v != "" {
		parts = append(parts, fmt.Sprintf(`agent="%s"`, prometheus.SanitizeLabel(v)))
	}
	if v := q.Get("repository"); v != "" {
		parts = append(parts, fmt.Sprintf(`repo="%s"`, prometheus.SanitizeLabel(v)))
	}
	if v := q.Get("issueNumber"); v != "" {
		parts = append(parts, fmt.Sprintf(`issue_id="%s"`, prometheus.SanitizeLabel(v)))
	}
	if len(parts) > 0 {
		filters = "{" + joinFilters(parts) + "}"
	}

	promql := fmt.Sprintf(
		`sum by (agent, repo, issue_id, model, token_type) (agent_tokens_used_total%s)`,
		filters,
	)

	result, err := s.prom.Query(r.Context(), promql)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query metrics: "+err.Error())
		return
	}

	// Merge input/output rows into TokenEntry objects.
	// Key: agent|repo|issue_id|model
	type entryKey struct {
		Agent       string
		Repository  string
		IssueNumber string
		Model       string
	}
	merged := map[entryKey]*TokenEntry{}

	for _, m := range result.Data {
		key := entryKey{
			Agent:       m.Labels["agent"],
			Repository:  m.Labels["repo"],
			IssueNumber: m.Labels["issue_id"],
			Model:       m.Labels["model"],
		}
		entry, ok := merged[key]
		if !ok {
			entry = &TokenEntry{
				Agent:       key.Agent,
				Repository:  key.Repository,
				IssueNumber: key.IssueNumber,
				Model:       key.Model,
			}
			merged[key] = entry
		}
		switch m.Labels["token_type"] {
		case "input":
			entry.InputTokens = m.Value
		case "output":
			entry.OutputTokens = m.Value
		}
	}

	tokens := make([]TokenEntry, 0, len(merged))
	var totals TokenTotals
	for _, entry := range merged {
		tokens = append(tokens, *entry)
		totals.InputTokens += entry.InputTokens
		totals.OutputTokens += entry.OutputTokens
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tokens": tokens,
		"total":  totals,
	})
}

func joinFilters(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}

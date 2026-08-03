package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
)

// Limits for the logs endpoint. Default mirrors the example in the issue;
// max is the cap requested by the spec to keep responses bounded.
const (
	logsDefaultLimit = 500
	logsMaxLimit     = 1000
)

// supported range tokens for /api/observability/logs.
var logsSupportedRanges = map[string]time.Duration{
	"1h":  1 * time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
}

// LogLine is one entry in the observability logs response.
type LogLine struct {
	Timestamp string            `json:"timestamp"`
	Message   string            `json:"message"`
	AgentName string            `json:"agentName,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// LogsResponse is the JSON envelope returned by /api/observability/logs.
// Mirrors the shape of /api/v1/errors so frontend components can share helpers.
type LogsResponse struct {
	Logs  []LogLine `json:"logs"`
	Total int       `json:"total"`
	Query string    `json:"query"`
}

// handleObservabilityLogs serves GET /api/observability/logs.
//
// Serves structured log entries from the hub's own task_logs table,
// populated by agents via NATS hub.task.* events. No external log
// backend required.
//
// Query parameters:
//
//	?app=<agent>        Filter by agent name.
//	?range=<1h|6h|24h>  Time window (default 1h).
//	?since=<duration>   Go duration string, overrides range.
//	?limit=N            Max entries (default 500, max 1000).
//
// Returns 503 if the task log store is not configured.
func (s *Server) handleObservabilityLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()

	limit, err := parseLogsLimit(q.Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	duration, err := parseLogsDuration(q.Get("since"), q.Get("range"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.taskLogs == nil {
		writeError(w, http.StatusServiceUnavailable, "log backend not configured")
		return
	}

	opts := tasklogs.ListOptions{
		AgentName: q.Get("app"),
		Since:     time.Now().Add(-duration),
		Limit:     limit,
	}

	entries, err := s.taskLogs.List(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to query logs: "+err.Error())
		return
	}

	nameMap := s.agentNameMap(r.Context())
	logs := make([]LogLine, 0, len(entries))
	for _, e := range entries {
		line := LogLine{
			Timestamp: e.CreatedAt.UTC().Format(time.RFC3339Nano),
			Message:   e.Message,
			Labels: map[string]string{
				"agent": e.AgentName,
				"level": e.Level,
			},
		}
		if e.CorrelationID != "" {
			line.Labels["correlation_id"] = e.CorrelationID
		}
		if e.InvocationID != "" {
			line.Labels["invocation_id"] = e.InvocationID
		}
		if name, ok := nameMap[e.AgentName]; ok && name != "" {
			line.AgentName = name
		}
		logs = append(logs, line)
	}

	writeJSON(w, http.StatusOK, LogsResponse{
		Logs:  logs,
		Total: len(logs),
		Query: "task_logs",
	})
}

// parseLogsLimit parses and clamps the limit query parameter.
// Empty -> default. Negative or non-numeric -> 400. Above max -> capped at max.
func parseLogsLimit(raw string) (int, error) {
	if raw == "" {
		return logsDefaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid limit %q: must be a positive integer", raw)
	}
	if n > logsMaxLimit {
		n = logsMaxLimit
	}
	return n, nil
}

// parseLogsRange validates the range query param and returns its duration.
// Empty defaults to 1h to match the most common dashboard refresh.
func parseLogsRange(raw string) (time.Duration, error) {
	if raw == "" {
		return logsSupportedRanges["1h"], nil
	}
	d, ok := logsSupportedRanges[raw]
	if !ok {
		return 0, fmt.Errorf("invalid range %q: must be one of 1h, 6h, 24h", raw)
	}
	return d, nil
}

// parseLogsDuration resolves the effective query window. The ?since=<duration>
// parameter accepts any Go duration string (e.g. "30m", "2h"). When absent,
// it falls back to the legacy ?range= token. Both empty → default 1h.
func parseLogsDuration(since, rangeToken string) (time.Duration, error) {
	if since != "" {
		d, err := time.ParseDuration(since)
		if err != nil || d <= 0 {
			return 0, fmt.Errorf("invalid since %q: must be a positive Go duration (e.g. 30m, 2h)", since)
		}
		return d, nil
	}
	return parseLogsRange(rangeToken)
}

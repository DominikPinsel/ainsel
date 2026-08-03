package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/tasklogs"
	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Stats is the top-level stats response.
type Stats struct {
	Agents       ResourceStats `json:"agents"`
	Connectors   ResourceStats `json:"connectors"`
	Triggers     ResourceStats `json:"triggers"`
	CronTriggers ResourceStats `json:"cronTriggers"`
	Errors       ErrorStats    `json:"errors"`
	Tokens       TokenTotals   `json:"tokens"`
}

// ResourceStats holds total and healthy counts for a resource type.
type ResourceStats struct {
	Total   int `json:"total"`
	Healthy int `json:"healthy"`
}

// ErrorStats holds error counts.
type ErrorStats struct {
	LastHour int `json:"lastHour"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getStats(r.Context(), w)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) getStats(ctx context.Context, w http.ResponseWriter) {
	stats := s.GetStats(ctx)
	writeJSON(w, http.StatusOK, stats)
}

// GetStats returns the current stats. Exported for use by the WebSocket broadcaster.
func (s *Server) GetStats(ctx context.Context) Stats {
	var stats Stats

	// Agents
	var agentList agentv1alpha1.AgentList
	if err := s.client.List(ctx, &agentList, client.InNamespace(s.ns)); err == nil {
		stats.Agents.Total = len(agentList.Items)
		for _, a := range agentList.Items {
			if isReady(a.Status.Conditions) {
				stats.Agents.Healthy++
			}
		}
	}

	// Connectors (WebhookConnector)
	var connectorList agentv1alpha1.WebhookConnectorList
	if err := s.client.List(ctx, &connectorList, client.InNamespace(s.ns)); err == nil {
		stats.Connectors.Total = len(connectorList.Items)
		for _, c := range connectorList.Items {
			if isReady(c.Status.Conditions) {
				stats.Connectors.Healthy++
			}
		}
	}

	// Triggers (from database)
	if s.triggerStore != nil {
		allTriggers, err := s.triggerStore.ListTriggers(ctx, "", "")
		if err == nil {
			stats.Triggers.Total = len(allTriggers)
			for _, t := range allTriggers {
				if t.AgentValid && t.ConnectorValid {
					stats.Triggers.Healthy++
				}
			}
		}
	}

	// CronTriggers (from database)
	if s.triggerStore != nil {
		allCronTriggers, err := s.triggerStore.ListCronTriggers(ctx, "")
		if err == nil {
			stats.CronTriggers.Total = len(allCronTriggers)
			for _, ct := range allCronTriggers {
				if ct.AgentValid && ct.ScheduleValid {
					stats.CronTriggers.Healthy++
				}
			}
		}
	}

	// Errors in the last hour — query task_logs
	if s.taskLogs != nil {
		count, err := s.taskLogs.CountByLevelSince(ctx, tasklogs.LevelError, time.Now().Add(-time.Hour))
		if err == nil {
			stats.Errors.LastHour = count
		} else {
			slog.Warn("task log error count failed", "error", err)
		}
	}

	// Token totals — query Prometheus
	if s.prom != nil {
		result, err := s.prom.Query(ctx, `sum by (token_type) (agent_tokens_used_total)`)
		if err == nil {
			for _, m := range result.Data {
				switch m.Labels["token_type"] {
				case "input":
					stats.Tokens.InputTokens = m.Value
				case "output":
					stats.Tokens.OutputTokens = m.Value
				}
			}
		} else {
			slog.Warn("prometheus token query failed", "error", err)
		}
	}

	return stats
}

// isReady returns true if the "Ready" condition is "True".
func isReady(conditions []metav1.Condition) bool {
	for _, c := range conditions {
		if c.Type == "Ready" && c.Status == "True" {
			return true
		}
	}
	return false
}

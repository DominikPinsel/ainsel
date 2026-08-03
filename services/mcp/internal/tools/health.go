package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type HealthTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewHealthTools(hubURL string) *HealthTools {
	return &HealthTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *HealthTools) GetPlatformHealthTool() mcp.Tool {
	return mcp.NewTool("get_platform_health",
		mcp.WithDescription("Overview of the ainsel namespace. By default returns a compact summary (status counts + problem pods only). Pass full=true for the complete pod detail."),
		mcp.WithBoolean("full", mcp.Description("Return the full unmodified health response (default: false, compact summary)")),
	)
}

func (h *HealthTools) GetPlatformHealth(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	body, err := hubGet(ctx, h.HTTPClient, h.HubURL, "/api/v1/platform/health")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get platform health: %v", err)), nil
	}

	args, _ := req.Params.Arguments.(map[string]any)
	full, _ := args["full"].(bool)
	if full {
		return mcp.NewToolResultText(string(body)), nil
	}

	// Compact summary: extract status counts and problem pods only.
	summary := compactHealthSummary(body)
	out, err := json.Marshal(summary)
	if err != nil {
		// Fallback to raw body on transform failure.
		return mcp.NewToolResultText(string(body)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

// compactHealthSummary transforms the full health response into a compact
// summary. If the response shape is unexpected, it returns a minimal object
// with the raw body embedded.
func compactHealthSummary(body []byte) map[string]any {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return map[string]any{"raw": string(body)}
	}

	statusCounts := map[string]int{}
	var problems []any
	totalPods := 0

	// The hub returns a "pods" array with per-pod status info.
	if pods, ok := raw["pods"].([]any); ok {
		totalPods = len(pods)
		for _, p := range pods {
			pod, ok := p.(map[string]any)
			if !ok {
				continue
			}
			phase, _ := pod["phase"].(string)
			if phase == "" {
				phase = "Unknown"
			}
			statusCounts[phase]++

			// A pod is a "problem" if it's not Running or not ready.
			isProblem := phase != "Running"
			if conditions, ok := pod["conditions"].([]any); ok {
				for _, c := range conditions {
					cond, ok := c.(map[string]any)
					if !ok {
						continue
					}
					if cond["type"] == "Ready" && cond["status"] != "True" {
						isProblem = true
					}
				}
			}
			if isProblem {
				problems = append(problems, pod)
			}
		}
	}

	summary := map[string]any{
		"statusCounts": statusCounts,
		"totalPods":    totalPods,
		"problems":     problems,
	}
	if len(problems) == 0 {
		summary["problems"] = []any{}
		summary["hint"] = "all pods healthy"
	} else {
		summary["hint"] = fmt.Sprintf("%d problem pod(s) — pass full=true for complete detail", len(problems))
	}

	// Preserve any top-level status field the hub may include.
	if status, ok := raw["status"]; ok {
		summary["status"] = status
	}

	return summary
}

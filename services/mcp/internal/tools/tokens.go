package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// UsageTools wraps the hub's cost (/api/v1/tokens) and dashboard summary
// (/api/v1/stats) endpoints. Both are backed by live Prometheus / Loki
// queries on the hub side; neither is a stub at the time of writing.
//
// /api/v1/tokens supports agent, repository, issueNumber filters (no
// since/until); we accept those plus pass-through since/until for forward
// compatibility — the hub silently drops unknown params.
type UsageTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewUsageTools(hubURL string) *UsageTools {
	return &UsageTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *UsageTools) GetTokenUsageTool() mcp.Tool {
	return mcp.NewTool("get_token_usage",
		mcp.WithDescription("Hub-aggregated cost view: token counts (input/output) per agent, repository, issue, and model. Results capped at 50 rows."),
		mcp.WithString("agent", mcp.Description("Filter by agent name")),
		mcp.WithString("repository", mcp.Description("Filter by repository")),
		mcp.WithString("issueNumber", mcp.Description("Filter by issue number")),
		mcp.WithString("since", mcp.Description("Earliest timestamp (RFC 3339); forwarded to the hub")),
		mcp.WithString("until", mcp.Description("Latest timestamp (RFC 3339); forwarded to the hub")),
	)
}

func (t *UsageTools) GetStatsTool() mcp.Tool {
	return mcp.NewTool("get_stats",
		mcp.WithDescription("Dashboard summary: counts of agents, triggers, connectors (with healthy subset), error count in the last hour, and aggregate token totals."),
	)
}

func (t *UsageTools) GetTokenUsage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	for _, k := range []string{"agent", "repository", "issueNumber", "since", "until"} {
		if v, ok := args[k].(string); ok && v != "" {
			q.Set(k, v)
		}
	}
	path := "/api/v1/tokens"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get token usage: %v", err)), nil
	}
	// The hub's /api/v1/tokens endpoint does not support a limit param, so
	// cap locally at 50 rows to keep the response compact for MCP clients.
	body = capBareArray(body, 50, "more token usage rows available — narrow filters (agent, repository, since/until) to refine")
	return mcp.NewToolResultText(string(body)), nil
}

func (t *UsageTools) GetStats(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, "/api/v1/stats")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get stats: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

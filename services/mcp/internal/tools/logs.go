package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type LogTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewLogTools(hubURL string) *LogTools {
	return &LogTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (l *LogTools) GetAgentLogsTool() mcp.Tool {
	return mcp.NewTool("get_agent_logs",
		mcp.WithDescription("Get recent logs for a specific agent"),
		mcp.WithString("agent", mcp.Required(), mcp.Description("Agent name")),
		mcp.WithNumber("limit", mcp.Description("Max log lines to return (default: 100)")),
		mcp.WithString("since", mcp.Description("Time range, e.g. '1h', '30m', '24h' (default: 1h)")),
		mcp.WithString("search", mcp.Description("Optional text to filter log lines")),
	)
}

func (l *LogTools) QueryLogsTool() mcp.Tool {
	return mcp.NewTool("query_logs",
		mcp.WithDescription("Run a freeform log query across the ainsel namespace"),
		mcp.WithString("query", mcp.Required(), mcp.Description("LogQL query, e.g. {namespace=\"ainsel\"} |= \"error\"")),
		mcp.WithNumber("limit", mcp.Description("Max log lines (default: 100)")),
		mcp.WithString("since", mcp.Description("Time range (default: 1h)")),
	)
}

func (l *LogTools) GetAgentLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	agent, ok := args["agent"].(string)
	if !ok || agent == "" {
		return mcp.NewToolResultError("agent name is required"), nil
	}

	// Build LogQL filter; the hub's namespace is baked into its stream selector.
	logql := fmt.Sprintf(`{app=%q}`, agent)
	if search, ok := args["search"].(string); ok && search != "" {
		logql += fmt.Sprintf(` |= %q`, search)
	}

	return l.queryHub(ctx, logql, args)
}

func (l *LogTools) QueryLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}
	return l.queryHub(ctx, query, args)
}

func (l *LogTools) queryHub(ctx context.Context, logql string, args map[string]any) (*mcp.CallToolResult, error) {
	since := "1h"
	if s, ok := args["since"].(string); ok && s != "" {
		since = s
	}
	limit := 100
	if lim, ok := args["limit"].(float64); ok && lim > 0 {
		limit = int(lim)
	}

	params := url.Values{
		"query": {logql},
		"since": {since},
		"limit": {fmt.Sprintf("%d", limit)},
	}
	body, err := hubGet(ctx, l.HTTPClient, l.HubURL, "/api/v1/observability/logs?"+params.Encode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to query logs: %v", err)), nil
	}
	// Compute truncation from the actual number of returned log lines
	// rather than hardcoding true — if fewer lines came back than the
	// limit, the result is not truncated.
	additions := map[string]any{}
	returned := countLogLines(body)
	if returned >= 0 {
		additions["truncated"] = returned >= limit
		additions["returned"] = returned
		if returned >= limit {
			additions["hint"] = fmt.Sprintf("showing %d lines (limit=%d) — pass limit=N for more, or narrow since/search", returned, limit)
		}
	} else {
		// Could not determine line count; include a generic hint.
		additions["hint"] = fmt.Sprintf("showing up to %d lines — pass limit=N for more, or narrow since/search", limit)
	}
	body = annotateJSON(body, additions)
	return mcp.NewToolResultText(string(body)), nil
}

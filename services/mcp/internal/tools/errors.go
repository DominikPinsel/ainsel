package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// ErrorTools wraps the hub's /api/v1/errors endpoint, which aggregates
// recent error entries from the hub's task_logs database. The hub
// understands the limit, severity, source, and since query parameters;
// agent / until are forwarded for forward compatibility (the hub silently
// drops unknown params).
type ErrorTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewErrorTools(hubURL string) *ErrorTools {
	return &ErrorTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *ErrorTools) GetRecentErrorsTool() mcp.Tool {
	return mcp.NewTool("get_recent_errors",
		mcp.WithDescription("Cross-agent error summary from the hub. Supports optional filters: agent, since (RFC 3339), limit, severity, source. Default limit: 50."),
		mcp.WithString("agent", mcp.Description("Filter by agent name")),
		mcp.WithString("since", mcp.Description("Earliest timestamp (RFC 3339); defaults to 24 hours ago on the hub")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of errors to return (default: 50)")),
		mcp.WithString("severity", mcp.Description("Filter by severity (e.g. error, warning)")),
		mcp.WithString("source", mcp.Description("Filter by source component (e.g. agent, hub, gateway)")),
	)
}

func (t *ErrorTools) GetRecentErrors(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	if v, ok := args["agent"].(string); ok && v != "" {
		q.Set("agent", v)
	}
	if v, ok := args["since"].(string); ok && v != "" {
		q.Set("since", v)
	}
	if v, ok := args["severity"].(string); ok && v != "" {
		q.Set("severity", v)
	}
	if v, ok := args["source"].(string); ok && v != "" {
		q.Set("source", v)
	}
	limit := defaultArgInt(args, "limit", 50)
	q.Set("limit", strconv.Itoa(limit))
	path := "/api/v1/errors?" + q.Encode()
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get recent errors: %v", err)), nil
	}
	body = capBareArray(body, limit, "more errors available — pass limit=N to fetch more")
	return mcp.NewToolResultText(string(body)), nil
}

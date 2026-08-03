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

// InvocationTools wraps the hub's /api/v1/invocations endpoints. The hub
// currently supports the agent, status, since, and page/pageSize query
// parameters; trigger and until are forwarded for forward compatibility (the
// hub silently ignores unknown query strings).
type InvocationTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewInvocationTools(hubURL string) *InvocationTools {
	return &InvocationTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (i *InvocationTools) ListInvocationsTool() mcp.Tool {
	return mcp.NewTool("list_invocations",
		mcp.WithDescription("List recent agent invocations. Supports optional filters: agent, trigger, since (RFC 3339), until (RFC 3339), limit. Default limit: 50."),
		mcp.WithString("agent", mcp.Description("Filter by agent name")),
		mcp.WithString("trigger", mcp.Description("Filter by trigger name")),
		mcp.WithString("since", mcp.Description("Earliest timestamp (RFC 3339)")),
		mcp.WithString("until", mcp.Description("Latest timestamp (RFC 3339)")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of invocations to return (default: 50)")),
	)
}

func (i *InvocationTools) GetInvocationTool() mcp.Tool {
	return mcp.NewTool("get_invocation",
		mcp.WithDescription("Get detailed information about one invocation by ID, including prompt, model output, tool calls, token usage, and errors."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Invocation ID")),
	)
}

func (i *InvocationTools) ListInvocations(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	if v, ok := args["agent"].(string); ok && v != "" {
		q.Set("agent", v)
	}
	if v, ok := args["trigger"].(string); ok && v != "" {
		q.Set("trigger", v)
	}
	if v, ok := args["since"].(string); ok && v != "" {
		q.Set("since", v)
	}
	if v, ok := args["until"].(string); ok && v != "" {
		q.Set("until", v)
	}
	limit := defaultArgInt(args, "limit", 50)
	q.Set("limit", strconv.Itoa(limit))
	path := "/api/v1/invocations"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	body, err := hubGet(ctx, i.HTTPClient, i.HubURL, path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list invocations: %v", err)), nil
	}
	body = annotateListWithTotal(body, "invocations", "more invocations available — pass limit=N to fetch more")
	return mcp.NewToolResultText(string(body)), nil
}

func (i *InvocationTools) GetInvocation(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	id, _ := args["id"].(string)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}
	body, err := hubGet(ctx, i.HTTPClient, i.HubURL, "/api/v1/invocations/"+id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get invocation %s: %v", id, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

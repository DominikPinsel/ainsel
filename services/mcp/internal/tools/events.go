package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type EventTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewEventTools(hubURL string) *EventTools {
	return &EventTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *EventTools) GetStreamInfoTool() mcp.Tool {
	return mcp.NewTool("get_stream_info",
		mcp.WithDescription("Get event queue stats (pending, claimed, completed, failed tasks)"),
		mcp.WithString("stream", mcp.Description("Stream name (default: EVENTS). Options: EVENTS, AGENTS")),
	)
}

func (e *EventTools) ListRecentEventsTool() mcp.Tool {
	return mcp.NewTool("list_recent_events",
		mcp.WithDescription("List recent events. The filter parameter matches a subject pattern of the form '<connector>.<eventType>', where the event type is taken from the webhook event-type header (e.g. push, issue_comment). Supports '*' for any single token and a trailing '>' for any remainder."),
		mcp.WithString("stream", mcp.Description("Stream name (default: EVENTS)")),
		mcp.WithNumber("count", mcp.Description("Number of recent events to retrieve (default: 20, max: 100)")),
		mcp.WithString("filter", mcp.Description("Subject filter pattern, e.g. 'forgejo.push', 'forgejo.*' (all forgejo events), '*.push' (push events from any connector)")),
	)
}

func (e *EventTools) GetStreamInfo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stream := "EVENTS"
	if args, ok := req.Params.Arguments.(map[string]any); ok {
		if s, ok := args["stream"].(string); ok && s != "" {
			stream = s
		}
	}
	params := url.Values{"stream": {stream}}
	body, err := hubGet(ctx, e.HTTPClient, e.HubURL, "/api/v1/queue/info?"+params.Encode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get stream info: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (e *EventTools) ListRecentEvents(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stream := "EVENTS"
	count := 20
	filter := ""
	if args, ok := req.Params.Arguments.(map[string]any); ok {
		if s, ok := args["stream"].(string); ok && s != "" {
			stream = s
		}
		if c, ok := args["count"].(float64); ok && c > 0 {
			count = int(c)
			if count > 100 {
				count = 100
			}
		}
		if f, ok := args["filter"].(string); ok {
			filter = f
		}
	}
	params := url.Values{
		"stream": {stream},
		"count":  {fmt.Sprintf("%d", count)},
	}
	if filter != "" {
		params.Set("filter", filter)
	}
	body, err := hubGet(ctx, e.HTTPClient, e.HubURL, "/api/v1/queue/recent?"+params.Encode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get recent events: %v", err)), nil
	}
	// Determine truncation from the actual number of returned events,
	// not the requested count — the requested count is an upper bound,
	// not evidence that more data exists.
	returned := countEvents(body)
	truncated := returned >= count && count > 0
	additions := map[string]any{
		"truncated": truncated,
		"returned":  returned,
	}
	if truncated {
		additions["hint"] = fmt.Sprintf("showing %d events (requested %d) — there may be more; pass count=N up to 100", returned, count)
	}
	body = annotateJSON(body, additions)
	return mcp.NewToolResultText(string(body)), nil
}

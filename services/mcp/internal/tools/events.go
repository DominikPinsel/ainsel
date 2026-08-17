package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
		mcp.WithDescription("List recent events. By default each event is reduced to a compact summary (id, connector, event type, action, key entity fields, payload size) to keep responses small. Pass full=true to get the complete raw webhook payload including headers and body. The filter parameter matches a subject pattern of the form '<connector>.<eventType>', where the event type is taken from the webhook event-type header (e.g. push, issue_comment). Supports '*' for any single token and a trailing '>' for any remainder."),
		mcp.WithString("stream", mcp.Description("Stream name (default: EVENTS)")),
		mcp.WithNumber("count", mcp.Description("Number of recent events to retrieve (default: 20, max: 100)")),
		mcp.WithString("filter", mcp.Description("Subject filter pattern, e.g. 'forgejo.push', 'forgejo.*' (all forgejo events), '*.push' (push events from any connector)")),
		mcp.WithBoolean("full", mcp.Description("Return the complete raw event payload (headers, full webhook body). Defaults to false, which returns a compact summary.")),
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
	full := false
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
		if f, ok := args["full"].(bool); ok {
			full = f
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
	// Raw webhook payloads are tens of KB each and flood the agent context.
	// Unless the caller explicitly opts in, reduce every event to a compact
	// summary before returning.
	if !full {
		body = summarizeEvents(body)
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

// Header keys preserved by summarizeEvent. Matching is case-insensitive:
// any key ending in "-event" (the webhook event type, e.g. X-Gitea-Event),
// any key ending in "-delivery" (the delivery id), plus content type and
// request id.
func isMeaningfulHeader(key string) bool {
	lk := strings.ToLower(key)
	switch {
	case strings.HasSuffix(lk, "-event"), strings.HasSuffix(lk, "-delivery"):
		return true
	case lk == "content-type", lk == "x-request-id":
		return true
	}
	return false
}

// summarizeEvents rewrites the hub's bare array of raw events into a compact
// form safe for agent consumption: the duplicate raw body and most headers
// are dropped, and data is replaced by a small summary of well-known fields.
// The original payload size is retained as payloadBytes. If the body is not a
// JSON array it is returned unchanged.
func summarizeEvents(body []byte) []byte {
	var events []map[string]any
	if err := json.Unmarshal(body, &events); err != nil {
		return body
	}
	out := make([]map[string]any, 0, len(events))
	for _, evt := range events {
		out = append(out, summarizeEvent(evt))
	}
	result, err := json.Marshal(out)
	if err != nil {
		return body
	}
	return result
}

// summarizeEvent reduces a single raw event to its compact form.
func summarizeEvent(evt map[string]any) map[string]any {
	summary := map[string]any{}
	for _, key := range []string{"id", "connector", "received_at"} {
		if v, ok := evt[key]; ok {
			summary[key] = v
		}
	}

	// Keep only headers that identify the event (type, delivery id).
	if headers, ok := evt["headers"].(map[string]any); ok {
		kept := map[string]any{}
		for k, v := range headers {
			if isMeaningfulHeader(k) {
				kept[k] = v
			}
		}
		if len(kept) > 0 {
			summary["headers"] = kept
		}
		if t := canonicalEventType(headers); t != "" {
			summary["type"] = t
		}
	}

	// Replace the full webhook body with a compact summary; record the
	// original size so callers can tell how much was elided.
	dataSize := 0
	var dataSummary map[string]any
	if raw, ok := evt["data"].(string); ok {
		dataSize = len(raw)
		dataSummary = summarizeEventData([]byte(raw))
	} else if raw, ok := evt["data"].(map[string]any); ok {
		if b, err := json.Marshal(raw); err == nil {
			dataSize = len(b)
		}
		dataSummary = summarizeEventMap(raw)
	}
	if dataSummary != nil {
		summary["summary"] = dataSummary
	}
	if dataSize > 0 {
		summary["payloadBytes"] = dataSize
	}
	// raw and the full data are intentionally omitted; pass full=true to
	// list_recent_events to retrieve them.
	return summary
}

// canonicalEventType extracts the event type from webhook headers: any header
// ending in "-Event" (X-Gitea-Event, X-GitHub-Event, ...), falling back to a
// generic "type" header. Mirrors the hub-side derivation.
func canonicalEventType(headers map[string]any) string {
	for k, v := range headers {
		if strings.HasSuffix(k, "-Event") || strings.HasSuffix(k, "-event") {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	if s, ok := headers["type"].(string); ok {
		return s
	}
	return ""
}

// summarizeEventData parses a raw webhook JSON body and extracts a small set
// of well-known fields. Returns nil when the body is empty or not a JSON
// object.
func summarizeEventData(data []byte) map[string]any {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	return summarizeEventMap(obj)
}

// summarizeEventMap extracts well-known identifying fields from a webhook
// payload object: action, ref, repository, sender, issue/PR number+title and
// commit count. Everything else is dropped.
func summarizeEventMap(obj map[string]any) map[string]any {
	summary := map[string]any{}
	if v, ok := obj["action"].(string); ok {
		summary["action"] = v
	}
	if v, ok := obj["ref"].(string); ok {
		summary["ref"] = v
	}
	if repo, ok := obj["repository"].(map[string]any); ok {
		if v, ok := repo["full_name"].(string); ok {
			summary["repository"] = v
		} else if v, ok := repo["name"].(string); ok {
			summary["repository"] = v
		}
	}
	for _, key := range []string{"sender", "user", "actor"} {
		if u, ok := obj[key].(map[string]any); ok {
			if login, ok := u["login"].(string); ok {
				summary["sender"] = login
				break
			}
		}
	}
	for _, key := range []string{"issue", "pull_request"} {
		if item, ok := obj[key].(map[string]any); ok {
			entry := map[string]any{}
			if n, ok := item["number"].(float64); ok {
				entry["number"] = int(n)
			}
			if title, ok := item["title"].(string); ok {
				entry["title"] = title
			}
			if len(entry) > 0 {
				summary[key] = entry
			}
		}
	}
	if commits, ok := obj["commits"].([]any); ok {
		summary["commitCount"] = len(commits)
	}
	if len(summary) == 0 {
		return nil
	}
	return summary
}

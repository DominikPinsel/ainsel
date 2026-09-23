package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// ChannelTools wraps the hub's channel registry. A channel is a named stream
// events are born in — one per connector, one per agent inbox, plus the custom
// grouping channels a user creates — so these tools answer "what flows where",
// which previously required joining connectors, agents and triggers by hand.
type ChannelTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewChannelTools(hubURL string) *ChannelTools {
	return &ChannelTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// channelPageSize caps the lookup pass that resolves a name to a channel id.
const channelPageSize = 200

// channelRecord is the slice of a channel response the resolver needs. The
// hub's own types are not imported here: the MCP service speaks HTTP to the
// hub, so a response-shape change shows up as a compile-free missing field —
// which is why the fields read here are named explicitly.
type channelRecord struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	EntityRef   string `json:"entityRef"`
	Description string `json:"description"`
}

func (c *ChannelTools) ListChannelsTool() mcp.Tool {
	return mcp.NewTool("list_channels",
		mcp.WithDescription("List channels: the event streams the platform knows. One per connector (where its events are born), one per agent (its inbox), plus custom grouping channels. Includes traffic counts over the window. Default page size: 50."),
		mcp.WithNumber("page", mcp.Description("Page number (1-based, default: 1)")),
		mcp.WithNumber("pageSize", mcp.Description("Page size (default: 50)")),
		mcp.WithString("kind", mcp.Description("Filter by channel kind: connector, agent or custom")),
		mcp.WithString("since", mcp.Description("Start of the rate window (RFC 3339). Defaults to 24 hours ago.")),
	)
}

func (c *ChannelTools) GetChannelTool() mcp.Tool {
	return mcp.NewTool("get_channel",
		mcp.WithDescription("Get one channel: its kind, counts, and every subscription that touches it in either direction. Accepts a channel id, a connector/agent name, or \"connector:forgejo\" style."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Channel id, or the connector/agent/custom channel name")),
	)
}

func (c *ChannelTools) ListChannelSubscriptionsTool() mcp.Tool {
	return mcp.NewTool("list_channel_subscriptions",
		mcp.WithDescription("List all channel-to-channel subscriptions in one call: the transfers owned by triggers (connector → agent inbox) and the bridges owned by custom channels. Use this instead of joining connectors, agents and triggers to reconstruct who receives what."),
	)
}

func (c *ChannelTools) GetChannelEventsTool() mcp.Tool {
	return mcp.NewTool("get_channel_events",
		mcp.WithDescription("A channel's event timeline: events born in it, plus events transferred into it. For an agent inbox this is what the agent actually received, wherever it came from."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Channel id or name")),
		mcp.WithNumber("limit", mcp.Description("Maximum events (default: 50, max: 200)")),
		mcp.WithString("since", mcp.Description("Earliest timestamp (RFC 3339)")),
		mcp.WithString("status", mcp.Description("Filter by derived status: matched, unmatched or error")),
		mcp.WithString("subject", mcp.Description("Event subject pattern \"<connector>.<eventType>\", e.g. forgejo.issue_open or cron.*")),
	)
}

func (c *ChannelTools) CreateChannelTool() mcp.Tool {
	return mcp.NewTool("create_channel",
		mcp.WithDescription("Create a custom grouping channel. Connector and agent channels are provisioned by the platform and cannot be created here; a custom channel exists to bundle subscriptions under one name and pass them to other agents."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Display name for the grouping channel")),
		mcp.WithString("description", mcp.Description("One line of intent for the channel")),
		mcp.WithString("groupId", mcp.Description("Group to assign the channel to; must be a group the caller has write access to. Required on hubs with access control enabled.")),
	)
}

func (c *ChannelTools) UpdateChannelTool() mcp.Tool {
	return mcp.NewTool("update_channel",
		mcp.WithDescription("Rename a custom channel or change its description. Provisioned channels carry their entity's name and cannot be renamed here."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Channel id or current name")),
		mcp.WithString("displayName", mcp.Description("New display name")),
		mcp.WithString("description", mcp.Description("New description")),
	)
}

func (c *ChannelTools) DeleteChannelTool() mcp.Tool {
	return mcp.NewTool("delete_channel",
		mcp.WithDescription("Delete a custom channel. Refused while subscriptions are still attached to it, so a stream cannot disappear underneath them."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Channel id or name")),
	)
}

func (c *ChannelTools) AttachChannelBridgeTool() mcp.Tool {
	return mcp.NewTool("attach_channel_bridge",
		mcp.WithDescription("Transfer a channel's events into another channel: attach a connector's or agent's stream to a custom grouping channel, or attach a grouping channel to an agent inbox so that agent receives everything the group collects. At least one end must be a custom channel — a plain connector → agent pairing is a trigger, not a bridge."),
		mcp.WithString("from", mcp.Required(), mcp.Description("Source channel id or name")),
		mcp.WithString("to", mcp.Required(), mcp.Description("Target channel id or name")),
		mcp.WithString("bridgeName", mcp.Description("Optional label for the subscription")),
	)
}

func (c *ChannelTools) DetachChannelBridgeTool() mcp.Tool {
	return mcp.NewTool("detach_channel_bridge",
		mcp.WithDescription("Remove a bridge between two channels."),
		mcp.WithString("from", mcp.Required(), mcp.Description("Source channel id or name")),
		mcp.WithString("to", mcp.Required(), mcp.Description("Target channel id or name")),
	)
}

// ---------------------------------------------------------------------------
// Tool implementations
// ---------------------------------------------------------------------------

func (c *ChannelTools) ListChannels(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	q.Set("page", strconv.Itoa(defaultArgInt(args, "page", 1)))
	q.Set("pageSize", strconv.Itoa(defaultArgInt(args, "pageSize", 50)))
	if kind, _ := args["kind"].(string); kind != "" {
		q.Set("kind", kind)
	}
	if since, _ := args["since"].(string); since != "" {
		q.Set("since", since)
	}
	body, err := hubGet(ctx, c.HTTPClient, c.HubURL, "/api/v1/channels?"+q.Encode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list channels: %v", err)), nil
	}
	body = annotatePageMeta(body, "more channels available — pass page=N to fetch the next page")
	return mcp.NewToolResultText(string(body)), nil
}

func (c *ChannelTools) GetChannel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	id, err := c.resolveChannel(ctx, name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	body, err := hubGet(ctx, c.HTTPClient, c.HubURL, "/api/v1/channels/"+url.PathEscape(id))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get channel %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (c *ChannelTools) ListChannelSubscriptions(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	body, err := hubGet(ctx, c.HTTPClient, c.HubURL, "/api/v1/channel-subscriptions")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list channel subscriptions: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (c *ChannelTools) GetChannelEvents(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	id, err := c.resolveChannel(ctx, name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(defaultArgInt(args, "limit", 50)))
	for _, key := range []string{"since", "status", "subject"} {
		if v, _ := args[key].(string); v != "" {
			q.Set(key, v)
		}
	}
	body, err := hubGet(ctx, c.HTTPClient, c.HubURL, "/api/v1/channels/"+url.PathEscape(id)+"/events?"+q.Encode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get events for channel %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (c *ChannelTools) CreateChannel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	if strings.TrimSpace(name) == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	payload := map[string]any{"name": name}
	if desc, _ := args["description"].(string); desc != "" {
		payload["description"] = desc
	}
	if groupID, _ := args["groupId"].(string); groupID != "" {
		payload["groupId"] = groupID
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal request: %v", err)), nil
	}
	body, err := hubPost(ctx, c.HTTPClient, c.HubURL, "/api/v1/channels", bytes.NewReader(data))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create channel: %v", appendGroupIDHint(err))), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (c *ChannelTools) UpdateChannel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	id, err := c.resolveChannel(ctx, name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	payload := map[string]any{}
	if v, ok := args["displayName"].(string); ok && v != "" {
		payload["name"] = v
	}
	if v, ok := args["description"].(string); ok {
		payload["description"] = v
	}
	if len(payload) == 0 {
		return mcp.NewToolResultError("nothing to update — pass displayName or description"), nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal request: %v", err)), nil
	}
	body, err := hubPut(ctx, c.HTTPClient, c.HubURL, "/api/v1/channels/"+url.PathEscape(id), bytes.NewReader(data))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to update channel %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (c *ChannelTools) DeleteChannel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	id, err := c.resolveChannel(ctx, name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if _, err := hubDelete(ctx, c.HTTPClient, c.HubURL, "/api/v1/channels/"+url.PathEscape(id)); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to delete channel %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("channel %s deleted", id)), nil
}

func (c *ChannelTools) AttachChannelBridge(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	fromName, _ := args["from"].(string)
	toName, _ := args["to"].(string)
	if fromName == "" || toName == "" {
		return mcp.NewToolResultError("from and to are required"), nil
	}
	fromID, err := c.resolveChannel(ctx, fromName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	toID, err := c.resolveChannel(ctx, toName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	payload := map[string]any{"to": toID}
	if v, _ := args["bridgeName"].(string); v != "" {
		payload["name"] = v
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal request: %v", err)), nil
	}
	body, err := hubPost(ctx, c.HTTPClient, c.HubURL, "/api/v1/channels/"+url.PathEscape(fromID)+"/bridges", bytes.NewReader(data))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to attach bridge %s → %s: %v", fromName, toName, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (c *ChannelTools) DetachChannelBridge(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	fromName, _ := args["from"].(string)
	toName, _ := args["to"].(string)
	if fromName == "" || toName == "" {
		return mcp.NewToolResultError("from and to are required"), nil
	}
	fromID, err := c.resolveChannel(ctx, fromName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	toID, err := c.resolveChannel(ctx, toName)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	bridgeID, err := c.findBridge(ctx, fromID, toID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if _, err := hubDelete(ctx, c.HTTPClient, c.HubURL,
		"/api/v1/channels/"+url.PathEscape(fromID)+"/bridges/"+url.PathEscape(bridgeID)); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to detach bridge %s → %s: %v", fromName, toName, err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("bridge %s detached", bridgeID)), nil
}

// ---------------------------------------------------------------------------
// Name resolution
// ---------------------------------------------------------------------------

// resolveChannel maps a user-supplied name onto a channel id. Channel ids are
// opaque, so an agent will more often say "forgejo" than "ch-9f3c…"; the same
// label can name a connector and an agent though, which is why a bare name that
// matches more than one channel is refused rather than guessed.
func (c *ChannelTools) resolveChannel(ctx context.Context, name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if strings.HasPrefix(trimmed, "ch-") {
		return trimmed, nil
	}

	body, err := hubGet(ctx, c.HTTPClient, c.HubURL,
		"/api/v1/channels?pageSize="+strconv.Itoa(channelPageSize))
	if err != nil {
		return "", fmt.Errorf("failed to look up channel %s: %v", name, err)
	}
	// The hub's list view embeds the channel record, so its fields are promoted
	// onto each item — decoding channelRecord directly is the whole shape needed.
	var page struct {
		Items []channelRecord `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return "", fmt.Errorf("failed to parse channel list: %v", err)
	}

	var matches []channelRecord
	for _, rec := range page.Items {
		if channelMatches(rec, trimmed) {
			matches = append(matches, rec)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0].ID, nil
	case 0:
		return "", fmt.Errorf("no channel named %q — list_channels shows the available ones", name)
	default:
		parts := make([]string, 0, len(matches))
		for _, m := range matches {
			parts = append(parts, fmt.Sprintf("%s:%s (%s)", m.Kind, m.EntityRef, m.ID))
		}
		sort.Strings(parts)
		return "", fmt.Errorf("channel %q is ambiguous: %s — retry with one of those", name, strings.Join(parts, ", "))
	}
}

// channelMatches accepts a name, an entity ref, or the qualified
// "kind:ref" / "kind:name" forms, all case-insensitively.
func channelMatches(rec channelRecord, want string) bool {
	if rec.ID == "" {
		return false
	}
	if strings.EqualFold(rec.Name, want) || strings.EqualFold(rec.EntityRef, want) {
		return true
	}
	// A qualified name always resolves unambiguously, which is how a caller
	// reaches the connector stream and the agent inbox that share a label.
	return strings.EqualFold(rec.Kind+":"+rec.EntityRef, want) ||
		strings.EqualFold(rec.Kind+":"+rec.Name, want)
}

// findBridge locates the bridge id between two channels, by reading the source
// channel's outgoing subscriptions. The hub keys a detach by bridge id, which is
// not something a caller would know.
func (c *ChannelTools) findBridge(ctx context.Context, fromID, toID string) (string, error) {
	body, err := hubGet(ctx, c.HTTPClient, c.HubURL, "/api/v1/channels/"+url.PathEscape(fromID))
	if err != nil {
		return "", fmt.Errorf("failed to look up bridges on channel %s: %v", fromID, err)
	}
	var detail struct {
		Outgoing []struct {
			Source    string `json:"source"`
			RefID     string `json:"refId"`
			ToChannel string `json:"toChannel"`
		} `json:"outgoing"`
	}
	if err := json.Unmarshal(body, &detail); err != nil {
		return "", fmt.Errorf("failed to parse channel detail: %v", err)
	}
	for _, edge := range detail.Outgoing {
		if edge.Source == "bridge" && edge.ToChannel == toID {
			return edge.RefID, nil
		}
	}
	return "", fmt.Errorf("no bridge from %s to %s", fromID, toID)
}

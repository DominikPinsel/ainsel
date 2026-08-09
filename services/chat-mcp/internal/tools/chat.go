// Package tools implements the MCP tools the chat sidecar exposes to the
// agent runtime. Each tool proxies to the hub backend's chat REST API.
//
// The sidecar is stateless: it forwards every request to the hub and returns
// the raw JSON response. The hub owns session storage, message history, and
// WebSocket routing to the frontend.
//
// Tools:
//   - chat.list_sessions   — list chat sessions for this agent
//   - chat.get_history     — fetch message history for a session
//   - chat.send_reply      — send the agent's response to a session
//   - chat.send_status     — send an intermediate status message to a session
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// ChatTools holds the configuration for all chat MCP tools.
type ChatTools struct {
	HubURL           string
	HubInternalToken string
	AgentName        string
	HTTPClient       *http.Client
}

// NewChatTools creates a ChatTools instance configured to proxy to the hub.
func NewChatTools(hubURL, hubInternalToken, agentName string) *ChatTools {
	return &ChatTools{
		HubURL:           strings.TrimRight(hubURL, "/"),
		HubInternalToken: hubInternalToken,
		AgentName:        agentName,
		HTTPClient:       &http.Client{Timeout: 30 * time.Second},
	}
}

// --- Tool definitions ---

// ListSessionsTool returns the MCP tool definition for chat.list_sessions.
func (t *ChatTools) ListSessionsTool() mcp.Tool {
	return mcp.NewTool("list_sessions",
		mcp.WithDescription(
			"List active chat sessions for this agent. Returns sessions that are "+
				"waiting for or currently engaged in conversation with the agent. "+
				"Each session includes its ID, the user who started it, and the "+
				"last message timestamp."),
	)
}

// GetHistoryTool returns the MCP tool definition for chat.get_history.
func (t *ChatTools) GetHistoryTool() mcp.Tool {
	return mcp.NewTool("get_history",
		mcp.WithDescription(
			"Fetch the full message history for a chat session. Use this to get "+
				"context before responding to a user's message. Returns all messages "+
				"in chronological order with their role (user/assistant/status) and "+
				"content."),
		mcp.WithString("session_id",
			mcp.Required(),
			mcp.Description("The chat session ID to fetch history for"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of messages to return (default: 50)"),
		),
	)
}

// SendReplyTool returns the MCP tool definition for chat.send_reply.
func (t *ChatTools) SendReplyTool() mcp.Tool {
	return mcp.NewTool("send_reply",
		mcp.WithDescription(
			"Send a reply message to a chat session. This is how the agent responds "+
				"to a user's chat message. The reply is stored in the session history "+
				"and delivered to the user in real-time via WebSocket. Call this once "+
				"per conversation turn with the agent's final answer."),
		mcp.WithString("session_id",
			mcp.Required(),
			mcp.Description("The chat session ID to reply to"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("The reply content (markdown supported)"),
		),
	)
}

// SendStatusTool returns the MCP tool definition for chat.send_status.
func (t *ChatTools) SendStatusTool() mcp.Tool {
	return mcp.NewTool("send_status",
		mcp.WithDescription(
			"Send an intermediate status message to a chat session. Use this to "+
				"give the user feedback while the agent is working (e.g., 'Looking "+
				"at the repository...', 'Running tests...'). Status messages appear "+
				"in the chat UI but are not part of the conversation history the "+
				"agent sees on the next turn."),
		mcp.WithString("session_id",
			mcp.Required(),
			mcp.Description("The chat session ID to send status to"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("The status message content"),
		),
	)
}

// --- Tool handlers ---

// ListSessions handles chat.list_sessions calls.
func (t *ChatTools) ListSessions(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := fmt.Sprintf("/api/internal/chat/sessions?agent=%s", t.AgentName)
	body, err := t.hubGet(ctx, path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list chat sessions: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

// GetHistory handles chat.get_history calls.
func (t *ChatTools) GetHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	p := fmt.Sprintf("/api/internal/chat/sessions/%s", sessionID)
	if limit, ok := args["limit"].(float64); ok && limit > 0 {
		p += fmt.Sprintf("?limit=%d", int(limit))
	}

	body, err := t.hubGet(ctx, p)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get chat history: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

// SendReply handles chat.send_reply calls.
func (t *ChatTools) SendReply(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}
	content, _ := args["content"].(string)
	if content == "" {
		return mcp.NewToolResultError("content is required"), nil
	}

	payload, err := json.Marshal(map[string]any{
		"role":    "assistant",
		"content": content,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode reply: %v", err)), nil
	}

	p := fmt.Sprintf("/api/internal/chat/sessions/%s/messages", sessionID)
	body, err := t.hubPost(ctx, p, payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to send reply: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

// SendStatus handles chat.send_status calls.
func (t *ChatTools) SendStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, _ := args["session_id"].(string)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}
	content, _ := args["content"].(string)
	if content == "" {
		return mcp.NewToolResultError("content is required"), nil
	}

	payload, err := json.Marshal(map[string]any{
		"role":    "status",
		"content": content,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode status: %v", err)), nil
	}

	p := fmt.Sprintf("/api/internal/chat/sessions/%s/messages", sessionID)
	body, err := t.hubPost(ctx, p, payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to send status: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

// --- HTTP helpers ---

func (t *ChatTools) hubGet(ctx context.Context, path string) ([]byte, error) {
	return t.hubRequest(ctx, http.MethodGet, path, nil)
}

func (t *ChatTools) hubPost(ctx context.Context, path string, body []byte) ([]byte, error) {
	return t.hubRequestWithCodes(ctx, http.MethodPost, path, body, http.StatusOK, http.StatusCreated)
}

func (t *ChatTools) hubRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	return t.hubRequestWithCodes(ctx, method, path, body, http.StatusOK)
}

func (t *ChatTools) hubRequestWithCodes(ctx context.Context, method, path string, body []byte, validCodes ...int) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, t.HubURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost || method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}
	if t.HubInternalToken != "" {
		req.Header.Set("X-Internal-Token", t.HubInternalToken)
	}
	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	for _, code := range validCodes {
		if resp.StatusCode == code {
			return respBody, nil
		}
	}
	return nil, fmt.Errorf("hub returned %d: %s", resp.StatusCode, string(respBody))
}
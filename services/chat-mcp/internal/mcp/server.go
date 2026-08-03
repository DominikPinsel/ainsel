// Package mcp wires the chat MCP server with all chat tools registered.
package mcp

import (
	"log/slog"
	"net/http"

	"github.com/mark3labs/mcp-go/server"

	"github.com/DominikPinsel/ainsel/services/chat-mcp/internal/tools"
)

// New creates a configured MCP server with all chat tools registered.
// The server exposes tools that let the agent interact with chat sessions
// by proxying to the hub backend's chat REST API.
func New(log *slog.Logger, hubURL, hubInternalToken, agentName string) *server.MCPServer {
	s := server.NewMCPServer(
		"ainsel-chat-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	chat := tools.NewChatTools(hubURL, hubInternalToken, agentName)
	s.AddTool(chat.ListSessionsTool(), chat.ListSessions)
	s.AddTool(chat.GetHistoryTool(), chat.GetHistory)
	s.AddTool(chat.SendReplyTool(), chat.SendReply)
	s.AddTool(chat.SendStatusTool(), chat.SendStatus)

	log.Info("chat MCP server initialized", "agent", agentName, "hub_url", hubURL)
	return s
}

// StreamableHTTPHandler returns an http.Handler for the MCP streamable HTTP transport.
func StreamableHTTPHandler(s *server.MCPServer) http.Handler {
	return server.NewStreamableHTTPServer(s)
}
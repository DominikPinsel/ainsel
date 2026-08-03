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

// MCPServerTools wraps the hub's /api/v1/mcp-servers endpoints. The MCP-server
// registry lists every MCP endpoint an agent is allowed to call out to; this
// tool surfaces that catalogue to an AI client.
type MCPServerTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewMCPServerTools(hubURL string) *MCPServerTools {
	return &MCPServerTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *MCPServerTools) ListMCPServersTool() mcp.Tool {
	return mcp.NewTool("list_mcp_servers",
		mcp.WithDescription("List the MCP servers registered in the platform's registry, including their URLs and the tools each one exposes. Default page size: 50."),
		mcp.WithNumber("page", mcp.Description("Page number (1-based, default: 1)")),
		mcp.WithNumber("pageSize", mcp.Description("Page size (default: 50)")),
	)
}

func (t *MCPServerTools) GetMCPServerTool() mcp.Tool {
	return mcp.NewTool("get_mcp_server",
		mcp.WithDescription("Get detail about one MCP server registry entry: URL, transport, tool names, and any auth config (non-secret fields)."),
		mcp.WithString("name", mcp.Required(), mcp.Description("MCP server name")),
	)
}

func (t *MCPServerTools) ListMCPServers(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	q.Set("page", strconv.Itoa(defaultArgInt(args, "page", 1)))
	q.Set("pageSize", strconv.Itoa(defaultArgInt(args, "pageSize", 50)))
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, "/api/v1/mcp-servers?"+q.Encode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list MCP servers: %v", err)), nil
	}
	body = annotatePageMeta(body, "more MCP servers available — pass page=N to fetch the next page")
	return mcp.NewToolResultText(string(body)), nil
}

func (t *MCPServerTools) GetMCPServer(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := req.Params.Arguments.(map[string]any)["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, "/api/v1/mcp-servers/"+name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get MCP server %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

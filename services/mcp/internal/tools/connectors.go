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

type ConnectorTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewConnectorTools(hubURL string) *ConnectorTools {
	return &ConnectorTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *ConnectorTools) ListConnectorsTool() mcp.Tool {
	return mcp.NewTool("list_connectors",
		mcp.WithDescription("List connectors with their status. Default page size: 50."),
		mcp.WithNumber("page", mcp.Description("Page number (1-based, default: 1)")),
		mcp.WithNumber("pageSize", mcp.Description("Page size (default: 50)")),
	)
}

func (c *ConnectorTools) GetConnectorTool() mcp.Tool {
	return mcp.NewTool("get_connector",
		mcp.WithDescription("Get detailed connector configuration and health"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Connector name")),
	)
}

func (c *ConnectorTools) ListConnectors(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	q.Set("page", strconv.Itoa(defaultArgInt(args, "page", 1)))
	q.Set("pageSize", strconv.Itoa(defaultArgInt(args, "pageSize", 50)))
	body, err := hubGet(ctx, c.HTTPClient, c.HubURL, "/api/v1/connectors?"+q.Encode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list connectors: %v", err)), nil
	}
	body = annotatePageMeta(body, "more connectors available — pass page=N to fetch the next page")
	return mcp.NewToolResultText(string(body)), nil
}

func (c *ConnectorTools) GetConnector(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := req.Params.Arguments.(map[string]any)["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	body, err := hubGet(ctx, c.HTTPClient, c.HubURL, "/api/v1/connectors/"+name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get connector %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

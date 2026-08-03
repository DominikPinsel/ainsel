package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

// handleAgentImageRefreshMCP handles POST /api/v1/agent-images/{name}/refresh-mcp.
// It queries each configured MCP server, discovers its tools, and merges them
// into the image's tool list: existing tool enabled/disabled state is preserved,
// newly discovered tools are added with Disabled=true and IsNew=true, and tools
// that have disappeared from the server are removed.
func (s *Server) handleAgentImageRefreshMCP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rest := extractName(r.URL.Path, "/api/v1/agent-images/")
	name := strings.TrimSuffix(rest, "/refresh-mcp")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing agent image name")
		return
	}
	// Method check is handled by the router before dispatch.

	var img agentv1alpha1.AgentImage
	if err := s.client.Get(ctx, types.NamespacedName{Name: name, Namespace: s.ns}, &img); err != nil {
		writeError(w, http.StatusNotFound, "agent image not found")
		return
	}

	if len(img.Spec.MCPServers) == 0 {
		writeError(w, http.StatusBadRequest, "no MCP servers configured on this image")
		return
	}

	freshByServer := make(map[string][]mcpToolSummary, len(img.Spec.MCPServers))
	var errs []string
	for _, mcpServer := range img.Spec.MCPServers {
		// tools/list does not require auth; never forward credentials from here.
		tools, err := listMCPTools(ctx, mcpServer.URL, "")
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", mcpServer.Name, err))
			continue
		}
		freshByServer[mcpServer.Name] = tools
	}

	// Build serverOrder from MCPServers slice for deterministic tool ordering.
	serverOrder := make([]string, 0, len(img.Spec.MCPServers))
	for _, mcpServer := range img.Spec.MCPServers {
		serverOrder = append(serverOrder, mcpServer.Name)
	}
	img.Spec.Tools = mergeMCPTools(img.Spec.Tools, freshByServer, serverOrder)

	if err := s.client.Update(ctx, &img); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := toSimpleAgentImageResponse(img)
	writeJSON(w, http.StatusOK, map[string]any{
		"image":    resp,
		"warnings": errs,
	})
}

// mergeMCPTools merges freshly-discovered MCP tools into the existing tool list.
//
// Rules:
//   - Non-MCP tools are kept as-is.
//   - MCP tools still present on their server keep their current Disabled state and
//     Description (updated from server); their IsNew flag is preserved.
//   - MCP tools newly discovered (not in existing) are added with Disabled=true, IsNew=true.
//   - MCP tools no longer present on their server are removed.
func mergeMCPTools(
	existing []agentv1alpha1.AgentImageTool,
	freshByServer map[string][]mcpToolSummary,
	serverOrder []string,
) []agentv1alpha1.AgentImageTool {
	// Build lookup: fullName → existing MCP tool for quick state retrieval.
	existingMCP := make(map[string]agentv1alpha1.AgentImageTool)
	for _, t := range existing {
		if t.Kind == agentv1alpha1.AgentImageToolKindMCP {
			existingMCP[t.Name] = t
		}
	}

	// Start with native tools (non-MCP), preserving order.
	result := make([]agentv1alpha1.AgentImageTool, 0, len(existing))
	for _, t := range existing {
		if t.Kind != agentv1alpha1.AgentImageToolKindMCP {
			result = append(result, t)
		}
	}

	// Append MCP tools from each server in the order servers are listed.
	for _, serverName := range serverOrder {
		freshTools, ok := freshByServer[serverName]
		if !ok {
			continue
		}
		for _, ft := range freshTools {
			fullName := fmt.Sprintf("mcp__%s__%s", serverName, ft.Name)
			if prev, existed := existingMCP[fullName]; existed {
				// Preserve enabled/disabled state and IsNew; update description.
				result = append(result, agentv1alpha1.AgentImageTool{
					Name:        fullName,
					Kind:        agentv1alpha1.AgentImageToolKindMCP,
					Description: ft.Description,
					McpSource:   serverName,
					Disabled:    prev.Disabled,
					IsNew:       prev.IsNew,
				})
			} else {
				// Brand-new tool: start disabled, mark as new.
				result = append(result, agentv1alpha1.AgentImageTool{
					Name:        fullName,
					Kind:        agentv1alpha1.AgentImageToolKindMCP,
					Description: ft.Description,
					McpSource:   serverName,
					Disabled:    true,
					IsNew:       true,
				})
			}
		}
	}

	return result
}

// mcpToolSummary holds the minimal fields we need from an MCP tool listing.
type mcpToolSummary struct {
	Name        string
	Description string
}

// listMCPTools connects to an MCP server over streamable HTTP and returns its tools.
func listMCPTools(ctx context.Context, url, token string) ([]mcpToolSummary, error) {
	opts := []transport.StreamableHTTPCOption{
		transport.WithHTTPTimeout(15 * time.Second),
	}
	if token != "" {
		opts = append(opts, transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + token,
		}))
	}

	trans, err := transport.NewStreamableHTTP(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("create transport: %w", err)
	}
	defer func() { _ = trans.Close() }()

	c := client.NewClient(trans)
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("start client: %w", err)
	}

	if _, err := c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ClientInfo: mcp.Implementation{Name: "ainsel-hub", Version: "0.1.0"},
		},
	}); err != nil {
		return nil, fmt.Errorf("initialize client: %w", err)
	}

	result, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	summary := make([]mcpToolSummary, 0, len(result.Tools))
	for _, t := range result.Tools {
		summary = append(summary, mcpToolSummary{
			Name:        t.Name,
			Description: t.Description,
		})
	}
	return summary, nil
}

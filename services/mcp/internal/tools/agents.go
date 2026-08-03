package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type AgentTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewAgentTools(hubURL string) *AgentTools {
	return &AgentTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *AgentTools) ListAgentsTool() mcp.Tool {
	return mcp.NewTool("list_agents",
		mcp.WithDescription("List agents in the Ainsel platform with their status. Default page size: 50."),
		mcp.WithNumber("page", mcp.Description("Page number (1-based, default: 1)")),
		mcp.WithNumber("pageSize", mcp.Description("Page size (default: 50)")),
	)
}

func (a *AgentTools) GetAgentTool() mcp.Tool {
	return mcp.NewTool("get_agent",
		mcp.WithDescription("Get detailed information about a specific agent including config, persona, and pod state"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Agent name")),
	)
}

func (a *AgentTools) ListAgents(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	q.Set("page", strconv.Itoa(defaultArgInt(args, "page", 1)))
	q.Set("pageSize", strconv.Itoa(defaultArgInt(args, "pageSize", 50)))
	body, err := hubGet(ctx, a.HTTPClient, a.HubURL, "/api/v1/agents?"+q.Encode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list agents: %v", err)), nil
	}
	body = annotatePageMeta(body, "more agents available — pass page=N to fetch the next page")
	return mcp.NewToolResultText(string(body)), nil
}

func (a *AgentTools) GetAgent(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := req.Params.Arguments.(map[string]any)["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	body, err := hubGet(ctx, a.HTTPClient, a.HubURL, "/api/v1/agents/"+name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get agent %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (a *AgentTools) UpdateAgentTool() mcp.Tool {
	return mcp.NewTool("update_agent",
		mcp.WithDescription("Update an agent's LLM runtime configuration. Only provided fields are changed; omitted fields are left unchanged."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Agent ID (e.g. a-director)")),
		mcp.WithString("model", mcp.Description("LLM model to use (e.g. glm-5.1, qwen3.5:cloud)")),
		mcp.WithNumber("max_turns", mcp.Description("Maximum turns per invocation")),
		mcp.WithNumber("temperature", mcp.Description("LLM temperature")),
	)
}

func (a *AgentTools) UpdateAgent(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	llm := map[string]any{}
	if v, ok := args["model"].(string); ok && v != "" {
		llm["model"] = v
	}
	if v, ok := args["max_turns"].(float64); ok && v != 0 {
		llm["maxTurns"] = int(v)
	}
	if v, ok := args["temperature"].(float64); ok {
		llm["temperature"] = v
	}

	if len(llm) == 0 {
		return mcp.NewToolResultError("at least one of model, max_turns, or temperature must be provided"), nil
	}

	payload := map[string]any{"llm": llm}
	bodyData, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode request: %v", err)), nil
	}
	respBody, err := hubPut(ctx, a.HTTPClient, a.HubURL, "/api/v1/agents/"+name, bytes.NewReader(bodyData))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to update agent %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(respBody)), nil
}

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

// PersonaTools wraps the hub's /api/v1/personas read endpoints (added in
// Project A-prime-1). Pure hub-proxy reads — no transformation, no
// aggregation. Mirrors the shape of AgentTools / InvocationTools.
type PersonaTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewPersonaTools(hubURL string) *PersonaTools {
	return &PersonaTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *PersonaTools) ListPersonasTool() mcp.Tool {
	return mcp.NewTool("list_personas",
		mcp.WithDescription("List personas registered with the hub. Metadata only — call get_persona by id to fetch the current text. Default page size: 50."),
		mcp.WithNumber("page", mcp.Description("Page number (1-based, default: 1)")),
		mcp.WithNumber("pageSize", mcp.Description("Page size (default: 50)")),
	)
}

func (p *PersonaTools) GetPersonaTool() mcp.Tool {
	return mcp.NewTool("get_persona",
		mcp.WithDescription("Get a persona by id, including the current version's text."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Persona id (ULID)")),
	)
}

func (p *PersonaTools) ListPersonaVersionsTool() mcp.Tool {
	return mcp.NewTool("list_persona_versions",
		mcp.WithDescription("List version history for a persona (metadata only, newest first). Pass version_number to get_persona_version to fetch a specific version's text."),
		mcp.WithString("persona_id", mcp.Required(), mcp.Description("Persona id (ULID)")),
	)
}

func (p *PersonaTools) GetPersonaVersionTool() mcp.Tool {
	return mcp.NewTool("get_persona_version",
		mcp.WithDescription("Get a specific historical version of a persona including its text."),
		mcp.WithString("persona_id", mcp.Required(), mcp.Description("Persona id (ULID)")),
		mcp.WithNumber("version_number", mcp.Required(), mcp.Description("Version number (1-based)")),
	)
}

func (p *PersonaTools) ListPersonas(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	q.Set("page", strconv.Itoa(defaultArgInt(args, "page", 1)))
	q.Set("pageSize", strconv.Itoa(defaultArgInt(args, "pageSize", 50)))
	path := "/api/v1/personas?" + q.Encode()
	body, err := hubGet(ctx, p.HTTPClient, p.HubURL, path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list personas: %v", err)), nil
	}
	body = annotatePageMeta(body, "more personas available — pass page=N to fetch the next page")
	return mcp.NewToolResultText(string(body)), nil
}

func (p *PersonaTools) GetPersona(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	id, _ := args["id"].(string)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}
	body, err := hubGet(ctx, p.HTTPClient, p.HubURL, "/api/v1/personas/"+id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get persona %s: %v", id, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (p *PersonaTools) ListPersonaVersions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	id, _ := args["persona_id"].(string)
	if id == "" {
		return mcp.NewToolResultError("persona_id is required"), nil
	}
	body, err := hubGet(ctx, p.HTTPClient, p.HubURL, "/api/v1/personas/"+id+"/versions")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list versions for persona %s: %v", id, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (p *PersonaTools) GetPersonaVersion(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	id, _ := args["persona_id"].(string)
	if id == "" {
		return mcp.NewToolResultError("persona_id is required"), nil
	}
	version, ok := args["version_number"].(float64)
	if !ok || version <= 0 {
		return mcp.NewToolResultError("version_number is required and must be a positive integer"), nil
	}
	path := fmt.Sprintf("/api/v1/personas/%s/versions/%d", id, int(version))
	body, err := hubGet(ctx, p.HTTPClient, p.HubURL, path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get persona version: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (p *PersonaTools) CreatePersonaTool() mcp.Tool {
	return mcp.NewTool("create_persona",
		mcp.WithDescription("Create a new persona with a name, optional description, and text. Returns the created persona including its id."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Unique persona name (max 200 chars)")),
		mcp.WithString("text", mcp.Required(), mcp.Description("Persona instruction text (max 100,000 chars)")),
		mcp.WithString("description", mcp.Description("Optional description (max 2000 chars)")),
	)
}

func (p *PersonaTools) CreatePersona(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	text, _ := args["text"].(string)
	if text == "" {
		return mcp.NewToolResultError("text is required"), nil
	}
	payload := map[string]any{"name": name, "text": text}
	if desc, _ := args["description"].(string); desc != "" {
		payload["description"] = desc
	}
	bodyData, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode request: %v", err)), nil
	}
	respBody, err := hubPost(ctx, p.HTTPClient, p.HubURL, "/api/v1/personas", bytes.NewReader(bodyData))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create persona: %v", err)), nil
	}
	return mcp.NewToolResultText(string(respBody)), nil
}

func (p *PersonaTools) UpdatePersonaTool() mcp.Tool {
	return mcp.NewTool("update_persona",
		mcp.WithDescription("Update an existing persona's name, description, or text. Updating text creates a new version. At least one field must be provided."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Persona id (ULID)")),
		mcp.WithString("text", mcp.Description("New persona instruction text (creates a new version)")),
		mcp.WithString("name", mcp.Description("New persona name")),
		mcp.WithString("description", mcp.Description("New persona description")),
	)
}

func (p *PersonaTools) UpdatePersona(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	id, _ := args["id"].(string)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}
	payload := map[string]any{}
	if v, _ := args["name"].(string); v != "" {
		payload["name"] = v
	}
	if v, _ := args["description"].(string); v != "" {
		payload["description"] = v
	}
	if v, _ := args["text"].(string); v != "" {
		payload["text"] = v
	}
	if len(payload) == 0 {
		return mcp.NewToolResultError("at least one of name, description, or text must be provided"), nil
	}
	bodyData, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode request: %v", err)), nil
	}
	respBody, err := hubPut(ctx, p.HTTPClient, p.HubURL, "/api/v1/personas/"+id, bytes.NewReader(bodyData))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to update persona %s: %v", id, err)), nil
	}
	return mcp.NewToolResultText(string(respBody)), nil
}

func (p *PersonaTools) DeletePersonaTool() mcp.Tool {
	return mcp.NewTool("delete_persona",
		mcp.WithDescription("Delete a persona by id. Fails if the persona is referenced by active agents."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Persona id (ULID)")),
	)
}

func (p *PersonaTools) DeletePersona(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	id, _ := args["id"].(string)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}
	_, err := hubDelete(ctx, p.HTTPClient, p.HubURL, "/api/v1/personas/"+id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to delete persona %s: %v", id, err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("persona %s deleted", id)), nil
}

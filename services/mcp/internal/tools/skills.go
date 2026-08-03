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

// SkillTools wraps the hub's /api/v1/skills endpoints so an MCP client
// can list, inspect, create, edit, and delete skills.
type SkillTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewSkillTools(hubURL string) *SkillTools {
	return &SkillTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *SkillTools) ListSkillsTool() mcp.Tool {
	return mcp.NewTool("list_skills",
		mcp.WithDescription("List skills registered with the hub. Metadata only — call get_skill by id to fetch the full body. Default page size: 50."),
		mcp.WithNumber("page", mcp.Description("Page number (1-based, default: 1)")),
		mcp.WithNumber("pageSize", mcp.Description("Page size (default: 50)")),
	)
}

func (t *SkillTools) GetSkillTool() mcp.Tool {
	return mcp.NewTool("get_skill",
		mcp.WithDescription("Get a skill by id, including its Markdown body."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Skill id (slug)")),
	)
}

func (t *SkillTools) CreateSkillTool() mcp.Tool {
	return mcp.NewTool("create_skill",
		mcp.WithDescription("Create a new skill. The id is a user-supplied slug (lowercase alphanumeric with hyphens, max 64 chars). Name is required (max 200 chars). Description max 2000 chars. Body is the Markdown instruction text (max 100,000 chars)."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Skill id (slug): lowercase alphanumeric with hyphens, no leading/trailing/consecutive hyphens, max 64 chars")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Skill display name (max 200 chars)")),
		mcp.WithString("description", mcp.Description("Optional description (max 2000 chars)")),
		mcp.WithString("body", mcp.Description("Optional Markdown instruction text (max 100,000 chars)")),
	)
}

func (t *SkillTools) UpdateSkillTool() mcp.Tool {
	return mcp.NewTool("update_skill",
		mcp.WithDescription("Update an existing skill's name, description, or body. Omitting a field preserves its current value. At least one field must be provided."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Skill id (slug)")),
		mcp.WithString("name", mcp.Description("New skill display name (max 200 chars)")),
		mcp.WithString("description", mcp.Description("New description (max 2000 chars)")),
		mcp.WithString("body", mcp.Description("New Markdown instruction text (max 100,000 chars)")),
	)
}

func (t *SkillTools) DeleteSkillTool() mcp.Tool {
	return mcp.NewTool("delete_skill",
		mcp.WithDescription("Delete a skill by id. Fails with a 409 error if the skill is referenced by any agent images."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Skill id (slug)")),
	)
}

func (t *SkillTools) ListSkills(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	q.Set("page", strconv.Itoa(defaultArgInt(args, "page", 1)))
	q.Set("pageSize", strconv.Itoa(defaultArgInt(args, "pageSize", 50)))
	path := "/api/v1/skills?" + q.Encode()
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list skills: %v", err)), nil
	}
	body = annotatePageMeta(body, "more skills available — pass page=N to fetch the next page")
	return mcp.NewToolResultText(string(body)), nil
}

func (t *SkillTools) GetSkill(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	id, _ := args["id"].(string)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, "/api/v1/skills/"+id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get skill %s: %v", id, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (t *SkillTools) CreateSkill(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	id, _ := args["id"].(string)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	payload := map[string]any{"id": id, "name": name}
	if desc, _ := args["description"].(string); desc != "" {
		payload["description"] = desc
	}
	if body, _ := args["body"].(string); body != "" {
		payload["body"] = body
	}
	bodyData, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode request: %v", err)), nil
	}
	respBody, err := hubPost(ctx, t.HTTPClient, t.HubURL, "/api/v1/skills", bytes.NewReader(bodyData))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create skill: %v", err)), nil
	}
	return mcp.NewToolResultText(string(respBody)), nil
}

func (t *SkillTools) UpdateSkill(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if v, _ := args["body"].(string); v != "" {
		payload["body"] = v
	}
	if len(payload) == 0 {
		return mcp.NewToolResultError("at least one of name, description, or body must be provided"), nil
	}
	bodyData, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to encode request: %v", err)), nil
	}
	respBody, err := hubPut(ctx, t.HTTPClient, t.HubURL, "/api/v1/skills/"+id, bytes.NewReader(bodyData))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to update skill %s: %v", id, err)), nil
	}
	return mcp.NewToolResultText(string(respBody)), nil
}

func (t *SkillTools) DeleteSkill(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	id, _ := args["id"].(string)
	if id == "" {
		return mcp.NewToolResultError("id is required"), nil
	}
	_, err := hubDelete(ctx, t.HTTPClient, t.HubURL, "/api/v1/skills/"+id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to delete skill %s: %v", id, err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("skill %s deleted", id)), nil
}
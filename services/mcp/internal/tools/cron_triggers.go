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

type CronTriggerTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewCronTriggerTools(hubURL string) *CronTriggerTools {
	return &CronTriggerTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *CronTriggerTools) ListCronTriggersTool() mcp.Tool {
	return mcp.NewTool("list_cron_triggers",
		mcp.WithDescription("List cron triggers with their schedule, agent binding, and status. Default page size: 50."),
		mcp.WithNumber("page", mcp.Description("Page number (1-based, default: 1)")),
		mcp.WithNumber("pageSize", mcp.Description("Page size (default: 50)")),
	)
}

func (t *CronTriggerTools) GetCronTriggerTool() mcp.Tool {
	return mcp.NewTool("get_cron_trigger",
		mcp.WithDescription("Get detailed cron trigger configuration: schedule, prompt, agent binding, and validation status"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Cron trigger name or ID")),
	)
}

func (t *CronTriggerTools) CreateCronTriggerTool() mcp.Tool {
	return mcp.NewTool("create_cron_trigger",
		mcp.WithDescription("Create a new cron trigger that delivers a prompt to an agent on a cron schedule. Schedule is a standard 5-field cron expression (minute hour day-of-month month day-of-week) in the hub's local time (default UTC). Numbers only — named days (sun/mon) are not supported. Example: \"0 9 * * 1-5\" fires at 09:00 on weekdays."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Display name for the cron trigger")),
		mcp.WithString("agentRef", mcp.Required(), mcp.Description("Agent reference (agent name or ID)")),
		mcp.WithString("schedule", mcp.Required(), mcp.Description("5-field cron expression, e.g. \"0 9 * * 1-5\" for weekdays at 9am, \"30 2 * * 0\" for Sundays at 2:30am")),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt text delivered to the agent as the user message on each fire. Sent verbatim — no event template wrapping.")),
		mcp.WithBoolean("enabled", mcp.Description("Whether the schedule is active. Defaults to true if omitted.")),
	)
}

func (t *CronTriggerTools) UpdateCronTriggerTool() mcp.Tool {
	return mcp.NewTool("update_cron_trigger",
		mcp.WithDescription("Update an existing cron trigger. All fields except name are optional. Pass an empty string for prompt or agentRef to clear those fields."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Cron trigger ID to update")),
		mcp.WithString("displayName", mcp.Description("New display name for the cron trigger")),
		mcp.WithString("agentRef", mcp.Description("New agent reference (pass empty string to clear)")),
		mcp.WithString("schedule", mcp.Description("New 5-field cron expression")),
		mcp.WithString("prompt", mcp.Description("New prompt text (pass empty string to clear)")),
		mcp.WithBoolean("enabled", mcp.Description("Enable or disable the schedule")),
	)
}

func (t *CronTriggerTools) DeleteCronTriggerTool() mcp.Tool {
	return mcp.NewTool("delete_cron_trigger",
		mcp.WithDescription("Delete a cron trigger by name"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Cron trigger name or ID to delete")),
	)
}

func (t *CronTriggerTools) CreateCronTrigger(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	agentRef, _ := args["agentRef"].(string)
	schedule, _ := args["schedule"].(string)
	prompt, _ := args["prompt"].(string)
	if name == "" || agentRef == "" || schedule == "" || prompt == "" {
		return mcp.NewToolResultError("name, agentRef, schedule, and prompt are required"), nil
	}

	payload := map[string]any{
		"name":     name,
		"agentRef": agentRef,
		"schedule": schedule,
		"prompt":   prompt,
	}
	if enabled, ok := args["enabled"].(bool); ok {
		payload["enabled"] = enabled
	}

	bodyData, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal request: %v", err)), nil
	}

	body, err := hubPost(ctx, t.HTTPClient, t.HubURL, "/api/v1/cron-triggers", bytes.NewReader(bodyData))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create cron trigger: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (t *CronTriggerTools) UpdateCronTrigger(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	payload := map[string]any{}
	if v, ok := args["displayName"].(string); ok && v != "" {
		payload["name"] = v
	}
	// Use ok-only check so empty string intentionally clears the field.
	if v, ok := args["agentRef"].(string); ok {
		payload["agentRef"] = v
	}
	if v, ok := args["schedule"].(string); ok && v != "" {
		payload["schedule"] = v
	}
	// Use ok-only check so empty string intentionally clears the field.
	if v, ok := args["prompt"].(string); ok {
		payload["prompt"] = v
	}
	if enabled, ok := args["enabled"].(bool); ok {
		payload["enabled"] = enabled
	}

	bodyData, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal request: %v", err)), nil
	}

	body, err := hubPut(ctx, t.HTTPClient, t.HubURL, "/api/v1/cron-triggers/"+name, bytes.NewReader(bodyData))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to update cron trigger %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (t *CronTriggerTools) ListCronTriggers(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	q.Set("page", strconv.Itoa(defaultArgInt(args, "page", 1)))
	q.Set("pageSize", strconv.Itoa(defaultArgInt(args, "pageSize", 50)))
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, "/api/v1/cron-triggers?"+q.Encode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list cron triggers: %v", err)), nil
	}
	body = annotatePageMeta(body, "more cron triggers available — pass page=N to fetch the next page")
	return mcp.NewToolResultText(string(body)), nil
}

func (t *CronTriggerTools) GetCronTrigger(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := req.Params.Arguments.(map[string]any)["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, "/api/v1/cron-triggers/"+name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get cron trigger %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (t *CronTriggerTools) DeleteCronTrigger(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := req.Params.Arguments.(map[string]any)["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	_, err := hubDelete(ctx, t.HTTPClient, t.HubURL, "/api/v1/cron-triggers/"+name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to delete cron trigger %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("cron trigger %s deleted", name)), nil
}
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

type TriggerTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewTriggerTools(hubURL string) *TriggerTools {
	return &TriggerTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *TriggerTools) ListTriggersTool() mcp.Tool {
	return mcp.NewTool("list_triggers",
		mcp.WithDescription("List triggers with their agent/connector bindings and filters. Default page size: 50."),
		mcp.WithNumber("page", mcp.Description("Page number (1-based, default: 1)")),
		mcp.WithNumber("pageSize", mcp.Description("Page size (default: 50)")),
	)
}

func (t *TriggerTools) GetTriggerTool() mcp.Tool {
	return mcp.NewTool("get_trigger",
		mcp.WithDescription("Get detailed trigger configuration: filters and routing"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Trigger name")),
	)
}

func (t *TriggerTools) DeleteTriggerTool() mcp.Tool {
	return mcp.NewTool("delete_trigger",
		mcp.WithDescription("Delete a trigger by name"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Trigger name or ID to delete")),
	)
}

type triggerFilter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

func (t *TriggerTools) CreateTriggerTool() mcp.Tool {
	return mcp.NewTool("create_trigger",
		mcp.WithDescription("Create a new trigger. Use filters for event matching. The 'type' field is derived from the webhook source's event-type header (any header ending in -Event). The 'action' field comes from the webhook payload body. Example filters: [{\"field\":\"type\",\"op\":\"eq\",\"value\":\"issue_assign\"},{\"field\":\"action\",\"op\":\"eq\",\"value\":\"assigned\"},{\"field\":\"issue.assignee.login\",\"op\":\"eq\",\"value\":\"dev-agent\"}]"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Display name for the trigger")),
		mcp.WithString("agentRef", mcp.Required(), mcp.Description("Agent reference")),
		mcp.WithString("connectorRef", mcp.Required(), mcp.Description("Connector reference")),
		mcp.WithString("filters", mcp.Description("Optional JSON array of filter objects with field, op, and value")),
		mcp.WithString("groupId", mcp.Description("Group to assign the trigger to; must be a group the caller has write access to. Required on hubs with access control enabled.")),
	)
}

func (t *TriggerTools) UpdateTriggerTool() mcp.Tool {
	return mcp.NewTool("update_trigger",
		mcp.WithDescription("Update an existing trigger. All fields except name are optional."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Trigger ID to update")),
		mcp.WithString("displayName", mcp.Description("New display name for the trigger")),
		mcp.WithString("agentRef", mcp.Description("New agent reference")),
		mcp.WithString("connectorRef", mcp.Description("New connector reference")),
		mcp.WithString("filters", mcp.Description("Optional JSON array of filter objects with field, op, and value. Pass empty array [] to clear filters.")),
	)
}

func (t *TriggerTools) CreateTrigger(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	agentRef, _ := args["agentRef"].(string)
	connectorRef, _ := args["connectorRef"].(string)
	if name == "" || agentRef == "" || connectorRef == "" {
		return mcp.NewToolResultError("name, agentRef, and connectorRef are required"), nil
	}

	payload := map[string]any{
		"name":         name,
		"agentRef":     agentRef,
		"connectorRef": connectorRef,
	}
	if filtersStr, ok := args["filters"].(string); ok && filtersStr != "" {
		var filters []triggerFilter
		if err := json.Unmarshal([]byte(filtersStr), &filters); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid filters JSON: %v", err)), nil
		}
		payload["filters"] = filters
	}
	if groupID, ok := args["groupId"].(string); ok && groupID != "" {
		payload["groupId"] = groupID
	}

	bodyData, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal request: %v", err)), nil
	}

	body, err := hubPost(ctx, t.HTTPClient, t.HubURL, "/api/v1/triggers", bytes.NewReader(bodyData))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create trigger: %v", appendGroupIDHint(err))), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

// appendGroupIDHint returns the error message plus a follow-up hint when the
// hub rejected the request because access control is enabled and no groupId
// was supplied.
func appendGroupIDHint(err error) string {
	if strings.Contains(err.Error(), "groupId is required") {
		return err.Error() + " — this hub has access control enabled; retry with a groupId your user has write access to"
	}
	return err.Error()
}

func (t *TriggerTools) UpdateTrigger(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.Params.Arguments.(map[string]any)
	name, _ := args["name"].(string)
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	payload := map[string]any{}
	if v, ok := args["displayName"].(string); ok && v != "" {
		payload["name"] = v
	}
	if v, ok := args["agentRef"].(string); ok && v != "" {
		payload["agentRef"] = v
	}
	if v, ok := args["connectorRef"].(string); ok && v != "" {
		payload["connectorRef"] = v
	}
	if filtersStr, ok := args["filters"].(string); ok && filtersStr != "" {
		var filters []triggerFilter
		if err := json.Unmarshal([]byte(filtersStr), &filters); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid filters JSON: %v", err)), nil
		}
		payload["filters"] = filters
	} else if filtersStr, ok := args["filters"].(string); ok && filtersStr == "[]" {
		payload["filters"] = []triggerFilter{}
	}

	bodyData, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal request: %v", err)), nil
	}

	body, err := hubPut(ctx, t.HTTPClient, t.HubURL, "/api/v1/triggers/"+name, bytes.NewReader(bodyData))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to update trigger %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (t *TriggerTools) ListTriggers(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, _ := req.Params.Arguments.(map[string]any)
	q := url.Values{}
	q.Set("page", strconv.Itoa(defaultArgInt(args, "page", 1)))
	q.Set("pageSize", strconv.Itoa(defaultArgInt(args, "pageSize", 50)))
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, "/api/v1/triggers?"+q.Encode())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list triggers: %v", err)), nil
	}
	body = annotatePageMeta(body, "more triggers available — pass page=N to fetch the next page")
	return mcp.NewToolResultText(string(body)), nil
}

func (t *TriggerTools) GetTrigger(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := req.Params.Arguments.(map[string]any)["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	body, err := hubGet(ctx, t.HTTPClient, t.HubURL, "/api/v1/triggers/"+name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get trigger %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (t *TriggerTools) DeleteTrigger(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := req.Params.Arguments.(map[string]any)["name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	_, err := hubDelete(ctx, t.HTTPClient, t.HubURL, "/api/v1/triggers/"+name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to delete trigger %s: %v", name, err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("trigger %s deleted", name)), nil
}

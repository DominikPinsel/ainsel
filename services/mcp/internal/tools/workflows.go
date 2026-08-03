package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// WorkflowTools exposes an agent-centric view of the platform's running
// workflows by joining /api/v1/agents and /api/v1/triggers from the hub.
type WorkflowTools struct {
	HubURL     string
	HTTPClient *http.Client
}

func NewWorkflowTools(hubURL string) *WorkflowTools {
	return &WorkflowTools{
		HubURL:     hubURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *WorkflowTools) SummarizeWorkflowsTool() mcp.Tool {
	return mcp.NewTool("summarize_workflows",
		mcp.WithDescription("Agent-centric view of every running workflow: each agent with the triggers, connectors, tools, and MCPs that drive it. Triggers pointing at missing agents appear under orphanedTriggers."),
	)
}

func (w *WorkflowTools) SummarizeWorkflows(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agentsBody, err := hubGet(ctx, w.HTTPClient, w.HubURL, "/api/v1/agents")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list agents: %v", err)), nil
	}
	triggersBody, err := hubGet(ctx, w.HTTPClient, w.HubURL, "/api/v1/triggers")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list triggers: %v", err)), nil
	}

	agents, err := decodeItems(agentsBody)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse agents: %v", err)), nil
	}
	triggers, err := decodeItems(triggersBody)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse triggers: %v", err)), nil
	}

	// Build per-agent triggers map keyed by agentRef (agent id).
	triggersByAgent := map[string][]map[string]any{}
	knownAgents := map[string]bool{}
	for _, a := range agents {
		id := agentID(a)
		if id == "" {
			continue
		}
		knownAgents[id] = true
		triggersByAgent[id] = []map[string]any{}
	}

	var orphaned []map[string]any
	for _, tr := range triggers {
		// The hub's SimpleTriggerResponse is flat: id, name, agentRef, etc.
		agentRefName, _ := tr["agentRef"].(string)
		summary := map[string]any{
			"id":          tr["id"],
			"displayName": tr["name"],
			"connector":   tr["connectorRef"],
			"filters":     tr["filters"],
		}
		if knownAgents[agentRefName] {
			triggersByAgent[agentRefName] = append(triggersByAgent[agentRefName], summary)
		} else {
			summary["agentRef"] = agentRefName
			orphaned = append(orphaned, summary)
		}
	}

	out := struct {
		Agents           []map[string]any `json:"agents"`
		OrphanedTriggers []map[string]any `json:"orphanedTriggers,omitempty"`
	}{
		Agents: make([]map[string]any, 0, len(agents)),
	}
	for _, a := range agents {
		id := agentID(a)
		if id == "" {
			continue
		}
		// SimpleAgentResponse is flat: id, name, imageRef, llm, persona, enabledTools, scaling.
		imageRef, _ := a["imageRef"].(map[string]any)
		llm, _ := a["llm"].(map[string]any)
		entry := map[string]any{
			"agent":        id,
			"displayName":  a["name"],
			"image":        imageRef["name"],
			"model":        llm["model"],
			"personaRef":   personaRef(a),
			"enabledTools": a["enabledTools"],
			"enabledMCPs":  a["enabledMCPs"],
			"scaling":      a["scaling"],
			"triggers":     triggersByAgent[id],
		}
		out.Agents = append(out.Agents, entry)
	}
	out.OrphanedTriggers = orphaned

	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal output: %v", err)), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

// decodeItems handles both the hub's paginated wrapper
// ({"items":[...], "total":..., ...}) and a bare top-level array. The
// MCP server is designed to follow whatever the hub returns; supporting
// both keeps these tools resilient if the hub flattens responses later.
//
// An object that looks like the wrapper but is missing the items key, or
// has items: null (e.g. {"items":null,"total":0}), is treated as an empty
// result set rather than an error — the hub's serializer can legitimately
// produce that shape on empty pages.
func decodeItems(body []byte) ([]map[string]any, error) {
	return decodeWrapped(body, "items")
}

// decodeWrapped is the shared parser for the hub's paginated list shapes.
// keyName is the JSON wrapper key ("items", "invocations", ...). Using a
// *[]map[string]any lets us distinguish "key missing" from "key present
// but explicitly null" and "key present with empty array".
func decodeWrapped(body []byte, keyName string) ([]map[string]any, error) {
	// First try to parse as a JSON object and look for the wrapper key.
	var asObject map[string]json.RawMessage
	if err := json.Unmarshal(body, &asObject); err == nil {
		raw, present := asObject[keyName]
		if !present {
			// Wrapper-shaped object without the expected key (e.g. {"total":0}).
			return []map[string]any{}, nil
		}
		var items *[]map[string]any
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("hub returned unparseable JSON for %q: %s", keyName, string(body))
		}
		if items == nil {
			// Explicit null.
			return []map[string]any{}, nil
		}
		return *items, nil
	}
	// Fallback: a bare top-level array (forward-compat if the hub flattens).
	var bare []map[string]any
	if err := json.Unmarshal(body, &bare); err == nil {
		return bare, nil
	}
	return nil, fmt.Errorf("hub returned unparseable JSON: %s", string(body))
}

// agentID extracts the stable identifier for an agent in the hub response.
// SimpleAgentResponse uses the K8s resource name as "id"; older callers may
// see it as metadata.name on a raw CRD. We try both.
func agentID(a map[string]any) string {
	if id, ok := a["id"].(string); ok && id != "" {
		return id
	}
	if meta, ok := a["metadata"].(map[string]any); ok {
		if name, ok := meta["name"].(string); ok {
			return name
		}
	}
	return ""
}

// personaRef inspects a hub agent response and returns a small descriptor of
// the persona source. The hub's SimpleAgentResponse uses a flat persona shape
// (inline + configMapRef-name string); we accept that and the richer
// {configMapRef:{name,key}} object form for forward-compatibility.
func personaRef(a map[string]any) map[string]any {
	persona, ok := a["persona"].(map[string]any)
	if !ok {
		return nil
	}
	if cmRef, ok := persona["configMapRef"].(map[string]any); ok {
		return map[string]any{
			"type": "configMap",
			"name": cmRef["name"],
			"key":  cmRef["key"],
		}
	}
	if cmName, ok := persona["configMapRef"].(string); ok && cmName != "" {
		return map[string]any{
			"type": "configMap",
			"name": cmName,
		}
	}
	if inline, ok := persona["inline"].(string); ok && inline != "" {
		return map[string]any{"type": "inline", "inline": true}
	}
	return nil
}

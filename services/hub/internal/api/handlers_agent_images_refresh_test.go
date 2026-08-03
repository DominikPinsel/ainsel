package api

import (
	"testing"

	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

func TestMergeMCPTools_PreservesEnabledState(t *testing.T) {
	existing := []agentv1alpha1.AgentImageTool{
		{Name: "run_shell", Kind: agentv1alpha1.AgentImageToolKindShell},
		{Name: "mcp__example-mcp__list", Kind: agentv1alpha1.AgentImageToolKindMCP, McpSource: "example-mcp"},
		{Name: "mcp__example-mcp__add", Kind: agentv1alpha1.AgentImageToolKindMCP, McpSource: "example-mcp", Disabled: true},
	}
	freshByServer := map[string][]mcpToolSummary{
		"example-mcp": {
			{Name: "list", Description: "List memories"},
			{Name: "add", Description: "Add memory"},
			{Name: "delete", Description: "Delete memory"}, // new tool
		},
	}

	got := mergeMCPTools(existing, freshByServer, []string{"example-mcp"})

	// native tool preserved
	if len(got) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(got))
	}
	if got[0].Name != "run_shell" {
		t.Errorf("expected run_shell first, got %s", got[0].Name)
	}
	// mcp__example-mcp__list: was not disabled, should remain not disabled
	listTool := findTool(got, "mcp__example-mcp__list")
	if listTool == nil {
		t.Fatal("mcp__example-mcp__list missing")
	}
	if listTool.Disabled {
		t.Error("mcp__example-mcp__list should remain enabled (Disabled=false)")
	}
	// mcp__example-mcp__add: was explicitly disabled, should stay disabled
	addTool := findTool(got, "mcp__example-mcp__add")
	if addTool == nil {
		t.Fatal("mcp__example-mcp__add missing")
	}
	if !addTool.Disabled {
		t.Error("mcp__example-mcp__add should remain disabled")
	}
	// mcp__example-mcp__delete: new — should be disabled and marked IsNew
	delTool := findTool(got, "mcp__example-mcp__delete")
	if delTool == nil {
		t.Fatal("mcp__example-mcp__delete missing")
	}
	if !delTool.Disabled {
		t.Error("new tool mcp__example-mcp__delete should start disabled")
	}
	if !delTool.IsNew {
		t.Error("new tool mcp__example-mcp__delete should be marked IsNew")
	}
}

func TestMergeMCPTools_RemovesGoneTools(t *testing.T) {
	existing := []agentv1alpha1.AgentImageTool{
		{Name: "mcp__example-mcp__list", Kind: agentv1alpha1.AgentImageToolKindMCP, McpSource: "example-mcp"},
		{Name: "mcp__example-mcp__gone", Kind: agentv1alpha1.AgentImageToolKindMCP, McpSource: "example-mcp"},
	}
	freshByServer := map[string][]mcpToolSummary{
		"example-mcp": {
			{Name: "list", Description: "List memories"},
			// "gone" is no longer on the server
		},
	}

	got := mergeMCPTools(existing, freshByServer, []string{"example-mcp"})

	if findTool(got, "mcp__example-mcp__gone") != nil {
		t.Error("mcp__example-mcp__gone should have been removed")
	}
	if findTool(got, "mcp__example-mcp__list") == nil {
		t.Error("mcp__example-mcp__list should still be present")
	}
}

func TestMergeMCPTools_DoesNotClearIsNewOnExisting(t *testing.T) {
	// A tool that was already IsNew (user hasn't saved yet) should keep IsNew
	// through a second refresh if it still exists on the server.
	existing := []agentv1alpha1.AgentImageTool{
		{Name: "mcp__example-mcp__new", Kind: agentv1alpha1.AgentImageToolKindMCP, McpSource: "example-mcp", Disabled: true, IsNew: true},
	}
	freshByServer := map[string][]mcpToolSummary{
		"example-mcp": {{Name: "new", Description: "new tool"}},
	}

	got := mergeMCPTools(existing, freshByServer, []string{"example-mcp"})
	tool := findTool(got, "mcp__example-mcp__new")
	if tool == nil {
		t.Fatal("tool missing")
	}
	if !tool.IsNew {
		t.Error("IsNew should be preserved across re-refresh")
	}
}

func findTool(tools []agentv1alpha1.AgentImageTool, name string) *agentv1alpha1.AgentImageTool {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}

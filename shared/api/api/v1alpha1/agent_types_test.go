package v1alpha1

import (
	"testing"
)

func TestAgentSpecDeepCopyEnabledMCPs(t *testing.T) {
	src := &AgentSpec{
		DisplayName: "test",
		ImageRef:    AgentImageRef{Name: "img"},
		Runtime:     AgentRuntime{},
		LLM:         AgentLLM{Model: "qwen"},
		Persona:     AgentPersona{ID: "01HXTESTPERSONA00000000000"},
		EnabledMCPs: []string{"example-mcp", "github"},
	}
	dst := src.DeepCopy()

	if dst == src {
		t.Fatal("DeepCopy returned same pointer")
	}
	if len(dst.EnabledMCPs) != 2 || dst.EnabledMCPs[0] != "example-mcp" || dst.EnabledMCPs[1] != "github" {
		t.Fatalf("EnabledMCPs not copied: %+v", dst.EnabledMCPs)
	}

	dst.EnabledMCPs[0] = "changed"
	if src.EnabledMCPs[0] == "changed" {
		t.Fatal("EnabledMCPs slice was aliased, not deep-copied")
	}

	dst.EnabledMCPs = append(dst.EnabledMCPs, "linear")
	if len(src.EnabledMCPs) != 2 {
		t.Fatalf("appending to copy changed original len: got %d want 2", len(src.EnabledMCPs))
	}
}

func TestAgentSpecDeepCopyEnabledMCPsNil(t *testing.T) {
	src := &AgentSpec{
		DisplayName: "test",
		ImageRef:    AgentImageRef{Name: "img"},
		Runtime:     AgentRuntime{},
		LLM:         AgentLLM{Model: "qwen"},
		Persona:     AgentPersona{ID: "01HXTESTPERSONA00000000000"},
	}
	dst := src.DeepCopy()
	if dst.EnabledMCPs != nil {
		t.Fatalf("nil EnabledMCPs became non-nil after DeepCopy: %+v", dst.EnabledMCPs)
	}
}

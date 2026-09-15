package v1alpha1

import (
	"testing"
)

func TestAgentSpecDeepCopyEnv(t *testing.T) {
	src := &AgentSpec{
		DisplayName: "test",
		ImageRef:    AgentImageRef{Name: "img"},
		Runtime:     AgentRuntime{},
		LLM:         AgentLLM{Model: "qwen"},
		Persona:     AgentPersona{ID: "01HXTESTPERSONA00000000000"},
		Env: []AgentEnvVar{
			{Name: "LOG_LEVEL", Value: "info"},
			{Name: "SESSION_TTL", Value: "3600", Secret: true},
		},
	}
	dst := src.DeepCopy()

	if dst == src {
		t.Fatal("DeepCopy returned same pointer")
	}
	if len(dst.Env) != 2 || dst.Env[0].Name != "LOG_LEVEL" || !dst.Env[1].Secret {
		t.Fatalf("Env not copied: %+v", dst.Env)
	}

	dst.Env[0].Value = "debug"
	if src.Env[0].Value == "debug" {
		t.Fatal("Env slice was aliased, not deep-copied")
	}

	dst.Env = append(dst.Env, AgentEnvVar{Name: "EXTRA"})
	if len(src.Env) != 2 {
		t.Fatalf("appending to copy changed original len: got %d want 2", len(src.Env))
	}
}

func TestAgentSpecDeepCopyEnvNil(t *testing.T) {
	src := &AgentSpec{
		DisplayName: "test",
		ImageRef:    AgentImageRef{Name: "img"},
		Runtime:     AgentRuntime{},
		LLM:         AgentLLM{Model: "qwen"},
		Persona:     AgentPersona{ID: "01HXTESTPERSONA00000000000"},
	}
	dst := src.DeepCopy()
	if dst.Env != nil {
		t.Fatalf("nil Env became non-nil after DeepCopy: %+v", dst.Env)
	}
}

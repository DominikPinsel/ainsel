package controller

import (
	"testing"

	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

// envNames flattens an env list to "name=value" pairs so assertions read as
// the merge result rather than as struct comparisons.
func envPairs(in []ainselv1alpha1.AgentImageEnvVar) []string {
	out := make([]string, 0, len(in))
	for _, e := range in {
		out = append(out, e.Name+"="+e.Value)
	}
	return out
}

func equalPairs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestEffectiveEnv(t *testing.T) {
	image := &ainselv1alpha1.AgentImage{
		Spec: ainselv1alpha1.AgentImageSpec{
			Env: []ainselv1alpha1.AgentImageEnvVar{
				{Name: "LOG_LEVEL", Value: "info"},
				{Name: "SHARED_ONLY", Value: "yes"},
			},
		},
	}
	agentWith := func(env ...ainselv1alpha1.AgentEnvVar) *ainselv1alpha1.Agent {
		return &ainselv1alpha1.Agent{Spec: ainselv1alpha1.AgentSpec{Env: env}}
	}

	t.Run("no overrides returns the image env unchanged", func(t *testing.T) {
		got := effectiveEnv(&ainselv1alpha1.Agent{}, image)
		if !equalPairs(envPairs(got), []string{"LOG_LEVEL=info", "SHARED_ONLY=yes"}) {
			t.Errorf("got %v", envPairs(got))
		}
	})

	t.Run("matching name overrides the image value in place", func(t *testing.T) {
		got := effectiveEnv(agentWith(
			ainselv1alpha1.AgentEnvVar{Name: "LOG_LEVEL", Value: "debug"},
		), image)
		// Order follows the image so the rendered Secret stays stable.
		if !equalPairs(envPairs(got), []string{"LOG_LEVEL=debug", "SHARED_ONLY=yes"}) {
			t.Errorf("got %v", envPairs(got))
		}
	})

	t.Run("agent-only names are appended in request order", func(t *testing.T) {
		got := effectiveEnv(agentWith(
			ainselv1alpha1.AgentEnvVar{Name: "AGENT_B", Value: "b"},
			ainselv1alpha1.AgentEnvVar{Name: "AGENT_A", Value: "a"},
		), image)
		if !equalPairs(envPairs(got), []string{
			"LOG_LEVEL=info", "SHARED_ONLY=yes", "AGENT_B=b", "AGENT_A=a",
		}) {
			t.Errorf("got %v", envPairs(got))
		}
	})

	t.Run("override and addition combine", func(t *testing.T) {
		got := effectiveEnv(agentWith(
			ainselv1alpha1.AgentEnvVar{Name: "SHARED_ONLY", Value: "no"},
			ainselv1alpha1.AgentEnvVar{Name: "AGENT_ONLY", Value: "1"},
		), image)
		if !equalPairs(envPairs(got), []string{
			"LOG_LEVEL=info", "SHARED_ONLY=no", "AGENT_ONLY=1",
		}) {
			t.Errorf("got %v", envPairs(got))
		}
	})

	t.Run("duplicate agent names keep the last value once", func(t *testing.T) {
		got := effectiveEnv(agentWith(
			ainselv1alpha1.AgentEnvVar{Name: "LOG_LEVEL", Value: "warn"},
			ainselv1alpha1.AgentEnvVar{Name: "LOG_LEVEL", Value: "debug"},
		), image)
		if !equalPairs(envPairs(got), []string{"LOG_LEVEL=debug", "SHARED_ONLY=yes"}) {
			t.Errorf("got %v", envPairs(got))
		}
	})

	t.Run("the agent entry supplies the secret flag", func(t *testing.T) {
		got := effectiveEnv(agentWith(
			ainselv1alpha1.AgentEnvVar{Name: "LOG_LEVEL", Value: "s3cret", Secret: true},
		), image)
		if len(got) != 2 || !got[0].Secret {
			t.Fatalf("expected the override to carry Secret=true, got %+v", got)
		}
	})

	t.Run("agent env alone works with an image that defines none", func(t *testing.T) {
		empty := &ainselv1alpha1.AgentImage{}
		got := effectiveEnv(agentWith(
			ainselv1alpha1.AgentEnvVar{Name: "AGENT_ONLY", Value: "1"},
		), empty)
		if !equalPairs(envPairs(got), []string{"AGENT_ONLY=1"}) {
			t.Errorf("got %v", envPairs(got))
		}
	})

	t.Run("neither side defining env yields none", func(t *testing.T) {
		if got := effectiveEnv(&ainselv1alpha1.Agent{}, &ainselv1alpha1.AgentImage{}); len(got) != 0 {
			t.Errorf("got %v, want empty", envPairs(got))
		}
	})
}

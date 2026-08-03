package personas

import "testing"

// TestPersonaConfigMapName pins the format of the persona ConfigMap name.
// The hub (services/hub/internal/personas) renders ConfigMaps under this
// name and the agent operator (operators/agent/internal/controller) mounts
// them by the same name. If this test changes, both call sites must change
// in lockstep.
func TestPersonaConfigMapName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "typical id", id: "01HXTESTPERSONA00000000000", want: "persona-01HXTESTPERSONA00000000000"},
		{name: "short id", id: "abc", want: "persona-abc"},
		{name: "empty id", id: "", want: "persona-"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := PersonaConfigMapName(tc.id); got != tc.want {
				t.Fatalf("PersonaConfigMapName(%q) = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}

func TestConfigMapNamePrefix(t *testing.T) {
	t.Parallel()
	if ConfigMapNamePrefix != "persona-" {
		t.Fatalf("ConfigMapNamePrefix = %q, want %q", ConfigMapNamePrefix, "persona-")
	}
}

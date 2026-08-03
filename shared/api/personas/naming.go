// Package personas exposes shared naming helpers for persona-related
// Kubernetes resources rendered by the hub and consumed by operators.
//
// Keeping the format in one place prevents producer (hub) and consumer
// (agent operator) from drifting apart on the ConfigMap name layout.
package personas

// ConfigMapNamePrefix is the constant prefix used for persona ConfigMap
// names. Exposed primarily so tests can pin it without re-deriving it.
const ConfigMapNamePrefix = "persona-"

// PersonaConfigMapName returns the deterministic ConfigMap name the hub
// renders for a persona with the given id. The agent operator mounts the
// ConfigMap under the same name when materialising agent pods.
func PersonaConfigMapName(id string) string {
	return ConfigMapNamePrefix + id
}

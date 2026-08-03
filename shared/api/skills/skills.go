// Package skills exposes shared naming helpers for skill-related
// Kubernetes resources rendered by the hub and consumed by operators.
//
// Keeping the format in one place prevents producer (hub) and consumer
// (agent operator) from drifting apart on the ConfigMap name.
package skills

// ConfigMapName is the name of the single global ConfigMap that the hub
// reconciler maintains with all skills as data keys. The agent operator
// mounts this ConfigMap (with items filtering) into agent pods.
const ConfigMapName = "skills"

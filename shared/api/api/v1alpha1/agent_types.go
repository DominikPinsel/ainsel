package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentSpec defines the desired state of an Agent.
type AgentSpec struct {
	DisplayName  string        `json:"displayName"`
	Description  string        `json:"description,omitempty"`
	ImageRef     AgentImageRef `json:"imageRef"`
	Runtime      AgentRuntime  `json:"runtime"`
	LLM          AgentLLM      `json:"llm"`
	Persona      AgentPersona  `json:"persona"`
	EnabledTools []string      `json:"enabledTools,omitempty"`
	// EnabledMCPs lists the names of MCPServer registry entries this agent
	// should connect to at runtime. Names refer to MCPServer rows managed
	// by the hub backend; the agent operator injects URLs into the agent
	// pod as MCP_SERVERS env. See docs/superpowers/specs/2026-05-19-mcp-registry-design.md.
	EnabledMCPs []string          `json:"enabledMCPs,omitempty"`
	Scaling     *AgentScaling     `json:"scaling,omitempty"`
	Memory      *AgentMemory      `json:"memory,omitempty"`
	OllamaCloud     *AgentOllamaCloud     `json:"ollamaCloud,omitempty"`
	OpenCode        *AgentOpenCode        `json:"openCode,omitempty"`
	AlibabaCloud    *AgentAlibabaCloud    `json:"alibabaCloud,omitempty"`
	CustomProvider  *AgentCustomProvider  `json:"customProvider,omitempty"`
}

// AgentImageRef references an AgentImage by metadata name in the same namespace.
type AgentImageRef struct {
	Name string `json:"name"`
}

// AgentRuntime holds operator-managed runtime configuration for the agent pod.
// Fields like ImagePullPolicy and Resources are set by the operator or Helm
// bootstrap — they are not exposed via the hub REST API.
type AgentRuntime struct {
	ImagePullPolicy corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
	Resources       corev1.ResourceRequirements `json:"resources,omitempty"`
	// SecurityHardened controls whether the operator applies pod and container
	// security hardening (readOnlyRootFilesystem, runAsNonRoot, drop ALL
	// capabilities, seccomp RuntimeDefault, etc.) to the agent Deployment.
	// Defaults to true when nil. Set to false to relax hardening for agent
	// images that require it (e.g. sidecar images that must run as root).
	// +optional
	SecurityHardened *bool `json:"securityHardened,omitempty"`
}

// AgentLLMProvider enumerates the supported LLM provider backends.
// The controller translates each provider into the corresponding Pi CLI
// models.json entry and --provider flag.
const (
	AgentLLMProviderOllamaCloud  = "ollama-cloud"
	AgentLLMProviderOpenCode     = "opencode"
	AgentLLMProviderAlibabaCloud = "alibaba-cloud"
	AgentLLMProviderCustom       = "custom"
)

type AgentLLM struct {
	Model       string   `json:"model"`
	Provider    string   `json:"provider,omitempty"`
	MaxTurns    int      `json:"maxTurns,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

// AgentPersona points the agent runtime at a persona managed by the hub.
type AgentPersona struct {
	// ID is the ULID of a persona managed by the hub.
	// The agent operator mounts a ConfigMap named persona-<id>
	// (rendered by the hub) at /etc/agent/persona.md.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`
}

// AgentScaling holds the desired replica count for the agent deployment.
type AgentScaling struct {
	Replicas *int32 `json:"replicas,omitempty"`
}

type AgentMemory struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider,omitempty"`
}

// AgentOllamaCloud configures the Ollama Cloud provider.
// The APIKey is stored in a secret created by the hub backend.
type AgentOllamaCloud struct {
	// APIKeySecretRef references the secret containing the Ollama Cloud API key.
	// The secret must have a key named "api-key".
	APIKeySecretRef *corev1.SecretKeySelector `json:"apiKeySecretRef,omitempty"`
}

// AgentOpenCode configures the OpenCode provider.
// The APIKey is stored in a secret created by the hub backend.
type AgentOpenCode struct {
	// APIKeySecretRef references the secret containing the OpenCode API key.
	// The secret must have a key named "api-key".
	APIKeySecretRef *corev1.SecretKeySelector `json:"apiKeySecretRef,omitempty"`
}

// AgentAlibabaCloud configures the Alibaba Token Plan provider.
// The APIKey is stored in a secret created by the hub backend.
type AgentAlibabaCloud struct {
	// APIKeySecretRef references the secret containing the Alibaba Token Plan API key.
	// The secret must have a key named "api-key".
	APIKeySecretRef *corev1.SecretKeySelector `json:"apiKeySecretRef,omitempty"`
}

// AgentCustomProvider configures a custom LLM provider.
// The URL is the OpenAI-compatible API endpoint and the API key is stored
// in a secret created by the hub backend.
type AgentCustomProvider struct {
	// URL is the base URL of the custom LLM API (e.g. https://api.openai.com/v1).
	URL string `json:"url"`
	// APIKeySecretRef references the secret containing the custom provider API key.
	// The secret must have a key named "api-key".
	APIKeySecretRef *corev1.SecretKeySelector `json:"apiKeySecretRef,omitempty"`
}

// AgentStatus defines the observed state of an Agent.
type AgentStatus struct {
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	Replicas           int32              `json:"replicas,omitempty"`
	LastInvocation     *metav1.Time       `json:"lastInvocation,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

const (
	AgentConditionReady                 = "Ready"
	AgentConditionDeploymentReady       = "DeploymentReady"
	AgentConditionConsumerReady         = "ConsumerReady"
	AgentConditionMCPDiscoveryComplete  = "MCPDiscoveryComplete"
	AgentConditionPersonaConfigMapReady = "PersonaConfigMapReady"
	AgentConditionImageEnvSecretReady   = "ImageEnvSecretReady"
	AgentConditionDegraded              = "Degraded"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.replicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Agent is the Schema for the agents API.
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentList contains a list of Agent.
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}


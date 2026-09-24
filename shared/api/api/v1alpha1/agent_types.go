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
	// Skills explicitly selects the skill library entries this agent mounts.
	// When nil, the agent runs on the referenced image's EnabledSkills — the
	// shared runtime profile's defaults; once set, the agent's list replaces
	// the image's, and an empty Items means no skills.
	// +optional
	Skills *AgentSkills `json:"skills,omitempty"`
	// MCP explicitly defines the MCP servers this agent connects to at
	// runtime, as resolved definitions (URL + token env reference), not just
	// names. When nil, the agent runs on the referenced image's MCPServers —
	// the shared runtime profile's defaults; once set, the agent's servers
	// replace the image's, and empty Servers means no MCP connections.
	// +optional
	MCP *AgentMCP `json:"mcp,omitempty"`
	// Env holds this agent's environment variable overrides, applied on top of
	// the referenced image's Env — the shared runtime profile's defaults. An
	// entry whose Name matches a profile entry overrides its value; any other
	// name is added for this agent only. Platform-managed names are ignored by
	// the operator. Absent or empty means the agent runs on the profile's
	// defaults alone.
	// +optional
	Env            []AgentEnvVar        `json:"env,omitempty"`
	Scaling        *AgentScaling        `json:"scaling,omitempty"`
	Memory         *AgentMemory         `json:"memory,omitempty"`
	OllamaCloud    *AgentOllamaCloud    `json:"ollamaCloud,omitempty"`
	OpenCode       *AgentOpenCode       `json:"openCode,omitempty"`
	AlibabaCloud   *AgentAlibabaCloud   `json:"alibabaCloud,omitempty"`
	CustomProvider *AgentCustomProvider `json:"customProvider,omitempty"`
}

// AgentImageRef references an AgentImage by metadata name in the same namespace.
type AgentImageRef struct {
	Name string `json:"name"`
}

// AgentSkills is the agent-scoped skill selection. The wrapper makes the
// distinction between "not configured" (nil = inherit the referenced
// image's EnabledSkills) and "explicitly empty" (Items: [] = no skills)
// representable in the CR — a bare []string could not.
type AgentSkills struct {
	// Items lists skill library ids (hub /skills API) to mount for this agent.
	// +optional
	Items []string `json:"items"`
}

// AgentMCPServer is one MCP server definition this agent connects to at
// runtime: the URL is resolved from the hub's MCP registry at write time,
// so the agent pod is decoupled from later registry edits.
type AgentMCPServer struct {
	// Name is the MCP registry entry name.
	Name string `json:"name"`
	// URL is the MCP server endpoint the agent runtime connects to.
	URL string `json:"url"`
	// TokenFromEnv names the env var on the agent pod whose value is sent
	// as the bearer token. Empty means the server needs no token.
	// +optional
	TokenFromEnv string `json:"tokenFromEnv,omitempty"`
}

// AgentMCP is the agent-scoped MCP selection: Servers replaces (never
// merges with) the referenced image's MCPServers when present.
type AgentMCP struct {
	// Servers lists the MCP servers this agent connects to.
	// +optional
	Servers []AgentMCPServer `json:"servers"`
}

// AgentEnvVar is one per-agent environment variable override.
//
// Unlike Skills and MCP, agent env is a plain list rather than a wrapper with
// nil/empty distinction: entries *merge* over the referenced image's Env by
// name instead of replacing it, so "not configured" and "explicitly none"
// mean the same thing — run on the profile's defaults.
type AgentEnvVar struct {
	// Name is the environment variable name. Matching a name in the
	// referenced image's Env overrides that value for this agent.
	Name string `json:"name"`
	// Value is the variable's value.
	Value string `json:"value"`
	// Secret marks Value as sensitive: the hub API masks it on read, and an
	// empty Value on write means "keep the existing value" — the same
	// contract as AgentImageEnvVar.
	// +optional
	Secret bool `json:"secret,omitempty"`
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
	// Vision declares that the model accepts image input. When true, the
	// generated pi models.json advertises "input": ["text", "image"] so
	// pi's media tools attach images instead of dropping them. Defaults to
	// false: a text-only model must never receive image payloads.
	// +optional
	Vision *bool `json:"vision,omitempty"`
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

// AgentScaling holds the pod count policy for the agent deployment.
//
// Two modes, selected by whether MinReplicas is set:
//
//   - Unset (the default, and the only mode that existed before scale-to-zero):
//     the operator pins exactly Replicas pods, regardless of queue depth.
//   - Set: the operator scales between MinReplicas and Replicas from the queue
//     depth the hub publishes on AgentStatus, so an idle agent can go dormant
//     and a burst can fan out to the full ceiling.
type AgentScaling struct {
	// Replicas is the maximum number of agent pods running at once. An agent
	// runtime claims exactly one task at a time, so this also caps concurrent
	// work for the agent. Defaults to 1.
	// +optional
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// MinReplicas is the floor for queue-driven scaling, and opting into it:
	// setting this field is what switches the agent out of static pinning.
	// 0 allows the agent to go dormant between tasks. Must not exceed Replicas.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinReplicas *int32 `json:"minReplicas,omitempty"`
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
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// Replicas is the number of ready pods, as observed by the operator.
	Replicas int32 `json:"replicas,omitempty"`

	// The four fields below are the queue signal the operator scales on. They
	// are written by the hub, which owns the task store, and read by the
	// operator; everything else in status is operator-owned. Both writers patch
	// only their own fields, so neither clobbers the other.

	// LastInvocation is when the hub last enqueued a task for this agent. It
	// is the idle clock for scaling down: quiet is measured from the last piece
	// of work arriving, not from the last pod exiting.
	LastInvocation *metav1.Time `json:"lastInvocation,omitempty"`

	// PendingTasks is agent_tasks rows waiting to be claimed.
	PendingTasks int32 `json:"pendingTasks,omitempty"`

	// ActiveTasks is agent_tasks rows currently claimed. This is the drain
	// signal: a claimed task is not recovered until the reaper gives up on it
	// (TASK_CLAIM_TIMEOUT_SECONDS, 30 minutes by default), so pods holding one
	// must not be scaled away.
	ActiveTasks int32 `json:"activeTasks,omitempty"`

	// QueueObservedAt is when PendingTasks/ActiveTasks were last measured.
	// The operator treats a stale value as unknown, which forbids scaling to
	// zero: a hub that died mid-flight would otherwise leave every dormant
	// agent asleep with a queue full of work nobody will wake them for.
	QueueObservedAt *metav1.Time `json:"queueObservedAt,omitempty"`

	// Scaling explains the pod count the operator is driving toward. It exists
	// because zero pods is now a valid steady state, and a reader needs to tell
	// "asleep, waiting for work" from "broken".
	// +optional
	Scaling *AgentScalingStatus `json:"scaling,omitempty"`

	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// AgentScalingStatus is the operator's account of what it is doing with this
// agent's pods.
type AgentScalingStatus struct {
	// Mode is "static" when the agent keeps a fixed pod count (no minReplicas
	// set) or "queue" when pod count follows queue depth.
	Mode string `json:"mode,omitempty"`
	// Desired is the pod count the operator is converging on. Compare against
	// status.replicas to see whether it is still catching up.
	Desired int32 `json:"desired,omitempty"`
	// Reason is a stable code for the decision: Static, QueueDepth, Dormant,
	// ScaledToZero, IdleGrace, QueueSignalStale or Disabled.
	Reason string `json:"reason,omitempty"`
	// Message is a one-line human-readable explanation, safe to show in a UI.
	// +optional
	Message string `json:"message,omitempty"`
}

// Scaling mode values used by AgentScalingStatus.Mode.
const (
	ScalingModeStatic = "static"
	ScalingModeQueue  = "queue"
)

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

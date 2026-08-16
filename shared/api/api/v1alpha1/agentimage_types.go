package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentImageSpec defines the desired state of an AgentImage.
type AgentImageSpec struct {
	DisplayName   string                `json:"displayName"`
	Description   string                `json:"description,omitempty"`
	ImageURL      string                `json:"imageURL"`
	Tools         []AgentImageTool      `json:"tools,omitempty"`
	Env           []AgentImageEnvVar    `json:"env,omitempty"`
	MCPServers    []AgentImageMCPServer `json:"mcpServers,omitempty"`
	Sidecars      []AgentImageSidecar  `json:"sidecars,omitempty"`
	EnabledSkills []string              `json:"enabledSkills,omitempty"`
}

// AgentImageEnvVar is a name/value environment variable injected into every
// agent pod that uses this image. These act as defaults: any variable already
// set by the operator (e.g. FORGEJO_URL from a wired connector) takes
// precedence.
//
// When Secret is true the value is treated as sensitive. The Hub API never
// returns the value for a secret env var; the frontend renders a masked
// input. On update, sending value "" for a secret env var means "keep the
// existing value".
type AgentImageEnvVar struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret,omitempty"`
}

// AgentImageTool describes a single capability exposed by the image.
type AgentImageTool struct {
	Name        string                  `json:"name"`
	Kind        AgentImageToolKind      `json:"kind"`
	Description string                  `json:"description,omitempty"`
	McpSource   string                  `json:"mcpSource,omitempty"`
	Disabled    bool                    `json:"disabled,omitempty"` // false = enabled (zero-value backward compat)
	IsNew       bool                    `json:"isNew,omitempty"`    // true = added by last refresh, cleared on next PUT
	Examples    []AgentImageToolExample `json:"examples,omitempty"`
}

// +kubebuilder:validation:Enum=container;shell;mcp
type AgentImageToolKind string

const (
	AgentImageToolKindContainer AgentImageToolKind = "container"
	AgentImageToolKindShell     AgentImageToolKind = "shell"
	AgentImageToolKindMCP       AgentImageToolKind = "mcp"
)

type AgentImageToolExample struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// AgentImageMCPServer configures an external MCP server whose tools are
// imported into the image tool catalog via an explicit refresh.
type AgentImageMCPServer struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	// TokenFromEnv is the name of an agent-pod env var whose value is
	// forwarded as the bearer token to this MCP server. Empty means anonymous.
	// +optional
	// +kubebuilder:validation:Pattern=`^[A-Z_][A-Z0-9_]*$`
	// +kubebuilder:validation:MaxLength=64
	TokenFromEnv string `json:"tokenFromEnv,omitempty"`
}

// AgentImageSidecar configures a sidecar container injected into every
// agent pod that uses this image. Sidecars run alongside the agent runtime
// in the same pod and share its network namespace (accessible via localhost).
//
// When a sidecar's MCPPath is non-empty, the operator automatically appends
// an entry to MCP_SERVERS in the form "<name>=http://localhost:<port><mcpPath>".
type AgentImageSidecar struct {
	// Name is the container name and the MCP server name used in MCP_SERVERS.
	Name string `json:"name"`
	// Image is the container image URL.
	Image string `json:"image"`
	// Port is the container listen port. Defaults to 8080.
	// +optional
	Port int32 `json:"port,omitempty"`
	// MCPPath, when non-empty, causes the operator to append an MCP_SERVERS
	// entry pointing to this sidecar (e.g. "/mcp"). Empty means the sidecar
	// is not an MCP server and no MCP_SERVERS entry is added.
	// +optional
	MCPPath string `json:"mcpPath,omitempty"`
	// Env is a list of environment variables injected into the sidecar container.
	// Values that reference Secret keys use the same <agent>-image-env Secret
	// as the main container.
	// +optional
	Env []AgentImageEnvVar `json:"env,omitempty"`
}

// AgentImageStatus defines the observed state of an AgentImage.
type AgentImageStatus struct {
	Phase AgentImagePhase `json:"phase,omitempty"`
}

// +kubebuilder:validation:Enum=Ready
type AgentImagePhase string

const (
	AgentImagePhaseReady AgentImagePhase = "Ready"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.imageURL`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentImage is the Schema for the agentimages API.
type AgentImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentImageSpec   `json:"spec,omitempty"`
	Status AgentImageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentImageList contains a list of AgentImage.
type AgentImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentImage `json:"items"`
}


package mcpservers

import "time"

// MCPServer is a registry entry for an external MCP endpoint.
type MCPServer struct {
	Name         string
	DisplayName  string
	Description  string
	URL          string
	TokenFromEnv string // name of the env var on the agent pod whose value is sent as the bearer token
	ManagedBy    string // "user" | "helm"
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

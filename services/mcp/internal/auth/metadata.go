package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// MetadataConfig is the input for ProtectedResourceMetadataHandler.
type MetadataConfig struct {
	ResourceURL string // Externally-reachable MCP URL, e.g. https://ainsel.example.com/mcp
	Issuer      string // Zitadel OIDC issuer URL — used only for scopes_supported; authorization_servers points to the proxy
	ProjectID   string // Zitadel project ID — surfaces in scopes_supported
}

// ProtectedResourceMetadataHandler serves /.well-known/oauth-protected-resource
// per RFC 9728. Claude Code's MCP HTTP transport reads this to discover the
// authorization server and start the OAuth flow.
func ProtectedResourceMetadataHandler(cfg MetadataConfig) (http.Handler, error) {
	body := struct {
		Resource               string   `json:"resource"`
		AuthorizationServers   []string `json:"authorization_servers"`
		BearerMethodsSupported []string `json:"bearer_methods_supported"`
		ScopesSupported        []string `json:"scopes_supported"`
	}{
		Resource:               cfg.ResourceURL,
		// Point at the MCP server itself as the authorization server so
		// Claude Code discovers our registration_endpoint proxy instead
		// of going directly to Zitadel (which has no DCR endpoint).
		AuthorizationServers:   []string{cfg.ResourceURL},
		BearerMethodsSupported: []string{"header"},
		ScopesSupported: []string{
			"openid",
			"profile",
			"email",
			fmt.Sprintf("urn:zitadel:iam:org:project:id:%s:aud", cfg.ProjectID),
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("metadata encode: %w", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoded)
	}), nil
}

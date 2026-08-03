package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DominikPinsel/ainsel/services/mcp/internal/auth"
)

func TestProtectedResourceMetadataHandler(t *testing.T) {
	h, err := auth.ProtectedResourceMetadataHandler(auth.MetadataConfig{
		ResourceURL: "https://mcp.test/ainsel-dev/mcp",
		Issuer:      "https://auth.test",
		ProjectID:   "proj-123",
	})
	if err != nil {
		t.Fatalf("ProtectedResourceMetadataHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", got)
	}

	var body struct {
		Resource               string   `json:"resource"`
		AuthorizationServers   []string `json:"authorization_servers"`
		BearerMethodsSupported []string `json:"bearer_methods_supported"`
		ScopesSupported        []string `json:"scopes_supported"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v; body = %s", err, rec.Body.String())
	}
	if body.Resource != "https://mcp.test/ainsel-dev/mcp" {
		t.Errorf("resource = %q; want https://mcp.test/ainsel-dev/mcp", body.Resource)
	}
	// authorization_servers points at the MCP server itself (the OAuth proxy),
	// not directly at Zitadel, so Claude Code discovers the registration_endpoint.
	if len(body.AuthorizationServers) != 1 || body.AuthorizationServers[0] != "https://mcp.test/ainsel-dev/mcp" {
		t.Errorf("authorization_servers = %v; want [https://mcp.test/ainsel-dev/mcp]", body.AuthorizationServers)
	}
	wantScope := "urn:zitadel:iam:org:project:id:proj-123:aud"
	found := false
	for _, s := range body.ScopesSupported {
		if s == wantScope {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("scopes_supported = %v; missing %q", body.ScopesSupported, wantScope)
	}
}

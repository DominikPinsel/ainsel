package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ProxyConfig configures the OAuth proxy auth server handlers.
// The proxy advertises itself as the authorization server so it can
// expose a registration_endpoint that Claude Code requires for DCR.
// It returns a pre-registered Zitadel PKCE client_id to all DCR
// requests, then proxies all real auth endpoints to Zitadel.
type ProxyConfig struct {
	// ProxyIssuer is the externally-reachable URL of the MCP server,
	// used as the "issuer" in the synthetic OpenID configuration.
	// e.g. https://ainsel.example.com/mcp
	ProxyIssuer string

	// ZitadelIssuer is the real Zitadel issuer URL.
	// e.g. https://oidc.example.com
	ZitadelIssuer string

	// ClientID is the pre-registered Zitadel PKCE application client ID
	// returned to all Dynamic Client Registration requests.
	ClientID string
}

// OIDCConfigHandler serves /.well-known/openid-configuration.
// It returns a synthetic OpenID Connect discovery document that:
//   - advertises the MCP server as the issuer (so Claude Code finds the registration_endpoint)
//   - points authorization_endpoint, token_endpoint, jwks_uri at the real Zitadel
//   - includes a registration_endpoint pointing back at this server
//
// Claude Code will do DCR against registration_endpoint, get the pre-registered
// client_id, then do PKCE authorization code flow directly with Zitadel.
// The resulting token is a real Zitadel JWT that the existing OIDC middleware
// validates without any changes.
func OIDCConfigHandler(cfg ProxyConfig) (http.Handler, error) {
	body := struct {
		Issuer                            string   `json:"issuer"`
		AuthorizationEndpoint             string   `json:"authorization_endpoint"`
		TokenEndpoint                     string   `json:"token_endpoint"`
		JwksURI                           string   `json:"jwks_uri"`
		UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
		RegistrationEndpoint              string   `json:"registration_endpoint"`
		ResponseTypesSupported            []string `json:"response_types_supported"`
		GrantTypesSupported               []string `json:"grant_types_supported"`
		SubjectTypesSupported             []string `json:"subject_types_supported"`
		TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
		CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	}{
		Issuer:                            cfg.ProxyIssuer,
		AuthorizationEndpoint:             cfg.ZitadelIssuer + "/oauth/v2/authorize",
		TokenEndpoint:                     cfg.ZitadelIssuer + "/oauth/v2/token",
		JwksURI:                           cfg.ZitadelIssuer + "/oauth/v2/keys",
		UserinfoEndpoint:                  cfg.ZitadelIssuer + "/oidc/v1/userinfo",
		RegistrationEndpoint:              cfg.ProxyIssuer + "/oauth/register",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		SubjectTypesSupported:             []string{"public"},
		TokenEndpointAuthMethodsSupported: []string{"none"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("oidc config encode: %w", err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoded)
	})
	return handler, nil
}

// OAuthASMetadataHandler serves /.well-known/oauth-authorization-server per RFC 8414.
// Content is identical to OIDCConfigHandler — this alias is needed because when
// the issuer URL has a path component, RFC 8414 mandates the AS metadata be at
// {scheme}://{host}/.well-known/oauth-authorization-server{path} (at the root of
// the domain, not under the issuer path). Claude Code follows RFC 8414, so without
// this endpoint it gets a 404/redirect and fails with "Failed to parse JSON".
func OAuthASMetadataHandler(cfg ProxyConfig) (http.Handler, error) {
	return OIDCConfigHandler(cfg)
}

// DCRHandler serves POST /oauth/register.
// It implements just enough of RFC 7591 Dynamic Client Registration to satisfy
// Claude Code: it always returns the pre-registered Zitadel PKCE client_id
// regardless of the registration request body. No actual client is created —
// the real Zitadel application was registered in advance.
func DCRHandler(cfg ProxyConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		resp := struct {
			ClientID                string   `json:"client_id"`
			ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
			RedirectURIs            []string `json:"redirect_uris,omitempty"`
			GrantTypes              []string `json:"grant_types"`
			ResponseTypes           []string `json:"response_types"`
			TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
		}{
			ClientID:                cfg.ClientID,
			ClientIDIssuedAt:        time.Now().Unix(),
			GrantTypes:              []string{"authorization_code"},
			ResponseTypes:           []string{"code"},
			TokenEndpointAuthMethod: "none",
		}

		// Echo back redirect_uris from the request if present.
		var req struct {
			RedirectURIs []string `json:"redirect_uris"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp.RedirectURIs = req.RedirectURIs

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

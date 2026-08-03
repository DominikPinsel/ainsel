package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DominikPinsel/ainsel/services/mcp/internal/auth"
)

func TestOIDCConfigHandler(t *testing.T) {
	cfg := auth.ProxyConfig{
		ProxyIssuer:   "https://mcp.test/ainsel-dev/mcp",
		ZitadelIssuer: "https://auth.test",
		ClientID:      "client-123",
	}

	h, err := auth.OIDCConfigHandler(cfg)
	if err != nil {
		t.Fatalf("OIDCConfigHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", got)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v; body = %s", err, rec.Body.String())
	}
	if body["issuer"] != cfg.ProxyIssuer {
		t.Errorf("issuer = %q; want %q", body["issuer"], cfg.ProxyIssuer)
	}
	if body["registration_endpoint"] != cfg.ProxyIssuer+"/oauth/register" {
		t.Errorf("registration_endpoint = %q; want %q", body["registration_endpoint"], cfg.ProxyIssuer+"/oauth/register")
	}
}

func TestOAuthASMetadataHandler(t *testing.T) {
	cfg := auth.ProxyConfig{
		ProxyIssuer:   "https://mcp.test/ainsel-dev/mcp",
		ZitadelIssuer: "https://auth.test",
		ClientID:      "client-123",
	}

	h, err := auth.OAuthASMetadataHandler(cfg)
	if err != nil {
		t.Fatalf("OAuthASMetadataHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server/ainsel-dev/mcp", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", got)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v; body = %s", err, rec.Body.String())
	}
	if body["issuer"] != cfg.ProxyIssuer {
		t.Errorf("issuer = %q; want %q", body["issuer"], cfg.ProxyIssuer)
	}
}

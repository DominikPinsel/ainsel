package config_test

import (
	"strings"
	"testing"

	"github.com/DominikPinsel/ainsel/services/mcp/internal/config"
)

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("OIDC_ISSUER", "https://auth.test")
	t.Setenv("OIDC_PROJECT_ID", "proj-123")
	t.Setenv("HUB_URL", "http://hub-backend.test:8080")

	cfg := config.Load()
	if cfg.OIDCIssuer != "https://auth.test" {
		t.Errorf("OIDCIssuer = %q; want https://auth.test", cfg.OIDCIssuer)
	}
	if cfg.OIDCProjectID != "proj-123" {
		t.Errorf("OIDCProjectID = %q; want proj-123", cfg.OIDCProjectID)
	}
	if cfg.HubURL != "http://hub-backend.test:8080" {
		t.Errorf("HubURL = %q; want http://hub-backend.test:8080", cfg.HubURL)
	}
}

func TestValidateRequiresOIDCFields(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr string
	}{
		{"missing issuer", func(c *config.Config) { c.OIDCIssuer = "" }, "OIDC_ISSUER"},
		{"missing project", func(c *config.Config) { c.OIDCProjectID = "" }, "OIDC_PROJECT_ID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{
				OIDCIssuer:    "x",
				OIDCProjectID: "y",
			}
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() returned nil; want error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q; want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

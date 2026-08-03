package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Port                   int
	OIDCIssuer             string
	OIDCProjectID          string
	OIDCClientID           string
	MCPResourceURL         string
	HubURL                 string
	InternalValidateSecret string
}

func Load() Config {
	return Config{
		Port:                   envInt("PORT", 8080),
		OIDCIssuer:             envStr("OIDC_ISSUER", ""),
		OIDCProjectID:          envStr("OIDC_PROJECT_ID", ""),
		OIDCClientID:           envStr("OIDC_CLIENT_ID", ""),
		MCPResourceURL:         envStr("MCP_RESOURCE_URL", ""),
		HubURL:                 envStr("HUB_URL", "http://hub-backend.ainsel.svc.cluster.local:8080"),
		InternalValidateSecret: envStr("INTERNAL_VALIDATE_SECRET", ""),
	}
}

func (c Config) Validate() error {
	if c.OIDCIssuer == "" {
		return errors.New("OIDC_ISSUER is required")
	}
	if c.OIDCProjectID == "" {
		return errors.New("OIDC_PROJECT_ID is required")
	}
	return nil
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

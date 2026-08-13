package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Discovery holds the subset of the OIDC discovery document
// (RFC 8414 / OpenID Connect Discovery 1.0) that the middleware needs to
// locate the issuer's endpoints.
type Discovery struct {
	Issuer           string `json:"issuer"`
	JWKSURI          string `json:"jwks_uri"`
	UserInfoEndpoint string `json:"userinfo_endpoint"`
}

// FetchDiscovery retrieves and parses the OIDC discovery document at
// {issuer}/.well-known/openid-configuration. Reading the endpoint URLs from
// discovery (instead of hardcoding provider-specific paths) is what makes
// the middleware work with any compliant IdP — Zitadel, Keycloak, Entra ID,
// Authelia, Okta, etc.
func FetchDiscovery(ctx context.Context, client *http.Client, issuer string) (*Discovery, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	url := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc discovery: %s returned status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: read body: %w", err)
	}

	var d Discovery
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("oidc discovery: parse: %w", err)
	}
	if d.JWKSURI == "" {
		return nil, fmt.Errorf("oidc discovery: jwks_uri missing in %s", url)
	}
	return &d, nil
}

// fallbackJWKSPath and fallbackUserInfoPath are the legacy Zitadel paths.
// They are only used when discovery is unreachable at startup so that a
// transient discovery failure does not prevent the service from booting.
const (
	fallbackJWKSPath     = "/oauth/v2/keys"
	fallbackUserInfoPath = "/oauth/v2/userinfo"
)

// ResolveEndpoints returns the JWKS and userinfo endpoint URLs for an issuer.
// It prefers the OIDC discovery document; if discovery fails it falls back
// to the legacy Zitadel paths and returns the discovery error so callers can
// log it.
func ResolveEndpoints(ctx context.Context, client *http.Client, issuer string) (jwksURL, userInfoURL string, err error) {
	base := strings.TrimRight(issuer, "/")
	d, err := FetchDiscovery(ctx, client, issuer)
	if err != nil {
		return base + fallbackJWKSPath, base + fallbackUserInfoPath, err
	}
	return d.JWKSURI, d.UserInfoEndpoint, nil
}

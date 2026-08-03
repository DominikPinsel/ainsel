// Package oidc provides OIDC JWT validation middleware shared across the
// AInsel services (hub, mcp). Tokens are validated against a JWKS endpoint
// (typically the OIDC issuer's /oauth/v2/keys for Zitadel). Only RS256 is
// accepted. On success a *User derived from the token claims (enriched via
// userinfo endpoint) is attached to the request context.
package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// User is the authenticated identity extracted from a validated OIDC token.
type User struct {
	Sub      string
	Email    string
	Username string
	Roles    []string
}

// Config configures the JWT middleware.
type Config struct {
	Issuer          string        // OIDC issuer URL (matches `iss` claim)
	Audience        string        // OIDC client ID, expected in `aud`
	JWKSURL         string        // JWKS endpoint, e.g. issuer + /oauth/v2/keys
	UserInfoURL     string        // Optional: userinfo endpoint for enriched claims
	RefreshEvery    time.Duration // JWKS refresh; defaults to 1h if zero
	WWWAuthenticate string        // Optional; emitted on 401 responses.
}

// zitadelRolesClaim is the claim Zitadel uses to assert project roles. The
// value is an object whose keys are role names (the values, per-org assertions,
// are ignored here).
const zitadelRolesClaim = "urn:zitadel:iam:org:project:roles"

// ctxKey is unexported so external packages can only access the *User via
// FromContext, preventing accidental overwrite from outside the middleware.
type ctxKey struct{}
type tokenCtxKey struct{}

// userInfo enrichment holds only the claims we get from the userinfo endpoint.
// We cache this separately from the full User because roles come from the JWT
// and can change on each request (e.g., if a user is granted/revoked a role
// and receives a new access token). Caching the full User would serve stale roles.
type userInfoEnrichment struct {
	Sub       string
	Username  string
	Email     string
	expiresAt time.Time
}

// userInfoCache is a simple in-memory cache for userinfo responses, keyed by
// subject. This avoids calling the userinfo endpoint on every request while
// still keeping data reasonably fresh. We cache only userinfo-derived fields
// (Username, Email), not roles which are JWT-specific.
type userInfoCache struct {
	mu      sync.RWMutex
	entries map[string]*userInfoEnrichment
}

var globalUserInfoCache = &userInfoCache{
	entries: make(map[string]*userInfoEnrichment),
}

// cacheTTL is how long we cache userinfo responses. Short enough to stay
// reasonably fresh, long enough to avoid hammering the OIDC provider.
const cacheTTL = 5 * time.Minute

func (c *userInfoCache) get(sub string) (*userInfoEnrichment, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[sub]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry, true
}

func (c *userInfoCache) set(sub string, enrichment *userInfoEnrichment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	enrichment.expiresAt = time.Now().Add(cacheTTL)
	c.entries[sub] = enrichment
}

// ClearUserInfoCache clears the global userinfo cache. Used for testing.
func ClearUserInfoCache() {
	globalUserInfoCache.mu.Lock()
	defer globalUserInfoCache.mu.Unlock()
	globalUserInfoCache.entries = make(map[string]*userInfoEnrichment)
}

// NewMiddleware builds a net/http middleware that validates RS256 Bearer
// tokens against the issuer's JWKS and attaches the resulting *User to the
// request context. JWKS fetching happens once here (so misconfiguration fails
// fast at startup) and then refreshes on the configured interval.
//
// If UserInfoURL is configured, the middleware calls the userinfo endpoint
// to enrich the user data with claims like preferred_username that may not be
// present in the JWT. Responses are cached per-subject for 5 minutes.
func NewMiddleware(cfg Config) (func(http.Handler) http.Handler, error) {
	if cfg.JWKSURL == "" {
		return nil, errors.New("auth: JWKSURL is required")
	}
	if cfg.Issuer == "" {
		return nil, errors.New("auth: Issuer is required")
	}
	if cfg.Audience == "" {
		return nil, errors.New("auth: Audience is required")
	}
	refresh := cfg.RefreshEvery
	if refresh <= 0 {
		refresh = time.Hour
	}

	// NewDefaultOverrideCtx threads our RefreshInterval into the underlying
	// jwkset HTTP client storage; NewDefaultCtx would silently fall back to
	// keyfunc's own 1h default and ignore Config.RefreshEvery entirely.
	k, err := keyfunc.NewDefaultOverrideCtx(context.Background(), []string{cfg.JWKSURL}, keyfunc.Override{
		RefreshInterval: refresh,
	})
	if err != nil {
		return nil, fmt.Errorf("auth: load JWKS from %s: %w", cfg.JWKSURL, err)
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithAudience(cfg.Audience),
		jwt.WithExpirationRequired(),
	)

	// HTTP client for userinfo requests. Reused across requests.
	userInfoClient := &http.Client{
		Timeout: 5 * time.Second,
	}
	userInfoURL := cfg.UserInfoURL

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If a prior middleware (e.g. userTokenMW) already authenticated the
			// request and placed a user in the context, skip JWT validation.
			if _, ok := FromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}

			raw, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w, cfg.WWWAuthenticate, "missing bearer token")
				return
			}

			claims := jwt.MapClaims{}
			if _, err := parser.ParseWithClaims(raw, claims, k.Keyfunc); err != nil {
				unauthorized(w, cfg.WWWAuthenticate, "invalid token")
				return
			}

			u := userFromClaims(claims)

			// If userinfo URL is configured, enrich user data from userinfo endpoint.
			// This gets claims like preferred_username that aren't in the JWT.
			// Note: We only cache userinfo-derived fields (Username, Email), not the
			// full User, because roles come from the JWT and can change each request.
			if userInfoURL != "" {
				// Check cache first for userinfo enrichment
				if cached, ok := globalUserInfoCache.get(u.Sub); ok {
					// Merge only userinfo-derived fields, preserving JWT roles
					if cached.Username != "" {
						u.Username = cached.Username
					}
					if cached.Email != "" {
						u.Email = cached.Email
					}
				} else {
					// Fetch from userinfo endpoint
					enriched, err := fetchUserInfo(r.Context(), userInfoClient, userInfoURL, raw)
					if err == nil && enriched != nil {
						// Enrich user from endpoint response
						if enriched.Username != "" {
							u.Username = enriched.Username
						}
						if enriched.Email != "" {
							u.Email = enriched.Email
						}
						if enriched.Sub != "" {
							u.Sub = enriched.Sub
						}
						// Cache only userinfo-derived fields (not roles)
						globalUserInfoCache.set(u.Sub, enriched)
					}
					// If userinfo fails, continue with JWT-only user data
				}
			}

			ctx := context.WithValue(r.Context(), ctxKey{}, u)
			ctx = context.WithValue(ctx, tokenCtxKey{}, raw)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}, nil
}

// UserInfoError is returned when the OIDC userinfo endpoint responds with a
// non-2xx status code. It carries the upstream HTTP status so callers can
// decide how to handle the failure (e.g. graceful degradation on 401 vs
// hard failure on 500).
type UserInfoError struct {
	StatusCode int
}

func (e *UserInfoError) Error() string {
	return fmt.Sprintf("userinfo: status %d", e.StatusCode)
}

// fetchUserInfo calls the OIDC userinfo endpoint with the bearer token and
// returns enriched user data (only userinfo-derived fields, not roles).
func fetchUserInfo(ctx context.Context, client *http.Client, userInfoURL, token string) (*userInfoEnrichment, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &UserInfoError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info map[string]any
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}

	return userInfoToEnrichment(info), nil
}

// userInfoToEnrichment extracts userinfo-derived fields from the userinfo response.
// Returns only the fields that come from userinfo (Sub, Username, Email), not roles.
func userInfoToEnrichment(info map[string]any) *userInfoEnrichment {
	e := &userInfoEnrichment{}
	if sub, ok := info["sub"].(string); ok {
		e.Sub = sub
	}
	if email, ok := info["email"].(string); ok {
		e.Email = email
	}
	// Try preferred_username first, fall back to name
	if username, ok := info["preferred_username"].(string); ok {
		e.Username = username
	} else if name, ok := info["name"].(string); ok {
		e.Username = name
	}
	return e
}

// UserInfoResult holds user identity data fetched from the OIDC userinfo
// endpoint. Used by explicit sync operations; the middleware uses an internal
// caching path instead.
type UserInfoResult struct {
	Sub      string
	Username string
	Email    string
}

// FetchUserInfo calls the OIDC userinfo endpoint with the given bearer token
// and returns the result. It does not consult or update the in-memory cache.
func FetchUserInfo(ctx context.Context, client *http.Client, userInfoURL, token string) (*UserInfoResult, error) {
	e, err := fetchUserInfo(ctx, client, userInfoURL, token)
	if err != nil {
		return nil, err
	}
	return &UserInfoResult{Sub: e.Sub, Username: e.Username, Email: e.Email}, nil
}

// ClearUserInfoCacheEntry evicts a single user's cached userinfo entry,
// forcing a fresh fetch on the next request for that subject.
func ClearUserInfoCacheEntry(sub string) {
	globalUserInfoCache.mu.Lock()
	defer globalUserInfoCache.mu.Unlock()
	delete(globalUserInfoCache.entries, sub)
}

// ContextWithUser attaches a *User to the context. Intended for tests and
// internal service-to-service calls where the middleware is bypassed.
func ContextWithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// FromContext returns the authenticated user attached by the middleware, or
// (nil, false) if the request did not pass through the middleware.
func FromContext(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(ctxKey{}).(*User)
	if !ok || u == nil {
		return nil, false
	}
	return u, true
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(header[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

func userFromClaims(c jwt.MapClaims) *User {
	sub := stringClaim(c, "sub")
	// Note: preferred_username may not be in the JWT (depends on provider config).
	// If UserInfoURL is configured, we'll enrich this via userinfo endpoint.
	// We intentionally do NOT fall back to sub here; an empty username lets
	// UpsertUser preserve whatever is already in the database instead of
	// overwriting it with the raw user id.
	username := firstStringClaim(c, "preferred_username", "name")
	u := &User{
		Sub:      sub,
		Email:    firstStringClaim(c, "email"),
		Username: username,
	}
	if raw, ok := c[zitadelRolesClaim].(map[string]any); ok {
		u.Roles = make([]string, 0, len(raw))
		for role := range raw {
			u.Roles = append(u.Roles, role)
		}
	}
	return u
}

func stringClaim(c jwt.MapClaims, key string) string {
	if v, ok := c[key].(string); ok {
		return v
	}
	return ""
}

// firstStringClaim returns the value of the first non-empty claim found.
func firstStringClaim(c jwt.MapClaims, keys ...string) string {
	for _, k := range keys {
		if v := stringClaim(c, k); v != "" {
			return v
		}
	}
	return ""
}

// TokenFromContext returns the raw bearer token attached by the middleware,
// or ("", false) if the request did not pass through the middleware.
// Downstream code (e.g. an MCP→hub call) uses this to forward the caller's
// credentials.
func TokenFromContext(ctx context.Context) (string, bool) {
	t, ok := ctx.Value(tokenCtxKey{}).(string)
	if !ok || t == "" {
		return "", false
	}
	return t, true
}

// ContextWithToken returns a context with the given raw bearer token attached,
// for use by callers that want to propagate credentials to downstream services
// (e.g. an MCP→hub call). Tests use this to set up the context that the real
// middleware would have produced.
func ContextWithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenCtxKey{}, token)
}

func unauthorized(w http.ResponseWriter, wwwAuth, msg string) {
	if wwwAuth != "" {
		w.Header().Set("WWW-Authenticate", wwwAuth)
	}
	http.Error(w, msg, http.StatusUnauthorized)
}

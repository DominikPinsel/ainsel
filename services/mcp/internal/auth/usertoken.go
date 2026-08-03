package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

type tokenCacheEntry struct {
	user      *oidc.User
	expiresAt time.Time
}

type tokenCache struct {
	mu      sync.RWMutex
	entries map[string]tokenCacheEntry
}

func newTokenCache() *tokenCache {
	return &tokenCache{entries: make(map[string]tokenCacheEntry)}
}

const tokenCacheTTL = 60 * time.Second

func (c *tokenCache) get(token string) (*oidc.User, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[token]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.user, true
}

func (c *tokenCache) set(token string, user *oidc.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[token] = tokenCacheEntry{user: user, expiresAt: time.Now().Add(tokenCacheTTL)}
}

// UserTokenMiddleware wraps the given OIDC middleware. Requests carrying a
// Bearer token prefixed with "ainsel_" are validated against the hub internal
// validate endpoint and cached for 60 seconds. All other requests are handed
// to the OIDC middleware unchanged.
//
// If validateSecret is empty, all requests fall through to oidcMW (user tokens
// disabled).
func UserTokenMiddleware(hubURL, validateSecret string, oidcMW func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	if validateSecret == "" {
		return oidcMW
	}
	cache := newTokenCache()
	client := &http.Client{Timeout: 5 * time.Second}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := extractBearer(r.Header.Get("Authorization"))
			if tok == "" || !strings.HasPrefix(tok, "ainsel_") {
				oidcMW(next).ServeHTTP(w, r)
				return
			}

			if u, ok := cache.get(tok); ok {
				ctx := oidc.ContextWithUser(r.Context(), u)
				ctx = oidc.ContextWithToken(ctx, tok)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			u, err := callValidate(client, hubURL, validateSecret, tok)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			cache.set(tok, u)
			ctx := oidc.ContextWithUser(r.Context(), u)
			ctx = oidc.ContextWithToken(ctx, tok)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func callValidate(client *http.Client, hubURL, secret, token string) (*oidc.User, error) {
	body, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return nil, fmt.Errorf("marshal validate request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, hubURL+"/api/internal/user-tokens/validate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", secret)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hub validate returned %d", resp.StatusCode)
	}

	var result struct {
		UserID   string `json:"userId"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &oidc.User{Sub: result.UserID, Username: result.Username}, nil
}

func extractBearer(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

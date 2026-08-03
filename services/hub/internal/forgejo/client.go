// Package forgejo is a tiny HTTP helper for the one operation hub-backend
// needs to perform against a Forgejo instance: mint a User Token for a bot
// user via basic auth.
//
// The Forgejo API has no admin-impersonation route for token creation —
// POST /api/v1/users/{username}/tokens accepts basic auth as that user
// only. The operator therefore supplies the bot's password on the connector
// form; hub-backend uses it once to mint a write-scoped token, stores the
// token, and discards the password.
package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// apiSuffixRE matches a trailing /api/vN path component (any digit version),
// so we can strip operator-typed URLs like https://forgejo.example.com/api/v1.
var apiSuffixRE = regexp.MustCompile(`/api/v\d+$`)

// BotTokenScopes are the scopes a Forgejo bot user token needs to act on
// repositories, issues, and on its own profile (e.g. to set a commit author).
// Hardcoded — the form does not let operators choose scopes.
var BotTokenScopes = []string{"write:repository", "write:issue", "write:user"}

// Client mints tokens against a single Forgejo instance.
type Client struct {
	httpClient *http.Client
}

// NewClient returns a Client with a sane default timeout. Pass nil to use it.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{httpClient: httpClient}
}

// MintUserToken basic-auths as the named user against the given Forgejo
// instance and creates a new access token with BotTokenScopes. It returns
// only the token value (the SHA1) on success; the password is not stored,
// not returned, and not logged here.
func (c *Client) MintUserToken(ctx context.Context, forgejoURL, username, password, tokenName string) (string, error) {
	base := normalizeBase(forgejoURL)
	if base == "" {
		return "", fmt.Errorf("forgejo url is empty or not a valid URL")
	}
	if username == "" {
		return "", fmt.Errorf("username is required")
	}
	if password == "" {
		return "", fmt.Errorf("password is required")
	}
	if tokenName == "" {
		return "", fmt.Errorf("tokenName is required")
	}

	body, err := json.Marshal(map[string]any{
		"name":   tokenName,
		"scopes": BotTokenScopes,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/api/v1/users/%s/tokens", base, url.PathEscape(username))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call forgejo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode/100 != 2 {
		// Surface Forgejo's `message` if it's there, but never echo back any
		// part of the request — the caller's password should not leak through
		// error logs.
		var parsed struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(respBody, &parsed)
		if parsed.Message != "" {
			return "", fmt.Errorf("forgejo %d: %s", resp.StatusCode, parsed.Message)
		}
		return "", fmt.Errorf("forgejo returned %d", resp.StatusCode)
	}

	var parsed struct {
		SHA1 string `json:"sha1"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if parsed.SHA1 == "" {
		return "", fmt.Errorf("mint response missing sha1")
	}
	return parsed.SHA1, nil
}

// normalizeBase strips /api/v1, /api, and trailing slashes from a user-typed
// Forgejo URL so the caller can pass either the instance root, /api, or
// /api/v1 interchangeably. Returns "" for unusable inputs.
func normalizeBase(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	path := strings.TrimRight(parsed.Path, "/")
	path = apiSuffixRE.ReplaceAllString(path, "")
	path = strings.TrimSuffix(path, "/api")
	path = strings.TrimRight(path, "/")
	return parsed.Scheme + "://" + parsed.Host + path
}

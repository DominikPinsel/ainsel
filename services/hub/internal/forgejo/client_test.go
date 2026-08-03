package forgejo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeBase(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://forgejo.example.com", "https://forgejo.example.com"},
		{"https://forgejo.example.com/", "https://forgejo.example.com"},
		{"https://forgejo.example.com", "https://forgejo.example.com"},
		{"https://forgejo.example.com/", "https://forgejo.example.com"},
		{"https://forgejo.example.com/api", "https://forgejo.example.com"},
		{"https://forgejo.example.com/api/", "https://forgejo.example.com"},
		{"https://forgejo.example.com/api/v1", "https://forgejo.example.com"},
		{"https://forgejo.example.com/api/v1/", "https://forgejo.example.com"},
		{"https://forgejo.example.com/api/v2", "https://forgejo.example.com"},
		{"", ""},
		{"   ", ""},
		{"not a url", ""},
		{"/relative/path", ""},
	}
	for _, tt := range tests {
		got := normalizeBase(tt.in)
		if got != tt.want {
			t.Errorf("normalizeBase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMintUserToken_Success(t *testing.T) {
	var gotPath, gotAuthUser, gotAuthPass, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		user, pass, ok := r.BasicAuth()
		if ok {
			gotAuthUser = user
			gotAuthPass = pass
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"name":"connector-x","sha1":"abc123","scopes":["write:repository"]}`))
	}))
	defer srv.Close()

	c := NewClient(nil)
	tok, err := c.MintUserToken(context.Background(), srv.URL, "ainsel-bot", "hunter2", "connector-x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "abc123" {
		t.Errorf("token = %q, want %q", tok, "abc123")
	}
	if gotPath != "/api/v1/users/ainsel-bot/tokens" {
		t.Errorf("path = %q, want /api/v1/users/ainsel-bot/tokens", gotPath)
	}
	if gotAuthUser != "ainsel-bot" || gotAuthPass != "hunter2" {
		t.Errorf("basic auth = (%q,%q), want (ainsel-bot,hunter2)", gotAuthUser, gotAuthPass)
	}
	var parsedBody map[string]any
	if err := json.Unmarshal([]byte(gotBody), &parsedBody); err != nil {
		t.Fatalf("request body not JSON: %v", err)
	}
	if parsedBody["name"] != "connector-x" {
		t.Errorf("body name = %v, want connector-x", parsedBody["name"])
	}
	scopes, _ := parsedBody["scopes"].([]any)
	if len(scopes) != 3 {
		t.Errorf("body scopes len = %d, want 3", len(scopes))
	}
}

func TestMintUserToken_NormalizesURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"sha1":"tok"}`))
	}))
	defer srv.Close()

	c := NewClient(nil)
	// Caller passes /api/v1 by mistake — should still hit /api/v1/users/...
	_, err := c.MintUserToken(context.Background(), srv.URL+"/api/v1", "u", "p", "n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/users/u/tokens" {
		t.Errorf("path = %q, want /api/v1/users/u/tokens", gotPath)
	}
}

func TestMintUserToken_ReturnsForgejoMessageOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Wrong username or password","url":"/api/swagger"}`))
	}))
	defer srv.Close()

	c := NewClient(nil)
	_, err := c.MintUserToken(context.Background(), srv.URL, "u", "wrong", "n")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Wrong username or password") {
		t.Errorf("error %q does not contain Forgejo message", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q does not include status code", err)
	}
}

func TestMintUserToken_ValidatesInputs(t *testing.T) {
	c := NewClient(nil)
	cases := []struct {
		name              string
		base, u, p, tName string
	}{
		{"empty url", "", "u", "p", "n"},
		{"invalid url", "not a url", "u", "p", "n"},
		{"empty username", "http://x", "", "p", "n"},
		{"empty password", "http://x", "u", "", "n"},
		{"empty token name", "http://x", "u", "p", ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.MintUserToken(context.Background(), tt.base, tt.u, tt.p, tt.tName)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// Sanity check: make sure we never accidentally start sending the password
// or username outside the BasicAuth header (e.g. in query params).
func TestMintUserToken_DoesNotLeakCredsInURL(t *testing.T) {
	var seenURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenURL = r.URL.String()
		_, _ = w.Write([]byte(`{"sha1":"x"}`))
	}))
	defer srv.Close()

	_, err := NewClient(nil).MintUserToken(context.Background(), srv.URL, "alice", "supersecret", "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(seenURL, "supersecret") || strings.Contains(seenURL, "alice"+":") {
		t.Errorf("URL leaked credentials: %s", seenURL)
	}
	// Also: password must not appear base64-encoded in the URL (some misuses
	// of url.UserPassword push creds into the URL itself).
	encoded := base64.StdEncoding.EncodeToString([]byte("alice:supersecret"))
	if strings.Contains(seenURL, encoded) {
		t.Errorf("URL leaked base64-encoded creds: %s", seenURL)
	}
}

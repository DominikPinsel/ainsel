package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DominikPinsel/ainsel/services/mcp/internal/auth"
)

func TestNewMiddlewareRejectsMissingToken_AddsWWWAuthenticate(t *testing.T) {
	// auth.NewMiddleware derives JWKS from Issuer + "/oauth/v2/keys", so we
	// expose that path on the test server.
	jwksWithPath := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/v2/keys" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	t.Cleanup(jwksWithPath.Close)

	mw, err := auth.NewMiddleware(auth.Config{
		Issuer:    jwksWithPath.URL,
		ProjectID: "proj-123",
	})
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called on missing token")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rec.Code)
	}
	got := rec.Header().Get("WWW-Authenticate")
	if got != `Bearer error="invalid_token"` {
		t.Errorf("WWW-Authenticate = %q; want Bearer error=\"invalid_token\"", got)
	}
	if !strings.Contains(rec.Body.String(), "missing bearer token") {
		t.Errorf("body = %q; want contains 'missing bearer token'", rec.Body.String())
	}
}

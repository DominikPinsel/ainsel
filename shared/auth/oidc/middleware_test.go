package oidc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	return key
}

func startJWKSServer(t *testing.T, key *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	// e = 65537 = 0x010001
	e := base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01})
	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": kid,
				"n":   n,
				"e":   e,
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func signedToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return s
}

func newMiddleware(t *testing.T, jwksURL, issuer, audience string) func(http.Handler) http.Handler {
	t.Helper()
	mw, err := oidc.NewMiddleware(oidc.Config{
		Issuer:   issuer,
		Audience: audience,
		JWKSURL:  jwksURL,
	})
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}
	return mw
}

func TestMiddleware_ValidToken_PassesAndSetsContext(t *testing.T) {
	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	const issuer = "https://issuer.example.com"
	const audience = "ainsel-hub"

	mw := newMiddleware(t, jwks.URL, issuer, audience)

	var (
		sawUser   *oidc.User
		sawExists bool
	)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUser, sawExists = oidc.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	token := signedToken(t, key, kid, jwt.MapClaims{
		"iss":                "https://issuer.example.com",
		"aud":                audience,
		"sub":                "user-123",
		"email":              "alice@example.com",
		"preferred_username": "alice",
		"exp":                time.Now().Add(5 * time.Minute).Unix(),
		"iat":                time.Now().Unix(),
		"urn:zitadel:iam:org:project:roles": map[string]any{
			"admin":  map[string]any{},
			"viewer": map[string]any{},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if !sawExists || sawUser == nil {
		t.Fatalf("FromContext: want user set, got exists=%v user=%v", sawExists, sawUser)
	}
	if sawUser.Sub != "user-123" {
		t.Fatalf("Sub: want user-123, got %q", sawUser.Sub)
	}
	if sawUser.Email != "alice@example.com" {
		t.Errorf("Email: want alice@example.com, got %q", sawUser.Email)
	}
	if sawUser.Username != "alice" {
		t.Errorf("Username: want alice, got %q", sawUser.Username)
	}
	if len(sawUser.Roles) != 2 {
		t.Errorf("Roles: want 2 entries, got %v", sawUser.Roles)
	}
	gotRoles := map[string]bool{}
	for _, r := range sawUser.Roles {
		gotRoles[r] = true
	}
	if !gotRoles["admin"] || !gotRoles["viewer"] {
		t.Errorf("Roles: want {admin, viewer}, got %v", sawUser.Roles)
	}
}

func TestMiddleware_MissingAuthHeader_401(t *testing.T) {
	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	mw := newMiddleware(t, jwks.URL, "https://issuer.example.com", "ainsel-hub")

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("downstream handler must not be called when auth header is missing")
	}
}

func TestMiddleware_WrongAudience_401(t *testing.T) {
	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	mw := newMiddleware(t, jwks.URL, "https://issuer.example.com", "ainsel-hub")

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	token := signedToken(t, key, kid, jwt.MapClaims{
		"iss": "https://issuer.example.com",
		"aud": "some-other-client",
		"sub": "user-123",
		"exp": time.Now().Add(5 * time.Minute).Unix(),
		"iat": time.Now().Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("downstream handler must not be called for wrong audience")
	}
}

func TestMiddleware_ExpiredToken_401(t *testing.T) {
	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	mw := newMiddleware(t, jwks.URL, "https://issuer.example.com", "ainsel-hub")

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	token := signedToken(t, key, kid, jwt.MapClaims{
		"iss": "https://issuer.example.com",
		"aud": "ainsel-hub",
		"sub": "user-123",
		"exp": time.Now().Add(-1 * time.Minute).Unix(),
		"iat": time.Now().Add(-2 * time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("downstream handler must not be called for expired token")
	}
}

// TestMiddleware_HS256TokenRejected pins the WithValidMethods([]string{"RS256"})
// defense against the classic key-confusion attack: an attacker who knows the
// JWKS public key tries to sign an HS256 token using the PEM/raw public key
// bytes as the HMAC secret. If the parser allowed HS256, the signature math
// would validate. We require the middleware to reject it as alg-not-allowed.
func TestMiddleware_HS256TokenRejected(t *testing.T) {
	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	mw := newMiddleware(t, jwks.URL, "https://issuer.example.com", "ainsel-hub")

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Use the raw public-modulus bytes as the HMAC "secret" — this simulates
	// the attacker reusing whatever public-key material they harvested from
	// the JWKS endpoint as a symmetric key.
	hmacSecret := key.N.Bytes()

	claims := jwt.MapClaims{
		"iss": "https://issuer.example.com",
		"aud": "ainsel-hub",
		"sub": "user-123",
		"exp": time.Now().Add(5 * time.Minute).Unix(),
		"iat": time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = kid
	token, err := tok.SignedString(hmacSecret)
	if err != nil {
		t.Fatalf("SignedString(HS256): %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("downstream handler must not be called for HS256 token (key-confusion attack)")
	}
}

// TestMiddleware_NoneAlgRejected pins WithValidMethods against the alg=none
// attack: a token whose header advertises alg=none and whose signature segment
// is empty. The parser must refuse to even consider it.
func TestMiddleware_NoneAlgRejected(t *testing.T) {
	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	mw := newMiddleware(t, jwks.URL, "https://issuer.example.com", "ainsel-hub")

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	header := map[string]any{"alg": "none", "typ": "JWT", "kid": kid}
	payload := map[string]any{
		"iss": "https://issuer.example.com",
		"aud": "ainsel-hub",
		"sub": "user-123",
		"exp": time.Now().Add(5 * time.Minute).Unix(),
		"iat": time.Now().Unix(),
	}
	hb, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	pb, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	// "<header>.<payload>." — empty signature segment, as the alg=none attack
	// classically encodes it.
	token := base64.RawURLEncoding.EncodeToString(hb) + "." +
		base64.RawURLEncoding.EncodeToString(pb) + "."

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("downstream handler must not be called for alg=none token")
	}
}

// Compile-time check that FromContext on a bare context returns (nil, false).
func TestFromContext_NoUser(t *testing.T) {
	u, ok := oidc.FromContext(context.Background())
	if ok || u != nil {
		t.Fatalf("FromContext(bare ctx): want (nil,false), got (%v,%v)", u, ok)
	}
}

func TestMeHandler_ReturnsUserFromContext(t *testing.T) {
	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	const issuer = "https://issuer.example.com"
	const audience = "ainsel-hub"

	mw := newMiddleware(t, jwks.URL, issuer, audience)

	// Run the middleware in front of MeHandler so the *User lands in the
	// request context exactly the way production traffic would deliver it.
	handler := mw(oidc.MeHandler())

	token := signedToken(t, key, kid, jwt.MapClaims{
		"iss":                issuer,
		"aud":                audience,
		"sub":                "user-123",
		"email":              "alice@example.com",
		"preferred_username": "alice",
		"exp":                time.Now().Add(5 * time.Minute).Unix(),
		"iat":                time.Now().Unix(),
		"urn:zitadel:iam:org:project:roles": map[string]any{
			"admin":  map[string]any{},
			"viewer": map[string]any{},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}

	var body struct {
		Sub      string   `json:"sub"`
		Email    string   `json:"email"`
		Username string   `json:"username"`
		Roles    []string `json:"roles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v (body=%q)", err, rec.Body.String())
	}
	if body.Sub != "user-123" {
		t.Errorf("sub: want user-123, got %q", body.Sub)
	}
	if body.Email != "alice@example.com" {
		t.Errorf("email: want alice@example.com, got %q", body.Email)
	}
	if body.Username != "alice" {
		t.Errorf("username: want alice, got %q", body.Username)
	}
	if len(body.Roles) != 2 {
		t.Fatalf("roles: want 2 entries, got %v", body.Roles)
	}
	gotRoles := map[string]bool{}
	for _, r := range body.Roles {
		gotRoles[r] = true
	}
	if !gotRoles["admin"] || !gotRoles["viewer"] {
		t.Errorf("roles: want {admin, viewer}, got %v", body.Roles)
	}
}

func TestMeHandler_NoUserInContext_401(t *testing.T) {
	// Call MeHandler directly with no middleware in front — the request
	// context carries no *User, so the handler must defensively return 401.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()

	oidc.MeHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d", rec.Code)
	}
	if got, want := rec.Body.String(), "unauthorized\n"; got != want {
		t.Errorf("body: want %q, got %q", want, got)
	}
}

func TestTokenFromContext(t *testing.T) {
	key := newRSAKey(t)
	jwks := startJWKSServer(t, key, "k1")
	mwFactory, err := oidc.NewMiddleware(oidc.Config{
		Issuer:   "test-issuer",
		Audience: "test-project",
		JWKSURL:  jwks.URL,
	})
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}

	rawToken := signedToken(t, key, "k1", jwt.MapClaims{
		"iss": "test-issuer",
		"aud": "test-project",
		"exp": time.Now().Add(time.Hour).Unix(),
		"sub": "user-123",
	})

	var gotToken string
	var gotOK bool
	handler := mwFactory(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken, gotOK = oidc.TokenFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !gotOK {
		t.Fatal("TokenFromContext returned ok=false; expected true")
	}
	if gotToken != rawToken {
		t.Errorf("TokenFromContext returned %q; want %q", gotToken, rawToken)
	}
}

func TestWWWAuthenticateHeaderOnUnauthorized(t *testing.T) {
	key := newRSAKey(t)
	jwks := startJWKSServer(t, key, "k1")
	mwFactory, err := oidc.NewMiddleware(oidc.Config{
		Issuer:          "test-issuer",
		Audience:        "test-project",
		JWKSURL:         jwks.URL,
		WWWAuthenticate: `Bearer resource_metadata="https://example.test/.well-known/oauth-protected-resource"`,
	})
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}

	handler := mwFactory(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called when auth fails")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil) // no Authorization header
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rec.Code)
	}
	want := `Bearer resource_metadata="https://example.test/.well-known/oauth-protected-resource"`
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Errorf("WWW-Authenticate = %q; want %q", got, want)
	}
}

func TestWWWAuthenticateHeaderOmittedWhenUnconfigured(t *testing.T) {
	key := newRSAKey(t)
	jwks := startJWKSServer(t, key, "k1")
	mwFactory, err := oidc.NewMiddleware(oidc.Config{
		Issuer:   "test-issuer",
		Audience: "test-project",
		JWKSURL:  jwks.URL,
		// WWWAuthenticate left empty
	})
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}

	handler := mwFactory(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Errorf("WWW-Authenticate = %q; want empty when config field is empty", got)
	}
}

// TestMiddleware_UsernameFallback_Name tests that when preferred_username is
// absent but name is present, name is used as the username.
func TestMiddleware_UsernameFallback_Name(t *testing.T) {
	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	mw := newMiddleware(t, jwks.URL, "https://issuer.example.com", "ainsel-hub")

	var sawUser *oidc.User
	var sawOK bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUser, sawOK = oidc.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Token without preferred_username but with name claim
	token := signedToken(t, key, kid, jwt.MapClaims{
		"iss":   "https://issuer.example.com",
		"aud":   "ainsel-hub",
		"sub":   "user-123",
		"email": "alice@example.com",
		"name":  "Alice Smith",
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if !sawOK || sawUser == nil {
		t.Fatalf("FromContext: want user set, got ok=%v user=%v", sawOK, sawUser)
	}
	if sawUser.Username != "Alice Smith" {
		t.Errorf("Username: want 'Alice Smith', got %q", sawUser.Username)
	}
}

// TestMiddleware_UsernameFallback_Empty tests that when neither preferred_username
// nor name are present, username is left empty so UpsertUser preserves the
// existing DB value instead of overwriting it with the raw user id.
func TestMiddleware_UsernameFallback_Empty(t *testing.T) {
	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	mw := newMiddleware(t, jwks.URL, "https://issuer.example.com", "ainsel-hub")

	var sawUser *oidc.User
	var sawOK bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUser, sawOK = oidc.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Token without preferred_username or name
	token := signedToken(t, key, kid, jwt.MapClaims{
		"iss":   "https://issuer.example.com",
		"aud":   "ainsel-hub",
		"sub":   "user-123",
		"email": "alice@example.com",
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if !sawOK || sawUser == nil {
		t.Fatalf("FromContext: want user set, got ok=%v user=%v", sawOK, sawUser)
	}
	// When no username claim is present, leave it empty so the DB is not overwritten
	if sawUser.Username != "" {
		t.Errorf("Username: want empty, got %q", sawUser.Username)
	}
}

// TestMiddleware_UserInfo_EnrichesUser tests that when UserInfoURL is configured,
// the middleware calls userinfo and enriches the user data.
func TestMiddleware_UserInfo_EnrichesUser(t *testing.T) {
	oidc.ClearUserInfoCache() // Clear cache from other tests

	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	// Start a mock userinfo server
	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":                "user-123",
			"preferred_username": "dpinsel",
			"email":             "dpinsel@example.com",
		})
	}))
	defer userinfoSrv.Close()

	mw, err := oidc.NewMiddleware(oidc.Config{
		Issuer:      "https://issuer.example.com",
		Audience:    "ainsel-hub",
		JWKSURL:     jwks.URL,
		UserInfoURL: userinfoSrv.URL,
	})
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}

	var sawUser *oidc.User
	var sawOK bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUser, sawOK = oidc.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Token WITHOUT preferred_username (JWT-only, will be enriched by userinfo)
	token := signedToken(t, key, kid, jwt.MapClaims{
		"iss":   "https://issuer.example.com",
		"aud":   "ainsel-hub",
		"sub":   "user-123",
		"email": "jwt-email@example.com",
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if !sawOK || sawUser == nil {
		t.Fatalf("FromContext: want user set, got ok=%v user=%v", sawOK, sawUser)
	}
	// Username should come from userinfo enrichment
	if sawUser.Username != "dpinsel" {
		t.Errorf("Username: want 'dpinsel' (from userinfo), got %q", sawUser.Username)
	}
}

// TestMiddleware_UserInfo_UsesCache tests that repeated requests use cached userinfo.
func TestMiddleware_UserInfo_UsesCache(t *testing.T) {
	oidc.ClearUserInfoCache() // Clear cache from other tests

	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	userinfoCalls := 0
	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userinfoCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":                "user-123",
			"preferred_username": "cached-user",
		})
	}))
	defer userinfoSrv.Close()

	mw, err := oidc.NewMiddleware(oidc.Config{
		Issuer:      "https://issuer.example.com",
		Audience:    "ainsel-hub",
		JWKSURL:     jwks.URL,
		UserInfoURL: userinfoSrv.URL,
	})
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token := signedToken(t, key, kid, jwt.MapClaims{
		"iss": "https://issuer.example.com",
		"aud": "ainsel-hub",
		"sub": "user-123",
		"exp": time.Now().Add(5 * time.Minute).Unix(),
		"iat": time.Now().Unix(),
	})

	// First request - should call userinfo
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.Header.Set("Authorization", "Bearer "+token)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if userinfoCalls != 1 {
		t.Errorf("first request: want 1 userinfo call, got %d", userinfoCalls)
	}

	// Second request - should use cache
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if userinfoCalls != 1 {
		t.Errorf("second request: want 1 userinfo call (cached), got %d", userinfoCalls)
	}
}

// TestMiddleware_UserInfo_FallbackOnFailure tests that if userinfo fails,
// the middleware continues with JWT claims as fallback.
func TestMiddleware_UserInfo_FallbackOnFailure(t *testing.T) {
	oidc.ClearUserInfoCache() // Clear cache from other tests

	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	// Userinfo server that returns 500
	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer userinfoSrv.Close()

	mw, err := oidc.NewMiddleware(oidc.Config{
		Issuer:      "https://issuer.example.com",
		Audience:    "ainsel-hub",
		JWKSURL:     jwks.URL,
		UserInfoURL: userinfoSrv.URL,
	})
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}

	var sawUser *oidc.User
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUser, _ = oidc.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Token WITH preferred_username in JWT
	token := signedToken(t, key, kid, jwt.MapClaims{
		"iss":                "https://issuer.example.com",
		"aud":                "ainsel-hub",
		"sub":                "user-123",
		"email":              "jwt-email@example.com",
		"preferred_username": "jwt-username",
		"exp":                time.Now().Add(5 * time.Minute).Unix(),
		"iat":                time.Now().Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	// Should still work, using JWT claims as fallback
	if sawUser.Username != "jwt-username" {
		t.Errorf("Username: want 'jwt-username' (JWT fallback), got %q", sawUser.Username)
	}
}

// TestMiddleware_UserInfo_PreservesJWTClaims tests that the cache only stores
// userinfo fields and does NOT replace JWT-derived fields like roles.
// This is a security-critical test: roles must come from the current JWT,
// not from a cached user from a previous request.
func TestMiddleware_UserInfo_PreservesJWTClaims(t *testing.T) {
	oidc.ClearUserInfoCache()

	key := newRSAKey(t)
	const kid = "test-kid"
	jwks := startJWKSServer(t, key, kid)

	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":                "user-123",
			"preferred_username": "dpinsel",
		})
	}))
	defer userinfoSrv.Close()

	mw, err := oidc.NewMiddleware(oidc.Config{
		Issuer:      "https://issuer.example.com",
		Audience:    "ainsel-hub",
		JWKSURL:     jwks.URL,
		UserInfoURL: userinfoSrv.URL,
	})
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}

	var sawUser *oidc.User
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUser, _ = oidc.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// First request with "admin" role
	token1 := signedToken(t, key, kid, jwt.MapClaims{
		"iss":     "https://issuer.example.com",
		"aud":     "ainsel-hub",
		"sub":     "user-123",
		"exp":     time.Now().Add(5 * time.Minute).Unix(),
		"iat":     time.Now().Unix(),
		"urn:zitadel:iam:org:project:roles": map[string]any{"admin": map[string]any{}},
	})

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.Header.Set("Authorization", "Bearer "+token1)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if sawUser.Username != "dpinsel" {
		t.Errorf("request 1: Username want 'dpinsel', got %q", sawUser.Username)
	}
	if len(sawUser.Roles) != 1 || sawUser.Roles[0] != "admin" {
		t.Errorf("request 1: Roles want ['admin'], got %v", sawUser.Roles)
	}

	// Second request with DIFFERENT role (user was granted "viewer" in new token)
	// Cache hit should NOT replace roles from this JWT
	token2 := signedToken(t, key, kid, jwt.MapClaims{
		"iss":     "https://issuer.example.com",
		"aud":     "ainsel-hub",
		"sub":     "user-123",
		"exp":     time.Now().Add(5 * time.Minute).Unix(),
		"iat":     time.Now().Unix(),
		"urn:zitadel:iam:org:project:roles": map[string]any{"viewer": map[string]any{}},
	})

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Authorization", "Bearer "+token2)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	// Username should still come from cache
	if sawUser.Username != "dpinsel" {
		t.Errorf("request 2: Username want 'dpinsel' (from cache), got %q", sawUser.Username)
	}
	// But roles MUST come from the current JWT, not the cache!
	if len(sawUser.Roles) != 1 || sawUser.Roles[0] != "viewer" {
		t.Errorf("request 2: Roles want ['viewer'] (from JWT, not cache), got %v", sawUser.Roles)
	}
}

func TestFetchUserInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer my-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":                "user-123",
			"preferred_username": "dpinsel",
			"email":              "dpinsel@example.com",
		})
	}))
	t.Cleanup(srv.Close)

	result, err := oidc.FetchUserInfo(context.Background(), &http.Client{Timeout: 5 * time.Second}, srv.URL, "my-token")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if result.Sub != "user-123" {
		t.Errorf("Sub: want user-123, got %q", result.Sub)
	}
	if result.Username != "dpinsel" {
		t.Errorf("Username: want dpinsel, got %q", result.Username)
	}
	if result.Email != "dpinsel@example.com" {
		t.Errorf("Email: want dpinsel@example.com, got %q", result.Email)
	}
}

func TestFetchUserInfo_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	_, err := oidc.FetchUserInfo(context.Background(), &http.Client{Timeout: 5 * time.Second}, srv.URL, "tok")
	if err == nil {
		t.Fatal("expected error on server 500, got nil")
	}
}

func TestClearUserInfoCacheEntry_ForcesReFetch(t *testing.T) {
	oidc.ClearUserInfoCache()

	var callCount atomic.Int32
	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":                "entry-user",
			"preferred_username": "alice",
		})
	}))
	t.Cleanup(userinfoSrv.Close)

	key := newRSAKey(t)
	jwks := startJWKSServer(t, key, "k1")
	mw, err := oidc.NewMiddleware(oidc.Config{
		Issuer:      "test-issuer",
		Audience:    "test-aud",
		JWKSURL:     jwks.URL,
		UserInfoURL: userinfoSrv.URL,
	})
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token := signedToken(t, key, "k1", jwt.MapClaims{
		"iss": "test-issuer", "aud": "test-aud", "sub": "entry-user",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	})

	makeReq := func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	makeReq() // populates cache
	makeReq() // uses cache
	if callCount.Load() != 1 {
		t.Fatalf("before eviction: want 1 call, got %d", callCount.Load())
	}

	oidc.ClearUserInfoCacheEntry("entry-user")

	makeReq() // cache miss — re-fetches
	if callCount.Load() != 2 {
		t.Fatalf("after eviction: want 2 calls, got %d", callCount.Load())
	}
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/localauth"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

var testLocalSecret = []byte("test-local-signing-secret-123456")

// localAuthTestServer wires a Server with the fake authz store, a real
// authz.Checker backed by that fake, and local auth enabled.
func localAuthTestServer(t *testing.T) (*Server, *fakeAuthzStore) {
	t.Helper()
	s := testServer(t)
	store := newFakeAuthzStore()
	s.authzStore = store
	s.authzChecker = authz.NewChecker(store, authz.NewGroupCache(func(string) (map[string]authz.GroupRole, error) {
		return nil, nil
	}, time.Minute))
	s.identityTracker = newIdentityPersistTracker()
	s.SetLocalAuthSecret(testLocalSecret)
	// testServer() does not register routes; wire the ones under test.
	s.mux.HandleFunc("/api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/v1/auth/logout", s.handleLogout)
	s.mux.HandleFunc("/api/v1/auth/password", s.handleChangePassword)
	s.mux.HandleFunc("/api/v1/users", s.handleUsers)
	s.mux.HandleFunc("/api/v1/users/", s.handleUser)
	s.mux.Handle("/api/v1/auth/me", oidc.MeHandler())
	return s, store
}

func seedAdmin(t *testing.T, store *fakeAuthzStore) {
	t.Helper()
	hash, err := localauth.HashPassword("admin-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateLocalUser(context.Background(), "local:admin", "", "admin", hash, true); err != nil {
		t.Fatal(err)
	}
}

func doJSON(t *testing.T, s *Server, method, path string, body any, ctxFn func(context.Context) context.Context) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if ctxFn != nil {
		req = req.WithContext(ctxFn(req.Context()))
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func asUser(sub, username string) func(context.Context) context.Context {
	return func(ctx context.Context) context.Context {
		return oidc.ContextWithUser(ctx, &oidc.User{Sub: sub, Username: username})
	}
}

// --- login ---

func TestLoginSuccess(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)

	rec := doJSON(t, s, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "admin", "password": "admin-password",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expiresAt"`
		User      struct {
			Sub      string `json:"sub"`
			Username string `json:"username"`
			IsAdmin  bool   `json:"isAdmin"`
		} `json:"user"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" || resp.User.Sub != "local:admin" || !resp.User.IsAdmin {
		t.Fatalf("unexpected response: %+v", resp)
	}
	// Issued token must validate against the same secret.
	if _, err := localauth.VerifyToken(testLocalSecret, resp.Token); err != nil {
		t.Fatalf("issued token does not verify: %v", err)
	}
}

func TestLoginFailures(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)

	cases := []struct {
		name string
		body map[string]string
	}{
		{"wrong password", map[string]string{"username": "admin", "password": "nope"}},
		{"unknown user", map[string]string{"username": "ghost", "password": "whatever1"}},
		{"empty username", map[string]string{"username": "", "password": "whatever1"}},
		{"uppercase user not in db", map[string]string{"username": "ADMIN2", "password": "whatever1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, s, http.MethodPost, "/api/v1/auth/login", tc.body, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (body %s)", rec.Code, rec.Body.String())
			}
			if !bytes.Contains(rec.Body.Bytes(), []byte("invalid credentials")) {
				t.Fatalf("error must be generic, got %s", rec.Body.String())
			}
		})
	}
}

func TestLoginDisabledWithoutSecret(t *testing.T) {
	s, _ := localAuthTestServer(t)
	s.localAuthSecret = nil
	s.loginThrottle = nil

	rec := doJSON(t, s, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "admin", "password": "x",
	}, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestLoginThrottle(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)

	for i := 0; i < throttleThreshold; i++ {
		rec := doJSON(t, s, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"username": "admin", "password": "wrong",
		}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d", i, rec.Code)
		}
	}
	// Next attempt is locked out even with the correct password.
	rec := doJSON(t, s, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "admin", "password": "admin-password",
	}, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
	// A different username is unaffected.
	rec = doJSON(t, s, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "other", "password": "whatever1",
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("other user: status = %d, want 401", rec.Code)
	}
}

func TestLogout(t *testing.T) {
	s, _ := localAuthTestServer(t)
	rec := doJSON(t, s, http.MethodPost, "/api/v1/auth/logout", nil, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

// --- change own password ---

func TestChangePassword(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)

	rec := doJSON(t, s, http.MethodPut, "/api/v1/auth/password", map[string]string{
		"current": "admin-password", "new": "new-password-1",
	}, asUser("local:admin", "admin"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	// Old password no longer works, new one does.
	if err := localauth.VerifyPassword("admin-password", store.passwords["local:admin"]); err == nil {
		t.Fatal("old password must be gone")
	}
	if err := localauth.VerifyPassword("new-password-1", store.passwords["local:admin"]); err != nil {
		t.Fatalf("new password must verify: %v", err)
	}
}

func TestChangePasswordWrongCurrent(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)

	rec := doJSON(t, s, http.MethodPut, "/api/v1/auth/password", map[string]string{
		"current": "wrong", "new": "new-password-1",
	}, asUser("local:admin", "admin"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestChangePasswordTooShort(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)

	rec := doJSON(t, s, http.MethodPut, "/api/v1/auth/password", map[string]string{
		"current": "admin-password", "new": "short",
	}, asUser("local:admin", "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestChangePasswordOIDCUserRejected(t *testing.T) {
	s, _ := localAuthTestServer(t)
	rec := doJSON(t, s, http.MethodPut, "/api/v1/auth/password", map[string]string{
		"current": "x", "new": "new-password-1",
	}, asUser("23456789-oidc-sub", "oidcuser"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- user management ---

func TestCreateUser(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)
	admin := asUser("local:admin", "admin")

	rec := doJSON(t, s, http.MethodPost, "/api/v1/users", map[string]any{
		"username": "alice", "password": "alice-password", "email": "alice@example.com",
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var u authz.User
	if err := json.NewDecoder(rec.Body).Decode(&u); err != nil {
		t.Fatal(err)
	}
	if u.ID != "local:alice" || u.Username != "alice" || u.IsAdmin {
		t.Fatalf("unexpected user: %+v", u)
	}
	if err := localauth.VerifyPassword("alice-password", store.passwords["local:alice"]); err != nil {
		t.Fatalf("stored password must verify: %v", err)
	}

	// Duplicate -> 409.
	rec = doJSON(t, s, http.MethodPost, "/api/v1/users", map[string]any{
		"username": "alice", "password": "alice-password",
	}, admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: status = %d, want 409", rec.Code)
	}
}

func TestCreateUserValidation(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)
	admin := asUser("local:admin", "admin")

	cases := []struct {
		name string
		body map[string]any
	}{
		{"uppercase normalized to valid", map[string]any{"username": "Carol", "password": "longenough1"}},
		{"bad username with space", map[string]any{"username": "al ice", "password": "longenough1"}},
		{"bad username punctuation start", map[string]any{"username": "-alice", "password": "longenough1"}},
		{"short password", map[string]any{"username": "bob", "password": "short"}},
		{"missing password", map[string]any{"username": "bob"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, s, http.MethodPost, "/api/v1/users", tc.body, admin)
			want := http.StatusBadRequest
			if tc.name == "uppercase normalized to valid" {
				want = http.StatusCreated
			}
			if rec.Code != want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, want, rec.Body.String())
			}
		})
	}
}

func TestCreateUserRequiresAdmin(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)
	// Non-admin local user.
	hash, _ := localauth.HashPassword("user-password")
	if _, err := store.CreateLocalUser(context.Background(), "local:bob", "", "bob", hash, false); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, s, http.MethodPost, "/api/v1/users", map[string]any{
		"username": "eve", "password": "eve-password",
	}, asUser("local:bob", "bob"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestDeleteUser(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)
	admin := asUser("local:admin", "admin")

	hash, _ := localauth.HashPassword("user-password")
	if _, err := store.CreateLocalUser(context.Background(), "local:bob", "", "bob", hash, false); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, s, http.MethodDelete, "/api/v1/users/local:bob", nil, admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}
	if _, ok := store.users["local:bob"]; ok {
		t.Fatal("user must be gone")
	}
}

func TestDeleteUserGuards(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)
	admin := asUser("local:admin", "admin")

	// Self-deletion blocked.
	rec := doJSON(t, s, http.MethodDelete, "/api/v1/users/local:admin", nil, admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("self-delete: status = %d, want 409", rec.Code)
	}
	// Last admin blocked (add second admin, delete self-guard check again).
	hash, _ := localauth.HashPassword("second-admin")
	if _, err := store.CreateLocalUser(context.Background(), "local:admin2", "", "admin2", hash, true); err != nil {
		t.Fatal(err)
	}
	rec = doJSON(t, s, http.MethodDelete, "/api/v1/users/local:admin2", nil, admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("second admin delete: status = %d, want 204", rec.Code)
	}
	// Now admin is the last admin again: deletion via another admin is
	// impossible to express here, so check demotion guard instead.
	rec = doJSON(t, s, http.MethodPatch, "/api/v1/users/local:admin", map[string]any{"isAdmin": false}, admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("last-admin demote: status = %d, want 409", rec.Code)
	}
}

func TestAdminPasswordReset(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)
	admin := asUser("local:admin", "admin")

	hash, _ := localauth.HashPassword("user-password")
	if _, err := store.CreateLocalUser(context.Background(), "local:bob", "", "bob", hash, false); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, s, http.MethodPatch, "/api/v1/users/local:bob", map[string]any{
		"password": "reset-password-1",
	}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if err := localauth.VerifyPassword("reset-password-1", store.passwords["local:bob"]); err != nil {
		t.Fatalf("reset password must verify: %v", err)
	}

	// Non-admin cannot reset passwords.
	rec = doJSON(t, s, http.MethodPatch, "/api/v1/users/local:admin", map[string]any{
		"password": "hijack12345",
	}, asUser("local:bob", "bob"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin reset: status = %d, want 403", rec.Code)
	}
}

// --- middleware integration: issued token authenticates API calls ---

func TestIssuedTokenAuthenticates(t *testing.T) {
	s, store := localAuthTestServer(t)
	seedAdmin(t, store)
	// Wire the real local middleware chain onto the server.
	s.SetAuthMiddleware(func(next http.Handler) http.Handler {
		return localauth.NewMiddleware(testLocalSecret)(s.IdentityPersistMiddleware(next))
	})

	rec := doJSON(t, s, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": "admin", "password": "admin-password",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d", rec.Code)
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&login); err != nil {
		t.Fatal(err)
	}

	// Authenticated /auth/me returns the local user.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+login.Token)
	rec2 := httptest.NewRecorder()
	s.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("/auth/me status = %d, body %s", rec2.Code, rec2.Body.String())
	}
	var me struct {
		Sub      string `json:"sub"`
		Username string `json:"username"`
		Roles    []string `json:"roles"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if me.Sub != "local:admin" || me.Username != "admin" {
		t.Fatalf("unexpected /auth/me: %+v", me)
	}

	// Garbage token is rejected.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	rec3 := httptest.NewRecorder()
	s.ServeHTTP(rec3, req)
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("garbage token: status = %d, want 401", rec3.Code)
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

// fakeAuthzStore is a minimal in-memory store for user-sync tests.
type fakeAuthzStore struct {
	users     map[string]authz.User
	passwords map[string]string
}

func newFakeAuthzStore() *fakeAuthzStore {
	return &fakeAuthzStore{users: make(map[string]authz.User), passwords: make(map[string]string)}
}

func (f *fakeAuthzStore) UpsertUser(_ context.Context, sub, email, username string) (*authz.User, error) {
	u, ok := f.users[sub]
	if !ok {
		u = authz.User{ID: sub, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	}
	// Empty values don't overwrite (matching the real store's semantics).
	if email != "" {
		u.Email = email
	}
	if username != "" {
		u.Username = username
	}
	u.UpdatedAt = time.Now()
	f.users[sub] = u
	return &u, nil
}

func (f *fakeAuthzStore) GetUser(_ context.Context, id string) (*authz.User, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, authz.ErrNotFound
	}
	return &u, nil
}

func (f *fakeAuthzStore) ListUsers(_ context.Context) ([]authz.User, error) {
	out := make([]authz.User, 0, len(f.users))
	for _, u := range f.users {
		out = append(out, u)
	}
	return out, nil
}
func (f *fakeAuthzStore) SetAdmin(_ context.Context, id string, isAdmin bool) error {
	u, ok := f.users[id]
	if !ok {
		return authz.ErrNotFound
	}
	u.IsAdmin = isAdmin
	f.users[id] = u
	return nil
}
func (f *fakeAuthzStore) CreateLocalUser(_ context.Context, id, email, username, passwordHash string, isAdmin bool) (*authz.User, error) {
	if _, ok := f.users[id]; ok {
		return nil, authz.ErrAlreadyExists
	}
	u := authz.User{ID: id, Email: email, Username: username, IsAdmin: isAdmin, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.users[id] = u
	f.passwords[id] = passwordHash
	return &u, nil
}
func (f *fakeAuthzStore) UserPasswordHash(_ context.Context, id string) (string, error) {
	if _, ok := f.users[id]; !ok {
		return "", authz.ErrNotFound
	}
	return f.passwords[id], nil
}
func (f *fakeAuthzStore) SetPassword(_ context.Context, id, hash string) error {
	if _, ok := f.users[id]; !ok {
		return authz.ErrNotFound
	}
	f.passwords[id] = hash
	return nil
}
func (f *fakeAuthzStore) DeleteUser(_ context.Context, id string) error {
	if _, ok := f.users[id]; !ok {
		return authz.ErrNotFound
	}
	delete(f.users, id)
	delete(f.passwords, id)
	return nil
}
func (f *fakeAuthzStore) ClearUsername(_ context.Context, id string) error {
	u, ok := f.users[id]
	if !ok {
		return authz.ErrNotFound
	}
	u.Username = ""
	f.users[id] = u
	return nil
}
func (f *fakeAuthzStore) UserGroupIDs(_ context.Context, _ string) ([]string, error)         { return nil, nil }
func (f *fakeAuthzStore) UserGroupRoles(_ context.Context, _ string) (map[string]authz.GroupRole, error) {
	return nil, nil
}
func (f *fakeAuthzStore) SetResourceGroup(_ context.Context, _, _, _ string, _ bool) error    { return nil }
func (f *fakeAuthzStore) GetResourceGroup(_ context.Context, _, _ string) (*authz.ResourceGroup, error) {
	return nil, authz.ErrNotFound
}
func (f *fakeAuthzStore) DeleteResourceGroup(_ context.Context, _, _ string) error            { return nil }
func (f *fakeAuthzStore) SetResourcePublic(_ context.Context, _, _ string, _ bool) error      { return nil }
func (f *fakeAuthzStore) ListResourcesByGroups(_ context.Context, _ string, _ []string, _ bool) ([]string, error) {
	return nil, nil
}
func (f *fakeAuthzStore) CreateGroup(_ context.Context, _, _ string) (*authz.Group, error) {
	return nil, nil
}
func (f *fakeAuthzStore) GetGroup(_ context.Context, _ string) (*authz.Group, error)            { return nil, nil }
func (f *fakeAuthzStore) ListGroups(_ context.Context) ([]authz.Group, error)                 { return nil, nil }
func (f *fakeAuthzStore) UpdateGroup(_ context.Context, _, _, _ string) (*authz.Group, error) {
	return nil, nil
}
func (f *fakeAuthzStore) DeleteGroup(_ context.Context, _ string) error                        { return nil }
func (f *fakeAuthzStore) AddGroupMember(_ context.Context, _, _ string, _ authz.GroupRole) error { return nil }
func (f *fakeAuthzStore) RemoveGroupMember(_ context.Context, _, _ string) error               { return nil }
func (f *fakeAuthzStore) ListGroupMembers(_ context.Context, _ string) ([]authz.MemberWithUser, error) {
	return nil, nil
}

func userSyncTestServer(t *testing.T) *Server {
	t.Helper()
	s := testServer(t)
	store := newFakeAuthzStore()
	s.authzStore = store
	s.identityTracker = newIdentityPersistTracker()
	return s
}

func TestHandleUserSync_WrongMethod(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/users/", s.handleUser)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/sync", nil)
	ctx := oidc.ContextWithUser(req.Context(), &oidc.User{Sub: "user-123", Email: "u@example.com", Username: "alice"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405 for GET /users/me/sync, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUserSync_NoAuthContext(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/users/", s.handleUser)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/sync", nil)
	rec := httptest.NewRecorder()

	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for unauthenticated POST /users/me/sync, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUserSync_AdminRequired(t *testing.T) {
	s := testServer(t)
	s.mux.HandleFunc("/api/v1/users/", s.handleUser)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/other-user/sync", nil)
	ctx := oidc.ContextWithUser(req.Context(), &oidc.User{Sub: "user-123", Email: "u@example.com", Username: "alice"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.mux.ServeHTTP(rec, req)

	// authzChecker is nil in testServer → requireAdmin returns 403
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403 for non-admin POST /users/other-user/sync, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUserSync_MeSync_UserInfo500_FallbackToJWT(t *testing.T) {
	// Mock userinfo server that returns 500
	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(userinfoSrv.Close)

	s := userSyncTestServer(t)
	s.userInfoURL = userinfoSrv.URL
	s.userInfoClient = &http.Client{}
	s.mux.HandleFunc("/api/v1/users/", s.handleUser)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/sync", nil)
	ctx := oidc.ContextWithUser(req.Context(), &oidc.User{Sub: "user-123", Email: "u@example.com", Username: "alice"})
	ctx = oidc.ContextWithToken(ctx, "test-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 fallback when userinfo fails, got %d: %s", rec.Code, rec.Body.String())
	}

	var got authz.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "user-123" {
		t.Errorf("want id user-123, got %q", got.ID)
	}
	if got.Username != "alice" {
		t.Errorf("want username alice, got %q", got.Username)
	}
}

func TestHandleUserSync_MeSync_UserInfo401_FallbackToJWT(t *testing.T) {
	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(userinfoSrv.Close)

	s := userSyncTestServer(t)
	s.userInfoURL = userinfoSrv.URL
	s.userInfoClient = &http.Client{}
	s.mux.HandleFunc("/api/v1/users/", s.handleUser)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/sync", nil)
	ctx := oidc.ContextWithUser(req.Context(), &oidc.User{Sub: "user-401", Email: "401@example.com", Username: "bob"})
	ctx = oidc.ContextWithToken(ctx, "test-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 fallback on upstream 401, got %d: %s", rec.Code, rec.Body.String())
	}

	var got authz.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "user-401" {
		t.Errorf("want id user-401, got %q", got.ID)
	}
	if got.Username != "bob" {
		t.Errorf("want username bob, got %q", got.Username)
	}
}

func TestHandleUserSync_MeSync_UserInfoTimeout_FallbackToJWT(t *testing.T) {
	// Server that hangs longer than the client timeout
	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(30 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			return
		}
	}))
	t.Cleanup(userinfoSrv.Close)

	s := userSyncTestServer(t)
	s.userInfoURL = userinfoSrv.URL
	s.userInfoClient = &http.Client{Timeout: 50 * time.Millisecond}
	s.mux.HandleFunc("/api/v1/users/", s.handleUser)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/sync", nil)
	ctx := oidc.ContextWithUser(req.Context(), &oidc.User{Sub: "user-timeout", Email: "to@example.com", Username: "timeout"})
	ctx = oidc.ContextWithToken(ctx, "test-token")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 fallback on timeout, got %d: %s", rec.Code, rec.Body.String())
	}

	var got authz.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "user-timeout" {
		t.Errorf("want id user-timeout, got %q", got.ID)
	}
	if got.Username != "timeout" {
		t.Errorf("want username timeout, got %q", got.Username)
	}
}

// TestHandleUserSync_MeSync_NoUserInfoURL_PersistsIdentity is the regression
// test for issue #634: when no userinfo URL is configured, the me-sync
// fallback must persist the identity from the JWT claims (not just read
// the stale row).
func TestHandleUserSync_MeSync_NoUserInfoURL_PersistsIdentity(t *testing.T) {
	s := userSyncTestServer(t)
	// No userInfoURL set — this is the fallback path.
	s.mux.HandleFunc("/api/v1/users/", s.handleUser)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/sync", nil)
	ctx := oidc.ContextWithUser(req.Context(), &oidc.User{Sub: "user-noinfo", Email: "noinfo@example.com", Username: "charlie"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got authz.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "user-noinfo" {
		t.Errorf("want id user-noinfo, got %q", got.ID)
	}
	if got.Email != "noinfo@example.com" {
		t.Errorf("want email noinfo@example.com, got %q", got.Email)
	}
	if got.Username != "charlie" {
		t.Errorf("want username charlie, got %q", got.Username)
	}

	// Verify the row was actually persisted in the store.
	store := s.authzStore.(*fakeAuthzStore)
	persisted, err := store.GetUser(context.Background(), "user-noinfo")
	if err != nil {
		t.Fatalf("expected user in store, got error: %v", err)
	}
	if persisted.Username != "charlie" {
		t.Errorf("stored username should be charlie, got %q", persisted.Username)
	}
}

// TestHandleUserSync_MeSync_NoUserInfoURL_NoToken_PersistsIdentity tests
// the fallback when userInfoURL is set but no token is available.
func TestHandleUserSync_MeSync_NoToken_PersistsIdentity(t *testing.T) {
	s := userSyncTestServer(t)
	s.userInfoURL = "https://userinfo.example.com" // URL is set...
	s.userInfoClient = &http.Client{}
	s.mux.HandleFunc("/api/v1/users/", s.handleUser)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/sync", nil)
	// ...but no token in context.
	ctx := oidc.ContextWithUser(req.Context(), &oidc.User{Sub: "user-notoken", Email: "notoken@example.com", Username: "dave"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got authz.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Username != "dave" {
		t.Errorf("want username dave, got %q", got.Username)
	}
}

// TestIdentityPersistMiddleware_PersistsOnFirstRequest verifies that the
// middleware upserts the identity on the first authenticated request.
func TestIdentityPersistMiddleware_PersistsOnFirstRequest(t *testing.T) {
	s := userSyncTestServer(t)
	store := s.authzStore.(*fakeAuthzStore)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := s.IdentityPersistMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	ctx := oidc.ContextWithUser(req.Context(), &oidc.User{Sub: "mw-user-1", Email: "mw@example.com", Username: "mwuser"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	// Verify the user was persisted.
	u, err := store.GetUser(context.Background(), "mw-user-1")
	if err != nil {
		t.Fatalf("expected user in store: %v", err)
	}
	if u.Username != "mwuser" {
		t.Errorf("want username mwuser, got %q", u.Username)
	}
	if u.Email != "mw@example.com" {
		t.Errorf("want email mw@example.com, got %q", u.Email)
	}
}

// TestIdentityPersistMiddleware_TTLGuard verifies that the middleware does
// not upsert on every request within the TTL window.
func TestIdentityPersistMiddleware_TTLGuard(t *testing.T) {
	s := userSyncTestServer(t)
	store := s.authzStore.(*fakeAuthzStore)

	// Pre-populate the user.
	_, _ = store.UpsertUser(context.Background(), "mw-user-2", "old@example.com", "oldname")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := s.IdentityPersistMiddleware(inner)

	// First request: should upsert.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	ctx := oidc.ContextWithUser(req.Context(), &oidc.User{Sub: "mw-user-2", Email: "new@example.com", Username: "newname"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	u, _ := store.GetUser(context.Background(), "mw-user-2")
	if u.Username != "newname" {
		t.Fatalf("first request should upsert: want newname, got %q", u.Username)
	}

	// Now set the username back to something else to detect if a second upsert happens.
	store.users["mw-user-2"] = authz.User{ID: "mw-user-2", Email: "new@example.com", Username: "newname", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	// Second request immediately: should NOT upsert (within TTL).
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	ctx2 := oidc.ContextWithUser(req2.Context(), &oidc.User{Sub: "mw-user-2", Email: "different@example.com", Username: "different"})
	req2 = req2.WithContext(ctx2)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	u2, _ := store.GetUser(context.Background(), "mw-user-2")
	// Username should still be "newname" because the second request was within TTL.
	if u2.Username != "newname" {
		t.Errorf("second request within TTL should not upsert: want newname, got %q", u2.Username)
	}
}

// TestIdentityPersistMiddleware_InvalidateAllowsImmediateRepopulate verifies
// that after tracker invalidation (as done by ClearUsername), the middleware
// persists immediately on the next request even if the TTL would normally
// block.
func TestIdentityPersistMiddleware_InvalidateAllowsImmediateRepopulate(t *testing.T) {
	s := userSyncTestServer(t)
	store := s.authzStore.(*fakeAuthzStore)

	// Pre-populate with empty username (simulating ClearUsername).
	store.users["mw-user-3"] = authz.User{ID: "mw-user-3", Email: "clear@example.com", Username: "", CreatedAt: time.Now(), UpdatedAt: time.Now()}

	// Prime the tracker so the TTL would normally block, then invalidate
	// (simulating what handleClearUsername does).
	s.identityTracker.shouldPersist("mw-user-3")
	s.identityTracker.invalidate("mw-user-3")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := s.IdentityPersistMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	ctx := oidc.ContextWithUser(req.Context(), &oidc.User{Sub: "mw-user-3", Email: "clear@example.com", Username: "repopulated"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	u, _ := store.GetUser(context.Background(), "mw-user-3")
	if u.Username != "repopulated" {
		t.Errorf("expected username repopulated after invalidation, got %q", u.Username)
	}
}

// TestIdentityPersistMiddleware_NoAuthContext_Passthrough verifies that
// requests without an auth context pass through without error.
func TestIdentityPersistMiddleware_NoAuthContext_Passthrough(t *testing.T) {
	s := userSyncTestServer(t)

	innerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler := s.IdentityPersistMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !innerCalled {
		t.Error("expected inner handler to be called for unauthenticated request")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

// TestIdentityPersistMiddleware_WireOrder verifies that the middleware
// works correctly when composed in the same order as wire.go:
// authMW → IdentityPersistMiddleware → inner. If the order is inverted
// (IdentityPersistMiddleware wrapping authMW), the OIDC context would
// not be populated and the middleware would always passthrough.
func TestIdentityPersistMiddleware_WireOrder(t *testing.T) {
	s := userSyncTestServer(t)
	store := s.authzStore.(*fakeAuthzStore)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Simulate the wire.go composition: an outer "auth" middleware that
	// sets the OIDC user context, then IdentityPersistMiddleware as the
	// innermost handler.
	authMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := oidc.ContextWithUser(r.Context(), &oidc.User{
				Sub:      "wire-order-user",
				Email:    "wire@example.com",
				Username: "wireuser",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	// Correct order (matches wire.go): authMW wraps IdentityPersistMiddleware.
	handler := authMW(s.IdentityPersistMiddleware(inner))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	u, err := store.GetUser(context.Background(), "wire-order-user")
	if err != nil {
		t.Fatalf("expected user in store: %v", err)
	}
	if u.Username != "wireuser" {
		t.Errorf("want username wireuser, got %q", u.Username)
	}
}

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
	"github.com/DominikPinsel/ainsel/services/hub/internal/chat"
	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Chat sessions record an owner at creation time. These tests pin down that
// the owner is actually enforced on every /api/v1/ verb: before the fix any
// authenticated caller could read, rename, delete and post into any session by
// id, and could list anyone's sessions with ?user=<victim>.
//
// The /api/internal/chat/ routes are deliberately not covered here — the agent
// sidecar authenticates with X-Internal-Token and legitimately serves every
// user's conversations.

// newAuthzChatEnv boots Postgres, migrates, and returns chat and authz stores
// over the same pool. Skips the test if Docker is unavailable.
func newAuthzChatEnv(t *testing.T) (*chat.Store, *authz.Store, func()) {
	t.Helper()
	ctx := context.Background()

	c, err := pgcontainer.Run(ctx, "postgres:17-alpine",
		pgcontainer.WithDatabase("ainsel_test"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		pgcontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("connection string: %v", err)
	}
	if err := db.Migrate(ctx, dsn); err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("open pool: %v", err)
	}
	cleanup := func() {
		pool.Close()
		_ = c.Terminate(context.Background())
	}
	return chat.NewStore(pool), authz.NewStore(pool), cleanup
}

// chatAuthzServer wires the v1 chat routes with authentication configured.
// The middleware is a pass-through: identity comes from the request context,
// exactly as the OIDC middleware leaves it.
func chatAuthzServer(t *testing.T, chatStore *chat.Store, authzStore *authz.Store) *Server {
	t.Helper()
	s := testServer(t)
	s.chat = chatStore
	if authzStore != nil {
		checker := authz.NewChecker(authzStore, authz.NewGroupCache(
			func(uid string) (map[string]authz.GroupRole, error) {
				return authzStore.UserGroupRoles(context.Background(), uid)
			}, time.Minute))
		s.authzStore = authzStore
		s.authzChecker = checker
	}
	s.SetAuthMiddleware(func(next http.Handler) http.Handler { return next })
	s.mux.HandleFunc("/api/v1/chat/sessions", s.handleChatSessions)
	s.mux.HandleFunc("/api/v1/chat/sessions/", s.handleChatSession)
	return s
}

// chatReqAs issues a request carrying an OIDC identity. sub == "" produces
// an unauthenticated request.
func chatReqAs(s *Server, method, path, sub string, body []byte) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if sub != "" {
		req = req.WithContext(asUser(sub, sub)(req.Context()))
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func makeAdmin(t *testing.T, store *authz.Store, sub string) {
	t.Helper()
	if _, err := store.UpsertUser(context.Background(), sub, sub+"@example.com", sub); err != nil {
		t.Fatalf("upsert user %s: %v", sub, err)
	}
	if err := store.SetAdmin(context.Background(), sub, true); err != nil {
		t.Fatalf("set admin %s: %v", sub, err)
	}
}

// TestChatSession_ForbiddenForOtherUser is the core IDOR regression: bob must
// not be able to read or mutate alice's session, and the mutation must not
// have happened.
func TestChatSession_ForbiddenForOtherUser(t *testing.T) {
	chatStore, authzStore, cleanup := newAuthzChatEnv(t)
	defer cleanup()

	ctx := context.Background()
	sess, err := chatStore.CreateSession(ctx, "a-agent", "alice")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := chatStore.AddMessage(ctx, sess.ID, chat.RoleUser, "alice secret", 0); err != nil {
		t.Fatalf("add message: %v", err)
	}

	s := chatAuthzServer(t, chatStore, authzStore)
	base := "/api/v1/chat/sessions/" + sess.ID

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"get", http.MethodGet, base, nil},
		{"patch", http.MethodPatch, base, []byte(`{"name":"hijacked"}`)},
		{"post message", http.MethodPost, base + "/messages", []byte(`{"role":"assistant","content":"forged reply"}`)},
		{"delete", http.MethodDelete, base, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := chatReqAs(s, tc.method, tc.path, "bob", tc.body)
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d (%s), want 403", rec.Code, rec.Body.String())
			}
		})
	}

	// Nothing above may have taken effect. Run delete last in the table, so
	// the session must still exist and be unchanged.
	got, err := chatStore.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("session disappeared after denied requests: %v", err)
	}
	if got.Name == "hijacked" {
		t.Errorf("rename took effect despite 403")
	}
	if len(got.Messages) != 1 {
		t.Errorf("message count = %d, want 1 — forged message must not be stored", len(got.Messages))
	}
	for _, m := range got.Messages {
		if m.Content == "forged reply" {
			t.Errorf("forged assistant message was stored despite 403")
		}
	}
}

// TestChatSession_AllowedForOwner is the counterweight: the gate must not lock
// the real owner out of their own conversation.
func TestChatSession_AllowedForOwner(t *testing.T) {
	chatStore, authzStore, cleanup := newAuthzChatEnv(t)
	defer cleanup()

	ctx := context.Background()
	sess, err := chatStore.CreateSession(ctx, "a-agent", "alice")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	s := chatAuthzServer(t, chatStore, authzStore)
	base := "/api/v1/chat/sessions/" + sess.ID

	if rec := chatReqAs(s, http.MethodGet, base, "alice", nil); rec.Code != http.StatusOK {
		t.Errorf("owner GET: status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	if rec := chatReqAs(s, http.MethodPost, base+"/messages", "alice",
		[]byte(`{"role":"user","content":"hello"}`)); rec.Code != http.StatusCreated {
		t.Errorf("owner POST message: status = %d (%s), want 201", rec.Code, rec.Body.String())
	}
	if rec := chatReqAs(s, http.MethodPatch, base, "alice", []byte(`{"name":"renamed"}`)); rec.Code != http.StatusOK {
		t.Errorf("owner PATCH: status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
}

// TestChatSessions_ListScopedToCaller covers the ?user= query parameter, which
// used to be taken at face value and let any caller list anyone's sessions.
func TestChatSessions_ListScopedToCaller(t *testing.T) {
	chatStore, authzStore, cleanup := newAuthzChatEnv(t)
	defer cleanup()

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := chatStore.CreateSession(ctx, "a-agent", "alice"); err != nil {
			t.Fatalf("create alice session: %v", err)
		}
	}
	bobSess, err := chatStore.CreateSession(ctx, "a-agent", "bob")
	if err != nil {
		t.Fatalf("create bob session: %v", err)
	}

	s := chatAuthzServer(t, chatStore, authzStore)

	decode := func(t *testing.T, body string) []struct {
		ID     string `json:"id"`
		UserID string `json:"userId"`
	} {
		t.Helper()
		var resp struct {
			Items []struct {
				ID     string `json:"id"`
				UserID string `json:"userId"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("decode list response %q: %v", body, err)
		}
		return resp.Items
	}

	t.Run("caller sees only their own", func(t *testing.T) {
		rec := chatReqAs(s, http.MethodGet, "/api/v1/chat/sessions", "alice", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		items := decode(t, rec.Body.String())
		if len(items) != 2 {
			t.Fatalf("got %d sessions, want 2 (alice's only)", len(items))
		}
		for _, it := range items {
			if it.UserID != "alice" {
				t.Errorf("leaked session %s owned by %q", it.ID, it.UserID)
			}
		}
	})

	t.Run("query parameter cannot widen", func(t *testing.T) {
		rec := chatReqAs(s, http.MethodGet, "/api/v1/chat/sessions?user=bob", "alice", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		items := decode(t, rec.Body.String())
		for _, it := range items {
			if it.UserID != "alice" {
				t.Errorf("?user=bob widened the result: got session %s owned by %q", it.ID, it.UserID)
			}
		}
		if len(items) != 2 {
			t.Errorf("got %d sessions, want 2 — ?user= must be ignored for a non-admin", len(items))
		}
	})

	t.Run("victim session is not listed for the attacker", func(t *testing.T) {
		rec := chatReqAs(s, http.MethodGet, "/api/v1/chat/sessions", "mallory", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
		}
		for _, it := range decode(t, rec.Body.String()) {
			if it.ID == bobSess.ID {
				t.Errorf("bob's session %s listed for mallory", it.ID)
			}
		}
	})
}

// TestChatSessions_ListAdminSeesAll keeps the gate from over-tightening: an
// admin still gets the cross-user view the UI's admin screens need.
func TestChatSessions_ListAdminSeesAll(t *testing.T) {
	chatStore, authzStore, cleanup := newAuthzChatEnv(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := chatStore.CreateSession(ctx, "a-agent", "alice"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := chatStore.CreateSession(ctx, "a-agent", "bob"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	makeAdmin(t, authzStore, "root")

	s := chatAuthzServer(t, chatStore, authzStore)

	rec := chatReqAs(s, http.MethodGet, "/api/v1/chat/sessions", "root", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []struct {
			UserID string `json:"userId"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("admin got %d sessions, want 2 (both users)", len(resp.Items))
	}

	// An admin may also open someone else's session.
	other, err := chatStore.CreateSession(ctx, "a-agent", "carol")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if rec := chatReqAs(s, http.MethodGet, "/api/v1/chat/sessions/"+other.ID, "root", nil); rec.Code != http.StatusOK {
		t.Errorf("admin GET other's session: status = %d (%s), want 200", rec.Code, rec.Body.String())
	}
}

// TestChatSession_FailsClosedWithoutIdentity guards the fallback: with
// authentication configured, a request carrying no identity must be refused
// rather than treated as "no owner to compare against, so allow".
func TestChatSession_FailsClosedWithoutIdentity(t *testing.T) {
	chatStore, authzStore, cleanup := newAuthzChatEnv(t)
	defer cleanup()

	sess, err := chatStore.CreateSession(context.Background(), "a-agent", "alice")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	s := chatAuthzServer(t, chatStore, authzStore)

	rec := chatReqAs(s, http.MethodGet, "/api/v1/chat/sessions/"+sess.ID, "", nil)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d (%s), want 401/403", rec.Code, rec.Body.String())
	}

	rec = chatReqAs(s, http.MethodGet, "/api/v1/chat/sessions", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("list without identity: status = %d (%s), want 401", rec.Code, rec.Body.String())
	}
}

// TestCreateChatSession_RequiresIdentity verifies a session can no longer be
// created with no owner, which previously fell back to the shared "anonymous"
// bucket and made the session invisible to its creator.
func TestCreateChatSession_RequiresIdentity(t *testing.T) {
	chatStore, authzStore, cleanup := newAuthzChatEnv(t)
	defer cleanup()

	s := chatAuthzServer(t, chatStore, authzStore)

	rec := chatReqAs(s, http.MethodPost, "/api/v1/chat/sessions", "",
		[]byte(`{"agentName":"a-agent"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d (%s), want 401", rec.Code, rec.Body.String())
	}

	// With an identity the session is created and owned by the caller.
	rec = chatReqAs(s, http.MethodPost, "/api/v1/chat/sessions", "alice",
		[]byte(`{"agentName":"a-agent"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (%s), want 201", rec.Code, rec.Body.String())
	}
	var created chat.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created session: %v", err)
	}
	if created.UserID != "alice" {
		t.Errorf("owner = %q, want alice", created.UserID)
	}
}

package authz_test

import (
	"context"
	"testing"
	"time"

	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
)

// testEnv bundles a live store, checker, and cache backed by a Postgres
// testcontainer with all migrations applied.
type testEnv struct {
	store   *authz.Store
	checker *authz.Checker
	cache   *authz.GroupCache
	cleanup func()
}

// newTestAuthZ boots a Postgres testcontainer, applies migrations, and wires a
// Store + GroupCache + Checker exactly as production does (see wireAuthZ).
// Skips the test if Docker is unavailable.
func newTestAuthZ(t *testing.T) *testEnv {
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

	store := authz.NewStore(pool)
	cache := authz.NewGroupCache(func(uid string) (map[string]authz.GroupRole, error) {
		return store.UserGroupRoles(ctx, uid)
	}, time.Minute)
	checker := authz.NewChecker(store, cache)

	return &testEnv{
		store:   store,
		checker: checker,
		cache:   cache,
		cleanup: func() {
			pool.Close()
			_ = c.Terminate(context.Background())
		},
	}
}

// mustUser upserts a user and returns its ID.
func (e *testEnv) mustUser(t *testing.T, sub string) string {
	t.Helper()
	u, err := e.store.UpsertUser(context.Background(), sub, sub+"@example.com", sub)
	if err != nil {
		t.Fatalf("upsert user %s: %v", sub, err)
	}
	return u.ID
}

// mustGroup creates a group and returns its ID.
func (e *testEnv) mustGroup(t *testing.T, name string) string {
	t.Helper()
	g, err := e.store.CreateGroup(context.Background(), name, "")
	if err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	return g.ID
}

func (e *testEnv) mustMember(t *testing.T, groupID, userID string, role authz.GroupRole) {
	t.Helper()
	if err := e.store.AddGroupMember(context.Background(), groupID, userID, role); err != nil {
		t.Fatalf("add member %s to %s: %v", userID, groupID, err)
	}
	e.cache.Invalidate(userID)
}

func TestStoreResourceGroupLifecycle(t *testing.T) {
	e := newTestAuthZ(t)
	defer e.cleanup()
	ctx := context.Background()

	gid := e.mustGroup(t, "platform")

	// Assign a resource to the group, non-public.
	if err := e.store.SetResourceGroup(ctx, "agent", "a1", gid, false); err != nil {
		t.Fatalf("set resource group: %v", err)
	}

	rg, err := e.store.GetResourceGroup(ctx, "agent", "a1")
	if err != nil {
		t.Fatalf("get resource group: %v", err)
	}
	if rg.GroupID != gid || rg.Public {
		t.Fatalf("unexpected resource group: %+v", rg)
	}

	// Toggle public flag.
	if err := e.store.SetResourcePublic(ctx, "agent", "a1", true); err != nil {
		t.Fatalf("set public: %v", err)
	}
	rg, _ = e.store.GetResourceGroup(ctx, "agent", "a1")
	if !rg.Public {
		t.Fatalf("expected public=true, got %+v", rg)
	}

	// Reassign to a different group resets the mapping (upsert semantics).
	gid2 := e.mustGroup(t, "other")
	if err := e.store.SetResourceGroup(ctx, "agent", "a1", gid2, false); err != nil {
		t.Fatalf("reassign resource group: %v", err)
	}
	rg, _ = e.store.GetResourceGroup(ctx, "agent", "a1")
	if rg.GroupID != gid2 || rg.Public {
		t.Fatalf("expected reassignment to %s non-public, got %+v", gid2, rg)
	}

	// Delete the mapping → GetResourceGroup returns ErrNotFound.
	if err := e.store.DeleteResourceGroup(ctx, "agent", "a1"); err != nil {
		t.Fatalf("delete resource group: %v", err)
	}
	if _, err := e.store.GetResourceGroup(ctx, "agent", "a1"); err != authz.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStoreListResourcesByGroups(t *testing.T) {
	e := newTestAuthZ(t)
	defer e.cleanup()
	ctx := context.Background()

	g1 := e.mustGroup(t, "g1")
	g2 := e.mustGroup(t, "g2")

	// a1, a2 in g1 (private); a3 in g2 (private); a4 in g2 (public).
	mustSet := func(name, gid string, public bool) {
		if err := e.store.SetResourceGroup(ctx, "agent", name, gid, public); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	mustSet("a1", g1, false)
	mustSet("a2", g1, false)
	mustSet("a3", g2, false)
	mustSet("a4", g2, true)

	// Member of g1 only, no public → sees a1, a2.
	got, err := e.store.ListResourcesByGroups(ctx, "agent", []string{g1}, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	assertSet(t, got, map[string]bool{"a1": true, "a2": true})

	// includePublic adds a4 (the only public resource).
	got, err = e.store.ListResourcesByGroups(ctx, "agent", []string{g1}, true)
	if err != nil {
		t.Fatalf("list public: %v", err)
	}
	assertSet(t, got, map[string]bool{"a1": true, "a2": true, "a4": true})

	// Member of both groups, no public → a1, a2 (g1) and a3, a4 (g2).
	// a4 is seen via group membership even though includePublic is false;
	// the public flag only governs cross-group visibility.
	got, err = e.store.ListResourcesByGroups(ctx, "agent", []string{g1, g2}, false)
	if err != nil {
		t.Fatalf("list both: %v", err)
	}
	assertSet(t, got, map[string]bool{"a1": true, "a2": true, "a3": true, "a4": true})

	// No groups, includePublic → only public a4.
	got, err = e.store.ListResourcesByGroups(ctx, "agent", nil, true)
	if err != nil {
		t.Fatalf("list public only: %v", err)
	}
	assertSet(t, got, map[string]bool{"a4": true})

	// No groups, no public → nothing.
	got, err = e.store.ListResourcesByGroups(ctx, "agent", nil, false)
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestStoreUserGroupRoles(t *testing.T) {
	e := newTestAuthZ(t)
	defer e.cleanup()
	ctx := context.Background()

	uid := e.mustUser(t, "alice")
	g1 := e.mustGroup(t, "g1")
	g2 := e.mustGroup(t, "g2")
	e.mustMember(t, g1, uid, authz.RoleOwner)
	e.mustMember(t, g2, uid, authz.RoleReader)

	roles, err := e.store.UserGroupRoles(ctx, uid)
	if err != nil {
		t.Fatalf("user group roles: %v", err)
	}
	if roles[g1] != authz.RoleOwner || roles[g2] != authz.RoleReader {
		t.Fatalf("unexpected roles: %+v", roles)
	}

	ids, err := e.store.UserGroupIDs(ctx, uid)
	if err != nil {
		t.Fatalf("user group ids: %v", err)
	}
	assertSet(t, ids, map[string]bool{g1: true, g2: true})
}

func TestCheckerRoleHierarchy(t *testing.T) {
	e := newTestAuthZ(t)
	defer e.cleanup()
	ctx := context.Background()

	reader := e.mustUser(t, "reader")
	writer := e.mustUser(t, "writer")
	owner := e.mustUser(t, "owner")
	outsider := e.mustUser(t, "outsider")

	gid := e.mustGroup(t, "team")
	e.mustMember(t, gid, reader, authz.RoleReader)
	e.mustMember(t, gid, writer, authz.RoleWriter)
	e.mustMember(t, gid, owner, authz.RoleOwner)

	if err := e.store.SetResourceGroup(ctx, "agent", "a1", gid, false); err != nil {
		t.Fatalf("set resource group: %v", err)
	}

	cases := []struct {
		user                string
		read, write, manage bool
	}{
		{reader, true, false, false},
		{writer, true, true, false},
		{owner, true, true, true},
		{outsider, false, false, false},
	}
	for _, tc := range cases {
		gotRead, err := e.checker.CanRead(ctx, tc.user, "agent", "a1")
		if err != nil {
			t.Fatalf("CanRead(%s): %v", tc.user, err)
		}
		gotWrite, err := e.checker.CanWrite(ctx, tc.user, "agent", "a1")
		if err != nil {
			t.Fatalf("CanWrite(%s): %v", tc.user, err)
		}
		gotManage, err := e.checker.CanManage(ctx, tc.user, "agent", "a1")
		if err != nil {
			t.Fatalf("CanManage(%s): %v", tc.user, err)
		}
		if gotRead != tc.read || gotWrite != tc.write || gotManage != tc.manage {
			t.Fatalf("user %s: got read=%v write=%v manage=%v, want read=%v write=%v manage=%v",
				tc.user, gotRead, gotWrite, gotManage, tc.read, tc.write, tc.manage)
		}
	}
}

func TestCheckerAdminBypass(t *testing.T) {
	e := newTestAuthZ(t)
	defer e.cleanup()
	ctx := context.Background()

	admin := e.mustUser(t, "admin")
	if err := e.store.SetAdmin(ctx, admin, true); err != nil {
		t.Fatalf("set admin: %v", err)
	}

	// Resource in a group the admin is NOT a member of.
	gid := e.mustGroup(t, "team")
	if err := e.store.SetResourceGroup(ctx, "agent", "a1", gid, false); err != nil {
		t.Fatalf("set resource group: %v", err)
	}

	for _, fn := range []struct {
		name string
		f    func() (bool, error)
	}{
		{"CanRead", func() (bool, error) { return e.checker.CanRead(ctx, admin, "agent", "a1") }},
		{"CanWrite", func() (bool, error) { return e.checker.CanWrite(ctx, admin, "agent", "a1") }},
		{"CanManage", func() (bool, error) { return e.checker.CanManage(ctx, admin, "agent", "a1") }},
		{"CanManageGroup", func() (bool, error) { return e.checker.CanManageGroup(ctx, admin, gid) }},
		{"CanWriteGroup", func() (bool, error) { return e.checker.CanWriteGroup(ctx, admin, gid) }},
	} {
		ok, err := fn.f()
		if err != nil {
			t.Fatalf("%s: %v", fn.name, err)
		}
		if !ok {
			t.Fatalf("admin should bypass %s", fn.name)
		}
	}
}

func TestCheckerPublicResource(t *testing.T) {
	e := newTestAuthZ(t)
	defer e.cleanup()
	ctx := context.Background()

	outsider := e.mustUser(t, "outsider")
	gid := e.mustGroup(t, "team")

	// Private resource: outsider cannot read.
	if err := e.store.SetResourceGroup(ctx, "agent", "private", gid, false); err != nil {
		t.Fatalf("set private: %v", err)
	}
	if ok, _ := e.checker.CanRead(ctx, outsider, "agent", "private"); ok {
		t.Fatal("outsider should not read private resource")
	}
	// Public read never grants write.
	if err := e.store.SetResourceGroup(ctx, "agent", "public", gid, true); err != nil {
		t.Fatalf("set public: %v", err)
	}
	if ok, _ := e.checker.CanRead(ctx, outsider, "agent", "public"); !ok {
		t.Fatal("outsider should read public resource")
	}
	if ok, _ := e.checker.CanWrite(ctx, outsider, "agent", "public"); ok {
		t.Fatal("public read must not grant write")
	}
}

func TestCheckerDefaultClosed(t *testing.T) {
	e := newTestAuthZ(t)
	defer e.cleanup()
	ctx := context.Background()

	uid := e.mustUser(t, "alice")

	// A resource with no group mapping is default-closed for everyone
	// (except admins, covered elsewhere).
	for _, fn := range []struct {
		name string
		f    func() (bool, error)
	}{
		{"CanRead", func() (bool, error) { return e.checker.CanRead(ctx, uid, "agent", "ghost") }},
		{"CanWrite", func() (bool, error) { return e.checker.CanWrite(ctx, uid, "agent", "ghost") }},
		{"CanManage", func() (bool, error) { return e.checker.CanManage(ctx, uid, "agent", "ghost") }},
	} {
		ok, err := fn.f()
		if err != nil {
			t.Fatalf("%s: %v", fn.name, err)
		}
		if ok {
			t.Fatalf("unmapped resource should be default-closed for %s", fn.name)
		}
	}
}

func TestCheckerGroupWriteAndManage(t *testing.T) {
	e := newTestAuthZ(t)
	defer e.cleanup()
	ctx := context.Background()

	reader := e.mustUser(t, "reader")
	writer := e.mustUser(t, "writer")
	owner := e.mustUser(t, "owner")
	outsider := e.mustUser(t, "outsider")

	gid := e.mustGroup(t, "team")
	e.mustMember(t, gid, reader, authz.RoleReader)
	e.mustMember(t, gid, writer, authz.RoleWriter)
	e.mustMember(t, gid, owner, authz.RoleOwner)

	cases := []struct {
		user          string
		write, manage bool
	}{
		{reader, false, false},
		{writer, true, false},
		{owner, true, true},
		{outsider, false, false},
	}
	for _, tc := range cases {
		gotWrite, err := e.checker.CanWriteGroup(ctx, tc.user, gid)
		if err != nil {
			t.Fatalf("CanWriteGroup(%s): %v", tc.user, err)
		}
		gotManage, err := e.checker.CanManageGroup(ctx, tc.user, gid)
		if err != nil {
			t.Fatalf("CanManageGroup(%s): %v", tc.user, err)
		}
		if gotWrite != tc.write || gotManage != tc.manage {
			t.Fatalf("user %s: got write=%v manage=%v, want write=%v manage=%v",
				tc.user, gotWrite, gotManage, tc.write, tc.manage)
		}
	}
}

func assertSet(t *testing.T, got []string, want map[string]bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("unexpected element %q in %v", g, got)
		}
	}
}

package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

// The version-history and rollback endpoints expose persona content, so they
// must be gated exactly like GET/PUT on the persona itself. These tests pin
// that down: reading history requires read access, rolling back requires write
// access, and a denied caller never reaches the service.
//
// They live in package api (not api_test) because RegisterPersonaRoutes takes
// the unexported *authzStore pointer.

// personaAuthzStub implements only the endpoints under test. The embedded nil
// PersonaService makes every other method panic if a handler reaches it.
type personaAuthzStub struct {
	PersonaService

	listVersionsCalls int
	getVersionCalls   int
	rollbackCalls     int
}

func (s *personaAuthzStub) ListVersions(_ context.Context, id string) ([]personas.VersionSummary, error) {
	s.listVersionsCalls++
	return []personas.VersionSummary{
		{PersonaID: id, VersionNumber: 2},
		{PersonaID: id, VersionNumber: 1},
	}, nil
}

func (s *personaAuthzStub) GetVersion(_ context.Context, id string, n int) (*personas.Version, error) {
	s.getVersionCalls++
	return &personas.Version{PersonaID: id, VersionNumber: n, Text: "historical text"}, nil
}

func (s *personaAuthzStub) Rollback(_ context.Context, id string, n int) (*personas.Persona, error) {
	s.rollbackCalls++
	return &personas.Persona{ID: id, Name: "code-reviewer", CurrentVersion: n + 1}, nil
}

// denyAuthzStore models a non-admin caller with no group mapping for the
// persona. CanRead and CanWrite both resolve to (false, nil) — default-closed —
// which is what a 403 must come from rather than a 500.
type denyAuthzStore struct{}

func (denyAuthzStore) GetUser(_ context.Context, id string) (*authz.User, error) {
	return &authz.User{ID: id, Username: id}, nil
}

func (denyAuthzStore) GetResourceGroup(_ context.Context, _, _ string) (*authz.ResourceGroup, error) {
	return nil, authz.ErrNotFound
}

// groupAuthzStore models a non-admin caller whose persona lives in group "g1",
// with the per-user roles supplied by the test.
type groupAuthzStore struct {
	public bool
	roles  map[string]authz.GroupRole // groupID -> role for user "u1"
}

func (groupAuthzStore) GetUser(_ context.Context, id string) (*authz.User, error) {
	return &authz.User{ID: id, Username: id}, nil
}

func (s groupAuthzStore) GetResourceGroup(_ context.Context, resourceType, resourceName string) (*authz.ResourceGroup, error) {
	return &authz.ResourceGroup{
		ResourceType: resourceType,
		ResourceName: resourceName,
		GroupID:      "g1",
		Public:       s.public,
	}, nil
}

// newPersonaAuthzServer wires the persona routes with a real authz.Checker.
// An empty sub produces an unauthenticated request (no OIDC user in context).
func newPersonaAuthzServer(t *testing.T, svc PersonaService, store authz.CheckerStore, roles map[string]authz.GroupRole, sub string) *httptest.Server {
	t.Helper()
	checker := authz.NewChecker(store, authz.NewGroupCache(func(userID string) (map[string]authz.GroupRole, error) {
		if userID != "u1" {
			return nil, nil
		}
		return roles, nil
	}, time.Minute))

	mux := http.NewServeMux()
	RegisterPersonaRoutes(mux, svc, nil, &checker)
	if sub == "" {
		return httptest.NewServer(mux)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := oidc.ContextWithUser(r.Context(), &oidc.User{Sub: sub, Username: sub})
		mux.ServeHTTP(w, r.WithContext(ctx))
	}))
}

func TestHandlerListVersionsForbidden(t *testing.T) {
	svc := &personaAuthzStub{}
	srv := newPersonaAuthzServer(t, svc, denyAuthzStore{}, nil, "u1")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/personas/01X/versions")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if svc.listVersionsCalls != 0 {
		t.Errorf("ListVersions called %d times, want 0 — denied caller must not reach the service", svc.listVersionsCalls)
	}
}

func TestHandlerGetVersionForbidden(t *testing.T) {
	svc := &personaAuthzStub{}
	srv := newPersonaAuthzServer(t, svc, denyAuthzStore{}, nil, "u1")
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/personas/01X/versions/2")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if svc.getVersionCalls != 0 {
		t.Errorf("GetVersion called %d times, want 0 — denied caller must not reach the service", svc.getVersionCalls)
	}
}

func TestHandlerRollbackForbidden(t *testing.T) {
	svc := &personaAuthzStub{}
	srv := newPersonaAuthzServer(t, svc, denyAuthzStore{}, nil, "u1")
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/personas/01X/rollback", "application/json", strings.NewReader(`{"toVersion":2}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if svc.rollbackCalls != 0 {
		t.Errorf("Rollback called %d times, want 0 — denied caller must not reach the service", svc.rollbackCalls)
	}
}

// A caller with no identity at all must get 401, not the persona's history.
func TestHandlerVersionsUnauthorized(t *testing.T) {
	for _, tc := range []struct {
		name   string
		do     func(url string) (*http.Response, error)
		called func(s *personaAuthzStub) int
	}{
		{"list", func(url string) (*http.Response, error) { return http.Get(url + "/api/v1/personas/01X/versions") }, func(s *personaAuthzStub) int { return s.listVersionsCalls }},
		{"get", func(url string) (*http.Response, error) { return http.Get(url + "/api/v1/personas/01X/versions/2") }, func(s *personaAuthzStub) int { return s.getVersionCalls }},
		{"rollback", func(url string) (*http.Response, error) {
			return http.Post(url+"/api/v1/personas/01X/rollback", "application/json", strings.NewReader(`{"toVersion":2}`))
		}, func(s *personaAuthzStub) int { return s.rollbackCalls }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &personaAuthzStub{}
			srv := newPersonaAuthzServer(t, svc, denyAuthzStore{}, nil, "")
			defer srv.Close()

			resp, err := tc.do(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
			if n := tc.called(svc); n != 0 {
				t.Errorf("service called %d times, want 0", n)
			}
		})
	}
}

// A writer of the persona's group gets through all three endpoints. Guards
// against over-tightening the check.
func TestHandlerVersionsAllowedForWriter(t *testing.T) {
	store := groupAuthzStore{roles: map[string]authz.GroupRole{"g1": authz.RoleWriter}}
	roles := map[string]authz.GroupRole{"g1": authz.RoleWriter}

	t.Run("list", func(t *testing.T) {
		svc := &personaAuthzStub{}
		srv := newPersonaAuthzServer(t, svc, store, roles, "u1")
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/api/v1/personas/01X/versions")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if svc.listVersionsCalls != 1 {
			t.Errorf("ListVersions called %d times, want 1", svc.listVersionsCalls)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `"versionNumber":2`) {
			t.Errorf("body missing version listing: %s", body)
		}
	})

	t.Run("get", func(t *testing.T) {
		svc := &personaAuthzStub{}
		srv := newPersonaAuthzServer(t, svc, store, roles, "u1")
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/api/v1/personas/01X/versions/2")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if svc.getVersionCalls != 1 {
			t.Errorf("GetVersion called %d times, want 1", svc.getVersionCalls)
		}
	})

	t.Run("rollback", func(t *testing.T) {
		svc := &personaAuthzStub{}
		srv := newPersonaAuthzServer(t, svc, store, roles, "u1")
		defer srv.Close()

		resp, err := http.Post(srv.URL+"/api/v1/personas/01X/rollback", "application/json", strings.NewReader(`{"toVersion":2}`))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if svc.rollbackCalls != 1 {
			t.Errorf("Rollback called %d times, want 1", svc.rollbackCalls)
		}
	})
}

// A public persona is readable by anyone, but "readable" must not imply
// "rollback-able": history is exposed while the mutation stays denied.
func TestHandlerPublicPersonaReadableButNotRollbackable(t *testing.T) {
	store := groupAuthzStore{public: true}

	svc := &personaAuthzStub{}
	srv := newPersonaAuthzServer(t, svc, store, nil, "u1")
	defer srv.Close()

	listResp, err := http.Get(srv.URL + "/api/v1/personas/01X/versions")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listResp.Body.Close() }()
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("list versions status = %d, want %d", listResp.StatusCode, http.StatusOK)
	}

	rollbackResp, err := http.Post(srv.URL+"/api/v1/personas/01X/rollback", "application/json", strings.NewReader(`{"toVersion":1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rollbackResp.Body.Close() }()
	if rollbackResp.StatusCode != http.StatusForbidden {
		t.Errorf("rollback status = %d, want %d", rollbackResp.StatusCode, http.StatusForbidden)
	}
	if svc.rollbackCalls != 0 {
		t.Errorf("Rollback called %d times, want 0", svc.rollbackCalls)
	}
}

package api

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

// identityPersistTTL is the minimum interval between automatic identity
// upserts for the same subject. This prevents a DB write on every
// authenticated request while still ensuring the registry self-heals
// within a reasonable window.
const identityPersistTTL = 5 * time.Minute

// identityPersistTracker tracks the last time an identity was persisted
// for each subject, so we don't write on every request.
type identityPersistTracker struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

func newIdentityPersistTracker() *identityPersistTracker {
	return &identityPersistTracker{
		entries: make(map[string]time.Time),
	}
}

// shouldPersist returns true if enough time has elapsed since the last
// persist for this subject, or if the subject has never been tracked.
// When it returns true it also records the current time.
func (t *identityPersistTracker) shouldPersist(sub string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	last, ok := t.entries[sub]
	if !ok || time.Since(last) >= identityPersistTTL {
		t.entries[sub] = time.Now()
		return true
	}
	return false
}

// invalidate removes the tracking entry for a subject, forcing the next
// call to shouldPersist to return true. Used after ClearUsername so the
// identity repopulates immediately on the user's next request.
func (t *identityPersistTracker) invalidate(sub string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, sub)
}

// IdentityPersistMiddleware is the innermost auth-chain handler. It runs
// after userTokenMW and oidcMW have populated the request context with
// the authenticated user's identity, and upserts that identity into the
// authz store. The upsert is TTL-guarded so it does not fire on every
// request.
//
// If authzStore is nil (local-dev escape hatch), the middleware is a
// passthrough.
func (s *Server) IdentityPersistMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Because this middleware is innermost in the auth chain, the OIDC
		// user context has already been set by the outer middleware.
		u, ok := oidc.FromContext(r.Context())
		if !ok || s.authzStore == nil || s.identityTracker == nil {
			next.ServeHTTP(w, r)
			return
		}

		// TTL-guarded persist: shouldPersist returns true at most once per
		// identityPersistTTL interval. After admin ClearUsername, the tracker
		// entry is invalidated so the next request always persists.
		if s.identityTracker.shouldPersist(u.Sub) && (u.Email != "" || u.Username != "") {
			if _, err := s.authzStore.UpsertUser(r.Context(), u.Sub, u.Email, u.Username); err != nil {
				slog.Error("identity persist: upsert failed", "sub", u.Sub, "error", err)
			}
		}

		next.ServeHTTP(w, r)
	})
}

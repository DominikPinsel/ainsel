package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/localauth"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

// SetLocalAuthSecret enables local username/password authentication with the
// given JWT signing secret. Must be called before the server starts serving.
func (s *Server) SetLocalAuthSecret(secret []byte) {
	s.localAuthSecret = secret
	s.loginThrottle = newLoginThrottle()
}

// LocalAuthEnabled reports whether local login is wired.
func (s *Server) LocalAuthEnabled() bool {
	return len(s.localAuthSecret) > 0
}

// minPasswordLen is the password policy floor for create/reset/change.
const minPasswordLen = 8

// usernameRe restricts local usernames to a safe, lowercase, URL-friendly
// character set so they can never collide with OIDC subs or break paths.
var usernameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]{0,62}[a-z0-9])?$`)

func validateUsername(u string) error {
	if !usernameRe.MatchString(u) {
		return errors.New("username must be 1-64 chars of lowercase a-z, 0-9, '.', '_', '-' (no leading/trailing punctuation)")
	}
	return nil
}

func validatePassword(p string) error {
	if len(p) < minPasswordLen {
		return errors.New("password must be at least 8 characters")
	}
	if len(p) > 128 {
		return errors.New("password must be at most 128 characters")
	}
	return nil
}

// handleLogin authenticates a local user and issues a session JWT. Mounted
// OUTSIDE the auth middleware (see serveHTTPInner) but behind rate limiting.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.LocalAuthEnabled() {
		writeError(w, http.StatusForbidden, "local login is not enabled on this hub")
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.Username = strings.ToLower(strings.TrimSpace(body.Username))

	// Per-username backoff on top of the global per-IP rate limiter.
	if locked := s.loginThrottle.denied(body.Username); locked > 0 {
		w.Header().Set("Retry-After", formatRetryAfter(locked))
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	// Generic 401 for every failure mode: unknown user, no local password,
	// wrong password. Never reveal which one it was.
	fail := func() {
		s.loginThrottle.recordFailure(body.Username)
		slog.Warn("auth_login_failed", "log_type", "auth_event", "username", body.Username)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
	}

	if body.Username == "" || body.Password == "" || validateUsername(body.Username) != nil {
		fail()
		return
	}

	id := authz.LocalUserIDPrefix + body.Username
	hash, err := s.authzStore.UserPasswordHash(r.Context(), id)
	if err != nil {
		fail()
		return
	}
	if err := localauth.VerifyPassword(body.Password, hash); err != nil {
		fail()
		return
	}

	u, err := s.authzStore.GetUser(r.Context(), id)
	if err != nil {
		fail()
		return
	}

	token, exp, err := localauth.IssueToken(s.localAuthSecret, u.ID, u.Username, u.IsAdmin)
	if err != nil {
		slog.Error("auth_login: token issue failed", "username", body.Username, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.loginThrottle.recordSuccess(body.Username)
	slog.Info("auth_login", "log_type", "auth_event", "username", body.Username, "isAdmin", u.IsAdmin)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":     token,
		"expiresAt": exp.UTC().Format(time.RFC3339),
		"user": map[string]any{
			"sub":      u.ID,
			"username": u.Username,
			"isAdmin":  u.IsAdmin,
		},
	})
}

// handleLogout is a stateless no-op kept for API symmetry: clients discard
// their token; there is no server-side session to tear down.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleChangePassword lets an authenticated LOCAL user change their own
// password after proving knowledge of the current one.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !strings.HasPrefix(u.Sub, authz.LocalUserIDPrefix) {
		writeError(w, http.StatusBadRequest, "password login is not available for this account")
		return
	}

	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	hash, err := s.authzStore.UserPasswordHash(r.Context(), u.Sub)
	if err != nil || localauth.VerifyPassword(body.Current, hash) != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	if err := validatePassword(body.New); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	newHash, err := localauth.HashPassword(body.New)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.authzStore.SetPassword(r.Context(), u.Sub, newHash); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("auth_password_changed", "log_type", "auth_event", "sub", u.Sub)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func formatRetryAfter(d time.Duration) string {
	return strconv.Itoa(int(d.Seconds()) + 1)
}

// loginThrottle tracks consecutive failed logins per username and imposes an
// exponentially growing cooldown after 3 failures. In-memory only: a restart
// resets it, which is acceptable because the per-IP rate limiter still
// applies and the counter recovers on the next failures.
type loginThrottle struct {
	mu      sync.Mutex
	entries map[string]*throttleEntry
}

type throttleEntry struct {
	failures int
	last     time.Time
}

const throttleThreshold = 3

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{entries: make(map[string]*throttleEntry)}
}

// denied returns the remaining cooldown (>0) when the username is locked out.
func (t *loginThrottle) denied(username string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[username]
	if !ok || e.failures < throttleThreshold {
		return 0
	}
	lock := throttleLockout(e.failures)
	remaining := lock - time.Since(e.last)
	if remaining <= 0 {
		return 0
	}
	return remaining
}

func (t *loginThrottle) recordFailure(username string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[username]
	if !ok {
		e = &throttleEntry{}
		t.entries[username] = e
	}
	e.failures++
	e.last = time.Now()
}

func (t *loginThrottle) recordSuccess(username string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, username)
}

// throttleLockout grows 5s, 10s, 20s, ... capped at 60s.
func throttleLockout(failures int) time.Duration {
	n := failures - throttleThreshold + 1
	lock := 5 * time.Second
	for i := 1; i < n && lock < 60*time.Second; i++ {
		lock *= 2
	}
	if lock > 60*time.Second {
		lock = 60 * time.Second
	}
	return lock
}

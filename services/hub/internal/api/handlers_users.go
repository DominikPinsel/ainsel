package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/localauth"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Any authenticated user can list users.
		users, err := s.authzStore.ListUsers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if users == nil {
			users = []authz.User{}
		}
		writeJSON(w, http.StatusOK, users)
	case http.MethodPost:
		s.handleCreateUser(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleCreateUser creates a locally-managed user (admin only). Local users
// are stored with ID "local:<username>" so they can never collide with OIDC
// subjects.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		IsAdmin  bool   `json:"isAdmin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	body.Username = strings.ToLower(strings.TrimSpace(body.Username))
	if err := validateUsername(body.Username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePassword(body.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := localauth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	id := authz.LocalUserIDPrefix + body.Username
	u, err := s.authzStore.CreateLocalUser(r.Context(), id, strings.TrimSpace(body.Email), body.Username, hash, body.IsAdmin)
	if errors.Is(err, authz.ErrAlreadyExists) {
		writeError(w, http.StatusConflict, "user already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("user_created", "log_type", "auth_event", "id", id, "isAdmin", body.IsAdmin)
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	id := extractName(r.URL.Path, "/api/v1/users/")
	if strings.HasSuffix(id, "/sync") {
		s.handleUserSync(w, r, strings.TrimSuffix(id, "/sync"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		u, err := s.authzStore.GetUser(r.Context(), id)
		if errors.Is(err, authz.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, u)
	case http.MethodPatch:
		if !s.requireAdmin(w, r) {
			return
		}
		var body struct {
			IsAdmin  *bool   `json:"isAdmin"`
			Password *string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.IsAdmin != nil {
			// Protect the last admin: demoting the only admin locks
			// everyone out of user management.
			if !*body.IsAdmin {
				target, err := s.authzStore.GetUser(r.Context(), id)
				if errors.Is(err, authz.ErrNotFound) {
					writeError(w, http.StatusNotFound, "user not found")
					return
				}
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				if target.IsAdmin {
					admins, err := s.countAdmins(r)
					if err != nil {
						writeError(w, http.StatusInternalServerError, err.Error())
						return
					}
					if admins <= 1 {
						writeError(w, http.StatusConflict, "cannot demote the last admin")
						return
					}
				}
			}
			if err := s.authzStore.SetAdmin(r.Context(), id, *body.IsAdmin); err != nil {
				if errors.Is(err, authz.ErrNotFound) {
					writeError(w, http.StatusNotFound, "user not found")
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if body.Password != nil {
			// Admin password reset for any user with local credentials.
			if err := validatePassword(*body.Password); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			hash, err := localauth.HashPassword(*body.Password)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if err := s.authzStore.SetPassword(r.Context(), id, hash); err != nil {
				if errors.Is(err, authz.ErrNotFound) {
					writeError(w, http.StatusNotFound, "user not found")
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			slog.Info("user_password_reset", "log_type", "auth_event", "id", id)
		}
		u, err := s.authzStore.GetUser(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, u)
	case http.MethodDelete:
		s.handleDeleteUser(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleDeleteUser removes a user registry row (admin only). Guards: no
// self-deletion, and the last admin cannot be deleted.
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request, id string) {
	if !s.requireAdmin(w, r) {
		return
	}
	caller, ok := oidc.FromContext(r.Context())
	if ok && caller.Sub == id {
		writeError(w, http.StatusConflict, "cannot delete your own account")
		return
	}
	target, err := s.authzStore.GetUser(r.Context(), id)
	if errors.Is(err, authz.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if target.IsAdmin {
		admins, err := s.countAdmins(r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if admins <= 1 {
			writeError(w, http.StatusConflict, "cannot delete the last admin")
			return
		}
	}
	if err := s.authzStore.DeleteUser(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("user_deleted", "log_type", "auth_event", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// countAdmins returns the number of users with the admin flag set.
func (s *Server) countAdmins(r *http.Request) (int, error) {
	users, err := s.authzStore.ListUsers(r.Context())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, u := range users {
		if u.IsAdmin {
			n++
		}
	}
	return n, nil
}

func (s *Server) handleUserSync(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if id == "me" {
		token, _ := oidc.TokenFromContext(r.Context())
		oidc.ClearUserInfoCacheEntry(u.Sub)

		if s.userInfoURL != "" && token != "" {
			result, err := oidc.FetchUserInfo(r.Context(), s.userInfoClient, s.userInfoURL, token)
			if err != nil {
				var uie *oidc.UserInfoError
				if errors.As(err, &uie) {
					slog.Error("userinfo fetch failed during sync; falling back to JWT claims", "upstream_status", uie.StatusCode, "error", err)
				} else {
					slog.Error("userinfo fetch failed during sync; falling back to JWT claims", "error", err)
				}
				// Graceful degradation: use the JWT claims we already have.
				updated, err := s.authzStore.UpsertUser(r.Context(), u.Sub, u.Email, u.Username)
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, updated)
				return
			}
			updated, err := s.authzStore.UpsertUser(r.Context(), result.Sub, result.Email, result.Username)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, updated)
			return
		}

		// No userinfo URL configured or no token available: persist the
		// identity we already know from the JWT claims / middleware enrichment
		// so the sync button actually updates the registry row.
		updated, err := s.authzStore.UpsertUser(r.Context(), u.Sub, u.Email, u.Username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}

	// Admin sync: evict cache + clear stored username
	if !s.requireAdmin(w, r) {
		return
	}

	oidc.ClearUserInfoCacheEntry(id)

	if err := s.authzStore.ClearUsername(r.Context(), id); err != nil {
		if errors.Is(err, authz.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Invalidate the identity persistence tracker so the username
	// repopulates immediately on the user's next authenticated request.
	if s.identityTracker != nil {
		s.identityTracker.invalidate(id)
	}

	updated, err := s.authzStore.GetUser(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Username is now '' in the response; it repopulates on the user's next authenticated request.
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	if s.authzChecker == nil {
		writeError(w, http.StatusForbidden, "admin required")
		return false
	}
	admin, err := s.authzChecker.IsAdmin(r.Context(), u.Sub)
	if err != nil || !admin {
		writeError(w, http.StatusForbidden, "admin required")
		return false
	}
	return true
}

func (s *Server) handleMyResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	resourceType := r.URL.Query().Get("type")
	if resourceType == "" {
		writeError(w, http.StatusBadRequest, "type query parameter is required")
		return
	}
	groupIDs, _ := s.authzStore.UserGroupIDs(r.Context(), u.Sub)
	page, err := ParsePageParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	includePublic := r.URL.Query().Get("includePublic") == "true"
	all, err := s.authzStore.ListResourcesByGroups(r.Context(), resourceType, groupIDs, includePublic)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	total := len(all)
	lo, hi := page.Slice(total)
	names := all[lo:hi]
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      names,
		"total":      total,
		"page":       page.Page,
		"pageSize":   page.PageSize,
		"totalPages": page.TotalPages(total),
	})
}

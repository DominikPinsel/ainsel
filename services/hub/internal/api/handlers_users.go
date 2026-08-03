package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
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
			IsAdmin *bool `json:"isAdmin"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.IsAdmin != nil {
			if err := s.authzStore.SetAdmin(r.Context(), id, *body.IsAdmin); err != nil {
				if errors.Is(err, authz.ErrNotFound) {
					writeError(w, http.StatusNotFound, "user not found")
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		u, err := s.authzStore.GetUser(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, u)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
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

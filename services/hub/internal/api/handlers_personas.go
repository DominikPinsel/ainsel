package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

// PersonaService is the surface the handlers depend on. Implemented by
// *personas.Service in production; stubbed in tests.
type PersonaService interface {
	Create(ctx context.Context, req personas.CreateRequest) (*personas.Persona, error)
	Get(ctx context.Context, id string) (*personas.Persona, error)
	List(ctx context.Context) ([]personas.PersonaSummary, error)
	Update(ctx context.Context, id string, req personas.UpdateRequest) (*personas.Persona, error)
	Delete(ctx context.Context, id string) error
	ListVersions(ctx context.Context, personaID string) ([]personas.VersionSummary, error)
	GetVersion(ctx context.Context, personaID string, n int) (*personas.Version, error)
	Rollback(ctx context.Context, personaID string, toVersion int) (*personas.Persona, error)
}

// personaHandlers groups handler functions plus their service dependency.
// authzStorePtr is a pointer-to-pointer so it picks up the value set by
// Server.SetAuthZ after construction.
type personaHandlers struct {
	svc           PersonaService
	authzStorePtr *authzStore
	checkerPtr    **authz.Checker
}

func (h *personaHandlers) getAuthzStore() authzStore {
	if h.authzStorePtr == nil {
		return nil
	}
	return *h.authzStorePtr
}

func (h *personaHandlers) getChecker() *authz.Checker {
	if h.checkerPtr == nil {
		return nil
	}
	return *h.checkerPtr
}

// requireRead enforces read access on a persona. Returns true if allowed.
func (h *personaHandlers) requireRead(w http.ResponseWriter, r *http.Request, id string) bool {
	c := h.getChecker()
	if c == nil {
		return true
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	ok, err := c.CanRead(r.Context(), u.Sub, "persona", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// requireWrite enforces write access on a persona. Returns true if allowed.
func (h *personaHandlers) requireWrite(w http.ResponseWriter, r *http.Request, id string) bool {
	c := h.getChecker()
	if c == nil {
		return true
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	ok, err := c.CanWrite(r.Context(), u.Sub, "persona", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// requireGroupWrite checks the caller can create resources in the group.
func (h *personaHandlers) requireGroupWrite(w http.ResponseWriter, r *http.Request, groupID string) bool {
	c := h.getChecker()
	if c == nil {
		return true
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	ok, err := c.CanWriteGroup(r.Context(), u.Sub, groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "writer or owner of the group required")
		return false
	}
	return true
}

// accessibleSet returns the set of persona IDs the caller can read.
// Returns nil if access control is not configured or the caller is an admin
// (meaning: no filtering).
func (h *personaHandlers) accessibleSet(r *http.Request) map[string]bool {
	store := h.getAuthzStore()
	c := h.getChecker()
	if store == nil || c == nil {
		return nil
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		return map[string]bool{}
	}
	admin, _ := c.IsAdmin(r.Context(), u.Sub)
	if admin {
		return nil
	}
	groupIDs, _ := store.UserGroupIDs(r.Context(), u.Sub)
	includePublic := r.URL.Query().Get("includePublic") == "true"
	names, err := store.ListResourcesByGroups(r.Context(), "persona", groupIDs, includePublic)
	if err != nil {
		return map[string]bool{}
	}
	return toAccessSet(names)
}

// RegisterPersonaRoutes wires the persona endpoints into the given mux.
// authzStorePtr points to the Server's authzStore field so it picks up
// the value set later by SetAuthZ.
func RegisterPersonaRoutes(mux *http.ServeMux, svc PersonaService, authzStorePtr *authzStore, checkerPtr **authz.Checker) {
	h := &personaHandlers{svc: svc, authzStorePtr: authzStorePtr, checkerPtr: checkerPtr}
	mux.HandleFunc("/api/v1/personas", h.handleCollection)
	mux.HandleFunc("/api/v1/personas/", h.handleItem)
}

func (h *personaHandlers) handleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		page, err := ParsePageParams(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		all, err := h.svc.List(r.Context())
		if err != nil {
			h.writePersonaError(w, err)
			return
		}
		if set := h.accessibleSet(r); set != nil {
			filtered := all[:0]
			for _, p := range all {
				if set[p.ID] {
					filtered = append(filtered, p)
				}
			}
			all = filtered
		}
		total := len(all)
		lo, hi := page.Slice(total)
		items := all[lo:hi]
		if items == nil {
			items = []personas.PersonaSummary{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":      items,
			"total":      total,
			"page":       page.Page,
			"pageSize":   page.PageSize,
			"totalPages": page.TotalPages(total),
		})
	case http.MethodPost:
		var req personas.CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if h.getChecker() != nil {
			if req.GroupID == "" {
				writeError(w, http.StatusBadRequest, "groupId is required")
				return
			}
			if !h.requireGroupWrite(w, r, req.GroupID) {
				return
			}
		}
		p, err := h.svc.Create(r.Context(), req)
		if err != nil {
			h.writePersonaError(w, err)
			return
		}
		if h.getAuthzStore() != nil {
			groupID := req.GroupID
			if groupID == "" {
				groupID = "legacy"
			}
			if err := h.getAuthzStore().SetResourceGroup(r.Context(), "persona", p.ID, groupID, false); err != nil {
				slog.Error("set resource group on create", "error", err, "resource", p.ID)
			}
		}
		writeJSON(w, http.StatusCreated, p)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleItem routes /api/v1/personas/{id}, /api/v1/personas/{id}/versions,
// /api/v1/personas/{id}/versions/{n}, /api/v1/personas/{id}/rollback.
func (h *personaHandlers) handleItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/personas/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	rest := parts[1:]

	switch {
	case len(rest) == 0:
		h.itemRoot(w, r, id)
	case len(rest) == 1 && rest[0] == "versions":
		h.itemVersions(w, r, id)
	case len(rest) == 2 && rest[0] == "versions":
		n, err := strconv.Atoi(rest[1])
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid version number")
			return
		}
		h.itemVersion(w, r, id, n)
	case len(rest) == 1 && rest[0] == "rollback":
		h.itemRollback(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (h *personaHandlers) itemRoot(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		if !h.requireRead(w, r, id) {
			return
		}
		p, err := h.svc.Get(r.Context(), id)
		if err != nil {
			h.writePersonaError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	case http.MethodPut:
		if !h.requireWrite(w, r, id) {
			return
		}
		var req personas.UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		p, err := h.svc.Update(r.Context(), id, req)
		if err != nil {
			h.writePersonaError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	case http.MethodDelete:
		if !h.requireWrite(w, r, id) {
			return
		}
		if err := h.svc.Delete(r.Context(), id); err != nil {
			h.writePersonaError(w, err)
			return
		}
		if h.getAuthzStore() != nil {
			if err := h.getAuthzStore().DeleteResourceGroup(r.Context(), "persona", id); err != nil {
				slog.Error("delete ownership", "error", err, "resource", id)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *personaHandlers) itemVersions(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	page, err := ParsePageParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	all, err := h.svc.ListVersions(r.Context(), id)
	if err != nil {
		h.writePersonaError(w, err)
		return
	}
	total := len(all)
	lo, hi := page.Slice(total)
	items := all[lo:hi]
	if items == nil {
		items = []personas.VersionSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"total":      total,
		"page":       page.Page,
		"pageSize":   page.PageSize,
		"totalPages": page.TotalPages(total),
	})
}

func (h *personaHandlers) itemVersion(w http.ResponseWriter, r *http.Request, id string, n int) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	v, err := h.svc.GetVersion(r.Context(), id, n)
	if err != nil {
		h.writePersonaError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *personaHandlers) itemRollback(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		ToVersion int `json:"toVersion"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ToVersion <= 0 {
		writeError(w, http.StatusBadRequest, "invalid body: expected {\"toVersion\": <positive int>}")
		return
	}
	p, err := h.svc.Rollback(r.Context(), id, body.ToVersion)
	if err != nil {
		h.writePersonaError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *personaHandlers) writePersonaError(w http.ResponseWriter, err error) {
	var verr *personas.ValidationError
	if errors.As(err, &verr) {
		writeError(w, http.StatusBadRequest, verr.Error())
		return
	}
	var inUse *personas.ErrInUse
	if errors.As(err, &inUse) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":     "persona in use",
			"referrers": inUse.Referrers,
		})
		return
	}
	if errors.Is(err, personas.ErrNameTaken) {
		writeError(w, http.StatusConflict, "persona name already in use")
		return
	}
	if errors.Is(err, personas.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal error: "+err.Error())
}

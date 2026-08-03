package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/DominikPinsel/ainsel/services/hub/internal/authz"
	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

// SkillService is the surface the handlers depend on.
type SkillService interface {
	Create(ctx context.Context, req skills.CreateRequest) (*skills.Skill, error)
	Get(ctx context.Context, id string) (*skills.Skill, error)
	List(ctx context.Context, filter skills.ListFilter) ([]skills.SkillSummary, error)
	Update(ctx context.Context, id string, req skills.UpdateRequest) (*skills.Skill, error)
	Delete(ctx context.Context, id string) error
	ListAssignments(ctx context.Context, skillID string) ([]skills.Referrer, error)
	Assign(ctx context.Context, skillID, agentImageName string) error
	Unassign(ctx context.Context, skillID, agentImageName string) error
}

// skillSortAllowed is the whitelist of columns that can be used with orderBy
// on the skills list endpoint. Values match the JSON field names of
// SkillSummary.
var skillSortAllowed = []string{"name", "id", "usedBy", "updatedAt", "createdAt"}

type skillHandlers struct {
	svc           SkillService
	authzStorePtr *authzStore
	checkerPtr    **authz.Checker
}

func (h *skillHandlers) getAuthzStore() authzStore {
	if h.authzStorePtr == nil {
		return nil
	}
	return *h.authzStorePtr
}

func (h *skillHandlers) getChecker() *authz.Checker {
	if h.checkerPtr == nil {
		return nil
	}
	return *h.checkerPtr
}

func (h *skillHandlers) requireRead(w http.ResponseWriter, r *http.Request, id string) bool {
	c := h.getChecker()
	if c == nil {
		return true
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	ok, err := c.CanRead(r.Context(), u.Sub, "skill", id)
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

func (h *skillHandlers) requireWrite(w http.ResponseWriter, r *http.Request, id string) bool {
	c := h.getChecker()
	if c == nil {
		return true
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	ok, err := c.CanWrite(r.Context(), u.Sub, "skill", id)
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
func (h *skillHandlers) requireGroupWrite(w http.ResponseWriter, r *http.Request, groupID string) bool {
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

// accessibleSet returns the set of skill IDs the caller can read.
// Returns nil if access control is not configured or the caller is an admin.
func (h *skillHandlers) accessibleSet(r *http.Request) map[string]bool {
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
	names, err := store.ListResourcesByGroups(r.Context(), "skill", groupIDs, includePublic)
	if err != nil {
		return map[string]bool{}
	}
	return toAccessSet(names)
}

// RegisterSkillRoutes wires the skill endpoints into the given mux.
func RegisterSkillRoutes(mux *http.ServeMux, svc SkillService, authzStorePtr *authzStore, checkerPtr **authz.Checker) {
	h := &skillHandlers{svc: svc, authzStorePtr: authzStorePtr, checkerPtr: checkerPtr}
	mux.HandleFunc("/api/v1/skills", h.handleCollection)
	mux.HandleFunc("/api/v1/skills/", h.handleItem)
}

func (h *skillHandlers) handleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		page, err := ParsePageParams(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		q := r.URL.Query()
		sortParams, err := ParseSortParams(q, skillSortAllowed)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		filter := skills.ListFilter{
			Search: q.Get("search"),
		}
		if tagsParam := q.Get("tags"); tagsParam != "" {
			for _, t := range strings.Split(tagsParam, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					filter.Tags = append(filter.Tags, t)
				}
			}
		}
		all, err := h.svc.List(r.Context(), filter)
		if err != nil {
			h.writeSkillError(w, err)
			return
		}
		if set := h.accessibleSet(r); set != nil {
			filtered := all[:0]
			for _, sk := range all {
				if set[sk.ID] {
					filtered = append(filtered, sk)
				}
			}
			all = filtered
		}
		// Apply sort if requested. Uses SliceStable with id tiebreaker for
		// deterministic pagination.
		if sortParams.OrderBy != "" {
			sortSkillSummaries(all, sortParams)
		}
		total := len(all)
		lo, hi := page.Slice(total)
		items := all[lo:hi]
		if items == nil {
			items = []skills.SkillSummary{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":      items,
			"total":      total,
			"page":       page.Page,
			"pageSize":   page.PageSize,
			"totalPages": page.TotalPages(total),
		})
	case http.MethodPost:
		var req skills.CreateRequest
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
		sk, err := h.svc.Create(r.Context(), req)
		if err != nil {
			h.writeSkillError(w, err)
			return
		}
		if h.getAuthzStore() != nil {
			groupID := req.GroupID
			if groupID == "" {
				groupID = "legacy"
			}
			if err := h.getAuthzStore().SetResourceGroup(r.Context(), "skill", sk.ID, groupID, false); err != nil {
				slog.Error("set resource group on create", "error", err, "resource", sk.ID)
			}
		}
		writeJSON(w, http.StatusCreated, sk)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// cmpInt returns -1, 0, or 1 for a < b, a == b, a > b.
func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// sortSkillSummaries sorts the slice in-place using SliceStable. The id field
// is used as a tiebreaker so that equal-key rows have a deterministic order
// across pages.
func sortSkillSummaries(items []skills.SkillSummary, sp SortParams) {
	asc := sp.OrderDir != "desc" // default to asc when dir is "" or "asc"

	sort.SliceStable(items, func(i, j int) bool {
		// Three-way primary comparison.
		var cmp int
		switch sp.OrderBy {
		case "name":
			cmp = strings.Compare(strings.ToLower(items[i].Name), strings.ToLower(items[j].Name))
		case "id":
			cmp = strings.Compare(strings.ToLower(items[i].ID), strings.ToLower(items[j].ID))
		case "usedby":
			cmp = cmpInt(items[i].UsedBy, items[j].UsedBy)
		case "updatedat":
			cmp = items[i].UpdatedAt.Compare(items[j].UpdatedAt)
		case "createdat":
			cmp = items[i].CreatedAt.Compare(items[j].CreatedAt)
		default:
			return false
		}
		if cmp != 0 {
			if asc {
				return cmp < 0
			}
			return cmp > 0
		}
		// Tiebreaker: only on genuine equality, always by id ascending.
		return strings.ToLower(items[i].ID) < strings.ToLower(items[j].ID)
	})
}

func (h *skillHandlers) handleItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/skills/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]

	// Sub-path dispatch: /api/v1/skills/:id/assignments[/:agentImageName]
	if len(parts) >= 2 && parts[1] == "assignments" {
		h.handleAssignments(w, r, id, parts[2:])
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !h.requireRead(w, r, id) {
			return
		}
		sk, err := h.svc.Get(r.Context(), id)
		if err != nil {
			h.writeSkillError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sk)
	case http.MethodPut:
		if !h.requireWrite(w, r, id) {
			return
		}
		var req skills.UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		sk, err := h.svc.Update(r.Context(), id, req)
		if err != nil {
			h.writeSkillError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sk)
	case http.MethodDelete:
		if !h.requireWrite(w, r, id) {
			return
		}
		if err := h.svc.Delete(r.Context(), id); err != nil {
			h.writeSkillError(w, err)
			return
		}
		if h.getAuthzStore() != nil {
			if err := h.getAuthzStore().DeleteResourceGroup(r.Context(), "skill", id); err != nil {
				slog.Error("delete ownership", "error", err, "resource", id)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAssignments dispatches assignment sub-routes:
//
//	GET    /api/v1/skills/:id/assignments           → list agent images with this skill
//	PUT    /api/v1/skills/:id/assignments/:imageName → add skill to image
//	DELETE /api/v1/skills/:id/assignments/:imageName → remove skill from image
func (h *skillHandlers) handleAssignments(w http.ResponseWriter, r *http.Request, skillID string, rest []string) {
	// GET /assignments — list referrers
	if len(rest) == 0 {
		switch r.Method {
		case http.MethodGet:
			if !h.requireRead(w, r, skillID) {
				return
			}
			// Verify skill exists.
			if _, err := h.svc.Get(r.Context(), skillID); err != nil {
				h.writeSkillError(w, err)
				return
			}
			refs, err := h.svc.ListAssignments(r.Context(), skillID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if refs == nil {
				refs = []skills.Referrer{}
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": refs})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// PUT/DELETE /assignments/:agentImageName
	if len(rest) == 1 {
		agentImageName := rest[0]
		if agentImageName == "" {
			http.NotFound(w, r)
			return
		}
		// Check method first so unsupported methods get 405 rather than 401/403.
		if r.Method != http.MethodPut && r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		// Authz: require write on the agent-image resource.
		if !h.requireImageWrite(w, r, agentImageName) {
			return
		}
		// Verify skill exists.
		if _, err := h.svc.Get(r.Context(), skillID); err != nil {
			h.writeSkillError(w, err)
			return
		}
		switch r.Method {
		case http.MethodPut:
			if err := h.svc.Assign(r.Context(), skillID, agentImageName); err != nil {
				h.writeSkillError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			if err := h.svc.Unassign(r.Context(), skillID, agentImageName); err != nil {
				h.writeSkillError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}

	http.NotFound(w, r)
}

// requireImageWrite checks the caller can modify the named agent image.
func (h *skillHandlers) requireImageWrite(w http.ResponseWriter, r *http.Request, imageName string) bool {
	c := h.getChecker()
	if c == nil {
		return true
	}
	u, ok := oidc.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	ok, err := c.CanWrite(r.Context(), u.Sub, "agent-image", imageName)
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

func (h *skillHandlers) writeSkillError(w http.ResponseWriter, err error) {
	var verr *skills.ValidationError
	if errors.As(err, &verr) {
		writeError(w, http.StatusBadRequest, verr.Error())
		return
	}
	var inUse *skills.ErrInUse
	if errors.As(err, &inUse) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":     "skill in use",
			"referrers": inUse.Referrers,
		})
		return
	}
	if errors.Is(err, skills.ErrIDTaken) {
		writeError(w, http.StatusConflict, "skill ID already in use")
		return
	}
	if errors.Is(err, skills.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal error: "+err.Error())
}

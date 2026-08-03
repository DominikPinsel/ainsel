package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/DominikPinsel/ainsel/services/hub/internal/mcpservers"
	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// handleAgentImages dispatches GET (list) and POST (create) for /api/v1/agent-images.
func (s *Server) handleAgentImages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		s.listAgentImages(ctx, w, r)
	case http.MethodPost:
		s.createAgentImage(ctx, w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleAgentImagePath peels the {name} segment from the URL and dispatches
// GET, PUT, DELETE for a single AgentImage, or POST for refresh-mcp.
func (s *Server) handleAgentImagePath(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rest := extractName(r.URL.Path, "/api/v1/agent-images/")
	if rest == "" {
		writeError(w, http.StatusBadRequest, "missing agent image name")
		return
	}

	// Handle sub-routes like {name}/refresh-mcp before treating rest as the name.
	if idx := strings.LastIndex(rest, "/"); idx > 0 {
		sub := rest[idx+1:]
		if sub == "refresh-mcp" && r.Method == http.MethodPost {
			s.handleAgentImageRefreshMCP(w, r.WithContext(ctx))
			return
		}
	}

	name := rest
	switch r.Method {
	case http.MethodGet:
		if !s.requireRead(w, r, "agent-image", name) {
			return
		}
		s.getAgentImage(ctx, w, name)
	case http.MethodPut:
		if !s.requireWrite(w, r, "agent-image", name) {
			return
		}
		s.updateAgentImage(ctx, w, r, name)
	case http.MethodDelete:
		if !s.requireWrite(w, r, "agent-image", name) {
			return
		}
		s.deleteAgentImage(ctx, w, name)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listAgentImages(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	page, err := ParsePageParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var list agentv1alpha1.AgentImageList
	if err := s.client.List(ctx, &list, client.InNamespace(s.ns)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].Name < list.Items[j].Name
	})

	// Filter to resources the caller can access.
	if s.authzStore != nil && !s.callerIsAdmin(r) {
		names := make([]string, len(list.Items))
		for i, img := range list.Items {
			names[i] = img.Name
		}
		set := toAccessSet(s.filterByAccess(r, "agent-image", names))
		filtered := list.Items[:0]
		for _, img := range list.Items {
			if set[img.Name] {
				filtered = append(filtered, img)
			}
		}
		list.Items = filtered
	}

	total := len(list.Items)
	lo, hi := page.Slice(total)
	items := make([]SimpleAgentImageResponse, 0, hi-lo)
	for _, img := range list.Items[lo:hi] {
		items = append(items, toSimpleAgentImageResponse(img))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":      items,
		"total":      total,
		"page":       page.Page,
		"pageSize":   page.PageSize,
		"totalPages": page.TotalPages(total),
	})
}

func (s *Server) getAgentImage(ctx context.Context, w http.ResponseWriter, name string) {
	var img agentv1alpha1.AgentImage
	if err := s.client.Get(ctx, types.NamespacedName{Name: name, Namespace: s.ns}, &img); err != nil {
		writeError(w, http.StatusNotFound, "agent image not found")
		return
	}
	writeJSON(w, http.StatusOK, toSimpleAgentImageResponse(img))
}

func (s *Server) createAgentImage(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var req SimpleAgentImageCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "displayName is required")
		return
	}
	if req.ImageURL == "" {
		writeError(w, http.StatusBadRequest, "imageURL is required")
		return
	}

	// Validate group membership for resource creation.
	if s.authzChecker != nil {
		if req.GroupID == "" {
			writeError(w, http.StatusBadRequest, "groupId is required")
			return
		}
		if !s.requireGroupWrite(w, r, req.GroupID) {
			return
		}
	}

	id := generateID("img")

	img := agentv1alpha1.AgentImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: s.ns,
		},
	}
	img.APIVersion = "ainsel.dev/v1alpha1"
	img.Kind = "AgentImage"

	img.Spec.DisplayName = req.DisplayName
	img.Spec.Description = req.Description
	img.Spec.ImageURL = req.ImageURL
	for _, e := range req.Env {
		img.Spec.Env = append(img.Spec.Env, agentv1alpha1.AgentImageEnvVar{Name: e.Name, Value: e.Value, Secret: e.Secret})
	}
	if len(req.MCPServers) > 0 {
		if s.mcp == nil {
			writeError(w, http.StatusServiceUnavailable, "mcp service not configured")
			return
		}
		for _, name := range req.MCPServers {
			mcpServer, err := s.mcp.Get(ctx, name)
			if err != nil {
				if errors.Is(err, mcpservers.ErrNotFound) {
					writeError(w, http.StatusBadRequest, "mcp server not found: "+name)
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			img.Spec.MCPServers = append(img.Spec.MCPServers, agentv1alpha1.AgentImageMCPServer{Name: mcpServer.Name, URL: mcpServer.URL, TokenFromEnv: mcpServer.TokenFromEnv})
		}
	}
	if err := s.validateEnabledSkills(ctx, w, req.EnabledSkills); err != nil {
		return
	}
	img.Spec.EnabledSkills = req.EnabledSkills
	for _, sc := range req.Sidecars {
		sidecar := agentv1alpha1.AgentImageSidecar{
			Name:    sc.Name,
			Image:   sc.Image,
			Port:    sc.Port,
			MCPPath: sc.MCPPath,
		}
		for _, e := range sc.Env {
			sidecar.Env = append(sidecar.Env, agentv1alpha1.AgentImageEnvVar{Name: e.Name, Value: e.Value, Secret: e.Secret})
		}
		img.Spec.Sidecars = append(img.Spec.Sidecars, sidecar)
	}
	img.Status.Phase = agentv1alpha1.AgentImagePhaseReady

	if err := s.client.Create(ctx, &img); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Assign the image to the creator's chosen group.
	if s.authzStore != nil {
		groupID := req.GroupID
		if groupID == "" {
			groupID = "legacy"
		}
		if err := s.authzStore.SetResourceGroup(r.Context(), "agent-image", id, groupID, false); err != nil {
			slog.Error("set resource group on create", "error", err, "resource", id)
		}
	}

	writeJSON(w, http.StatusCreated, toSimpleAgentImageResponse(img))
}

// validateEnabledSkills checks that every skill ID in ids exists in the skills
// service. Returns a non-nil error (already written to w) when any ID is unknown.
func (s *Server) validateEnabledSkills(ctx context.Context, w http.ResponseWriter, ids []string) error {
	if len(ids) == 0 || s.skills == nil {
		return nil
	}
	for _, id := range ids {
		if _, err := s.skills.Get(ctx, id); err != nil {
			if errors.Is(err, skills.ErrNotFound) {
				writeError(w, http.StatusBadRequest, "skill not found: "+id)
				return err
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return err
		}
	}
	return nil
}

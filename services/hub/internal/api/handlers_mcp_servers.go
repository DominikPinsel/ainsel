package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/mcpservers"
	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// envVarNamePattern matches POSIX-style uppercase env-var names. Stricter than
// the Kubernetes EnvVar.Name regex so users get clear errors on typos like
// "forgejo_pat" instead of silent runtime failures.
var envVarNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

const envVarNameMaxLen = 64

type mcpServerDTO struct {
	Name         string    `json:"name"`
	DisplayName  string    `json:"displayName"`
	Description  string    `json:"description,omitempty"`
	URL          string    `json:"url"`
	TokenFromEnv string    `json:"tokenFromEnv,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type mcpServerWriteRequest struct {
	Name         string `json:"name"`
	GroupID      string `json:"groupId"`
	DisplayName  string `json:"displayName"`
	Description  string `json:"description,omitempty"`
	URL          string `json:"url"`
	TokenFromEnv string `json:"tokenFromEnv,omitempty"`
}

func toMCPDTO(m *mcpservers.MCPServer) mcpServerDTO {
	return mcpServerDTO{
		Name:         m.Name,
		DisplayName:  m.DisplayName,
		Description:  m.Description,
		URL:          m.URL,
		TokenFromEnv: m.TokenFromEnv,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// validateTokenFromEnv enforces the env-var-name shape. Empty is allowed and
// means "this MCP server is anonymous; no bearer token is forwarded".
func validateTokenFromEnv(v string) error {
	if v == "" {
		return nil
	}
	if len(v) > envVarNameMaxLen {
		return errors.New("tokenFromEnv must be at most 64 characters")
	}
	if !envVarNamePattern.MatchString(v) {
		return errors.New("tokenFromEnv must match ^[A-Z_][A-Z0-9_]*$")
	}
	return nil
}

func (s *Server) handleMCPServers(w http.ResponseWriter, r *http.Request) {
	if s.mcp == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp service not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		servers, err := s.mcp.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Filter to resources the caller can access.
		if s.authzStore != nil && !s.callerIsAdmin(r) {
			names := make([]string, len(servers))
			for i, m := range servers {
				names[i] = m.Name
			}
			set := toAccessSet(s.filterByAccess(r, "mcp-server", names))
			filtered := servers[:0]
			for _, m := range servers {
				if set[m.Name] {
					filtered = append(filtered, m)
				}
			}
			servers = filtered
		}
		out := make([]mcpServerDTO, 0, len(servers))
		for _, m := range servers {
			out = append(out, toMCPDTO(m))
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req mcpServerWriteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		if req.URL == "" {
			writeError(w, http.StatusBadRequest, "url is required")
			return
		}
		if err := validateTokenFromEnv(req.TokenFromEnv); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
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
		m := &mcpservers.MCPServer{
			Name:         req.Name,
			DisplayName:  req.DisplayName,
			Description:  req.Description,
			URL:          req.URL,
			TokenFromEnv: req.TokenFromEnv,
		}
		if err := s.mcp.Create(r.Context(), m); err != nil {
			if errors.Is(err, mcpservers.ErrAlreadyExists) {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if s.authzStore != nil {
			groupID := req.GroupID
			if groupID == "" {
				groupID = "legacy"
			}
			if err := s.authzStore.SetResourceGroup(r.Context(), "mcp-server", m.Name, groupID, false); err != nil {
				slog.Error("set resource group on create", "error", err, "resource", m.Name)
			}
		}

		saved, _ := s.mcp.Get(r.Context(), m.Name)
		writeJSON(w, http.StatusCreated, toMCPDTO(saved))
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.mcp == nil {
		writeError(w, http.StatusServiceUnavailable, "mcp service not configured")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/mcp-servers/")

	// Handle sub-route /:name/tools
	if idx := strings.LastIndex(rest, "/"); idx > 0 {
		name := rest[:idx]
		sub := rest[idx+1:]
		if sub == "tools" && r.Method == http.MethodGet {
			s.handleMCPServerTools(w, r, name)
			return
		}
	}

	name := rest
	if name == "" || strings.Contains(name, "/") {
		writeError(w, http.StatusBadRequest, "missing or invalid name")
		return
	}

	// Enforce access control before dispatching to the method handler.
	switch r.Method {
	case http.MethodGet:
		if !s.requireRead(w, r, "mcp-server", name) {
			return
		}
	default:
		if !s.requireWrite(w, r, "mcp-server", name) {
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		m, err := s.mcp.Get(r.Context(), name)
		if err != nil {
			if errors.Is(err, mcpservers.ErrNotFound) {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, toMCPDTO(m))
	case http.MethodPut:
		var req mcpServerWriteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		existing, err := s.mcp.Get(r.Context(), name)
		if err != nil {
			if errors.Is(err, mcpservers.ErrNotFound) {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := validateTokenFromEnv(req.TokenFromEnv); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		existing.DisplayName = req.DisplayName
		existing.Description = req.Description
		existing.URL = req.URL
		// tokenFromEnv is not a secret; PUT fully replaces it (empty unsets).
		existing.TokenFromEnv = req.TokenFromEnv
		if err := s.mcp.Update(r.Context(), existing); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		saved, _ := s.mcp.Get(r.Context(), name)
		writeJSON(w, http.StatusOK, toMCPDTO(saved))
	case http.MethodDelete:
		affectedImages, err := s.agentImagesReferencingMCPServer(r.Context(), name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(affectedImages) > 0 {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":          "mcp server referenced by agent image(s)",
				"affectedImages": affectedImages,
			})
			return
		}
		if err := s.mcp.Delete(r.Context(), name); err != nil {
			switch {
			case errors.Is(err, mcpservers.ErrNotFound):
				writeError(w, http.StatusNotFound, err.Error())
			case errors.Is(err, mcpservers.ErrManaged):
				writeError(w, http.StatusConflict, err.Error())
			default:
				writeError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		if s.authzStore != nil {
			if err := s.authzStore.DeleteResourceGroup(r.Context(), "mcp-server", name); err != nil {
				slog.Error("delete ownership", "error", err, "resource", name)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// agentImagesReferencingMCPServer returns the names of all AgentImages in the
// server namespace that have the given MCP server name in their Spec.MCPServers.
func (s *Server) agentImagesReferencingMCPServer(ctx context.Context, name string) ([]string, error) {
	var list agentv1alpha1.AgentImageList
	if err := s.client.List(ctx, &list, client.InNamespace(s.ns)); err != nil {
		return nil, err
	}
	var images []string
	for _, img := range list.Items {
		for _, m := range img.Spec.MCPServers {
			if m.Name == name {
				images = append(images, img.Name)
				break
			}
		}
	}
	return images, nil
}

// handleMCPServerTools proxies a tools/list request to the named MCP server
// and returns the discovered tools. tools/list does not require authentication,
// so the proxy sends no bearer.
func (s *Server) handleMCPServerTools(w http.ResponseWriter, r *http.Request, name string) {
	m, err := s.mcp.Get(r.Context(), name)
	if err != nil {
		if errors.Is(err, mcpservers.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	tools, err := listMCPTools(r.Context(), m.URL, "")
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to list tools: "+err.Error())
		return
	}

	type toolDTO struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	out := make([]toolDTO, 0, len(tools))
	for _, t := range tools {
		out = append(out, toolDTO(t))
	}
	writeJSON(w, http.StatusOK, out)
}

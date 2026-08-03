package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/DominikPinsel/ainsel/services/hub/internal/mcpservers"
	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) updateAgentImage(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	var existing agentv1alpha1.AgentImage
	if err := s.client.Get(ctx, types.NamespacedName{Name: name, Namespace: s.ns}, &existing); err != nil {
		writeError(w, http.StatusNotFound, "agent image not found")
		return
	}

	var req SimpleAgentImageUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.DisplayName != nil {
		existing.Spec.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		existing.Spec.Description = *req.Description
	}
	if req.ImageURL != nil {
		existing.Spec.ImageURL = *req.ImageURL
	}

	if req.Env != nil {
		existingSecrets := make(map[string]string, len(existing.Spec.Env))
		for _, e := range existing.Spec.Env {
			if e.Secret {
				existingSecrets[e.Name] = e.Value
			}
		}

		newEnv := make([]agentv1alpha1.AgentImageEnvVar, 0, len(*req.Env))
		for _, e := range *req.Env {
			ev := agentv1alpha1.AgentImageEnvVar{Name: e.Name, Value: e.Value, Secret: e.Secret}
			if ev.Secret && ev.Value == "" {
				if existingVal, ok := existingSecrets[ev.Name]; ok {
					ev.Value = existingVal
				}
			}
			newEnv = append(newEnv, ev)
		}
		existing.Spec.Env = newEnv
	}

	if req.Tools != nil {
		afterNames := make(map[string]struct{}, len(*req.Tools))
		for _, t := range *req.Tools {
			afterNames[t.Name] = struct{}{}
		}

		var removedTools []string
		for _, t := range existing.Spec.Tools {
			if _, keep := afterNames[t.Name]; !keep {
				removedTools = append(removedTools, t.Name)
			}
		}

		if len(removedTools) > 0 {
			affectedAgents, err := s.agentsReferencingTools(ctx, name, removedTools)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if len(affectedAgents) > 0 {
				writeJSON(w, http.StatusConflict, map[string]interface{}{
					"error":          "tool referenced by agent(s)",
					"affectedAgents": affectedAgents,
					"removedTools":   removedTools,
				})
				return
			}
		}

		newTools := make([]agentv1alpha1.AgentImageTool, 0, len(*req.Tools))
		for _, t := range *req.Tools {
			tool := agentv1alpha1.AgentImageTool{
				Name:        t.Name,
				Kind:        agentv1alpha1.AgentImageToolKind(t.Kind),
				Description: t.Description,
				McpSource:   t.McpSource,
				Disabled:    !t.Enabled,
				IsNew:       false,
			}
			for _, ex := range t.Examples {
				tool.Examples = append(tool.Examples, agentv1alpha1.AgentImageToolExample{
					Title:   ex.Title,
					Snippet: ex.Snippet,
				})
			}
			newTools = append(newTools, tool)
		}
		existing.Spec.Tools = newTools
	}

	if req.MCPServers != nil {
		if s.mcp == nil {
			writeError(w, http.StatusServiceUnavailable, "mcp service not configured")
			return
		}
		newMCPServers := make([]agentv1alpha1.AgentImageMCPServer, 0, len(*req.MCPServers))
		for _, name := range *req.MCPServers {
			mcpServer, err := s.mcp.Get(ctx, name)
			if err != nil {
				if errors.Is(err, mcpservers.ErrNotFound) {
					writeError(w, http.StatusBadRequest, "mcp server not found: "+name)
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			newMCPServers = append(newMCPServers, agentv1alpha1.AgentImageMCPServer{
				Name:         mcpServer.Name,
				URL:          mcpServer.URL,
				TokenFromEnv: mcpServer.TokenFromEnv,
			})
		}
		existing.Spec.MCPServers = newMCPServers
	}

	if req.Sidecars != nil {
		newSidecars := make([]agentv1alpha1.AgentImageSidecar, 0, len(*req.Sidecars))
		for _, sc := range *req.Sidecars {
			sidecar := agentv1alpha1.AgentImageSidecar{
				Name:    sc.Name,
				Image:   sc.Image,
				Port:    sc.Port,
				MCPPath: sc.MCPPath,
			}
			for _, e := range sc.Env {
				sidecar.Env = append(sidecar.Env, agentv1alpha1.AgentImageEnvVar{Name: e.Name, Value: e.Value, Secret: e.Secret})
			}
			newSidecars = append(newSidecars, sidecar)
		}
		existing.Spec.Sidecars = newSidecars
	}

	if req.EnabledSkills != nil {
		if err := s.validateEnabledSkills(ctx, w, *req.EnabledSkills); err != nil {
			return
		}
		existing.Spec.EnabledSkills = *req.EnabledSkills
	}

	if err := s.client.Update(ctx, &existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toSimpleAgentImageResponse(existing))
}

func (s *Server) deleteAgentImage(ctx context.Context, w http.ResponseWriter, name string) {
	var img agentv1alpha1.AgentImage
	if err := s.client.Get(ctx, types.NamespacedName{Name: name, Namespace: s.ns}, &img); err != nil {
		if apierrors.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "agent image not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	affectedAgents, err := s.agentsReferencingImage(ctx, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(affectedAgents) > 0 {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error":          "agent image referenced by agent(s)",
			"affectedAgents": affectedAgents,
		})
		return
	}

	if err := s.client.Delete(ctx, &img); err != nil {
		if apierrors.IsNotFound(err) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.authzStore != nil {
		if err := s.authzStore.DeleteResourceGroup(ctx, "agent-image", name); err != nil {
			slog.Error("delete ownership", "error", err, "resource", name)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) agentsReferencingImage(ctx context.Context, imageName string) ([]string, error) {
	var agentList agentv1alpha1.AgentList
	if err := s.client.List(ctx, &agentList, client.InNamespace(s.ns)); err != nil {
		return nil, err
	}
	var names []string
	for _, a := range agentList.Items {
		if a.Spec.ImageRef.Name == imageName {
			names = append(names, a.Name)
		}
	}
	return names, nil
}

func (s *Server) agentsReferencingTools(ctx context.Context, imageName string, removedToolNames []string) ([]string, error) {
	removed := make(map[string]struct{}, len(removedToolNames))
	for _, n := range removedToolNames {
		removed[n] = struct{}{}
	}

	var agentList agentv1alpha1.AgentList
	if err := s.client.List(ctx, &agentList, client.InNamespace(s.ns)); err != nil {
		return nil, err
	}

	var affected []string
	for _, a := range agentList.Items {
		if a.Spec.ImageRef.Name != imageName {
			continue
		}
		for _, tool := range a.Spec.EnabledTools {
			if _, isRemoved := removed[tool]; isRemoved {
				affected = append(affected, a.Name)
				break
			}
		}
	}
	return affected, nil
}

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AgentImageRefInfo identifies which AgentImage an Agent uses.
type AgentImageRefInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
}

// SimpleAgentResponse is the simplified API representation of an Agent.
type SimpleAgentResponse struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Description    string                   `json:"description,omitempty"`
	ImageRef       AgentImageRefInfo        `json:"imageRef"`
	LLM            AgentLLMInfo             `json:"llm"`
	Persona        *AgentPersonaInfo        `json:"persona,omitempty"`
	EnabledTools   []string                 `json:"enabledTools,omitempty"`
	Replicas       *int32                   `json:"replicas,omitempty"`
	Memory         *AgentMemoryInfo         `json:"memory,omitempty"`
	OllamaCloud    *AgentOllamaCloudInfo    `json:"ollamaCloud,omitempty"`
	OpenCode       *AgentOpenCodeInfo       `json:"openCode,omitempty"`
	AlibabaCloud   *AgentAlibabaCloudInfo   `json:"alibabaCloud,omitempty"`
	CustomProvider *AgentCustomProviderInfo `json:"customProvider,omitempty"`
	Status         *SimpleAgentStatus       `json:"status,omitempty"`
}

type AgentLLMInfo struct {
	Model       string   `json:"model"`
	Provider    string   `json:"provider,omitempty"`
	MaxTurns    int      `json:"maxTurns,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

// AgentPersonaInfo is the REST representation of an Agent's persona reference.
// ID points at a persona managed by the hub at /api/v1/personas.
type AgentPersonaInfo struct {
	ID string `json:"id"`
}

type AgentMemoryInfo struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider,omitempty"`
}

type AgentOllamaCloudInfo struct {
	// APIKey is the Ollama Cloud API key. If provided, the backend will create
	// a secret to store it. On response, this field is always empty for security.
	APIKey string `json:"apiKey,omitempty"`
}

type AgentOpenCodeInfo struct {
	// APIKey is the OpenCode API key. If provided, the backend will create
	// a secret to store it. On response, this field is always empty for security.
	APIKey string `json:"apiKey,omitempty"`
}

type AgentAlibabaCloudInfo struct {
	// APIKey is the Alibaba Token Plan API key. If provided, the backend will create
	// a secret to store it. On response, this field is always empty for security.
	APIKey string `json:"apiKey,omitempty"`
}

// AgentCustomProviderInfo is the REST representation of a custom LLM provider.
type AgentCustomProviderInfo struct {
	// URL is the custom LLM API base URL.
	URL string `json:"url"`
	// APIKey is the custom API key. If provided, the backend will create
	// a secret to store it. On response, this field is always empty for security.
	APIKey string `json:"apiKey,omitempty"`
}

type SimpleAgentStatus struct {
	Ready    bool  `json:"ready"`
	Replicas int32 `json:"replicas"`
}

// SimpleAgentCreateRequest is used to create a new Agent.
type SimpleAgentCreateRequest struct {
	Name           string                   `json:"name"`
	GroupID        string                   `json:"groupId"`
	Description    string                   `json:"description,omitempty"`
	ImageRef       AgentImageRefInfo        `json:"imageRef"`
	LLM            AgentLLMInfo             `json:"llm"`
	Persona        *AgentPersonaInfo        `json:"persona,omitempty"`
	EnabledTools   []string                 `json:"enabledTools,omitempty"`
	Replicas       *int32                   `json:"replicas,omitempty"`
	Memory         *AgentMemoryInfo         `json:"memory,omitempty"`
	OllamaCloud    *AgentOllamaCloudInfo    `json:"ollamaCloud,omitempty"`
	OpenCode       *AgentOpenCodeInfo       `json:"openCode,omitempty"`
	AlibabaCloud   *AgentAlibabaCloudInfo   `json:"alibabaCloud,omitempty"`
	CustomProvider *AgentCustomProviderInfo `json:"customProvider,omitempty"`
}

// SimpleAgentUpdateRequest is used to update an existing Agent. All fields are optional.
type SimpleAgentUpdateRequest struct {
	Name           *string                  `json:"name,omitempty"`
	Description    *string                  `json:"description,omitempty"`
	ImageRef       *AgentImageRefInfo       `json:"imageRef,omitempty"`
	LLM            *AgentLLMInfo            `json:"llm,omitempty"`
	Persona        *AgentPersonaInfo        `json:"persona,omitempty"`
	EnabledTools   *[]string                `json:"enabledTools,omitempty"`
	Replicas       *int32                   `json:"replicas,omitempty"`
	Memory         *AgentMemoryInfo         `json:"memory,omitempty"`
	OllamaCloud    *AgentOllamaCloudInfo    `json:"ollamaCloud,omitempty"`
	OpenCode       *AgentOpenCodeInfo       `json:"openCode,omitempty"`
	AlibabaCloud   *AgentAlibabaCloudInfo   `json:"alibabaCloud,omitempty"`
	CustomProvider *AgentCustomProviderInfo `json:"customProvider,omitempty"`
}

func toSimpleAgentResponse(a agentv1alpha1.Agent, imageDisplayName string) SimpleAgentResponse {
	resp := SimpleAgentResponse{
		ID:          a.Name,
		Name:        a.Spec.DisplayName,
		Description: a.Spec.Description,
		ImageRef:    AgentImageRefInfo{Name: a.Spec.ImageRef.Name, DisplayName: imageDisplayName},
		LLM: AgentLLMInfo{
			Model:       a.Spec.LLM.Model,
			Provider:    a.Spec.LLM.Provider,
			MaxTurns:    a.Spec.LLM.MaxTurns,
			Temperature: a.Spec.LLM.Temperature,
		},
		EnabledTools: a.Spec.EnabledTools,
	}

	if a.Spec.Persona.ID != "" {
		resp.Persona = &AgentPersonaInfo{ID: a.Spec.Persona.ID}
	}

	// Replicas
	if a.Spec.Scaling != nil {
		resp.Replicas = a.Spec.Scaling.Replicas
	}

	// Memory
	if a.Spec.Memory != nil {
		resp.Memory = &AgentMemoryInfo{
			Enabled:  a.Spec.Memory.Enabled,
			Provider: a.Spec.Memory.Provider,
		}
	}

	// OllamaCloud - note: API key is never returned in responses for security
	if a.Spec.OllamaCloud != nil {
		resp.OllamaCloud = &AgentOllamaCloudInfo{}
	}

	// OpenCode - note: API key is never returned in responses for security
	if a.Spec.OpenCode != nil {
		resp.OpenCode = &AgentOpenCodeInfo{}
	}

	// AlibabaCloud - note: API key is never returned in responses for security
	if a.Spec.AlibabaCloud != nil {
		resp.AlibabaCloud = &AgentAlibabaCloudInfo{}
	}

	// CustomProvider - note: API key is never returned in responses for security
	if a.Spec.CustomProvider != nil {
		resp.CustomProvider = &AgentCustomProviderInfo{
			URL: a.Spec.CustomProvider.URL,
		}
	}

	// Status
	ready := false
	for _, c := range a.Status.Conditions {
		if c.Type == agentv1alpha1.AgentConditionReady && c.Status == metav1.ConditionTrue {
			ready = true
			break
		}
	}
	resp.Status = &SimpleAgentStatus{
		Ready:    ready,
		Replicas: a.Status.Replicas,
	}

	return resp
}

// piBuiltinToolNames are the built-in tools provided by the pi agent runtime
// (read, bash, edit, write, grep, find, ls). They are always available to any
// agent regardless of whether the AgentImage explicitly lists them, because pi
// registers them by default.
var piBuiltinToolNames = map[string]struct{}{
	"read":  {},
	"bash":  {},
	"edit":  {},
	"write": {},
	"grep":  {},
	"find":  {},
	"ls":    {},
}

// validateAgentImageRef checks that refName references an existing AgentImage and that
// all entries in enabled are tools declared by that image (or pi built-in tools).
// Returns the image's display name along with a non-zero status code and error
// message on failure; returns displayName, 0, "" on success.
func (s *Server) validateAgentImageRef(ctx context.Context, refName string, enabled []string) (displayName string, statusCode int, errMessage string) {
	if strings.TrimSpace(refName) == "" {
		return "", http.StatusBadRequest, "imageRef.name is required"
	}
	var img agentv1alpha1.AgentImage
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.ns, Name: refName}, &img); err != nil {
		if apierrors.IsNotFound(err) {
			return "", http.StatusBadRequest, fmt.Sprintf("imageRef.name %q does not match any AgentImage", refName)
		}
		return "", http.StatusInternalServerError, "lookup image: " + err.Error()
	}
	valid := make(map[string]struct{}, len(img.Spec.Tools))
	for _, t := range img.Spec.Tools {
		valid[t.Name] = struct{}{}
	}
	var unknown []string
	for _, t := range enabled {
		if _, ok := valid[t]; !ok {
			if _, ok := piBuiltinToolNames[t]; !ok {
				unknown = append(unknown, t)
			}
		}
	}
	if len(unknown) > 0 {
		return "", http.StatusBadRequest, "enabledTools not in image: " + strings.Join(unknown, ",")
	}
	return img.Spec.DisplayName, 0, ""
}

// resolveImageDisplayName looks up the AgentImage by resource name and returns
// its Spec.DisplayName. Returns empty string on any lookup error (not found,
// RBAC, network, etc.).
func (s *Server) resolveImageDisplayName(ctx context.Context, imageName string) string {
	var img agentv1alpha1.AgentImage
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.ns, Name: imageName}, &img); err != nil {
		return ""
	}
	return img.Spec.DisplayName
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		s.listAgents(ctx, w, r)
	case http.MethodPost:
		s.createAgent(ctx, w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := extractName(r.URL.Path, "/api/v1/agents/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing agent name")
		return
	}

	// Dispatch event-queue sub-routes before CRUD.
	if strings.HasSuffix(name, "/next-task") {
		s.handleAgentNextTask(w, r)
		return
	}
	if strings.Contains(name, "/tasks/") {
		if strings.HasSuffix(name, "/ack") {
			s.handleAgentTaskAck(w, r)
			return
		}
		if strings.HasSuffix(name, "/nack") {
			s.handleAgentTaskNack(w, r)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		if !s.requireRead(w, r, "agent", name) {
			return
		}
		s.getAgent(ctx, w, name)
	case http.MethodPut:
		if !s.requireWrite(w, r, "agent", name) {
			return
		}
		s.updateAgent(ctx, w, r, name)
	case http.MethodDelete:
		if !s.requireWrite(w, r, "agent", name) {
			return
		}
		s.deleteAgent(ctx, w, name)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleInternalAgent serves agent task endpoints under /api/internal/agents/.
// These paths bypass the OIDC middleware (they don't match /api/v1/) and are
// protected by X-Internal-Token at the handler level. Only task operations
// (next-task, ack, nack) are exposed; CRUD operations remain on /api/v1/.
func (s *Server) handleInternalAgent(w http.ResponseWriter, r *http.Request) {
	name := extractName(r.URL.Path, "/api/internal/agents/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing agent name")
		return
	}

	if strings.HasSuffix(name, "/next-task") {
		s.handleAgentNextTask(w, r)
		return
	}
	if strings.Contains(name, "/tasks/") {
		if strings.HasSuffix(name, "/ack") {
			s.handleAgentTaskAck(w, r)
			return
		}
		if strings.HasSuffix(name, "/nack") {
			s.handleAgentTaskNack(w, r)
			return
		}
	}

	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) listAgents(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	page, err := ParsePageParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var list agentv1alpha1.AgentList
	if err := s.client.List(ctx, &list, client.InNamespace(s.ns)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Sort by resource name for stable pagination across requests. Without an
	// explicit sort the underlying client returns items in map-iteration order
	// (in tests) or arbitrary kube-apiserver order, so successive page= calls
	// could see the same item twice or miss it entirely.
	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].Name < list.Items[j].Name
	})

	// Filter to resources the caller can access (own groups + optional public).
	if s.authzStore != nil && !s.callerIsAdmin(r) {
		names := make([]string, len(list.Items))
		for i, a := range list.Items {
			names[i] = a.Name
		}
		set := toAccessSet(s.filterByAccess(r, "agent", names))
		filtered := list.Items[:0]
		for _, a := range list.Items {
			if set[a.Name] {
				filtered = append(filtered, a)
			}
		}
		list.Items = filtered
	}

	// Build image id→displayName map for enrichment. A single namespace-scoped
	// List avoids N+1 Gets per agent.
	imageDisplayNames := make(map[string]string)
	var imageList agentv1alpha1.AgentImageList
	if err := s.client.List(ctx, &imageList, client.InNamespace(s.ns)); err == nil {
		for _, img := range imageList.Items {
			imageDisplayNames[img.Name] = img.Spec.DisplayName
		}
	} else {
		slog.Warn("failed to list AgentImages for display name enrichment", "error", err)
	}

	total := len(list.Items)
	lo, hi := page.Slice(total)
	items := make([]SimpleAgentResponse, 0, hi-lo)
	for _, a := range list.Items[lo:hi] {
		items = append(items, toSimpleAgentResponse(a, imageDisplayNames[a.Spec.ImageRef.Name]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":      items,
		"total":      total,
		"page":       page.Page,
		"pageSize":   page.PageSize,
		"totalPages": page.TotalPages(total),
	})
}

func (s *Server) getAgent(ctx context.Context, w http.ResponseWriter, name string) {
	var agent agentv1alpha1.Agent
	if err := s.client.Get(ctx, types.NamespacedName{Name: name, Namespace: s.ns}, &agent); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	imageDisplayName := s.resolveImageDisplayName(ctx, agent.Spec.ImageRef.Name)
	writeJSON(w, http.StatusOK, toSimpleAgentResponse(agent, imageDisplayName))
}

func (s *Server) createAgent(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var req SimpleAgentCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	imageDisplayName, code, msg := s.validateAgentImageRef(ctx, req.ImageRef.Name, req.EnabledTools)
	if code != 0 {
		http.Error(w, msg, code)
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

	id := generateID("a")

	agent := agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: s.ns,
		},
	}
	agent.APIVersion = "ainsel.dev/v1alpha1"
	agent.Kind = "Agent"

	agent.Spec.DisplayName = req.Name
	agent.Spec.Description = req.Description
	agent.Spec.ImageRef = agentv1alpha1.AgentImageRef{Name: req.ImageRef.Name}
	agent.Spec.EnabledTools = req.EnabledTools
	agent.Spec.LLM = agentv1alpha1.AgentLLM{
		Model:       req.LLM.Model,
		Provider:    req.LLM.Provider,
		MaxTurns:    req.LLM.MaxTurns,
		Temperature: req.LLM.Temperature,
	}
	if req.Persona != nil {
		agent.Spec.Persona = agentv1alpha1.AgentPersona{
			ID: req.Persona.ID,
		}
	}
	if req.Replicas != nil {
		agent.Spec.Scaling = &agentv1alpha1.AgentScaling{
			Replicas: req.Replicas,
		}
	}
	if req.Memory != nil {
		agent.Spec.Memory = &agentv1alpha1.AgentMemory{
			Enabled:  req.Memory.Enabled,
			Provider: req.Memory.Provider,
		}
	}

	// Handle Ollama Cloud API key - create secret if provided
	if req.OllamaCloud != nil && req.OllamaCloud.APIKey != "" {
		secretName := fmt.Sprintf("%s-llm-key", agent.Name)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: s.ns,
			},
			StringData: map[string]string{
				"api-key": req.OllamaCloud.APIKey,
			},
		}
		if err := s.client.Create(ctx, secret); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create Ollama Cloud secret: "+err.Error())
			return
		}
		agent.Spec.OllamaCloud = &agentv1alpha1.AgentOllamaCloud{
			APIKeySecretRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  "api-key",
			},
		}
	}

	// Handle OpenCode API key - create secret if provided
	if req.OpenCode != nil && req.OpenCode.APIKey != "" {
		secretName := fmt.Sprintf("%s-opencode-key", agent.Name)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: s.ns,
			},
			StringData: map[string]string{
				"api-key": req.OpenCode.APIKey,
			},
		}
		if err := s.client.Create(ctx, secret); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create OpenCode secret: "+err.Error())
			return
		}
		agent.Spec.OpenCode = &agentv1alpha1.AgentOpenCode{
			APIKeySecretRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  "api-key",
			},
		}
	}

	// Handle Alibaba Token Plan API key - create secret if provided
	if req.AlibabaCloud != nil && req.AlibabaCloud.APIKey != "" {
		secretName := fmt.Sprintf("%s-alibaba-key", agent.Name)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: s.ns,
			},
			StringData: map[string]string{
				"api-key": req.AlibabaCloud.APIKey,
			},
		}
		if err := s.client.Create(ctx, secret); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create Alibaba Token Plan secret: "+err.Error())
			return
		}
		agent.Spec.AlibabaCloud = &agentv1alpha1.AgentAlibabaCloud{
			APIKeySecretRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  "api-key",
			},
		}
	}

	// Handle custom provider - create secret + store URL if provided
	if req.CustomProvider != nil {
		// URL is required for custom providers
		if req.CustomProvider.URL == "" {
			writeError(w, http.StatusBadRequest, "custom provider URL is required")
			return
		}

		agent.Spec.CustomProvider = &agentv1alpha1.AgentCustomProvider{
			URL: req.CustomProvider.URL,
		}

		// API key is optional on the wire; create a secret only if provided.
		if req.CustomProvider.APIKey != "" {
			secretName := fmt.Sprintf("%s-custom-provider-key", agent.Name)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: s.ns,
				},
				StringData: map[string]string{
					"api-key": req.CustomProvider.APIKey,
				},
			}
			if err := s.client.Create(ctx, secret); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to create custom provider secret: "+err.Error())
				return
			}
			agent.Spec.CustomProvider.APIKeySecretRef = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  "api-key",
			}
		}
	}

	if err := s.client.Create(ctx, &agent); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Assign the agent to the creator's chosen group.
	if s.authzStore != nil {
		groupID := req.GroupID
		if groupID == "" {
			groupID = "legacy"
		}
		if err := s.authzStore.SetResourceGroup(r.Context(), "agent", id, groupID, false); err != nil {
			slog.Error("set resource group on create", "error", err, "resource", id)
		}
	}

	writeJSON(w, http.StatusCreated, toSimpleAgentResponse(agent, imageDisplayName))
}

func (s *Server) updateAgent(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	var existing agentv1alpha1.Agent
	if err := s.client.Get(ctx, types.NamespacedName{Name: name, Namespace: s.ns}, &existing); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	var req SimpleAgentUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate imageRef/enabledTools if either is being changed.
	effectiveRef := existing.Spec.ImageRef.Name
	if req.ImageRef != nil {
		effectiveRef = req.ImageRef.Name
	}
	effectiveTools := existing.Spec.EnabledTools
	if req.EnabledTools != nil {
		effectiveTools = *req.EnabledTools
	}
	var validatedDisplayName string
	if req.ImageRef != nil || req.EnabledTools != nil {
		var code int
		var msg string
		validatedDisplayName, code, msg = s.validateAgentImageRef(ctx, effectiveRef, effectiveTools)
		if code != 0 {
			http.Error(w, msg, code)
			return
		}
	}

	if req.Name != nil {
		existing.Spec.DisplayName = *req.Name
	}
	if req.Description != nil {
		existing.Spec.Description = *req.Description
	}
	if req.ImageRef != nil {
		existing.Spec.ImageRef = agentv1alpha1.AgentImageRef{Name: req.ImageRef.Name}
	}
	if req.EnabledTools != nil {
		existing.Spec.EnabledTools = *req.EnabledTools
	}
	if req.LLM != nil {
		if req.LLM.Model != "" {
			existing.Spec.LLM.Model = req.LLM.Model
		}
		if req.LLM.Provider != "" {
			existing.Spec.LLM.Provider = req.LLM.Provider
		}
		if req.LLM.MaxTurns != 0 {
			existing.Spec.LLM.MaxTurns = req.LLM.MaxTurns
		}
		if req.LLM.Temperature != nil {
			existing.Spec.LLM.Temperature = req.LLM.Temperature
		}
	}
	if req.Persona != nil && req.Persona.ID != "" {
		existing.Spec.Persona = agentv1alpha1.AgentPersona{ID: req.Persona.ID}
	}
	if req.Replicas != nil {
		if existing.Spec.Scaling == nil {
			existing.Spec.Scaling = &agentv1alpha1.AgentScaling{}
		}
		existing.Spec.Scaling.Replicas = req.Replicas
	}
	if req.Memory != nil {
		existing.Spec.Memory = &agentv1alpha1.AgentMemory{
			Enabled:  req.Memory.Enabled,
			Provider: req.Memory.Provider,
		}
	}

	// Handle Ollama Cloud API key update
	if req.OllamaCloud != nil {
		if req.OllamaCloud.APIKey != "" {
			secretName := fmt.Sprintf("%s-llm-key", existing.Name)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: s.ns,
				},
				StringData: map[string]string{
					"api-key": req.OllamaCloud.APIKey,
				},
			}
			if err := s.client.Create(ctx, secret); err != nil {
				if apierrors.IsAlreadyExists(err) {
					if err := s.client.Update(ctx, secret); err != nil {
						writeError(w, http.StatusInternalServerError, "failed to update Ollama Cloud secret: "+err.Error())
						return
					}
				} else {
					writeError(w, http.StatusInternalServerError, "failed to create Ollama Cloud secret: "+err.Error())
					return
				}
			}
			existing.Spec.OllamaCloud = &agentv1alpha1.AgentOllamaCloud{
				APIKeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
					Key:                  "api-key",
				},
			}
		}
	}

	// Handle OpenCode API key update
	if req.OpenCode != nil {
		if req.OpenCode.APIKey != "" {
			secretName := fmt.Sprintf("%s-opencode-key", existing.Name)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: s.ns,
				},
				StringData: map[string]string{
					"api-key": req.OpenCode.APIKey,
				},
			}
			if err := s.client.Create(ctx, secret); err != nil {
				if apierrors.IsAlreadyExists(err) {
					if err := s.client.Update(ctx, secret); err != nil {
						writeError(w, http.StatusInternalServerError, "failed to update OpenCode secret: "+err.Error())
						return
					}
				} else {
					writeError(w, http.StatusInternalServerError, "failed to create OpenCode secret: "+err.Error())
					return
				}
			}
			existing.Spec.OpenCode = &agentv1alpha1.AgentOpenCode{
				APIKeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
					Key:                  "api-key",
				},
			}
		}
	}

	// Handle Alibaba Token Plan API key update
	if req.AlibabaCloud != nil {
		if req.AlibabaCloud.APIKey != "" {
			secretName := fmt.Sprintf("%s-alibaba-key", existing.Name)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: s.ns,
				},
				StringData: map[string]string{
					"api-key": req.AlibabaCloud.APIKey,
				},
			}
			if err := s.client.Create(ctx, secret); err != nil {
				if apierrors.IsAlreadyExists(err) {
					if err := s.client.Update(ctx, secret); err != nil {
						writeError(w, http.StatusInternalServerError, "failed to update Alibaba Token Plan secret: "+err.Error())
						return
					}
				} else {
					writeError(w, http.StatusInternalServerError, "failed to create Alibaba Token Plan secret: "+err.Error())
					return
				}
			}
			existing.Spec.AlibabaCloud = &agentv1alpha1.AgentAlibabaCloud{
				APIKeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
					Key:                  "api-key",
				},
			}
		}
	}

	// Handle custom provider update (URL and optional API key).
	// When the agent has no existing custom provider, URL is required: otherwise
	// we'd create an orphan API-key secret while leaving spec.CustomProvider nil
	// (and dereferencing it crashed the handler).
	if req.CustomProvider != nil {
		if existing.Spec.CustomProvider == nil {
			if req.CustomProvider.URL == "" {
				writeError(w, http.StatusBadRequest, "custom provider URL is required")
				return
			}
			existing.Spec.CustomProvider = &agentv1alpha1.AgentCustomProvider{}
		}
		if req.CustomProvider.URL != "" {
			existing.Spec.CustomProvider.URL = req.CustomProvider.URL
		}
		if req.CustomProvider.APIKey != "" {
			secretName := fmt.Sprintf("%s-custom-provider-key", existing.Name)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: s.ns,
				},
				StringData: map[string]string{
					"api-key": req.CustomProvider.APIKey,
				},
			}
			if err := s.client.Create(ctx, secret); err != nil {
				if apierrors.IsAlreadyExists(err) {
					if err := s.client.Update(ctx, secret); err != nil {
						writeError(w, http.StatusInternalServerError, "failed to update custom provider secret: "+err.Error())
						return
					}
				} else {
					writeError(w, http.StatusInternalServerError, "failed to create custom provider secret: "+err.Error())
					return
				}
			}
			existing.Spec.CustomProvider.APIKeySecretRef = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  "api-key",
			}
		}
	}

	if err := s.client.Update(ctx, &existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Resolve image display name for the response. If we already validated the
	// image ref above, reuse that display name; otherwise look it up.
	imageDisplayName := validatedDisplayName
	if imageDisplayName == "" && req.ImageRef == nil {
		imageDisplayName = s.resolveImageDisplayName(ctx, existing.Spec.ImageRef.Name)
	}
	writeJSON(w, http.StatusOK, toSimpleAgentResponse(existing, imageDisplayName))
}

func (s *Server) deleteAgent(ctx context.Context, w http.ResponseWriter, name string) {
	agent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.ns},
	}
	if err := s.client.Delete(ctx, agent); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Clean up ownership record.
	if s.authzStore != nil {
		if err := s.authzStore.DeleteResourceGroup(ctx, "agent", name); err != nil {
			slog.Error("delete ownership", "error", err, "resource", name)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

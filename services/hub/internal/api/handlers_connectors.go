package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	connectorv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type WebhookConnectorCreateRequest struct {
	Name            string `json:"name"`
	GroupID         string `json:"groupId"`
	SignatureHeader string `json:"signatureHeader"`
}

type WebhookConnectorUpdateRequest struct {
	Name     string `json:"name,omitempty"`
	Disabled *bool  `json:"disabled,omitempty"`
}

type WebhookConnectorResponse struct {
	ID                 string                  `json:"id"`
	Name               string                  `json:"name"`
	SignatureHeader    string                  `json:"signatureHeader"`
	WebhookEndpoint    string                  `json:"webhookEndpoint,omitempty"`
	WebhookSecretValue string                  `json:"webhookSecretValue,omitempty"`
	Status             *WebhookConnectorStatus `json:"status,omitempty"`
	Disabled           bool                    `json:"disabled"`
}

type WebhookConnectorStatus struct {
	Ready      bool        `json:"ready"`
	Conditions interface{} `json:"conditions,omitempty"`
}

func (s *Server) handleConnectors(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		s.listConnectors(ctx, w, r)
	case http.MethodPost:
		s.createConnector(ctx, w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleConnector(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/connectors/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "missing connector name")
		return
	}

	if parts := strings.SplitN(path, "/", 2); len(parts) == 2 {
		name, action := parts[0], parts[1]
		if action == "rotate-secret" && r.Method == http.MethodPost {
			if !s.requireWrite(w, r, "connector", name) {
				return
			}
			s.rotateConnectorSecret(ctx, w, name)
			return
		}
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	name := path
	switch r.Method {
	case http.MethodGet:
		if !s.requireRead(w, r, "connector", name) {
			return
		}
		s.getConnector(ctx, w, name)
	case http.MethodPut:
		if !s.requireWrite(w, r, "connector", name) {
			return
		}
		s.updateConnector(ctx, w, r, name)
	case http.MethodDelete:
		if !s.requireWrite(w, r, "connector", name) {
			return
		}
		s.deleteConnector(ctx, w, name)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) listConnectors(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	page, err := ParsePageParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var list connectorv1alpha1.WebhookConnectorList
	if err := s.client.List(ctx, &list, client.InNamespace(s.ns)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Filter to resources the caller can access.
	if s.authzStore != nil && !s.callerIsAdmin(r) {
		names := make([]string, len(list.Items))
		for i, c := range list.Items {
			names[i] = c.Name
		}
		set := toAccessSet(s.filterByAccess(r, "connector", names))
		filtered := list.Items[:0]
		for _, c := range list.Items {
			if set[c.Name] {
				filtered = append(filtered, c)
			}
		}
		list.Items = filtered
	}

	results := make([]WebhookConnectorResponse, 0, len(list.Items))
	for _, c := range list.Items {
		results = append(results, toWebhookConnectorResponse(c))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	total := len(results)
	lo, hi := page.Slice(total)
	items := results[lo:hi]
	if items == nil {
		items = []WebhookConnectorResponse{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":      items,
		"total":      total,
		"page":       page.Page,
		"pageSize":   page.PageSize,
		"totalPages": page.TotalPages(total),
	})
}

func (s *Server) getConnector(ctx context.Context, w http.ResponseWriter, name string) {
	nn := types.NamespacedName{Name: name, Namespace: s.ns}
	var c connectorv1alpha1.WebhookConnector
	if err := s.client.Get(ctx, nn, &c); err != nil {
		writeError(w, http.StatusNotFound, "connector not found")
		return
	}
	writeJSON(w, http.StatusOK, toWebhookConnectorResponse(c))
}

func (s *Server) createConnector(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var req WebhookConnectorCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
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

	webhookHMAC, err := generateWebhookSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate webhook secret: "+err.Error())
		return
	}

	id := generateID("c")
	secretName := webhookSecretName(id)
	if err := s.createSecret(ctx, secretName,
		map[string][]byte{"secret": []byte(webhookHMAC)},
		map[string]string{webhookConnectorLabel: id},
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create webhook secret: "+err.Error())
		return
	}

	imageRepo, imageTag, err := s.connectorCfg.WebhookImage()
	if err != nil {
		_ = s.deleteSecret(ctx, secretName)
		writeError(w, http.StatusInternalServerError, "failed to resolve image: "+err.Error())
		return
	}

	webhookEndpoint := s.connectorCfg.WebhookEndpoint(id)
	signatureHeader := req.SignatureHeader
	if signatureHeader == "" {
		signatureHeader = "X-Hub-Signature-256"
	}

	connector := &connectorv1alpha1.WebhookConnector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: s.ns,
		},
		Spec: connectorv1alpha1.WebhookConnectorSpec{
			DisplayName:     req.Name,
			WebhookEndpoint: webhookEndpoint,
			SignatureHeader: signatureHeader,
			WebhookSecret: connectorv1alpha1.SecretKeyRef{
				SecretRef: connectorv1alpha1.SecretRef{
					Name: secretName,
					Key:  "secret",
				},
			},
			Image: connectorv1alpha1.ConnectorImage{
				Repository: imageRepo,
				Tag:        imageTag,
			},
		},
	}
	connector.APIVersion = "ainsel.dev/v1alpha1"
	connector.Kind = "WebhookConnector"

	if err := s.client.Create(ctx, connector); err != nil {
		_ = s.deleteSecret(ctx, secretName)
		writeError(w, http.StatusInternalServerError, "failed to create connector: "+err.Error())
		return
	}

	if s.authzStore != nil {
		groupID := req.GroupID
		if groupID == "" {
			groupID = "legacy"
		}
		if err := s.authzStore.SetResourceGroup(ctx, "connector", id, groupID, false); err != nil {
			slog.Error("set resource group on create", "error", err, "resource", id)
		}
	}

	resp := toWebhookConnectorResponse(*connector)
	resp.WebhookSecretValue = webhookHMAC
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) updateConnector(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	var req WebhookConnectorUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	nn := types.NamespacedName{Name: name, Namespace: s.ns}
	var c connectorv1alpha1.WebhookConnector
	if err := s.client.Get(ctx, nn, &c); err != nil {
		writeError(w, http.StatusNotFound, "connector not found")
		return
	}

	if req.Name != "" {
		c.Spec.DisplayName = req.Name
	}
	if req.Disabled != nil {
		c.Spec.Disabled = *req.Disabled
	}

	if err := s.client.Update(ctx, &c); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toWebhookConnectorResponse(c))
}

func (s *Server) deleteConnector(ctx context.Context, w http.ResponseWriter, name string) {
	nn := types.NamespacedName{Name: name, Namespace: s.ns}
	var c connectorv1alpha1.WebhookConnector
	if err := s.client.Get(ctx, nn, &c); err != nil {
		writeError(w, http.StatusNotFound, "connector not found")
		return
	}

	if err := s.client.Delete(ctx, &c); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.deleteSecret(ctx, webhookSecretName(name))

	if s.authzStore != nil {
		if err := s.authzStore.DeleteResourceGroup(ctx, "connector", name); err != nil {
			slog.Error("delete ownership", "error", err, "resource", name)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func toWebhookConnectorResponse(c connectorv1alpha1.WebhookConnector) WebhookConnectorResponse {
	resp := WebhookConnectorResponse{
		ID:              c.Name,
		Name:            c.Spec.DisplayName,
		SignatureHeader: c.Spec.SignatureHeader,
		WebhookEndpoint: c.Spec.WebhookEndpoint,
		Disabled:        c.Spec.Disabled,
	}
	if len(c.Status.Conditions) > 0 {
		s := &WebhookConnectorStatus{Conditions: c.Status.Conditions}
		for _, cond := range c.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
				s.Ready = true
			}
		}
		resp.Status = s
	}
	return resp
}

func (s *Server) rotateConnectorSecret(ctx context.Context, w http.ResponseWriter, name string) {
	nn := types.NamespacedName{Name: name, Namespace: s.ns}
	var c connectorv1alpha1.WebhookConnector
	if err := s.client.Get(ctx, nn, &c); err != nil {
		writeError(w, http.StatusNotFound, "connector not found")
		return
	}

	newHMAC, err := generateWebhookSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate webhook secret: "+err.Error())
		return
	}

	var sec corev1.Secret
	snn := types.NamespacedName{Name: webhookSecretName(name), Namespace: s.ns}
	if err := s.client.Get(ctx, snn, &sec); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch webhook secret: "+err.Error())
		return
	}
	sec.Data = map[string][]byte{"secret": []byte(newHMAC)}
	if err := s.client.Update(ctx, &sec); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update webhook secret: "+err.Error())
		return
	}

	resp := toWebhookConnectorResponse(c)
	resp.WebhookSecretValue = newHMAC
	writeJSON(w, http.StatusOK, resp)
}

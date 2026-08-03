package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	connectorv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// testServer builds a Server wired with a fake client for use in tests.
// It is defined here because it is the central shared helper for all api
// package tests. Optional seed objects are pre-loaded into the fake client.
func testServer(t *testing.T, objects ...runtime.Object) *Server {
	t.Helper()
	fc := newFakeClient(objects...)
	return &Server{
		client:             fc,
		mux:                http.NewServeMux(),
		ns:                 "test-ns",
		observabilityCache: newPromCache(observabilityCacheTTL),
		invocations:        invocations.NewStore(100),
	}
}

// connectorTestServer builds a Server wired with a fake client and the connector
// config needed to create WebhookConnectors in tests.
func connectorTestServer(t *testing.T) *Server {
	t.Helper()
	s := testServer(t)
	s.connectorCfg = ConnectorConfig{
		PlatformExternalURL: "https://hub.test",
		ImageWebhook:        "registry.test/webhook-connector:v1.0.0",
	}
	s.mux.HandleFunc("/api/v1/connectors", s.handleConnectors)
	s.mux.HandleFunc("/api/v1/connectors/", s.handleConnector)
	return s
}

// createWebhookConnectorForTest POSTs a WebhookConnectorCreateRequest and
// decodes the response, asserting the status code.
func createWebhookConnectorForTest(t *testing.T, s *Server, body WebhookConnectorCreateRequest) (int, WebhookConnectorResponse) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	var resp WebhookConnectorResponse
	if rec.Body.Len() > 0 && rec.Code/100 == 2 {
		_ = json.NewDecoder(rec.Body).Decode(&resp)
	}
	return rec.Code, resp
}

func TestCreateWebhookConnector_ReturnsWebhookFields(t *testing.T) {
	s := connectorTestServer(t)

	code, created := createWebhookConnectorForTest(t, s, WebhookConnectorCreateRequest{
		Name: "My Webhook",
	})
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}

	// ID should match the c-XXXXXXXX pattern
	idPattern := regexp.MustCompile(`^c-[0-9a-f]{8}$`)
	if !idPattern.MatchString(created.ID) {
		t.Errorf("expected id to match c-XXXXXXXX pattern, got %s", created.ID)
	}

	if created.Name != "My Webhook" {
		t.Errorf("expected name 'My Webhook', got %q", created.Name)
	}

	// Default signature header
	if created.SignatureHeader != "X-Hub-Signature-256" {
		t.Errorf("expected default signatureHeader 'X-Hub-Signature-256', got %q", created.SignatureHeader)
	}

	// Webhook endpoint and secret value must be present on create
	wantEndpoint := s.connectorCfg.WebhookEndpoint(created.ID)
	if created.WebhookEndpoint != wantEndpoint {
		t.Errorf("webhookEndpoint = %q, want %q", created.WebhookEndpoint, wantEndpoint)
	}
	if created.WebhookSecretValue == "" {
		t.Errorf("expected webhookSecretValue in create response, got empty")
	}

	// Subsequent GET must expose webhookEndpoint but NOT the HMAC secret value
	req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/"+created.ID, nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET, got %d", rec.Code)
	}
	var got WebhookConnectorResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if got.WebhookEndpoint != wantEndpoint {
		t.Errorf("GET webhookEndpoint = %q, want %q", got.WebhookEndpoint, wantEndpoint)
	}
	if got.WebhookSecretValue != "" {
		t.Errorf("GET should not echo webhookSecretValue, got %q", got.WebhookSecretValue)
	}
}

func TestCreateWebhookConnector_CustomSignatureHeader(t *testing.T) {
	s := connectorTestServer(t)

	code, created := createWebhookConnectorForTest(t, s, WebhookConnectorCreateRequest{
		Name:            "Forgejo Webhook",
		SignatureHeader: "X-Forgejo-Signature",
	})
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}

	if created.SignatureHeader != "X-Forgejo-Signature" {
		t.Errorf("expected signatureHeader 'X-Forgejo-Signature', got %q", created.SignatureHeader)
	}

	// CR should carry the custom header
	var cr connectorv1alpha1.WebhookConnector
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: created.ID, Namespace: s.ns}, &cr); err != nil {
		t.Fatalf("failed to fetch created connector CR: %v", err)
	}
	if cr.Spec.SignatureHeader != "X-Forgejo-Signature" {
		t.Errorf("expected CR signatureHeader 'X-Forgejo-Signature', got %q", cr.Spec.SignatureHeader)
	}
}

func TestCreateWebhookConnector_RequiresName(t *testing.T) {
	s := connectorTestServer(t)

	body, _ := json.Marshal(WebhookConnectorCreateRequest{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when name is empty, got %d", rec.Code)
	}

	// No connectors should have been created
	var list connectorv1alpha1.WebhookConnectorList
	_ = s.client.List(context.Background(), &list)
	if len(list.Items) != 0 {
		t.Errorf("expected no connectors created on validation failure, got %d", len(list.Items))
	}
}

func TestCreateWebhookConnector_CreatesWebhookSecret(t *testing.T) {
	s := connectorTestServer(t)

	code, created := createWebhookConnectorForTest(t, s, WebhookConnectorCreateRequest{Name: "Test"})
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}

	// The webhook HMAC secret must be stored in a K8s Secret
	secretName := webhookSecretName(created.ID)
	var sec corev1.Secret
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: s.ns}, &sec); err != nil {
		t.Fatalf("expected webhook secret %q to exist: %v", secretName, err)
	}
	if len(sec.Data["secret"]) == 0 {
		t.Errorf("expected secret data 'secret' to be set")
	}
	// The echoed value should match what's in the Secret
	if string(sec.Data["secret"]) != created.WebhookSecretValue {
		t.Errorf("secret in K8s (%q) does not match webhookSecretValue in response (%q)",
			string(sec.Data["secret"]), created.WebhookSecretValue)
	}
}

func TestUpdateWebhookConnector_DisableAndRename(t *testing.T) {
	s := connectorTestServer(t)

	code, created := createWebhookConnectorForTest(t, s, WebhookConnectorCreateRequest{Name: "Original"})
	if code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", code)
	}

	// Disable and rename
	disabled := true
	updateReq := WebhookConnectorUpdateRequest{
		Name:     "Renamed",
		Disabled: &disabled,
	}
	ub, _ := json.Marshal(updateReq)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/connectors/"+created.ID, bytes.NewReader(ub))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	s.mux.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	var updated WebhookConnectorResponse
	if err := json.NewDecoder(putRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.Name != "Renamed" {
		t.Errorf("expected name 'Renamed', got %q", updated.Name)
	}
	if !updated.Disabled {
		t.Errorf("expected disabled=true, got false")
	}

	// CR must reflect the changes
	var cr connectorv1alpha1.WebhookConnector
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: created.ID, Namespace: s.ns}, &cr); err != nil {
		t.Fatalf("failed to fetch CR: %v", err)
	}
	if cr.Spec.DisplayName != "Renamed" {
		t.Errorf("expected CR displayName 'Renamed', got %q", cr.Spec.DisplayName)
	}
	if !cr.Spec.Disabled {
		t.Errorf("expected CR disabled=true, got false")
	}

	// Re-enable
	enabled := false
	updateReq2 := WebhookConnectorUpdateRequest{Disabled: &enabled}
	ub2, _ := json.Marshal(updateReq2)
	putReq2 := httptest.NewRequest(http.MethodPut, "/api/v1/connectors/"+created.ID, bytes.NewReader(ub2))
	putReq2.Header.Set("Content-Type", "application/json")
	putRec2 := httptest.NewRecorder()
	s.mux.ServeHTTP(putRec2, putReq2)
	if putRec2.Code != http.StatusOK {
		t.Fatalf("re-enable: expected 200, got %d: %s", putRec2.Code, putRec2.Body.String())
	}

	var reenabled WebhookConnectorResponse
	if err := json.NewDecoder(putRec2.Body).Decode(&reenabled); err != nil {
		t.Fatalf("decode re-enable response: %v", err)
	}
	if reenabled.Disabled {
		t.Errorf("expected disabled=false after re-enabling, got true")
	}
}

func TestDeleteWebhookConnector_RemovesSecretAndCR(t *testing.T) {
	s := connectorTestServer(t)

	code, created := createWebhookConnectorForTest(t, s, WebhookConnectorCreateRequest{Name: "To Delete"})
	if code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", code)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/connectors/"+created.ID, nil)
	delRec := httptest.NewRecorder()
	s.mux.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", delRec.Code, delRec.Body.String())
	}

	// CR must be gone
	var cr connectorv1alpha1.WebhookConnector
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: created.ID, Namespace: s.ns}, &cr); err == nil {
		t.Errorf("expected connector CR to be deleted, but it still exists")
	}

	// Webhook secret must also be gone
	var sec corev1.Secret
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: webhookSecretName(created.ID), Namespace: s.ns}, &sec); err == nil {
		t.Errorf("expected webhook secret to be deleted, but it still exists")
	}
}

func TestListWebhookConnectors_Pagination(t *testing.T) {
	s := connectorTestServer(t)

	// Create 3 connectors
	for _, name := range []string{"A", "B", "C"} {
		code, _ := createWebhookConnectorForTest(t, s, WebhookConnectorCreateRequest{Name: name})
		if code != http.StatusCreated {
			t.Fatalf("create %q: expected 201, got %d", name, code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors?pageSize=2&page=1", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var page struct {
		Items      []WebhookConnectorResponse `json:"items"`
		Total      int                        `json:"total"`
		Page       int                        `json:"page"`
		PageSize   int                        `json:"pageSize"`
		TotalPages int                        `json:"totalPages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if page.Total != 3 {
		t.Errorf("expected total=3, got %d", page.Total)
	}
	if len(page.Items) != 2 {
		t.Errorf("expected 2 items on page 1 with pageSize=2, got %d", len(page.Items))
	}
	if page.TotalPages != 2 {
		t.Errorf("expected totalPages=2, got %d", page.TotalPages)
	}
}

func TestGetWebhookConnector_NotFound(t *testing.T) {
	s := connectorTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/does-not-exist", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestCreateWebhookConnector_ImageNotConfigured(t *testing.T) {
	s := testServer(t)
	// ConnectorConfig with no ImageWebhook set
	s.connectorCfg = ConnectorConfig{
		PlatformExternalURL: "https://hub.test",
		ImageWebhook:        "",
	}
	s.mux.HandleFunc("/api/v1/connectors", s.handleConnectors)
	s.mux.HandleFunc("/api/v1/connectors/", s.handleConnector)

	body, _ := json.Marshal(WebhookConnectorCreateRequest{Name: "Test"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when image not configured, got %d", rec.Code)
	}

	// No CRs and no Secrets should have been created (secret rolled back on image error)
	var list connectorv1alpha1.WebhookConnectorList
	_ = s.client.List(context.Background(), &list)
	if len(list.Items) != 0 {
		t.Errorf("expected no connectors after image error, got %d", len(list.Items))
	}
	var secs corev1.SecretList
	_ = s.client.List(context.Background(), &secs)
	if len(secs.Items) != 0 {
		t.Errorf("expected no secrets after image error, got %d (%v)", len(secs.Items), secretNames(secs.Items))
	}
}

func secretNames(items []corev1.Secret) []string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		out = append(out, s.Name)
	}
	return out
}

// TestConnector_PreExistingCR verifies that a WebhookConnector created
// directly as a K8s resource (e.g. by Helm) can be retrieved via the API.
func TestConnector_PreExistingCR(t *testing.T) {
	existing := &connectorv1alpha1.WebhookConnector{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "helm-connector",
			Namespace: "test-ns",
		},
		Spec: connectorv1alpha1.WebhookConnectorSpec{
			DisplayName:     "Helm Connector",
			WebhookEndpoint: "https://hub.example.com/webhooks/helm-connector",
			SignatureHeader:  "X-Hub-Signature-256",
		},
	}

	s := testServer(t, existing)
	s.connectorCfg = ConnectorConfig{PlatformExternalURL: "https://hub.example.com"}
	s.mux.HandleFunc("/api/v1/connectors", s.handleConnectors)
	s.mux.HandleFunc("/api/v1/connectors/", s.handleConnector)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/helm-connector", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp WebhookConnectorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != "helm-connector" {
		t.Errorf("expected id 'helm-connector', got %q", resp.ID)
	}
	if resp.Name != "Helm Connector" {
		t.Errorf("expected name 'Helm Connector', got %q", resp.Name)
	}
}

func TestRotateConnectorSecret_ReturnsNewSecretValue(t *testing.T) {
	s := connectorTestServer(t)

	code, created := createWebhookConnectorForTest(t, s, WebhookConnectorCreateRequest{
		Name: "Rotate Test",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", code)
	}
	originalSecret := created.WebhookSecretValue

	req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/"+created.ID+"/rotate-secret", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp WebhookConnectorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode rotate response: %v", err)
	}
	if resp.WebhookSecretValue == "" {
		t.Error("expected webhookSecretValue in rotate response, got empty")
	}
	if resp.WebhookSecretValue == originalSecret {
		t.Errorf("expected new secret to differ from original %q", originalSecret)
	}

	// K8s Secret must hold the new value.
	var sec corev1.Secret
	if err := s.client.Get(context.Background(), types.NamespacedName{Name: webhookSecretName(created.ID), Namespace: s.ns}, &sec); err != nil {
		t.Fatalf("expected webhook secret to exist: %v", err)
	}
	if got := string(sec.Data["secret"]); got != resp.WebhookSecretValue {
		t.Errorf("K8s secret = %q, want %q", got, resp.WebhookSecretValue)
	}
}

func TestRotateConnectorSecret_UnknownConnector(t *testing.T) {
	s := connectorTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/nonexistent/rotate-secret", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown connector, got %d", rec.Code)
	}
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	connectorv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// seedAgents builds N Agent objects with names a-001..a-NNN so list responses
// have a predictable order.
func seedAgents(n int) []runtime.Object {
	out := make([]runtime.Object, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, &connectorv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("a-%03d", i),
				Namespace: "test-ns",
			},
			Spec: connectorv1alpha1.AgentSpec{
				DisplayName: fmt.Sprintf("Agent %03d", i),
			},
		})
	}
	return out
}

func TestAgents_Pagination(t *testing.T) {
	srv := testServer(t, seedAgents(25)...)
	srv.mux.HandleFunc("/api/v1/agents", srv.handleAgents)

	type pageResp struct {
		Items      []SimpleAgentResponse `json:"items"`
		Total      int                   `json:"total"`
		Page       int                   `json:"page"`
		PageSize   int                   `json:"pageSize"`
		TotalPages int                   `json:"totalPages"`
	}

	get := func(t *testing.T, qs string) pageResp {
		t.Helper()
		url := "/api/v1/agents"
		if qs != "" {
			url += "?" + qs
		}
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d: %s", url, rec.Code, rec.Body.String())
		}
		var p pageResp
		if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
			t.Fatalf("GET %s: decode: %v", url, err)
		}
		return p
	}

	t.Run("defaults return first page of 50", func(t *testing.T) {
		p := get(t, "")
		if p.Total != 25 {
			t.Errorf("expected total=25, got %d", p.Total)
		}
		if p.Page != 1 {
			t.Errorf("expected page=1, got %d", p.Page)
		}
		if p.PageSize != 50 {
			t.Errorf("expected pageSize=50, got %d", p.PageSize)
		}
		if p.TotalPages != 1 {
			t.Errorf("expected totalPages=1, got %d", p.TotalPages)
		}
		if len(p.Items) != 25 {
			t.Errorf("expected 25 items returned, got %d", len(p.Items))
		}
	})

	t.Run("small pageSize splits results", func(t *testing.T) {
		p1 := get(t, "pageSize=10&page=1")
		if len(p1.Items) != 10 || p1.Total != 25 || p1.TotalPages != 3 {
			t.Fatalf("page1 unexpected: items=%d total=%d totalPages=%d", len(p1.Items), p1.Total, p1.TotalPages)
		}
		if p1.Items[0].ID != "a-000" || p1.Items[9].ID != "a-009" {
			t.Errorf("page1 first/last ID unexpected: %s..%s", p1.Items[0].ID, p1.Items[9].ID)
		}

		p2 := get(t, "pageSize=10&page=2")
		if len(p2.Items) != 10 {
			t.Fatalf("page2 expected 10 items, got %d", len(p2.Items))
		}
		if p2.Items[0].ID != "a-010" || p2.Items[9].ID != "a-019" {
			t.Errorf("page2 first/last ID unexpected: %s..%s", p2.Items[0].ID, p2.Items[9].ID)
		}

		p3 := get(t, "pageSize=10&page=3")
		if len(p3.Items) != 5 {
			t.Fatalf("page3 expected 5 items, got %d", len(p3.Items))
		}
		if p3.Items[0].ID != "a-020" || p3.Items[4].ID != "a-024" {
			t.Errorf("page3 first/last ID unexpected: %s..%s", p3.Items[0].ID, p3.Items[4].ID)
		}
	})

	t.Run("page past end returns empty items", func(t *testing.T) {
		p := get(t, "pageSize=10&page=99")
		if p.Total != 25 {
			t.Errorf("expected total=25, got %d", p.Total)
		}
		if len(p.Items) != 0 {
			t.Errorf("expected 0 items on out-of-range page, got %d", len(p.Items))
		}
	})

	t.Run("invalid page returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents?page=abc", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid page, got %d", rec.Code)
		}
	})

	t.Run("pageSize clamped at maximum", func(t *testing.T) {
		p := get(t, "pageSize=999999")
		if p.PageSize != maxPageSize {
			t.Errorf("expected pageSize clamped to %d, got %d", maxPageSize, p.PageSize)
		}
	})
}

func TestTriggers_Pagination(t *testing.T) {
	srv := triggerTestServer(t)

	for i := 0; i < 12; i++ {
		seedTrigger(t, srv, fmt.Sprintf("t-%02d", i), fmt.Sprintf("Trigger %02d", i), "dev-agent", "forgejo-dev", nil)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/triggers?page=2&pageSize=5", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var p struct {
		Items      []SimpleTriggerResponse `json:"items"`
		Total      int                     `json:"total"`
		Page       int                     `json:"page"`
		PageSize   int                     `json:"pageSize"`
		TotalPages int                     `json:"totalPages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Total != 12 || p.TotalPages != 3 || p.Page != 2 || p.PageSize != 5 {
		t.Errorf("unexpected meta: %+v", p)
	}
	if len(p.Items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(p.Items))
	}
	if p.Items[0].ID != "t-05" || p.Items[4].ID != "t-09" {
		t.Errorf("unexpected page contents: %s..%s", p.Items[0].ID, p.Items[4].ID)
	}
}

func TestTriggers_PaginationCombinedWithFilters(t *testing.T) {
	// Seed 6 triggers: 4 for dev-agent, 2 for reviewer. Pagination over the
	// filtered set should give 2 pages of 2 for dev-agent.
	srv := triggerTestServer(t)

	seedTrigger(t, srv, "t-1", "T1", "dev-agent", "forgejo-dev", nil)
	seedTrigger(t, srv, "t-2", "T2", "dev-agent", "forgejo-dev", nil)
	seedTrigger(t, srv, "t-3", "T3", "dev-agent", "forgejo-dev", nil)
	seedTrigger(t, srv, "t-4", "T4", "dev-agent", "forgejo-dev", nil)
	seedTrigger(t, srv, "t-5", "T5", "reviewer", "forgejo-dev", nil)
	seedTrigger(t, srv, "t-6", "T6", "reviewer", "forgejo-dev", nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/triggers?agent=dev-agent&pageSize=2&page=2", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var p struct {
		Items      []SimpleTriggerResponse `json:"items"`
		Total      int                     `json:"total"`
		TotalPages int                     `json:"totalPages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Total should reflect filtered count (4), not all 6.
	if p.Total != 4 {
		t.Errorf("expected filtered total=4, got %d", p.Total)
	}
	if p.TotalPages != 2 {
		t.Errorf("expected totalPages=2, got %d", p.TotalPages)
	}
	if len(p.Items) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(p.Items))
	}
	if p.Items[0].ID != "t-3" || p.Items[1].ID != "t-4" {
		t.Errorf("unexpected items on page 2: %s, %s", p.Items[0].ID, p.Items[1].ID)
	}
}

func TestConnectors_Pagination(t *testing.T) {
	// Seed WebhookConnectors; the list should paginate and sort by ID.
	seed := []runtime.Object{
		&connectorv1alpha1.WebhookConnector{ObjectMeta: metav1.ObjectMeta{Name: "c-01", Namespace: "test-ns"}, Spec: connectorv1alpha1.WebhookConnectorSpec{DisplayName: "Webhook 1"}},
		&connectorv1alpha1.WebhookConnector{ObjectMeta: metav1.ObjectMeta{Name: "c-02", Namespace: "test-ns"}, Spec: connectorv1alpha1.WebhookConnectorSpec{DisplayName: "Webhook 2"}},
		&connectorv1alpha1.WebhookConnector{ObjectMeta: metav1.ObjectMeta{Name: "c-03", Namespace: "test-ns"}, Spec: connectorv1alpha1.WebhookConnectorSpec{DisplayName: "Webhook 3"}},
		&connectorv1alpha1.WebhookConnector{ObjectMeta: metav1.ObjectMeta{Name: "c-04", Namespace: "test-ns"}, Spec: connectorv1alpha1.WebhookConnectorSpec{DisplayName: "Webhook 4"}},
	}
	srv := testServer(t, seed...)
	srv.mux.HandleFunc("/api/v1/connectors", srv.handleConnectors)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors?pageSize=2&page=1", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var p struct {
		Items      []WebhookConnectorResponse `json:"items"`
		Total      int                        `json:"total"`
		Page       int                        `json:"page"`
		PageSize   int                        `json:"pageSize"`
		TotalPages int                        `json:"totalPages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Total != 4 || p.TotalPages != 2 || len(p.Items) != 2 {
		t.Errorf("unexpected page1: %+v", p)
	}
	if p.Items[0].ID != "c-01" || p.Items[1].ID != "c-02" {
		t.Errorf("unexpected page1 contents: %s, %s", p.Items[0].ID, p.Items[1].ID)
	}
}

func TestInvocations_Pagination(t *testing.T) {
	srv := testServer(t)
	srv.mux.HandleFunc("/api/v1/invocations", srv.handleInvocations)

	now := time.Now().UTC()
	// Insert 7 invocations with different start times so the newest-first
	// order is deterministic.
	for i := 0; i < 7; i++ {
		srv.invocations.Record(invocations.Invocation{
			AgentName: "agent",
			StartTime: now.Add(time.Duration(i) * time.Second),
		})
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invocations?pageSize=3&page=2", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Invocations []invocations.Invocation `json:"invocations"`
		Total       int                      `json:"total"`
		Page        int                      `json:"page"`
		PageSize    int                      `json:"pageSize"`
		TotalPages  int                      `json:"totalPages"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 7 || resp.TotalPages != 3 || resp.Page != 2 || resp.PageSize != 3 {
		t.Errorf("unexpected meta: %+v", resp)
	}
	if len(resp.Invocations) != 3 {
		t.Fatalf("expected 3 items on page 2, got %d", len(resp.Invocations))
	}
}

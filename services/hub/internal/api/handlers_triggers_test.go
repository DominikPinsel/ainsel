package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

func triggerTestServer(t *testing.T) *Server {
	t.Helper()
	ctx := t.Context()
	c, err := pgcontainer.Run(ctx, "postgres:17-alpine",
		pgcontainer.WithDatabase("test"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		pgcontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	dsn, _ := c.ConnectionString(ctx, "sslmode=disable")
	if err := db.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	s := testServer(t)
	s.triggerStore = triggers.NewStore(pool)
	s.mux.HandleFunc("/api/v1/triggers", s.handleTriggers)
	s.mux.HandleFunc("/api/v1/triggers/", s.handleTrigger)
	return s
}

func seedTrigger(t *testing.T, s *Server, id, name, agentRef, connectorRef string, filters []ainselapishared.Filter) {
	t.Helper()
	tr := &triggers.Trigger{
		ID:           id,
		DisplayName:  name,
		AgentRef:     agentRef,
		ConnectorRef: connectorRef,
		Filters:      filters,
	}
	if err := s.triggerStore.CreateTrigger(context.Background(), tr); err != nil {
		t.Fatalf("seed trigger %s: %v", id, err)
	}
}

func TestTriggers_CreateAndList(t *testing.T) {
	srv := triggerTestServer(t)

	// Create a trigger
	createReq := SimpleTriggerCreateRequest{
		Name:         "my-trigger",
		AgentRef:     "agent-abc",
		ConnectorRef: "connector-xyz",
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/triggers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created SimpleTriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify ID format: t-XXXXXXXX (10 chars total)
	idPattern := regexp.MustCompile(`^t-[0-9a-f]{8}$`)
	if !idPattern.MatchString(created.ID) {
		t.Errorf("expected id to match t-XXXXXXXX pattern, got %s", created.ID)
	}

	// Verify Name matches request name
	if created.Name != "my-trigger" {
		t.Errorf("expected name my-trigger, got %s", created.Name)
	}
	if created.AgentRef != "agent-abc" {
		t.Errorf("expected agentRef agent-abc, got %s", created.AgentRef)
	}
	if created.ConnectorRef != "connector-xyz" {
		t.Errorf("expected connectorRef connector-xyz, got %s", created.ConnectorRef)
	}

	// List and verify count
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/triggers", nil)
	listRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	var page struct {
		Items      []SimpleTriggerResponse `json:"items"`
		Total      int                     `json:"total"`
		Page       int                     `json:"page"`
		PageSize   int                     `json:"pageSize"`
		TotalPages int                     `json:"totalPages"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&page); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected 1 trigger, got total=%d items=%d", page.Total, len(page.Items))
	}
	if page.Page != 1 || page.PageSize != 50 || page.TotalPages != 1 {
		t.Errorf("unexpected pagination fields: page=%d pageSize=%d totalPages=%d", page.Page, page.PageSize, page.TotalPages)
	}
	if page.Items[0].Name != "my-trigger" {
		t.Errorf("expected name my-trigger, got %s", page.Items[0].Name)
	}
}

func TestTriggers_UpdateRename(t *testing.T) {
	srv := triggerTestServer(t)

	// Create a trigger
	createReq := SimpleTriggerCreateRequest{
		Name:         "original-name",
		AgentRef:     "agent-abc",
		ConnectorRef: "connector-xyz",
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/triggers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created SimpleTriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	originalID := created.ID

	// Update with a new name
	updateReq := SimpleTriggerUpdateRequest{
		Name: "renamed-trigger",
	}
	updateBody, _ := json.Marshal(updateReq)
	updateHTTPReq := httptest.NewRequest(http.MethodPut, "/api/v1/triggers/"+originalID, bytes.NewReader(updateBody))
	updateHTTPReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(updateRec, updateHTTPReq)

	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	var updated SimpleTriggerResponse
	if err := json.NewDecoder(updateRec.Body).Decode(&updated); err != nil {
		t.Fatalf("failed to decode update response: %v", err)
	}

	// Verify name changed
	if updated.Name != "renamed-trigger" {
		t.Errorf("expected name renamed-trigger, got %s", updated.Name)
	}

	// Verify ID unchanged
	if updated.ID != originalID {
		t.Errorf("expected ID %s unchanged after update, got %s", originalID, updated.ID)
	}

	// Verify other fields preserved
	if updated.AgentRef != "agent-abc" {
		t.Errorf("expected agentRef agent-abc after rename, got %s", updated.AgentRef)
	}

	// GET should also return the new name
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/triggers/"+originalID, nil)
	getRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET after update: expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	var fetched SimpleTriggerResponse
	if err := json.NewDecoder(getRec.Body).Decode(&fetched); err != nil {
		t.Fatalf("failed to decode GET response: %v", err)
	}
	if fetched.Name != "renamed-trigger" {
		t.Errorf("GET after update: expected name renamed-trigger, got %q", fetched.Name)
	}
}

// TestTriggers_UpdateNameOnPreExistingTrigger simulates the real-world scenario:
// a trigger was created directly in the DB (e.g. by a migration script) without a
// DisplayName. The user then sets a name via the API.
func TestTriggers_UpdateNameOnPreExistingTrigger(t *testing.T) {
	srv := triggerTestServer(t)

	// Pre-create a trigger directly in the DB — no DisplayName
	seedTrigger(t, srv, "develop-on-assign", "", "dev-agent", "forgejo-dev", nil)

	// GET should return empty name and id=develop-on-assign
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/triggers/develop-on-assign", nil)
	getRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET: expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	var fetched SimpleTriggerResponse
	if err := json.NewDecoder(getRec.Body).Decode(&fetched); err != nil {
		t.Fatalf("failed to decode GET response: %v", err)
	}
	if fetched.ID != "develop-on-assign" {
		t.Errorf("expected id develop-on-assign, got %s", fetched.ID)
	}
	if fetched.Name != "" {
		t.Errorf("expected empty name for pre-existing trigger, got %q", fetched.Name)
	}

	// UPDATE: set a display name
	updateReq := SimpleTriggerUpdateRequest{
		Name: "Developer Assigned",
	}
	updateBody, _ := json.Marshal(updateReq)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/triggers/develop-on-assign", bytes.NewReader(updateBody))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT: expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	var updated SimpleTriggerResponse
	if err := json.NewDecoder(putRec.Body).Decode(&updated); err != nil {
		t.Fatalf("failed to decode PUT response: %v", err)
	}

	// The response should reflect the new name
	if updated.Name != "Developer Assigned" {
		t.Errorf("PUT response: expected name %q, got %q", "Developer Assigned", updated.Name)
	}
	if updated.ID != "develop-on-assign" {
		t.Errorf("PUT response: expected id unchanged, got %s", updated.ID)
	}
	if updated.AgentRef != "dev-agent" {
		t.Errorf("PUT response: expected agentRef preserved as dev-agent, got %s", updated.AgentRef)
	}

	// GET again — the name should be persisted
	getReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/triggers/develop-on-assign", nil)
	getRec2 := httptest.NewRecorder()
	srv.mux.ServeHTTP(getRec2, getReq2)

	if getRec2.Code != http.StatusOK {
		t.Fatalf("GET after PUT: expected 200, got %d: %s", getRec2.Code, getRec2.Body.String())
	}

	var refetched SimpleTriggerResponse
	if err := json.NewDecoder(getRec2.Body).Decode(&refetched); err != nil {
		t.Fatalf("failed to decode second GET response: %v", err)
	}
	if refetched.Name != "Developer Assigned" {
		t.Errorf("GET after PUT: expected name %q persisted, got %q", "Developer Assigned", refetched.Name)
	}
}

// TestTriggers_ListFilters verifies the query-parameter filtering on
// GET /api/v1/triggers. The endpoint accepts `agent` and `connector` filters
// that match the trigger spec by exact (case-sensitive) string equality.
// Filters compose with AND semantics; an unset filter matches everything.
func TestTriggers_ListFilters(t *testing.T) {
	srv := triggerTestServer(t)

	// Seed three triggers covering distinct agent/connector combinations
	// so each filter axis can be exercised independently.
	seedTrigger(t, srv, "t-1", "alpha", "dev-agent", "forgejo-dev", nil)
	seedTrigger(t, srv, "t-2", "beta", "dev-agent", "github-prod", nil)
	seedTrigger(t, srv, "t-3", "gamma", "reviewer", "forgejo-dev", nil)

	list := func(t *testing.T, qs string) []SimpleTriggerResponse {
		t.Helper()
		url := "/api/v1/triggers"
		if qs != "" {
			url += "?" + qs
		}
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d: %s", url, rec.Code, rec.Body.String())
		}
		var page struct {
			Items []SimpleTriggerResponse `json:"items"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
			t.Fatalf("GET %s: failed to decode response: %v", url, err)
		}
		return page.Items
	}

	idsOf := func(items []SimpleTriggerResponse) map[string]bool {
		out := make(map[string]bool, len(items))
		for _, it := range items {
			out[it.ID] = true
		}
		return out
	}

	t.Run("no filters returns all", func(t *testing.T) {
		got := list(t, "")
		if len(got) != 3 {
			t.Fatalf("expected 3 triggers without filters, got %d", len(got))
		}
	})

	t.Run("filter by agent", func(t *testing.T) {
		got := list(t, "agent=dev-agent")
		if len(got) != 2 {
			t.Fatalf("expected 2 triggers for agent=dev-agent, got %d", len(got))
		}
		ids := idsOf(got)
		if !ids["t-1"] || !ids["t-2"] {
			t.Errorf("expected t-1 and t-2, got %v", ids)
		}
	})

	t.Run("filter by connector", func(t *testing.T) {
		got := list(t, "connector=forgejo-dev")
		if len(got) != 2 {
			t.Fatalf("expected 2 triggers for connector=forgejo-dev, got %d", len(got))
		}
		ids := idsOf(got)
		if !ids["t-1"] || !ids["t-3"] {
			t.Errorf("expected t-1 and t-3, got %v", ids)
		}
	})

	t.Run("filters AND together", func(t *testing.T) {
		got := list(t, "agent=dev-agent&connector=github-prod")
		if len(got) != 1 {
			t.Fatalf("expected 1 trigger matching agent=dev-agent AND connector=github-prod, got %d", len(got))
		}
		if got[0].ID != "t-2" {
			t.Errorf("expected t-2, got %s", got[0].ID)
		}
	})

	t.Run("no match returns empty array", func(t *testing.T) {
		got := list(t, "agent=does-not-exist")
		if len(got) != 0 {
			t.Errorf("expected 0 triggers for non-matching filter, got %d", len(got))
		}
	})

	t.Run("filters are case-sensitive", func(t *testing.T) {
		// AgentRef in seed is exactly "dev-agent" — uppercase should not match.
		got := list(t, "agent=Dev-Agent")
		if len(got) != 0 {
			t.Errorf("expected 0 triggers for case-mismatched agent, got %d", len(got))
		}
	})
}
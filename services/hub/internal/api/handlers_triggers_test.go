package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
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

// wireFilter and wireTrigger mirror the JSON contract of the trigger endpoints
// independently of the server's Go structs, so a field the API layer forgets to
// carry is caught rather than silently compiled away.
type wireFilter struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"`
	Value  string   `json:"value"`
	Values []string `json:"values"`
}

type wireTrigger struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Filters []wireFilter `json:"filters"`
}

func decodeWireTrigger(t *testing.T, body []byte) wireTrigger {
	t.Helper()
	var w wireTrigger
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("failed to decode trigger json: %v\n%s", err, body)
	}
	return w
}

// filterValuesByName indexes a wire filter list by field, keeping the list order
// irrelevant, and records the single-value operators with a nil value list.
func filterValuesByName(filters []wireFilter) map[string][]string {
	out := make(map[string][]string, len(filters))
	for _, f := range filters {
		out[f.Field] = f.Values
	}
	return out
}

// TestTriggers_FilterValuesArePreserved is a regression test for the REST API
// silently dropping the `values` field of `in`/`not-in` filters. A dropped
// `values` leaves the stored filter with an empty list, which never matches —
// the trigger stops firing without any error surfacing anywhere.
//
// The field has to survive POST, GET and LIST on the wire, and must reach the
// database as a filter whose Match behaves per the documented operator semantics.
func TestTriggers_FilterValuesArePreserved(t *testing.T) {
	srv := triggerTestServer(t)
	ctx := context.Background()

	const createBody = `{
		"name": "pr-opened-not-wontfix",
		"agentRef": "dev-agent",
		"connectorRef": "forgejo-dev",
		"filters": [
			{"field": "action", "op": "in", "values": ["opened", "synchronized"]},
			{"field": "title", "op": "not-in", "values": ["wontfix", "needs-discussion"]},
			{"field": "sender.login", "op": "eq", "value": "bot"}
		]
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/triggers", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	created := decodeWireTrigger(t, rec.Body.Bytes())
	if created.ID == "" {
		t.Fatal("create: empty id in response")
	}

	want := map[string][]string{
		"action":       {"opened", "synchronized"},
		"title":        {"wontfix", "needs-discussion"},
		"sender.login": nil,
	}

	// 1. The create response must echo the values back; the console re-reads a
	//    trigger to repopulate the edit form.
	if got := filterValuesByName(created.Filters); !reflect.DeepEqual(want, got) {
		t.Errorf("create response dropped values:\n want %v\n  got %v", want, got)
	}

	// 2. The values must actually reach the store.
	stored, err := srv.triggerStore.GetTrigger(ctx, created.ID)
	if err != nil {
		t.Fatalf("get from store: %v", err)
	}
	if len(stored.Filters) != 3 {
		t.Fatalf("expected 3 stored filters, got %d", len(stored.Filters))
	}
	for _, f := range stored.Filters {
		switch f.Field {
		case "action":
			if !reflect.DeepEqual(f.Values, []string{"opened", "synchronized"}) {
				t.Errorf("stored filter %q: expected values [opened synchronized], got %v", f.Field, f.Values)
			}
		case "title":
			if !reflect.DeepEqual(f.Values, []string{"wontfix", "needs-discussion"}) {
				t.Errorf("stored filter %q: expected values [wontfix needs-discussion], got %v", f.Field, f.Values)
			}
		}
	}

	// 3. The stored filters must match the way the operator table documents it —
	//    this is what breaks when values are dropped.
	payload := map[string]any{"action": "opened", "title": "add login", "sender": map[string]any{"login": "bot"}}
	for i := range stored.Filters {
		if !stored.Filters[i].Match(payload) {
			t.Errorf("stored filter %+v does not match %+v — values were lost", stored.Filters[i], payload)
		}
	}
	nonMatch := map[string]any{"action": "closed", "title": "add login", "sender": map[string]any{"login": "bot"}}
	for i := range stored.Filters {
		if stored.Filters[i].Field == "action" && stored.Filters[i].Match(nonMatch) {
			t.Errorf("in-filter matched action=closed; operator is not enforcing values")
		}
	}

	// 4. GET and LIST must expose the values too.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/triggers/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	if got := filterValuesByName(decodeWireTrigger(t, getRec.Body.Bytes()).Filters); !reflect.DeepEqual(want, got) {
		t.Errorf("GET dropped values:\n want %v\n  got %v", want, got)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/triggers", nil)
	listRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(listRec, listReq)
	var page struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &page); err != nil {
		t.Fatalf("list: failed to decode: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("list: expected 1 item, got %d", len(page.Items))
	}
	if got := filterValuesByName(decodeWireTrigger(t, page.Items[0]).Filters); !reflect.DeepEqual(want, got) {
		t.Errorf("LIST dropped values:\n want %v\n  got %v", want, got)
	}

	// 5. PUT must replace the values, not discard them.
	const updateBody = `{"filters": [{"field": "action", "op": "in", "values": ["reopened"]}]}`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/triggers/"+created.ID, strings.NewReader(updateBody))
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}
	updated := decodeWireTrigger(t, putRec.Body.Bytes())
	if got := filterValuesByName(updated.Filters); !reflect.DeepEqual(got["action"], []string{"reopened"}) {
		t.Errorf("update response dropped values: got %v", got["action"])
	}
	restored, err := srv.triggerStore.GetTrigger(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if len(restored.Filters) != 1 || !reflect.DeepEqual(restored.Filters[0].Values, []string{"reopened"}) {
		t.Errorf("stored filters after update: expected one in-filter with [reopened], got %+v", restored.Filters)
	}
	if !restored.Filters[0].Match(map[string]any{"action": "reopened"}) {
		t.Error("updated in-filter does not match action=reopened")
	}
	if restored.Filters[0].Match(map[string]any{"action": "opened"}) {
		t.Error("updated in-filter still matches the replaced value action=opened")
	}
}

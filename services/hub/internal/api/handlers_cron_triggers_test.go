package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
)

func cronTriggerTestServer(t *testing.T) *Server {
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
	s.mux.HandleFunc("/api/v1/cron-triggers", s.handleCronTriggers)
	s.mux.HandleFunc("/api/v1/cron-triggers/", s.handleCronTrigger)
	return s
}

func TestCronTriggers_CreateAndList(t *testing.T) {
	srv := cronTriggerTestServer(t)

	createReq := CronTriggerCreateRequest{
		Name:     "daily-standup",
		AgentRef: "standup-bot",
		Schedule: "0 9 * * 1-5",
		Prompt:   "Summarize open PRs and stale issues.",
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cron-triggers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created CronTriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !regexp.MustCompile(`^c-[0-9a-f]{8}$`).MatchString(created.ID) {
		t.Errorf("expected id c-XXXXXXXX, got %s", created.ID)
	}
	if created.Name != "daily-standup" {
		t.Errorf("name = %q, want daily-standup", created.Name)
	}
	if created.AgentRef != "standup-bot" {
		t.Errorf("agentRef = %q, want standup-bot", created.AgentRef)
	}
	if created.Schedule != "0 9 * * 1-5" {
		t.Errorf("schedule = %q, want 0 9 * * 1-5", created.Schedule)
	}
	if created.Prompt != "Summarize open PRs and stale issues." {
		t.Errorf("prompt mismatch: %q", created.Prompt)
	}
	if !created.Enabled {
		t.Errorf("enabled should default to true")
	}

	// List and verify.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/cron-triggers", nil)
	listRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var page struct {
		Items []CronTriggerResponse `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&page); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got total=%d items=%d", page.Total, len(page.Items))
	}
	if page.Items[0].AgentRef != "standup-bot" {
		t.Errorf("list item agentRef = %q", page.Items[0].AgentRef)
	}
}

func TestCronTriggers_CreateRejectsMissingFields(t *testing.T) {
	srv := cronTriggerTestServer(t)

	// Missing schedule and prompt.
	body, _ := json.Marshal(CronTriggerCreateRequest{Name: "x", AgentRef: "a"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cron-triggers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing fields, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCronTriggers_GetUpdateDelete(t *testing.T) {
	srv := cronTriggerTestServer(t)

	// Create.
	body, _ := json.Marshal(CronTriggerCreateRequest{
		Name: "nightly", AgentRef: "bot", Schedule: "0 0 * * *", Prompt: "p",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cron-triggers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var created CronTriggerResponse
	_ = json.NewDecoder(rec.Body).Decode(&created)

	// Get.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/cron-triggers/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	// Update prompt + disable.
	upd, _ := json.Marshal(CronTriggerUpdateRequest{Prompt: "new prompt", Enabled: ptr(false)})
	updReq := httptest.NewRequest(http.MethodPut, "/api/v1/cron-triggers/"+created.ID, bytes.NewReader(upd))
	updReq.Header.Set("Content-Type", "application/json")
	updRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(updRec, updReq)
	if updRec.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d: %s", updRec.Code, updRec.Body.String())
	}
	var updated CronTriggerResponse
	_ = json.NewDecoder(updRec.Body).Decode(&updated)
	if updated.Prompt != "new prompt" {
		t.Errorf("updated prompt = %q, want new prompt", updated.Prompt)
	}
	if updated.Enabled {
		t.Errorf("updated enabled should be false")
	}

	// Delete.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/cron-triggers/"+created.ID, nil)
	delRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("DELETE expected 204, got %d: %s", delRec.Code, delRec.Body.String())
	}

	// Get again -> 404.
	getReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/cron-triggers/"+created.ID, nil)
	getRec2 := httptest.NewRecorder()
	srv.mux.ServeHTTP(getRec2, getReq2)
	if getRec2.Code != http.StatusNotFound {
		t.Errorf("GET after delete expected 404, got %d", getRec2.Code)
	}
}

func ptr[T any](v T) *T { return &v }
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/skills"
)

// mockSkillService implements SkillService for handler tests.
type mockSkillService struct {
	skills      map[string]*skills.Skill
	assignments map[string][]skills.Referrer // skillID -> referrers
	assignErr   error
	unassignErr error
	assignCalls []assignCallRecord
}

type assignCallRecord struct {
	skillID        string
	agentImageName string
}

func newMockSkillService() *mockSkillService {
	return &mockSkillService{
		skills:      make(map[string]*skills.Skill),
		assignments: make(map[string][]skills.Referrer),
	}
}

func (m *mockSkillService) Create(_ context.Context, req skills.CreateRequest) (*skills.Skill, error) {
	sk := &skills.Skill{ID: req.ID, Name: req.Name}
	m.skills[req.ID] = sk
	return sk, nil
}

func (m *mockSkillService) Get(_ context.Context, id string) (*skills.Skill, error) {
	sk, ok := m.skills[id]
	if !ok {
		return nil, skills.ErrNotFound
	}
	return sk, nil
}

func (m *mockSkillService) List(_ context.Context, _ skills.ListFilter) ([]skills.SkillSummary, error) {
	return nil, nil
}

func (m *mockSkillService) Update(_ context.Context, _ string, _ skills.UpdateRequest) (*skills.Skill, error) {
	return nil, nil
}

func (m *mockSkillService) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockSkillService) ListAssignments(_ context.Context, skillID string) ([]skills.Referrer, error) {
	return m.assignments[skillID], nil
}

func (m *mockSkillService) Assign(_ context.Context, skillID, agentImageName string) error {
	m.assignCalls = append(m.assignCalls, assignCallRecord{skillID: skillID, agentImageName: agentImageName})
	return m.assignErr
}

func (m *mockSkillService) Unassign(_ context.Context, skillID, agentImageName string) error {
	m.assignCalls = append(m.assignCalls, assignCallRecord{skillID: skillID, agentImageName: agentImageName})
	return m.unassignErr
}

// registerSkillRoutesForTest wires skill routes onto the test server's mux.
func registerSkillRoutesForTest(svc SkillService) http.Handler {
	mux := http.NewServeMux()
	RegisterSkillRoutes(mux, svc, nil, nil)
	return mux
}

func TestSkillAssignments_GETReturnsAssignments(t *testing.T) {
	svc := newMockSkillService()
	svc.skills["code-review"] = &skills.Skill{ID: "code-review", Name: "Code Review"}
	svc.assignments["code-review"] = []skills.Referrer{
		{AgentImageName: "img-1"},
		{AgentImageName: "img-2"},
	}

	mux := registerSkillRoutesForTest(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/code-review/assignments", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Items []skills.Referrer `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
}

func TestSkillAssignments_GETReturnsEmptyArrayWhenNoAssignments(t *testing.T) {
	svc := newMockSkillService()
	svc.skills["code-review"] = &skills.Skill{ID: "code-review", Name: "Code Review"}

	mux := registerSkillRoutesForTest(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/code-review/assignments", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Items []skills.Referrer `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Items == nil {
		t.Errorf("expected non-nil items array")
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}
}

func TestSkillAssignments_GETSkillNotFound(t *testing.T) {
	svc := newMockSkillService()

	mux := registerSkillRoutesForTest(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/nonexistent/assignments", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSkillAssignments_GETMethodNotAllowed(t *testing.T) {
	svc := newMockSkillService()
	svc.skills["code-review"] = &skills.Skill{ID: "code-review", Name: "Code Review"}

	mux := registerSkillRoutesForTest(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/skills/code-review/assignments", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSkillAssignments_PUTReturns204(t *testing.T) {
	svc := newMockSkillService()
	svc.skills["code-review"] = &skills.Skill{ID: "code-review", Name: "Code Review"}

	mux := registerSkillRoutesForTest(svc)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/skills/code-review/assignments/img-abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(svc.assignCalls) != 1 {
		t.Fatalf("expected 1 assign call, got %d", len(svc.assignCalls))
	}
	if svc.assignCalls[0].skillID != "code-review" || svc.assignCalls[0].agentImageName != "img-abc" {
		t.Errorf("unexpected assign call: %+v", svc.assignCalls[0])
	}
}

func TestSkillAssignments_PUTSkillNotFound(t *testing.T) {
	svc := newMockSkillService()

	mux := registerSkillRoutesForTest(svc)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/skills/nonexistent/assignments/img-abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSkillAssignments_DELETEReturns204(t *testing.T) {
	svc := newMockSkillService()
	svc.skills["code-review"] = &skills.Skill{ID: "code-review", Name: "Code Review"}

	mux := registerSkillRoutesForTest(svc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/skills/code-review/assignments/img-abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(svc.assignCalls) != 1 {
		t.Fatalf("expected 1 unassign call, got %d", len(svc.assignCalls))
	}
	if svc.assignCalls[0].skillID != "code-review" || svc.assignCalls[0].agentImageName != "img-abc" {
		t.Errorf("unexpected unassign call: %+v", svc.assignCalls[0])
	}
}

func TestSkillAssignments_DELETEMethodNotAllowed(t *testing.T) {
	svc := newMockSkillService()
	svc.skills["code-review"] = &skills.Skill{ID: "code-review", Name: "Code Review"}

	mux := registerSkillRoutesForTest(svc)
	// POST on /assignments/:name should be 405
	req := httptest.NewRequest(http.MethodPost, "/api/v1/skills/code-review/assignments/img-abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSkillAssignments_PUTEmptyImageNameReturns404(t *testing.T) {
	svc := newMockSkillService()
	svc.skills["code-review"] = &skills.Skill{ID: "code-review", Name: "Code Review"}

	mux := registerSkillRoutesForTest(svc)
	// /api/v1/skills/code-review/assignments/ with trailing slash but empty name
	req := httptest.NewRequest(http.MethodPut, "/api/v1/skills/code-review/assignments/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// The empty agentImageName path should result in 404
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSkillAssignments_TooManyPathSegmentsReturns404(t *testing.T) {
	svc := newMockSkillService()
	svc.skills["code-review"] = &skills.Skill{ID: "code-review", Name: "Code Review"}

	mux := registerSkillRoutesForTest(svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/code-review/assignments/img-abc/extra", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

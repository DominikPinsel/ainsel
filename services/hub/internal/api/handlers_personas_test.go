package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/api"
	"github.com/DominikPinsel/ainsel/services/hub/internal/personas"
)

// stubPersonaService is a hand-rolled mock of the persona Service. The
// Server depends on the PersonaService interface (defined in
// handlers_personas.go).
type stubPersonaService struct {
	createFn   func(ctx context.Context, req personas.CreateRequest) (*personas.Persona, error)
	getFn      func(ctx context.Context, id string) (*personas.Persona, error)
	listFn     func(ctx context.Context) ([]personas.PersonaSummary, error)
	updateFn   func(ctx context.Context, id string, req personas.UpdateRequest) (*personas.Persona, error)
	deleteFn   func(ctx context.Context, id string) error
	listVerFn  func(ctx context.Context, id string) ([]personas.VersionSummary, error)
	getVerFn   func(ctx context.Context, id string, n int) (*personas.Version, error)
	rollbackFn func(ctx context.Context, id string, n int) (*personas.Persona, error)
}

func (s *stubPersonaService) Create(ctx context.Context, req personas.CreateRequest) (*personas.Persona, error) {
	return s.createFn(ctx, req)
}
func (s *stubPersonaService) Get(ctx context.Context, id string) (*personas.Persona, error) {
	return s.getFn(ctx, id)
}
func (s *stubPersonaService) List(ctx context.Context) ([]personas.PersonaSummary, error) {
	return s.listFn(ctx)
}
func (s *stubPersonaService) Update(ctx context.Context, id string, req personas.UpdateRequest) (*personas.Persona, error) {
	return s.updateFn(ctx, id, req)
}
func (s *stubPersonaService) Delete(ctx context.Context, id string) error {
	return s.deleteFn(ctx, id)
}
func (s *stubPersonaService) ListVersions(ctx context.Context, id string) ([]personas.VersionSummary, error) {
	return s.listVerFn(ctx, id)
}
func (s *stubPersonaService) GetVersion(ctx context.Context, id string, n int) (*personas.Version, error) {
	return s.getVerFn(ctx, id, n)
}
func (s *stubPersonaService) Rollback(ctx context.Context, id string, n int) (*personas.Persona, error) {
	return s.rollbackFn(ctx, id, n)
}

func newPersonaServer(t *testing.T, svc api.PersonaService) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	api.RegisterPersonaRoutes(mux, svc, nil, nil)
	return httptest.NewServer(mux)
}

func TestHandlerCreatePersona(t *testing.T) {
	svc := &stubPersonaService{
		createFn: func(ctx context.Context, req personas.CreateRequest) (*personas.Persona, error) {
			if req.Name != "code-reviewer" || req.Text != "Hello." {
				t.Errorf("unexpected req: %+v", req)
			}
			return &personas.Persona{ID: "01X", Name: req.Name, Text: req.Text, CurrentVersion: 1}, nil
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()

	body := bytes.NewBufferString(`{"name":"code-reviewer","text":"Hello."}`)
	resp, err := http.Post(srv.URL+"/api/v1/personas", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	var p personas.Persona
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.ID != "01X" {
		t.Errorf("expected ID=01X, got %q", p.ID)
	}
}

func TestHandlerCreatePersonaValidationError(t *testing.T) {
	svc := &stubPersonaService{
		createFn: func(ctx context.Context, req personas.CreateRequest) (*personas.Persona, error) {
			return nil, &personas.ValidationError{Field: "name", Message: "is required"}
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()

	body := bytes.NewBufferString(`{"name":"","text":"x"}`)
	resp, err := http.Post(srv.URL+"/api/v1/personas", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerCreatePersonaInvalidJSON(t *testing.T) {
	svc := &stubPersonaService{}
	srv := newPersonaServer(t, svc)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/personas", "application/json", bytes.NewBufferString(`not-json`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerCreatePersonaNameConflict(t *testing.T) {
	svc := &stubPersonaService{
		createFn: func(ctx context.Context, req personas.CreateRequest) (*personas.Persona, error) {
			return nil, personas.ErrNameTaken
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	body := bytes.NewBufferString(`{"name":"dup","text":"x"}`)
	resp, err := http.Post(srv.URL+"/api/v1/personas", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
}

func TestHandlerGetPersona(t *testing.T) {
	svc := &stubPersonaService{
		getFn: func(ctx context.Context, id string) (*personas.Persona, error) {
			if id != "01X" {
				return nil, personas.ErrNotFound
			}
			return &personas.Persona{ID: "01X", Name: "n", Text: "t", CurrentVersion: 3}, nil
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/personas/01X")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandlerGetPersonaNotFound(t *testing.T) {
	svc := &stubPersonaService{
		getFn: func(ctx context.Context, id string) (*personas.Persona, error) {
			return nil, personas.ErrNotFound
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/personas/missing")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandlerListPersonas(t *testing.T) {
	svc := &stubPersonaService{
		listFn: func(ctx context.Context) ([]personas.PersonaSummary, error) {
			return []personas.PersonaSummary{
				{ID: "a", Name: "alpha"},
				{ID: "b", Name: "beta"},
			}, nil
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/personas")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var page struct {
		Items      []personas.PersonaSummary `json:"items"`
		Total      int                       `json:"total"`
		Page       int                       `json:"page"`
		PageSize   int                       `json:"pageSize"`
		TotalPages int                       `json:"totalPages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page.Items) != 2 || page.Total != 2 {
		t.Errorf("unexpected items/total: %+v", page)
	}
	if page.Page != 1 || page.PageSize != 50 || page.TotalPages != 1 {
		t.Errorf("unexpected pagination fields: page=%d pageSize=%d totalPages=%d", page.Page, page.PageSize, page.TotalPages)
	}
}

func TestHandlerListPersonasPagination(t *testing.T) {
	// Build 25 personas so we can exercise paging beyond a single page.
	all := make([]personas.PersonaSummary, 25)
	for i := range all {
		all[i] = personas.PersonaSummary{ID: "p" + strconv.Itoa(i), Name: "p" + strconv.Itoa(i)}
	}
	svc := &stubPersonaService{
		listFn: func(ctx context.Context) ([]personas.PersonaSummary, error) {
			return all, nil
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/personas?page=2&pageSize=10")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var page struct {
		Items      []personas.PersonaSummary `json:"items"`
		Total      int                       `json:"total"`
		Page       int                       `json:"page"`
		PageSize   int                       `json:"pageSize"`
		TotalPages int                       `json:"totalPages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page.Items) != 10 || page.Total != 25 || page.Page != 2 || page.PageSize != 10 || page.TotalPages != 3 {
		t.Errorf("unexpected paged response: items=%d total=%d page=%d pageSize=%d totalPages=%d",
			len(page.Items), page.Total, page.Page, page.PageSize, page.TotalPages)
	}
}

func TestHandlerUpdatePersona(t *testing.T) {
	svc := &stubPersonaService{
		updateFn: func(ctx context.Context, id string, req personas.UpdateRequest) (*personas.Persona, error) {
			if req.Text == nil || *req.Text != "v2" {
				t.Errorf("expected text=v2, got %+v", req)
			}
			return &personas.Persona{ID: id, Name: "n", Text: "v2", CurrentVersion: 2}, nil
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/personas/01X", bytes.NewBufferString(`{"text":"v2"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandlerDeletePersonaSuccess(t *testing.T) {
	svc := &stubPersonaService{
		deleteFn: func(ctx context.Context, id string) error { return nil },
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/personas/01X", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestHandlerDeletePersonaInUse(t *testing.T) {
	svc := &stubPersonaService{
		deleteFn: func(ctx context.Context, id string) error {
			return &personas.ErrInUse{Referrers: []personas.Referrer{{AgentName: "x"}}}
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/personas/01X", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "referrers") {
		t.Errorf("expected body to mention referrers, got %s", body)
	}
}

func TestHandlerListVersions(t *testing.T) {
	svc := &stubPersonaService{
		listVerFn: func(ctx context.Context, id string) ([]personas.VersionSummary, error) {
			return []personas.VersionSummary{{PersonaID: id, VersionNumber: 2}, {PersonaID: id, VersionNumber: 1}}, nil
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/personas/01X/versions")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var page struct {
		Items      []personas.VersionSummary `json:"items"`
		Total      int                       `json:"total"`
		Page       int                       `json:"page"`
		PageSize   int                       `json:"pageSize"`
		TotalPages int                       `json:"totalPages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page.Items) != 2 || page.Total != 2 {
		t.Errorf("unexpected items/total: %+v", page)
	}
	if page.Page != 1 || page.PageSize != 50 || page.TotalPages != 1 {
		t.Errorf("unexpected pagination fields: page=%d pageSize=%d totalPages=%d", page.Page, page.PageSize, page.TotalPages)
	}
}

func TestHandlerListVersionsPagination(t *testing.T) {
	// 12 versions so page=2&pageSize=5 returns 5 items on page 2 of 3.
	all := make([]personas.VersionSummary, 12)
	for i := range all {
		all[i] = personas.VersionSummary{PersonaID: "01X", VersionNumber: 12 - i}
	}
	svc := &stubPersonaService{
		listVerFn: func(ctx context.Context, id string) ([]personas.VersionSummary, error) {
			return all, nil
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/personas/01X/versions?page=2&pageSize=5")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var page struct {
		Items      []personas.VersionSummary `json:"items"`
		Total      int                       `json:"total"`
		Page       int                       `json:"page"`
		PageSize   int                       `json:"pageSize"`
		TotalPages int                       `json:"totalPages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page.Items) != 5 || page.Total != 12 || page.Page != 2 || page.PageSize != 5 || page.TotalPages != 3 {
		t.Errorf("unexpected paged response: items=%d total=%d page=%d pageSize=%d totalPages=%d",
			len(page.Items), page.Total, page.Page, page.PageSize, page.TotalPages)
	}
}

func TestHandlerGetVersion(t *testing.T) {
	svc := &stubPersonaService{
		getVerFn: func(ctx context.Context, id string, n int) (*personas.Version, error) {
			if id != "01X" || n != 2 {
				return nil, personas.ErrNotFound
			}
			return &personas.Version{PersonaID: id, VersionNumber: n, Text: "v2"}, nil
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/personas/01X/versions/2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandlerGetVersionInvalidNumber(t *testing.T) {
	svc := &stubPersonaService{}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/personas/01X/versions/abc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerRollback(t *testing.T) {
	svc := &stubPersonaService{
		rollbackFn: func(ctx context.Context, id string, n int) (*personas.Persona, error) {
			if n != 2 {
				return nil, errors.New("unexpected version")
			}
			return &personas.Persona{ID: id, Name: "n", CurrentVersion: 5, Text: "rolled"}, nil
		},
	}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	body := bytes.NewBufferString(`{"toVersion":2}`)
	resp, err := http.Post(srv.URL+"/api/v1/personas/01X/rollback", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandlerRollbackInvalidBody(t *testing.T) {
	svc := &stubPersonaService{}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	body := bytes.NewBufferString(`{"toVersion":0}`)
	resp, err := http.Post(srv.URL+"/api/v1/personas/01X/rollback", "application/json", body)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	svc := &stubPersonaService{}
	srv := newPersonaServer(t, svc)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/api/v1/personas", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

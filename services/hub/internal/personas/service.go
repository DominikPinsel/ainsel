package personas

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"
)

// AgentLister returns the Agent CRs that reference a persona by ID.
// Implementations live outside the personas package (see agent_lister.go)
// because they read from the K8s API of the Agent CRD which the hub
// already owns.
type AgentLister interface {
	ListReferrers(ctx context.Context, personaID string) ([]Referrer, error)
}

// Service orchestrates Store + Reconciler + AgentLister.
type Service struct {
	store       *Store
	rec         *Reconciler
	agentLister AgentLister
}

// NewService wires a Service against its dependencies.
func NewService(s *Store, r *Reconciler, a AgentLister) *Service {
	return &Service{store: s, rec: r, agentLister: a}
}

// Reconciler exposes the underlying reconciler; primarily for tests.
func (s *Service) Reconciler() *Reconciler {
	return s.rec
}

// CreateRequest is the API-shape input to Create.
type CreateRequest struct {
	Name        string `json:"name"`
	GroupID     string `json:"groupId"`
	Description string `json:"description"`
	Text        string `json:"text"`
}

// ValidationError is returned on invalid input. handlers map it to HTTP 400.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation: %s: %s", e.Field, e.Message)
}

// ErrInUse is returned by Delete when the persona has referrers.
// handlers map it to HTTP 409 with the referrer list in the body.
type ErrInUse struct {
	Referrers []Referrer
}

func (e *ErrInUse) Error() string {
	return fmt.Sprintf("persona is in use by %d agent(s)", len(e.Referrers))
}

const (
	maxNameLen        = 200
	maxDescriptionLen = 2000
	maxTextLen        = 100_000
)

func validateName(name string) error {
	if name == "" {
		return &ValidationError{Field: "name", Message: "is required"}
	}
	if len(name) > maxNameLen {
		return &ValidationError{Field: "name", Message: fmt.Sprintf("must be <= %d chars", maxNameLen)}
	}
	return nil
}

func validateDescription(d string) error {
	if len(d) > maxDescriptionLen {
		return &ValidationError{Field: "description", Message: fmt.Sprintf("must be <= %d chars", maxDescriptionLen)}
	}
	return nil
}

func validateText(text string) error {
	if text == "" {
		return &ValidationError{Field: "text", Message: "is required"}
	}
	if len(text) > maxTextLen {
		return &ValidationError{Field: "text", Message: fmt.Sprintf("must be <= %d chars", maxTextLen)}
	}
	return nil
}

// Create persists a new persona and renders its ConfigMap.
//
// Transactional contract on K8s failure: option (a) from the spec —
// the DB row is deleted so the next attempt can retry cleanly. (b)
// would require reverting current_version_id on a row that was just
// inserted, which is no simpler than delete-then-retry here because
// the row had no prior version.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Persona, error) {
	if err := validateName(req.Name); err != nil {
		return nil, err
	}
	if err := validateDescription(req.Description); err != nil {
		return nil, err
	}
	if err := validateText(req.Text); err != nil {
		return nil, err
	}

	id := strings.ToLower(ulid.Make().String())
	p := &Persona{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Text:        req.Text,
	}
	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}
	if err := s.rec.Ensure(ctx, p); err != nil {
		// Best-effort rollback: remove the DB row so the next attempt
		// can retry cleanly (option (a) from the spec's flagged
		// "transactional rollback" decision).
		_ = s.store.Delete(ctx, p.ID)
		return nil, fmt.Errorf("render configmap: %w", err)
	}
	return p, nil
}

// Get returns a persona by ID.
func (s *Service) Get(ctx context.Context, id string) (*Persona, error) {
	return s.store.Get(ctx, id)
}

// List returns all personas (metadata only).
func (s *Service) List(ctx context.Context) ([]PersonaSummary, error) {
	return s.store.List(ctx)
}

// Update applies a partial update; re-renders the ConfigMap if text changed.
// (Ensure is idempotent so we re-render unconditionally — the cost is
// negligible and it heals out-of-band changes to non-text fields.)
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Persona, error) {
	if req.Name != nil {
		if err := validateName(*req.Name); err != nil {
			return nil, err
		}
	}
	if req.Description != nil {
		if err := validateDescription(*req.Description); err != nil {
			return nil, err
		}
	}
	if req.Text != nil {
		if err := validateText(*req.Text); err != nil {
			return nil, err
		}
	}
	updated, err := s.store.Update(ctx, id, req)
	if err != nil {
		return nil, err
	}
	if err := s.rec.Ensure(ctx, updated); err != nil {
		return nil, fmt.Errorf("render configmap: %w", err)
	}
	return updated, nil
}

// ListVersions delegates to the store.
func (s *Service) ListVersions(ctx context.Context, personaID string) ([]VersionSummary, error) {
	return s.store.ListVersions(ctx, personaID)
}

// GetVersion delegates to the store.
func (s *Service) GetVersion(ctx context.Context, personaID string, n int) (*Version, error) {
	return s.store.GetVersion(ctx, personaID, n)
}

// Rollback copies a past version's text into a new version and re-renders
// the ConfigMap.
func (s *Service) Rollback(ctx context.Context, personaID string, toVersion int) (*Persona, error) {
	p, err := s.store.Rollback(ctx, personaID, toVersion)
	if err != nil {
		return nil, err
	}
	if err := s.rec.Ensure(ctx, p); err != nil {
		return nil, fmt.Errorf("render configmap: %w", err)
	}
	return p, nil
}

// Delete checks referrers first and refuses if any exist.
func (s *Service) Delete(ctx context.Context, id string) error {
	refs, err := s.agentLister.ListReferrers(ctx, id)
	if err != nil {
		return fmt.Errorf("check referrers: %w", err)
	}
	if len(refs) > 0 {
		return &ErrInUse{Referrers: refs}
	}
	if err := s.store.Delete(ctx, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if err := s.rec.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete configmap: %w", err)
	}
	return nil
}

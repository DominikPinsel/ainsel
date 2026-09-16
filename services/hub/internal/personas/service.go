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
	// OwnerAgent marks the persona as owned by that Agent CR (empty =
	// shared template). Owned personas are hidden from List.
	OwnerAgent string `json:"ownerAgent,omitempty"`
}

// ErrNotOwned is returned by DeleteOwned when the persona is not owned by
// the given agent — a shared template, or another agent's persona.
var ErrNotOwned = errors.New("persona is not owned by this agent")

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
		OwnerAgent:  req.OwnerAgent,
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

// List returns shared template personas (metadata only). Agent-owned
// personas are excluded — they are addressed through their owning agent.
func (s *Service) List(ctx context.Context) ([]PersonaSummary, error) {
	return s.store.List(ctx)
}

// OwnedBy returns the persona owned by agentName, or nil when the agent has
// no owned persona (it still references a shared template, or none at all).
func (s *Service) OwnedBy(ctx context.Context, agentName string) (*Persona, error) {
	p, err := s.store.GetByOwner(ctx, agentName)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// EnsureOwned returns the agent's own persona: the existing one when the
// agent already owns a persona, otherwise a new one created copy-on-write
// from the request. defaultName is used when the request carries no name —
// typically derived from the agent's display name.
//
// Callers re-point the Agent CR at the returned persona when its ID differs
// from the current reference. If that write fails the persona is orphaned
// but reused by the next EnsureOwned call, so the operation is idempotent.
func (s *Service) EnsureOwned(ctx context.Context, agentName, defaultName string, req UpdateRequest) (*Persona, error) {
	if req.Text == nil {
		return nil, &ValidationError{Field: "text", Message: "is required"}
	}
	existing, err := s.store.GetByOwner(ctx, agentName)
	switch {
	case err == nil:
		return s.Update(ctx, existing.ID, req)
	case errors.Is(err, ErrNotFound):
		// No owned persona yet: create one below.
	default:
		return nil, err
	}
	name := defaultName
	if req.Name != nil && *req.Name != "" {
		name = *req.Name
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	return s.Create(ctx, CreateRequest{
		Name:        name,
		Description: description,
		Text:        *req.Text,
		OwnerAgent:  agentName,
	})
}

// DeleteOwned removes an agent-owned persona, bypassing the referrer check
// in Delete: the owning agent is itself the referrer, and the persona is
// invisible to the library, so leaving it behind would leak an orphan.
// Deleting a persona that is already gone is a no-op; refusing anything not
// owned by agentName keeps shared templates and other agents' personas safe.
func (s *Service) DeleteOwned(ctx context.Context, personaID, agentName string) error {
	p, err := s.store.Get(ctx, personaID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if p.OwnerAgent != agentName {
		return ErrNotOwned
	}
	if err := s.store.Delete(ctx, personaID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	if err := s.rec.Delete(ctx, personaID); err != nil {
		return fmt.Errorf("delete configmap: %w", err)
	}
	return nil
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

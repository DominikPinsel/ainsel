package skills

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	sharedskills "github.com/DominikPinsel/ainsel/shared/api/skills"
)

// AgentImageLister returns the AgentImage CRs that reference a skill by ID.
type AgentImageLister interface {
	ListReferrers(ctx context.Context, skillID string) ([]Referrer, error)
	// UsageCounts lists AgentImage CRs once and tallies how many CRs
	// reference each skill ID via spec.enabledSkills. A CR that lists
	// the same skill ID more than once counts once for that skill.
	UsageCounts(ctx context.Context) (map[string]int, error)
	// Assign adds a skill ID to an AgentImage's spec.enabledSkills.
	Assign(ctx context.Context, skillID, agentImageName string) error
	// Unassign removes a skill ID from an AgentImage's spec.enabledSkills.
	Unassign(ctx context.Context, skillID, agentImageName string) error
}

// Service orchestrates Store + Reconciler + AgentImageLister.
type Service struct {
	store            *Store
	rec              *Reconciler
	agentImageLister AgentImageLister
}

// NewService wires a Service against its dependencies.
func NewService(s *Store, r *Reconciler, a AgentImageLister) *Service {
	return &Service{store: s, rec: r, agentImageLister: a}
}

// Reconciler exposes the underlying reconciler; primarily for tests.
func (s *Service) Reconciler() *Reconciler {
	return s.rec
}

// CreateRequest is the API-shape input to Create.
type CreateRequest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	GroupID     string   `json:"groupId"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
	Tags        []string `json:"tags"`
}

// ValidationError is returned on invalid input. handlers map it to HTTP 400.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation: %s: %s", e.Field, e.Message)
}

// ErrInUse is returned by Delete when the skill has referrers.
type ErrInUse struct {
	Referrers []Referrer
}

func (e *ErrInUse) Error() string {
	return fmt.Sprintf("skill is in use by %d agent image(s)", len(e.Referrers))
}

var slugRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
	maxIDLen          = 64
	maxNameLen        = 200
	maxDescriptionLen = 2000
	maxBodyLen        = 100_000
	maxTags           = 10
	maxTagLen         = 50
)

func validateID(id string) error {
	if id == "" {
		return &ValidationError{Field: "id", Message: "is required"}
	}
	if len(id) > maxIDLen {
		return &ValidationError{Field: "id", Message: fmt.Sprintf("must be <= %d chars", maxIDLen)}
	}
	if !slugRegex.MatchString(id) {
		return &ValidationError{Field: "id", Message: "must be lowercase alphanumeric with hyphens, no leading/trailing/consecutive hyphens"}
	}
	return nil
}

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

func validateBody(body string) error {
	if len(body) > maxBodyLen {
		return &ValidationError{Field: "body", Message: fmt.Sprintf("must be <= %d chars", maxBodyLen)}
	}
	return nil
}

func validateTags(tags []string) ([]string, error) {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" {
			continue
		}
		if len(t) > maxTagLen {
			return nil, &ValidationError{Field: "tags", Message: fmt.Sprintf("each tag must be <= %d chars", maxTagLen)}
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		normalized = append(normalized, t)
	}
	// Count after normalization so empty/duplicate entries don't count toward
	// the limit (e.g. 11 empty strings normalize to zero tags, not a rejection).
	if len(normalized) > maxTags {
		return nil, &ValidationError{Field: "tags", Message: fmt.Sprintf("must have <= %d tags", maxTags)}
	}
	return normalized, nil
}

// assembleSKILLMD builds the full SKILL.md content with YAML frontmatter.
func assembleSKILLMD(sk *Skill) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n%s", sk.ID, sk.Description, sk.Body)
}

// Create persists a new skill and renders its ConfigMap entry.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Skill, error) {
	if err := validateID(req.ID); err != nil {
		return nil, err
	}
	if err := validateName(req.Name); err != nil {
		return nil, err
	}
	if err := validateDescription(req.Description); err != nil {
		return nil, err
	}
	if err := validateBody(req.Body); err != nil {
		return nil, err
	}
	tags, err := validateTags(req.Tags)
	if err != nil {
		return nil, err
	}

	sk := &Skill{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Body:        req.Body,
		Tags:        tags,
	}
	if err := s.store.Create(ctx, sk); err != nil {
		return nil, err
	}
	if s.rec != nil {
		if err := s.rec.Ensure(ctx, sk); err != nil {
			if delErr := s.store.Delete(ctx, sk.ID); delErr != nil {
				slog.Error("skills: compensating delete after failed reconcile",
					"skill_id", sk.ID, "reconcile_err", err, "delete_err", delErr)
			}
			return nil, fmt.Errorf("render configmap: %w", err)
		}
	}
	return sk, nil
}

// Get returns a skill by ID.
func (s *Service) Get(ctx context.Context, id string) (*Skill, error) {
	return s.store.Get(ctx, id)
}

// List returns skills (metadata only), optionally filtered. Each
// summary is enriched with UsedBy, the number of AgentImage CRs that
// reference the skill. Usage is best-effort: if no lister is
// configured or the usage lookup fails, UsedBy is left 0 and the
// skills are still returned.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]SkillSummary, error) {
	summaries, err := s.store.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	if s.agentImageLister == nil {
		return summaries, nil
	}
	counts, err := s.agentImageLister.UsageCounts(ctx)
	if err != nil {
		slog.Error("skills: usage counts unavailable", "err", err)
		return summaries, nil
	}
	for i := range summaries {
		summaries[i].UsedBy = counts[summaries[i].ID]
	}
	return summaries, nil
}

// Update applies a partial update; re-renders the ConfigMap.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Skill, error) {
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
	if req.Body != nil {
		if err := validateBody(*req.Body); err != nil {
			return nil, err
		}
	}
	if req.Tags != nil {
		normalized, err := validateTags(*req.Tags)
		if err != nil {
			return nil, err
		}
		req.Tags = &normalized
	}
	updated, err := s.store.Update(ctx, id, req)
	if err != nil {
		return nil, err
	}
	if s.rec != nil {
		if err := s.rec.Ensure(ctx, updated); err != nil {
			return nil, fmt.Errorf("render configmap: %w", err)
		}
	}
	return updated, nil
}

// Delete checks referrers first and refuses if any exist.
//
// Ordering: the DB row is deleted before the ConfigMap entry. If the
// ConfigMap update fails, the row is gone but the data key lingers. The
// orphaned key is harmless for read paths (no DB row → never listed/got)
// and gets overwritten if the same ID is recreated. A later reconcile
// loop is the intended cleanup path; for v1 we accept the asymmetry.
func (s *Service) Delete(ctx context.Context, id string) error {
	if s.agentImageLister != nil {
		refs, err := s.agentImageLister.ListReferrers(ctx, id)
		if err != nil {
			return fmt.Errorf("check referrers: %w", err)
		}
		if len(refs) > 0 {
			return &ErrInUse{Referrers: refs}
		}
	}
	if err := s.store.Delete(ctx, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if s.rec != nil {
		if err := s.rec.Delete(ctx, id); err != nil {
			return fmt.Errorf("delete configmap entry: %w", err)
		}
	}
	return nil
}

// ListAssignments returns the AgentImage CRs that reference the given skill.
func (s *Service) ListAssignments(ctx context.Context, skillID string) ([]Referrer, error) {
	if s.agentImageLister == nil {
		return nil, nil
	}
	return s.agentImageLister.ListReferrers(ctx, skillID)
}

// Assign adds the skill to an AgentImage's enabledSkills.
func (s *Service) Assign(ctx context.Context, skillID, agentImageName string) error {
	// Verify the skill exists.
	if _, err := s.store.Get(ctx, skillID); err != nil {
		return err
	}
	if s.agentImageLister == nil {
		return fmt.Errorf("agent image lister not configured")
	}
	return s.agentImageLister.Assign(ctx, skillID, agentImageName)
}

// Unassign removes the skill from an AgentImage's enabledSkills.
func (s *Service) Unassign(ctx context.Context, skillID, agentImageName string) error {
	// Verify the skill exists.
	if _, err := s.store.Get(ctx, skillID); err != nil {
		return err
	}
	if s.agentImageLister == nil {
		return fmt.Errorf("agent image lister not configured")
	}
	return s.agentImageLister.Unassign(ctx, skillID, agentImageName)
}

// ConfigMapName returns the shared ConfigMap name for skills.
func ConfigMapName() string {
	return sharedskills.ConfigMapName
}

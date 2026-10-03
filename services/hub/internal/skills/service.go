package skills

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"

	sharedskills "github.com/DominikPinsel/ainsel/shared/api/skills"
)

// AgentImageLister returns the AgentImage CRs that reference a skill by ID.
type AgentImageLister interface {
	ListReferrers(ctx context.Context, skillID string) ([]Referrer, error)
	// UsageCounts lists AgentImage CRs once and tallies how many CRs
	// reference each skill ID via spec.enabledSkills. A CR that lists
	// the same skill ID more than once counts once for that skill.
	UsageCounts(ctx context.Context) (map[string]int, error)
	// EnabledSkillIDs tallies every skill ID any pod projects: the
	// image-level spec.enabledSkills plus the agent-level
	// spec.skills.items that replaces it. Used to decide what the shared
	// ConfigMap must carry.
	EnabledSkillIDs(ctx context.Context) (map[string]int, error)
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

	mu sync.Mutex
	// undelivered remembers why each skill was last reported as not
	// mounted, so a failure that repeats on the next retry is not logged
	// again. See logDeliveryChanges.
	undelivered map[string]string
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

// RenderSkillMD builds the full SKILL.md content: YAML frontmatter
// followed by the body. It is the single place the hub turns a registry
// row into the file an agent reads, so the ConfigMap mount and the skill
// catalogue MCP cannot drift in how the same skill is presented.
//
// The description is emitted as a double-quoted YAML scalar via %q, which
// handles the escaping. Plain scalars break on ": " — which is how skill
// descriptions are routinely written ("Read pull requests: metadata,
// commits, ...") — and that invalid YAML cost discovery for every skill
// whose summary happened to contain a colon.
func RenderSkillMD(sk *Skill) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %q\n---\n%s", sk.ID, sk.Description, sk.Body)
}

// renderBestEffort writes one skill to the shared ConfigMap without
// letting delivery failure undo the write. Postgres is the source of
// truth and Converge re-applies whatever is missing, so a skill that is
// stored but not yet mounted is a degraded state rather than an invalid
// one.
func (s *Service) renderBestEffort(ctx context.Context, op string, sk *Skill) {
	if s.rec == nil {
		return
	}
	if err := s.rec.Ensure(ctx, sk); err != nil {
		slog.Warn("skills: configmap render failed; convergence pass will retry",
			"op", op, "skill_id", sk.ID, "err", err)
	}
}

// deliverIfEnabled renders a skill only when some AgentImage or Agent
// enables it (the same union Converge keeps, via EnabledSkillIDs).
//
// The shared ConfigMap is sized to what agents mount, so writing a skill
// nobody has opted into spends ceiling and re-hashes the object, which
// the operator reads as a skill change and answers by restarting every
// skill-bearing agent. Catalogue writes must therefore stay out of it
// until an enable actually asks for the content.
func (s *Service) deliverIfEnabled(ctx context.Context, op string, sk *Skill) {
	if s.rec == nil || s.agentImageLister == nil {
		return
	}
	counts, err := s.agentImageLister.EnabledSkillIDs(ctx)
	if err != nil {
		// Not knowing the enabled set is not a reason to skip delivery:
		// an agent waiting on this content would stall on a transient
		// list failure. Write it; the next pass prunes it if unwanted.
		slog.Warn("skills: enabled-set lookup failed, rendering anyway",
			"op", op, "skill_id", sk.ID, "err", err)
		s.renderBestEffort(ctx, op, sk)
		return
	}
	if counts[sk.ID] < 1 {
		return
	}
	s.renderBestEffort(ctx, op, sk)
}

// Create persists a new skill. It deliberately does not render into the
// shared ConfigMap: nothing can be enabling a skill that did not exist a
// moment ago, so the entry would be pure catalogue weight until some
// agent assigned it. Delivery happens on Assign and on the next
// convergence pass.
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

// SearchForDiscovery returns skill metadata for the skill MCP, without
// the UsedBy enrichment List performs.
//
// The enrichment is a Kubernetes LIST of every AgentImage in the
// namespace. That is fine for an admin page loaded occasionally, but this
// method backs an agent-facing tool that a model may call repeatedly
// inside one task, and the answer it computes is one an agent has no use
// for: whether some image happens to mount the skill says nothing about
// whether the skill helps with the task at hand. Skipping it keeps the
// read to a single Postgres query.
//
// The full match set is returned: bounding it is the caller's job, and a
// caller that wants to say "there is more" needs to know whether there
// is, which a pre-truncated list cannot show.
//
// The cost of that is an unbounded read -- Store.List has no LIMIT, so a
// query-less browse fetches the whole registry to slice <=50 off it. At the
// current size this is one small sequential scan and the truthfulness of
// `matched` is worth more than the rows. If the registry grows by orders of
// magnitude, the fix belongs in the store (LIMIT n+1 with a COUNT for the
// total), not in a smaller slice here.
func (s *Service) SearchForDiscovery(ctx context.Context, query string, tags []string) ([]SkillSummary, error) {
	return s.store.List(ctx, ListFilter{Search: query, Tags: tags})
}

// Update applies a partial update. It re-renders the ConfigMap only when
// some AgentImage or Agent selects the skill: an edit to a catalogue
// entry must not touch the shared, size-capped delivery object.
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
	s.deliverIfEnabled(ctx, "update", updated)
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
	sk, err := s.store.Get(ctx, skillID)
	if err != nil {
		return err
	}
	if s.agentImageLister == nil {
		return fmt.Errorf("agent image lister not configured")
	}
	if err := s.agentImageLister.Assign(ctx, skillID, agentImageName); err != nil {
		return err
	}
	// This is the moment the content becomes load-bearing: an agent that
	// just asked for the skill should not wait out a convergence tick to
	// get it. Render now; the pass remains the safety net for CRs edited
	// outside the hub and for renders that fail on size.
	s.renderBestEffort(ctx, "assign", sk)
	return nil
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

// DeliveryReport is the outcome of one Converge pass. Both ID lists are
// sorted: they come out of map iteration, and a report that shuffles
// between passes is useless for comparison or logging.
type DeliveryReport struct {
	Enabled     int      `json:"enabled"`
	Delivered   []string `json:"delivered"`
	Undelivered []string `json:"undelivered"`
}

// Converge reconciles the shared skills ConfigMap to exactly the skills
// that at least one AgentImage (spec.enabledSkills) or Agent
// (spec.skills.items) enables.
//
// The ConfigMap is a single Kubernetes object and so carries the
// apiserver's 1 MiB ceiling. Treating it as a mirror of the whole
// registry made that ceiling a platform-wide cap on how many skills may
// exist, and — because the operator projects only the enabled keys —
// charged it against skills no agent had ever opted into. Mirroring the
// enabled set instead keeps the object sized to what agents actually
// mount.
//
// Catalogue skills that nothing enables are deliberately not delivered:
// they live in Postgres and are read through the API (and, in the shape
// of issue #300, the skill MCP) rather than mounted everywhere.
func (s *Service) Converge(ctx context.Context) (*DeliveryReport, error) {
	if s.rec == nil || s.agentImageLister == nil {
		return nil, fmt.Errorf("skills: converge requires a reconciler and an agent image lister")
	}
	counts, err := s.agentImageLister.EnabledSkillIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("skills: converge enabled skill ids: %w", err)
	}
	summaries, err := s.store.List(ctx, ListFilter{})
	if err != nil {
		return nil, err
	}

	keep := make(map[string]*Skill, len(counts))
	for _, sum := range summaries {
		if counts[sum.ID] < 1 {
			continue
		}
		sk, err := s.store.Get(ctx, sum.ID)
		if err != nil {
			// A usage count with no row means the image references a
			// deleted skill; the operator projects a key that will never
			// exist. Skip it rather than abort the whole pass.
			slog.Warn("skills: enabled skill has no registry row", "skill_id", sum.ID, "err", err)
			continue
		}
		keep[sum.ID] = sk
	}

	delivered, undelivered, err := s.rec.Converge(ctx, keep)
	if err != nil {
		return nil, err
	}
	report := &DeliveryReport{Enabled: len(keep), Delivered: delivered}
	for id := range undelivered {
		report.Undelivered = append(report.Undelivered, id)
	}
	sort.Strings(report.Delivered)
	sort.Strings(report.Undelivered)
	s.logDeliveryChanges(delivered, undelivered, len(keep))
	return report, nil
}

// logDeliveryChanges reports undelivered skills whose situation *changed*
// and nothing else.
//
// Per skill, with the reason, because undelivered is a valid steady state
// and nothing alerts on it: this WARN line is the only place an operator
// learns which skill is not mounted, and that the shared object's size
// ceiling is what said so. It fires when a skill first fails to be
// delivered and again if the error changes. It deliberately does not fire
// on every pass: the delivery loop retries every 30 s while anything is
// undelivered, so an unconditional line would repeat the same message
// ~100k times a day for a full ConfigMap and bury the one that reports a
// new failure. The per-pass aggregate stays with runSkillsDelivery, which
// logs the counts and the retry interval.
//
// A skill that stops being reported without being delivered was disabled
// (or deleted) rather than fixed, so it is forgotten silently; only a skill
// that actually arrived gets the recovery line.
//
// The state is per hub replica. With hub.replicas > 1 two replicas run
// their own pass, so each logs its own first observation of a failure.
func (s *Service) logDeliveryChanges(delivered []string, undelivered map[string]error, enabled int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.undelivered == nil {
		s.undelivered = make(map[string]string)
	}

	ids := make([]string, 0, len(undelivered))
	for id := range undelivered {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		reason := undelivered[id].Error()
		if prev, seen := s.undelivered[id]; seen && prev == reason {
			continue
		}
		slog.Warn("skills: enabled skill not delivered to the configmap",
			"skill_id", id, "err", undelivered[id], "enabled", enabled)
		s.undelivered[id] = reason
	}

	arrived := make(map[string]bool, len(delivered))
	for _, id := range delivered {
		arrived[id] = true
	}
	for id := range s.undelivered {
		if _, still := undelivered[id]; still {
			continue
		}
		if arrived[id] {
			slog.Info("skills: enabled skill delivered after retry",
				"skill_id", id, "enabled", enabled)
		}
		delete(s.undelivered, id)
	}
}

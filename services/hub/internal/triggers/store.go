package triggers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a trigger or cron trigger doesn't exist.
var ErrNotFound = errors.New("not found")

// ErrIDTaken is returned when a trigger ID collides.
var ErrIDTaken = errors.New("ID already in use")

// Store is the database layer for triggers and cron triggers.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires a Store against the given pgxpool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ---------------------------------------------------------------------------
// Trigger CRUD
// ---------------------------------------------------------------------------

// CreateTrigger inserts a new trigger.
func (s *Store) CreateTrigger(ctx context.Context, t *Trigger) error {
	now := time.Now().UTC()
	filtersJSON, err := json.Marshal(t.Filters)
	if err != nil {
		return fmt.Errorf("triggers.CreateTrigger: marshal filters: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO triggers (id, display_name, agent_ref, connector_ref, filters, agent_valid, connector_valid, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		t.ID, t.DisplayName, t.AgentRef, t.ConnectorRef, filtersJSON, t.AgentValid, t.ConnectorValid, now,
	); err != nil {
		if isUniqueViolation(err) {
			return ErrIDTaken
		}
		return fmt.Errorf("insert trigger: %w", err)
	}
	t.CreatedAt = now
	t.UpdatedAt = now
	return nil
}

// GetTrigger returns a trigger by id.
func (s *Store) GetTrigger(ctx context.Context, id string) (*Trigger, error) {
	var t Trigger
	var filtersJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, display_name, agent_ref, connector_ref, filters, agent_valid, connector_valid, created_at, updated_at
		FROM triggers WHERE id = $1
	`, id).Scan(&t.ID, &t.DisplayName, &t.AgentRef, &t.ConnectorRef, &filtersJSON, &t.AgentValid, &t.ConnectorValid, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("triggers.GetTrigger: %w", err)
	}
	if len(filtersJSON) > 0 {
		if err := json.Unmarshal(filtersJSON, &t.Filters); err != nil {
			return nil, fmt.Errorf("triggers.GetTrigger: unmarshal filters: %w", err)
		}
	}
	return &t, nil
}

// ListTriggers returns all triggers, optionally filtered by agentRef and connectorRef.
func (s *Store) ListTriggers(ctx context.Context, agentRef, connectorRef string) ([]Trigger, error) {
	q := `SELECT id, display_name, agent_ref, connector_ref, filters, agent_valid, connector_valid, created_at, updated_at FROM triggers`
	args := []any{}
	conds := []string{}
	if agentRef != "" {
		args = append(args, agentRef)
		conds = append(conds, fmt.Sprintf("agent_ref = $%d", len(args)))
	}
	if connectorRef != "" {
		args = append(args, connectorRef)
		conds = append(conds, fmt.Sprintf("connector_ref = $%d", len(args)))
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY id"
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("triggers.ListTriggers: %w", err)
	}
	defer rows.Close()
	var out []Trigger
	for rows.Next() {
		var t Trigger
		var filtersJSON []byte
		if err := rows.Scan(&t.ID, &t.DisplayName, &t.AgentRef, &t.ConnectorRef, &filtersJSON, &t.AgentValid, &t.ConnectorValid, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("triggers.ListTriggers: %w", err)
		}
		if len(filtersJSON) > 0 {
			if err := json.Unmarshal(filtersJSON, &t.Filters); err != nil {
				return nil, fmt.Errorf("triggers.ListTriggers: unmarshal filters: %w", err)
			}
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateTrigger applies a partial update to a trigger.
func (s *Store) UpdateTrigger(ctx context.Context, id string, displayName, agentRef, connectorRef *string, filters *[]ainselapishared.Filter) (*Trigger, error) {
	var (
		curName, curAgent, curConnector string
		curFilters                      []byte
		agentValid, connectorValid      bool
		createdAt                       time.Time
	)
	err := s.pool.QueryRow(ctx, `
		SELECT display_name, agent_ref, connector_ref, filters, agent_valid, connector_valid, created_at
		FROM triggers WHERE id = $1 FOR UPDATE
	`, id).Scan(&curName, &curAgent, &curConnector, &curFilters, &agentValid, &connectorValid, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("triggers.UpdateTrigger: %w", err)
	}

	name := curName
	agent := curAgent
	connector := curConnector
	var newFilters []byte

	if displayName != nil {
		name = *displayName
	}
	if agentRef != nil {
		agent = *agentRef
		// Changing the agent ref invalidates prior agent validation.
		agentValid = false
	}
	if connectorRef != nil {
		connector = *connectorRef
		// Changing the connector ref invalidates prior connector validation.
		connectorValid = false
	}
	if filters != nil {
		newFilters, err = json.Marshal(*filters)
		if err != nil {
			return nil, fmt.Errorf("triggers.UpdateTrigger: marshal filters: %w", err)
		}
	} else {
		newFilters = curFilters
	}

	now := time.Now().UTC()
	if _, err := s.pool.Exec(ctx,
		`UPDATE triggers SET display_name=$1, agent_ref=$2, connector_ref=$3, filters=$4, agent_valid=$5, connector_valid=$6, updated_at=$7 WHERE id=$8`,
		name, agent, connector, newFilters, agentValid, connectorValid, now, id,
	); err != nil {
		return nil, fmt.Errorf("triggers.UpdateTrigger: %w", err)
	}

	var fl []ainselapishared.Filter
	if len(newFilters) > 0 {
		if err := json.Unmarshal(newFilters, &fl); err != nil {
			return nil, fmt.Errorf("triggers.UpdateTrigger: unmarshal filters: %w", err)
		}
	}
	return &Trigger{
		ID:             id,
		DisplayName:    name,
		AgentRef:       agent,
		ConnectorRef:   connector,
		Filters:        fl,
		AgentValid:     agentValid,
		ConnectorValid: connectorValid,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
	}, nil
}

// DeleteTrigger removes a trigger by id.
func (s *Store) DeleteTrigger(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM triggers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("triggers.DeleteTrigger: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetTriggerValidity updates the agent_valid and connector_valid flags.
func (s *Store) SetTriggerValidity(ctx context.Context, id string, agentValid, connectorValid bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE triggers SET agent_valid=$1, connector_valid=$2, updated_at=now() WHERE id=$3`,
		agentValid, connectorValid, id)
	if err != nil {
		return fmt.Errorf("triggers.SetTriggerValidity: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// CronTrigger CRUD
// ---------------------------------------------------------------------------

// CreateCronTrigger inserts a new cron trigger.
func (s *Store) CreateCronTrigger(ctx context.Context, ct *CronTrigger) error {
	now := time.Now().UTC()
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO cron_triggers (id, display_name, agent_ref, schedule, prompt, enabled, agent_valid, schedule_valid, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		ct.ID, ct.DisplayName, ct.AgentRef, ct.Schedule, ct.Prompt, ct.Enabled, ct.AgentValid, ct.ScheduleValid, now,
	); err != nil {
		if isUniqueViolation(err) {
			return ErrIDTaken
		}
		return fmt.Errorf("insert cron trigger: %w", err)
	}
	ct.CreatedAt = now
	ct.UpdatedAt = now
	return nil
}

// GetCronTrigger returns a cron trigger by id.
func (s *Store) GetCronTrigger(ctx context.Context, id string) (*CronTrigger, error) {
	var ct CronTrigger
	var lastRun, nextRun *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, display_name, agent_ref, schedule, prompt, enabled, agent_valid, schedule_valid, last_run, next_run, created_at, updated_at
		FROM cron_triggers WHERE id = $1
	`, id).Scan(&ct.ID, &ct.DisplayName, &ct.AgentRef, &ct.Schedule, &ct.Prompt, &ct.Enabled, &ct.AgentValid, &ct.ScheduleValid, &lastRun, &nextRun, &ct.CreatedAt, &ct.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("triggers.GetCronTrigger: %w", err)
	}
	ct.LastRun = lastRun
	ct.NextRun = nextRun
	return &ct, nil
}

// ListCronTriggers returns all cron triggers, optionally filtered by agentRef.
func (s *Store) ListCronTriggers(ctx context.Context, agentRef string) ([]CronTrigger, error) {
	q := `SELECT id, display_name, agent_ref, schedule, prompt, enabled, agent_valid, schedule_valid, last_run, next_run, created_at, updated_at FROM cron_triggers`
	args := []any{}
	if agentRef != "" {
		q += " WHERE agent_ref = $1"
		args = append(args, agentRef)
	}
	q += " ORDER BY id"
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("triggers.ListCronTriggers: %w", err)
	}
	defer rows.Close()
	var out []CronTrigger
	for rows.Next() {
		var ct CronTrigger
		var lastRun, nextRun *time.Time
		if err := rows.Scan(&ct.ID, &ct.DisplayName, &ct.AgentRef, &ct.Schedule, &ct.Prompt, &ct.Enabled, &ct.AgentValid, &ct.ScheduleValid, &lastRun, &nextRun, &ct.CreatedAt, &ct.UpdatedAt); err != nil {
			return nil, fmt.Errorf("triggers.ListCronTriggers: %w", err)
		}
		ct.LastRun = lastRun
		ct.NextRun = nextRun
		out = append(out, ct)
	}
	return out, rows.Err()
}

// UpdateCronTrigger applies a partial update to a cron trigger.
func (s *Store) UpdateCronTrigger(ctx context.Context, id string, displayName, agentRef, schedule, prompt *string, enabled *bool) (*CronTrigger, error) {
	var (
		curName, curAgent, curSchedule, curPrompt string
		curEnabled                                bool
		agentValid, scheduleValid                 bool
		createdAt                                 time.Time
	)
	err := s.pool.QueryRow(ctx, `
		SELECT display_name, agent_ref, schedule, prompt, enabled, agent_valid, schedule_valid, created_at
		FROM cron_triggers WHERE id = $1 FOR UPDATE
	`, id).Scan(&curName, &curAgent, &curSchedule, &curPrompt, &curEnabled, &agentValid, &scheduleValid, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("triggers.UpdateCronTrigger: %w", err)
	}

	name := curName
	agent := curAgent
	sched := curSchedule
	promp := curPrompt
	enab := curEnabled

	if displayName != nil {
		name = *displayName
	}
	if agentRef != nil {
		agent = *agentRef
		// Changing the agent ref invalidates prior agent validation.
		agentValid = false
	}
	if schedule != nil {
		sched = *schedule
		// Changing the schedule invalidates prior schedule validation.
		scheduleValid = false
	}
	if prompt != nil {
		promp = *prompt
	}
	if enabled != nil {
		enab = *enabled
	}

	now := time.Now().UTC()
	if _, err := s.pool.Exec(ctx,
		`UPDATE cron_triggers SET display_name=$1, agent_ref=$2, schedule=$3, prompt=$4, enabled=$5, agent_valid=$6, schedule_valid=$7, updated_at=$8 WHERE id=$9`,
		name, agent, sched, promp, enab, agentValid, scheduleValid, now, id,
	); err != nil {
		return nil, fmt.Errorf("triggers.UpdateCronTrigger: %w", err)
	}

	return &CronTrigger{
		ID:            id,
		DisplayName:   name,
		AgentRef:      agent,
		Schedule:      sched,
		Prompt:        promp,
		Enabled:       enab,
		AgentValid:    agentValid,
		ScheduleValid: scheduleValid,
		CreatedAt:     createdAt,
		UpdatedAt:     now,
	}, nil
}

// DeleteCronTrigger removes a cron trigger by id.
func (s *Store) DeleteCronTrigger(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM cron_triggers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("triggers.DeleteCronTrigger: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetCronTriggerValidity updates the agent_valid and schedule_valid flags.
func (s *Store) SetCronTriggerValidity(ctx context.Context, id string, agentValid, scheduleValid bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE cron_triggers SET agent_valid=$1, schedule_valid=$2, updated_at=now() WHERE id=$3`,
		agentValid, scheduleValid, id)
	if err != nil {
		return fmt.Errorf("triggers.SetCronTriggerValidity: %w", err)
	}
	return nil
}

// UpdateCronTriggerRunTimes updates last_run and next_run for a cron trigger.
func (s *Store) UpdateCronTriggerRunTimes(ctx context.Context, id string, lastRun, nextRun *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE cron_triggers SET last_run=$1, next_run=$2, updated_at=now() WHERE id=$3`,
		lastRun, nextRun, id)
	if err != nil {
		return fmt.Errorf("triggers.UpdateCronTriggerRunTimes: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

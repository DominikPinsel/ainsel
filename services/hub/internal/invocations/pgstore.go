package invocations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// queryTimeout bounds each store operation. The Store interface methods do
// not carry a caller context (they are called synchronously from the router
// and the ack/nack handlers), so each operation derives its own short-lived
// context instead of blocking indefinitely on a stalled database.
const queryTimeout = 5 * time.Second

// PgStore is the Postgres-backed invocation history. Unlike MemoryStore it
// survives hub restarts, so past events keep showing their invocations — and
// therefore their conversation transcripts — until records age out via
// Prune (see Retention, aligned with the conversation retention in tasklogs).
type PgStore struct {
	pool *pgxpool.Pool
}

// NewPgStore wires a PgStore against the given pool. The caller is
// responsible for running migrations (db.Migrate) before first use.
func NewPgStore(pool *pgxpool.Pool) *PgStore {
	return &PgStore{pool: pool}
}

// withTimeout derives the per-operation context used by all PgStore methods.
func withTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), queryTimeout)
}

// Record inserts a new invocation row and returns the stored record with ID,
// StartTime and Status populated. An insert failure is logged and the record
// returned as-is: the invocation ID is referenced by the already-enqueued
// task, so the enqueue path must not block or retry (matches MemoryStore,
// which never fails).
func (s *PgStore) Record(inv Invocation) Invocation {
	if inv.ID == "" {
		inv.ID = generateID()
	}
	if inv.StartTime.IsZero() {
		inv.StartTime = time.Now().UTC()
	}
	if inv.Status == "" {
		inv.Status = StatusRunning
	}

	ctx, cancel := withTimeout()
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO invocations (id, agent_name, trigger_name, event_id, connector, status, error, start_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, inv.ID, inv.AgentName, inv.TriggerName, inv.EventID, inv.Connector, inv.Status, inv.Error, inv.StartTime)
	if err != nil {
		slog.Error("invocations: record failed", "error", err, "invocation_id", inv.ID)
	}
	return inv
}

// Complete updates an existing invocation with its terminal status, computing
// the duration from the stored start time. Returns false when no row matched
// (e.g. the record was pruned).
func (s *PgStore) Complete(id, status, errMsg string, endTime time.Time) bool {
	if endTime.IsZero() {
		endTime = time.Now().UTC()
	}
	ctx, cancel := withTimeout()
	defer cancel()
	tag, err := s.pool.Exec(ctx, `
		UPDATE invocations
		SET status = $1,
		    error = $2,
		    end_time = $3,
		    duration_ms = (EXTRACT(EPOCH FROM ($3::timestamptz - start_time)) * 1000)::bigint
		WHERE id = $4
	`, status, errMsg, endTime, id)
	if err != nil {
		slog.Error("invocations: complete failed", "error", err, "invocation_id", id)
		return false
	}
	return tag.RowsAffected() == 1
}

// Get returns the invocation with the given ID, or false if absent.
func (s *PgStore) Get(id string) (Invocation, bool) {
	ctx, cancel := withTimeout()
	defer cancel()
	row := s.pool.QueryRow(ctx, `
		SELECT id, agent_name, trigger_name, event_id, connector, status, error, start_time, end_time, duration_ms
		FROM invocations
		WHERE id = $1
	`, id)
	inv, err := scanInvocation(row.Scan)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("invocations: get failed", "error", err, "invocation_id", id)
		}
		return Invocation{}, false
	}
	return inv, true
}

// List returns invocations sorted newest-first, applying the given filters.
func (s *PgStore) List(opts ListOptions) []Invocation {
	items, _ := s.ListWithTotal(opts)
	return items
}

// ListWithTotal behaves like List but additionally returns the number of
// invocations matching the filters before opts.Limit is applied.
func (s *PgStore) ListWithTotal(opts ListOptions) ([]Invocation, int) {
	where, args := listFilterClause(opts)

	countCtx, cancelCount := withTimeout()
	defer cancelCount()
	var total int
	if err := s.pool.QueryRow(countCtx, `SELECT count(*) FROM invocations`+where, args...).Scan(&total); err != nil {
		slog.Error("invocations: count failed", "error", err)
		return []Invocation{}, 0
	}

	query := `SELECT id, agent_name, trigger_name, event_id, connector, status, error, start_time, end_time, duration_ms
		FROM invocations` + where + ` ORDER BY start_time DESC, id DESC`
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	ctx, cancel := withTimeout()
	defer cancel()
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		slog.Error("invocations: list failed", "error", err)
		return []Invocation{}, total
	}
	defer rows.Close()

	out := make([]Invocation, 0)
	for rows.Next() {
		inv, err := scanInvocation(rows.Scan)
		if err != nil {
			slog.Error("invocations: scan failed", "error", err)
			continue
		}
		out = append(out, inv)
	}
	return out, total
}

// Len returns the number of invocations currently stored.
func (s *PgStore) Len() int {
	ctx, cancel := withTimeout()
	defer cancel()
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM invocations`).Scan(&n); err != nil {
		slog.Error("invocations: len failed", "error", err)
		return 0
	}
	return n
}

// Capacity reports DefaultCapacity for API compatibility. The Postgres store
// is bounded by time-based retention (see Retention), not a fixed count.
func (s *PgStore) Capacity() int {
	return DefaultCapacity
}

// Prune deletes invocation records that started before now-retention,
// returning the number of removed rows. Stale "running" rows are pruned like
// any other record: every normal path eventually calls Complete on them
// (tasks survive hub restarts in the event queue), and 48 hours is far
// beyond the longest retry window.
func (s *PgStore) Prune(ctx context.Context, retention time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM invocations WHERE start_time < now() - make_interval(secs => $1)
	`, int64(retention.Seconds()))
	if err != nil {
		return 0, fmt.Errorf("invocations.Prune: %w", err)
	}
	return tag.RowsAffected(), nil
}

// scanInvocation scans one invocation row via the given scan function (both
// pgx.Row and pgx.Rows have the matching Scan signature). end_time and
// duration_ms are nullable; a NULL end_time leaves the record in its running
// state (EndTime/DurationMs nil).
func scanInvocation(scan func(dest ...any) error) (Invocation, error) {
	var inv Invocation
	var startTime time.Time
	var endTime *time.Time
	var durationMs *int64
	if err := scan(
		&inv.ID, &inv.AgentName, &inv.TriggerName, &inv.EventID, &inv.Connector,
		&inv.Status, &inv.Error, &startTime, &endTime, &durationMs,
	); err != nil {
		return Invocation{}, err
	}
	inv.StartTime = startTime.UTC()
	if endTime != nil {
		t := endTime.UTC()
		inv.EndTime = &t
	}
	inv.DurationMs = durationMs
	return inv, nil
}

// listFilterClause builds the WHERE clause (with placeholder args) shared by
// ListWithTotal's count and select queries.
func listFilterClause(opts ListOptions) (string, []any) {
	conds := []string{}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if opts.AgentName != "" {
		add("agent_name = $%d", opts.AgentName)
	}
	if opts.Status != "" {
		add("status = $%d", opts.Status)
	}
	if opts.TriggerName != "" {
		add("trigger_name = $%d", opts.TriggerName)
	}
	if opts.EventID != "" {
		add("event_id = $%d", opts.EventID)
	}
	if !opts.Since.IsZero() {
		add("start_time >= $%d", opts.Since)
	}
	if !opts.Until.IsZero() {
		add("start_time < $%d", opts.Until)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}
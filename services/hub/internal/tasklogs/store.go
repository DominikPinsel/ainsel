package tasklogs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultRetention is how long log entries are kept before pruning.
const DefaultRetention = 7 * 24 * time.Hour

// ConversationRetention is how long conversation messages are kept.
// Shorter than log retention since conversations are high-volume.
const ConversationRetention = 48 * time.Hour

// Store is the task_logs database layer.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires a Store against the given pgxpool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Insert persists a single log entry. The entry's ID and CreatedAt fields
// are populated from the database on success.
func (s *Store) Insert(ctx context.Context, e *Entry) error {
	fieldsJSON, err := json.Marshal(e.Fields)
	if err != nil {
		return fmt.Errorf("tasklogs.Insert: marshal fields: %w", err)
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO task_logs (invocation_id, correlation_id, agent_name, level, message, fields)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at
	`, e.InvocationID, e.CorrelationID, e.AgentName, e.Level, e.Message, fieldsJSON).
		Scan(&e.ID, &e.CreatedAt)
}

// List returns entries newest-first, applying the given filters.
func (s *Store) List(ctx context.Context, opts ListOptions) ([]Entry, error) {
	query := `SELECT id, invocation_id, correlation_id, agent_name, level, message, fields, created_at FROM task_logs WHERE 1=1`
	args := []any{}
	argN := 1

	if opts.AgentName != "" {
		query += fmt.Sprintf(" AND agent_name = $%d", argN)
		args = append(args, opts.AgentName)
		argN++
	}
	if opts.Level != "" {
		query += fmt.Sprintf(" AND level = $%d", argN)
		args = append(args, opts.Level)
		argN++
	}
	if !opts.Since.IsZero() {
		query += fmt.Sprintf(" AND created_at >= $%d", argN)
		args = append(args, opts.Since)
		argN++
	}
	if !opts.Until.IsZero() {
		query += fmt.Sprintf(" AND created_at < $%d", argN)
		args = append(args, opts.Until)
		argN++
	}

	query += " ORDER BY created_at DESC"

	limit := opts.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}
	query += fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tasklogs.List: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var fieldsJSON []byte
		if err := rows.Scan(&e.ID, &e.InvocationID, &e.CorrelationID,
			&e.AgentName, &e.Level, &e.Message, &fieldsJSON, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("tasklogs.List scan: %w", err)
		}
		if len(fieldsJSON) > 0 {
			if err := json.Unmarshal(fieldsJSON, &e.Fields); err != nil {
				return nil, fmt.Errorf("tasklogs.List unmarshal fields: %w", err)
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Prune deletes entries older than the given retention duration.
// Returns the number of deleted rows.
func (s *Store) Prune(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	tag, err := s.pool.Exec(ctx, `DELETE FROM task_logs WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("tasklogs.Prune: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CountByLevelSince returns the number of entries with the given level
// created at or after the given time.
func (s *Store) CountByLevelSince(ctx context.Context, level string, since time.Time) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM task_logs WHERE level = $1 AND created_at >= $2`,
		level, since,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("tasklogs.CountByLevelSince: %w", err)
	}
	return count, nil
}

// InsertConversation persists a single conversation message.
func (s *Store) InsertConversation(ctx context.Context, m *ConversationMessage) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO task_conversations
			(invocation_id, correlation_id, agent_name, role, content, model, input_tokens, output_tokens, stop_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`, m.InvocationID, m.CorrelationID, m.AgentName, m.Role, m.Content,
		m.Model, m.InputTokens, m.OutputTokens, m.StopReason).Scan(&m.ID, &m.CreatedAt)
}

// ListConversations returns conversation messages for a given agent,
// newest-first. Filter by invocation or correlation ID when provided.
func (s *Store) ListConversations(ctx context.Context, agentName, invocationID, correlationID string, limit int) ([]ConversationMessage, error) {
	query := `SELECT id, invocation_id, correlation_id, agent_name, role, content, model, input_tokens, output_tokens, stop_reason, created_at
		FROM task_conversations WHERE 1=1`
	args := []any{}
	argN := 1

	if agentName != "" {
		query += fmt.Sprintf(" AND agent_name = $%d", argN)
		args = append(args, agentName)
		argN++
	}
	if invocationID != "" {
		query += fmt.Sprintf(" AND invocation_id = $%d", argN)
		args = append(args, invocationID)
		argN++
	}
	if correlationID != "" {
		query += fmt.Sprintf(" AND correlation_id = $%d", argN)
		args = append(args, correlationID)
		argN++
	}

	query += " ORDER BY created_at ASC, id ASC"

	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tasklogs.ListConversations: %w", err)
	}
	defer rows.Close()

	var out []ConversationMessage
	for rows.Next() {
		var m ConversationMessage
		if err := rows.Scan(&m.ID, &m.InvocationID, &m.CorrelationID,
			&m.AgentName, &m.Role, &m.Content, &m.Model,
			&m.InputTokens, &m.OutputTokens, &m.StopReason, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("tasklogs.ListConversations scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PruneConversations deletes conversation messages older than the given
// retention duration. Returns the number of deleted rows.
func (s *Store) PruneConversations(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)
	tag, err := s.pool.Exec(ctx, `DELETE FROM task_conversations WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("tasklogs.PruneConversations: %w", err)
	}
	return tag.RowsAffected(), nil
}

// InvocationTokenRow holds token totals grouped by invocation ID.
type InvocationTokenRow struct {
	InvocationID string
	InputTokens  int
	OutputTokens int
}

// TokensByInvocationSince returns token totals grouped by invocation_id for
// conversation messages created at or after the given time. Only rows with a
// non-empty invocation_id are returned.
func (s *Store) TokensByInvocationSince(ctx context.Context, since time.Time) ([]InvocationTokenRow, error) {
	query := `SELECT invocation_id, COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0)
		FROM task_conversations
		WHERE invocation_id != ''`
	args := []any{}
	argN := 1

	if !since.IsZero() {
		query += fmt.Sprintf(" AND created_at >= $%d", argN)
		args = append(args, since)
	}

	query += " GROUP BY invocation_id"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tasklogs.TokensByInvocationSince: %w", err)
	}
	defer rows.Close()

	var out []InvocationTokenRow
	for rows.Next() {
		var r InvocationTokenRow
		if err := rows.Scan(&r.InvocationID, &r.InputTokens, &r.OutputTokens); err != nil {
			return nil, fmt.Errorf("tasklogs.TokensByInvocationSince scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

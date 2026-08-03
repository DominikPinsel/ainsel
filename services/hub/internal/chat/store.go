package chat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSessionNotFound is returned when a chat session doesn't exist.
var ErrSessionNotFound = errors.New("chat session not found")

// Store is the chat session and message database layer.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires a Store against the given pgxpool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// generateSessionID returns a new opaque session ID.
func generateSessionID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("sess-%s", hex.EncodeToString(b))
}

// CreateSession creates a new chat session for the given agent and user.
// The session name defaults to the generated ID.
func (s *Store) CreateSession(ctx context.Context, agentName, userID string) (*Session, error) {
	id := generateSessionID()
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO chat_sessions (id, name, agent_name, user_id, created_at, updated_at)
		 VALUES ($1, $1, $2, $3, $4, $4)`,
		id, agentName, userID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create chat session: %w", err)
	}
	return &Session{
		ID:        id,
		Name:      id,
		AgentName: agentName,
		UserID:    userID,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// GetSession returns a session by ID, including its message history.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	var sess Session
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, agent_name, user_id, created_at, updated_at
		 FROM chat_sessions WHERE id = $1`, id,
	).Scan(&sess.ID, &sess.Name, &sess.AgentName, &sess.UserID, &sess.CreatedAt, &sess.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get chat session: %w", err)
	}

	messages, err := s.getMessages(ctx, id, 0)
	if err != nil {
		return nil, err
	}
	sess.Messages = messages
	return &sess, nil
}

// ListSessionsOptions filters session list results.
type ListSessionsOptions struct {
	AgentName string
	UserID    string
	Limit     int
}

// ListSessions returns sessions matching the given filters, newest first.
func (s *Store) ListSessions(ctx context.Context, opts ListSessionsOptions) ([]Session, error) {
	q := `SELECT id, name, agent_name, user_id, created_at, updated_at FROM chat_sessions`
	var args []any
	var where []string
	if opts.AgentName != "" {
		args = append(args, opts.AgentName)
		where = append(where, fmt.Sprintf("agent_name = $%d", len(args)))
	}
	if opts.UserID != "" {
		args = append(args, opts.UserID)
		where = append(where, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if len(where) > 0 {
		q += " WHERE " + joinStrings(where, " AND ")
	}
	q += " ORDER BY updated_at DESC"
	if opts.Limit > 0 {
		args = append(args, opts.Limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list chat sessions: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.Name, &sess.AgentName, &sess.UserID, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chat session: %w", err)
		}
		out = append(out, sess)
	}
	return out, nil
}

// UpdateSessionName sets the display name of a session. Returns
// ErrSessionNotFound if the session does not exist.
func (s *Store) UpdateSessionName(ctx context.Context, id, name string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE chat_sessions SET name = $2 WHERE id = $1`,
		id, name,
	)
	if err != nil {
		return fmt.Errorf("update chat session name: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// DeleteSession removes a session and all its messages.
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM chat_sessions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete chat session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// AddMessage appends a message to a session and updates the session's
// updated_at timestamp. Returns the created message.
func (s *Store) AddMessage(ctx context.Context, sessionID, role, content string, tokens int) (*Message, error) {
	var msg Message
	now := time.Now().UTC()
	err := s.pool.QueryRow(ctx,
		`INSERT INTO chat_messages (session_id, role, content, tokens, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, session_id, role, content, tokens, created_at`,
		sessionID, role, content, tokens, now,
	).Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.Tokens, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("add chat message: %w", err)
	}

	// Bump the session's updated_at so list queries sort correctly.
	if _, err := s.pool.Exec(ctx,
		`UPDATE chat_sessions SET updated_at = $2 WHERE id = $1`,
		sessionID, now,
	); err != nil {
		return nil, fmt.Errorf("update session timestamp: %w", err)
	}
	return &msg, nil
}

// getMessages returns messages for a session, ordered oldest first.
// If limit > 0, only the most recent `limit` messages are returned.
func (s *Store) getMessages(ctx context.Context, sessionID string, limit int) ([]Message, error) {
	q := `SELECT id, session_id, role, content, tokens, created_at
	      FROM chat_messages WHERE session_id = $1
	      ORDER BY created_at ASC`
	args := []any{sessionID}
	if limit > 0 {
		// Subquery to get the most recent N, then re-sort oldest-first.
		q = `SELECT id, session_id, role, content, tokens, created_at FROM (
		       SELECT id, session_id, role, content, tokens, created_at
		       FROM chat_messages WHERE session_id = $1
		       ORDER BY created_at DESC LIMIT $2
		     ) sub ORDER BY created_at ASC`
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("get chat messages: %w", err)
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.Tokens, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		out = append(out, msg)
	}
	return out, nil
}

// joinStrings joins strings with sep. (Avoids pulling in strings.Join for
// a single call site.)
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += sep + p
	}
	return out
}
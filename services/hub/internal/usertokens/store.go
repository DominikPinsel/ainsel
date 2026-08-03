package usertokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

var ErrNotFound = errors.New("not found")

// Token is a single user API token row. The plaintext is never stored.
type Token struct {
	ID          string
	UserID      string
	Name        string
	ExpiresAt   time.Time
	LastUsedAt  *time.Time
	CreatedAt   time.Time
	RevokedAt   *time.Time
}

// Store is the Postgres-backed user token store.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create generates a new token for userID, stores the SHA-256 hash, and
// returns the Token row plus the one-time plaintext. The plaintext is never
// persisted.
func (s *Store) Create(ctx context.Context, userID, name string, expiresAt time.Time) (Token, string, error) {
	plaintext, hash, err := generateToken()
	if err != nil {
		return Token{}, "", fmt.Errorf("generate token: %w", err)
	}
	id := ulid.Make().String()
	var t Token
	err = s.pool.QueryRow(ctx, `
		INSERT INTO user_tokens (id, user_id, name, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, now())
		RETURNING id, user_id, name, expires_at, last_used_at, created_at, revoked_at
	`, id, userID, name, hash, expiresAt).Scan(
		&t.ID, &t.UserID, &t.Name, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &t.RevokedAt,
	)
	if err != nil {
		return Token{}, "", fmt.Errorf("insert user token: %w", err)
	}
	return t, plaintext, nil
}

// List returns all tokens for the given userID ordered newest first, including
// expired and revoked tokens. Callers must inspect RevokedAt and ExpiresAt to
// determine whether each token is active.
func (s *Store) List(ctx context.Context, userID string) ([]Token, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, expires_at, last_used_at, created_at, revoked_at
		FROM user_tokens WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Token
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Revoke soft-deletes the token. Returns ErrNotFound if the token does not
// exist, is already revoked, or belongs to a different user.
func (s *Store) Revoke(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE user_tokens SET revoked_at = now()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Validate looks up the token by hash, marks last_used_at, and returns the
// Token row. Returns ErrNotFound if the token is unknown, expired, or revoked.
func (s *Store) Validate(ctx context.Context, plaintext string) (*Token, error) {
	sum := sha256.Sum256([]byte(plaintext))
	hash := hex.EncodeToString(sum[:])
	var t Token
	err := s.pool.QueryRow(ctx, `
		UPDATE user_tokens
		SET last_used_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		RETURNING id, user_id, name, expires_at, last_used_at, created_at, revoked_at
	`, hash).Scan(&t.ID, &t.UserID, &t.Name, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func generateToken() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	plaintext = "ainsel_" + hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	return
}

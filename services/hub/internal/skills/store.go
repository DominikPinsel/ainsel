package skills

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a skill doesn't exist.
var ErrNotFound = errors.New("skill not found")

// ErrIDTaken is returned when a skill ID collides.
var ErrIDTaken = errors.New("skill ID already in use")

// Store is the skill database layer.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires a Store against the given pgxpool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// UpdateRequest is the partial-update input shape. Nil fields stay unchanged.
type UpdateRequest struct {
	Name        *string
	Description *string
	Body        *string
	Tags        *[]string
}

// ListFilter holds optional search/tag filters for List.
type ListFilter struct {
	Search string
	Tags   []string
}

// Create inserts a new skill. Populates s.CreatedAt, s.UpdatedAt.
func (s *Store) Create(ctx context.Context, sk *Skill) error {
	now := time.Now().UTC()
	if sk.Tags == nil {
		sk.Tags = []string{}
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO skills (id, name, description, body, tags, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		sk.ID, sk.Name, sk.Description, sk.Body, sk.Tags, now,
	); err != nil {
		if isUniqueViolation(err) {
			return ErrIDTaken
		}
		return fmt.Errorf("insert skill: %w", err)
	}
	sk.CreatedAt = now
	sk.UpdatedAt = now
	return nil
}

// Get returns a skill by id.
func (s *Store) Get(ctx context.Context, id string) (*Skill, error) {
	var sk Skill
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, description, body, tags, created_at, updated_at
		FROM skills WHERE id = $1
	`, id).Scan(&sk.ID, &sk.Name, &sk.Description, &sk.Body, &sk.Tags, &sk.CreatedAt, &sk.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("skills.Get: %w", err)
	}
	if sk.Tags == nil {
		sk.Tags = []string{}
	}
	return &sk, nil
}

// List returns skills as summaries (no body), newest first,
// optionally filtered by search term and/or tags.
func (s *Store) List(ctx context.Context, filter ListFilter) ([]SkillSummary, error) {
	query := `SELECT id, name, description, tags, created_at, updated_at FROM skills`
	var conditions []string
	var args []any
	argIdx := 1

	if filter.Search != "" {
		conditions = append(conditions,
			fmt.Sprintf("(id ILIKE '%%' || $%d || '%%' OR name ILIKE '%%' || $%d || '%%' OR description ILIKE '%%' || $%d || '%%')", argIdx, argIdx, argIdx))
		args = append(args, escapeILIKE(filter.Search))
		argIdx++
	}
	if len(filter.Tags) > 0 {
		conditions = append(conditions, fmt.Sprintf("tags && $%d::text[]", argIdx))
		args = append(args, filter.Tags)
	}
	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for _, c := range conditions[1:] {
			query += " AND " + c
		}
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("skills.List: %w", err)
	}
	defer rows.Close()
	var out []SkillSummary
	for rows.Next() {
		var ss SkillSummary
		if err := rows.Scan(&ss.ID, &ss.Name, &ss.Description, &ss.Tags, &ss.CreatedAt, &ss.UpdatedAt); err != nil {
			return nil, fmt.Errorf("skills.List: %w", err)
		}
		if ss.Tags == nil {
			ss.Tags = []string{}
		}
		out = append(out, ss)
	}
	return out, rows.Err()
}

// Update applies a partial update.
func (s *Store) Update(ctx context.Context, id string, req UpdateRequest) (*Skill, error) {
	var (
		name, description, body string
		tags                    []string
		createdAt               time.Time
	)
	err := s.pool.QueryRow(ctx, `
		SELECT name, description, body, tags, created_at
		FROM skills WHERE id = $1
		FOR UPDATE
	`, id).Scan(&name, &description, &body, &tags, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("skills.Update: %w", err)
	}

	if req.Name != nil {
		name = *req.Name
	}
	if req.Description != nil {
		description = *req.Description
	}
	if req.Body != nil {
		body = *req.Body
	}
	if req.Tags != nil {
		tags = *req.Tags
	}
	if tags == nil {
		tags = []string{}
	}

	now := time.Now().UTC()
	if _, err := s.pool.Exec(ctx,
		`UPDATE skills SET name=$1, description=$2, body=$3, tags=$4, updated_at=$5 WHERE id=$6`,
		name, description, body, tags, now, id,
	); err != nil {
		return nil, fmt.Errorf("skills.Update: %w", err)
	}

	return &Skill{
		ID:          id,
		Name:        name,
		Description: description,
		Body:        body,
		Tags:        tags,
		CreatedAt:   createdAt,
		UpdatedAt:   now,
	}, nil
}

// Delete removes a skill. Returns ErrNotFound if no row was removed.
func (s *Store) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("skills.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// escapeILIKE escapes LIKE/ILIKE metacharacters (\, %, _) so user-supplied
// search terms match literally instead of acting as wildcards. The value is
// still bound as a query parameter, so this is about wildcard semantics, not
// injection safety. PostgreSQL's default LIKE escape character is backslash.
func escapeILIKE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

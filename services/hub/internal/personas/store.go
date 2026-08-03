package personas

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a persona or version doesn't exist.
var ErrNotFound = errors.New("persona not found")

// ErrNameTaken is returned when a persona name collides.
var ErrNameTaken = errors.New("persona name already in use")

// Store is the persona database layer.
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
	Text        *string
}

// Create inserts a new persona along with its initial version (1) in a
// single transaction. Populates p.CurrentVersion, p.CreatedAt, p.UpdatedAt.
func (s *Store) Create(ctx context.Context, p *Persona) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	// Insert persona with placeholder current_version_id=0; bound to the
	// real version id before commit (deferrable FK lets the row exist
	// with a dangling reference until COMMIT).
	if _, err := tx.Exec(ctx,
		`INSERT INTO personas (id, name, description, current_version_id, created_at, updated_at)
		 VALUES ($1, $2, $3, 0, $4, $4)`,
		p.ID, p.Name, p.Description, now,
	); err != nil {
		if isUniqueViolation(err) {
			return ErrNameTaken
		}
		return fmt.Errorf("insert persona: %w", err)
	}

	var versionID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO persona_versions (persona_id, version_number, text, created_at)
		 VALUES ($1, 1, $2, $3)
		 RETURNING id`,
		p.ID, p.Text, now,
	).Scan(&versionID); err != nil {
		return fmt.Errorf("insert version: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE personas SET current_version_id = $1 WHERE id = $2`,
		versionID, p.ID,
	); err != nil {
		return fmt.Errorf("update current_version_id: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	p.CurrentVersion = 1
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

// Get returns a persona by id with its current text.
func (s *Store) Get(ctx context.Context, id string) (*Persona, error) {
	var p Persona
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, p.name, p.description, pv.version_number, pv.text, p.created_at, p.updated_at
		FROM personas p
		JOIN persona_versions pv ON pv.id = p.current_version_id
		WHERE p.id = $1
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.CurrentVersion, &p.Text, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("personas.Get: %w", err)
	}
	return &p, nil
}

// List returns all personas as summaries (no text), newest first.
func (s *Store) List(ctx context.Context) ([]PersonaSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.name, p.description, pv.version_number, p.created_at, p.updated_at
		FROM personas p
		JOIN persona_versions pv ON pv.id = p.current_version_id
		ORDER BY p.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("personas.List: %w", err)
	}
	defer rows.Close()
	var out []PersonaSummary
	for rows.Next() {
		var ps PersonaSummary
		if err := rows.Scan(&ps.ID, &ps.Name, &ps.Description, &ps.CurrentVersion, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
			return nil, fmt.Errorf("personas.List: %w", err)
		}
		out = append(out, ps)
	}
	return out, rows.Err()
}

// Update applies a partial update. If req.Text differs from the current
// text, a new version row is inserted and current_version_id is bumped.
// If the persona doesn't exist, returns ErrNotFound.
func (s *Store) Update(ctx context.Context, id string, req UpdateRequest) (*Persona, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("personas.Update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		name, description, currentText string
		currentVersion                 int
		createdAt                      time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT p.name, p.description, pv.text, pv.version_number, p.created_at
		FROM personas p
		JOIN persona_versions pv ON pv.id = p.current_version_id
		WHERE p.id = $1
		FOR UPDATE
	`, id).Scan(&name, &description, &currentText, &currentVersion, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("personas.Update: %w", err)
	}

	if req.Name != nil {
		name = *req.Name
	}
	if req.Description != nil {
		description = *req.Description
	}
	newText := currentText
	textChanged := false
	if req.Text != nil && *req.Text != currentText {
		newText = *req.Text
		textChanged = true
	}

	now := time.Now().UTC()
	if textChanged {
		var newVersionID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO persona_versions (persona_id, version_number, text, created_at)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id`,
			id, currentVersion+1, newText, now,
		).Scan(&newVersionID); err != nil {
			return nil, fmt.Errorf("insert version: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE personas SET name=$1, description=$2, current_version_id=$3, updated_at=$4 WHERE id=$5`,
			name, description, newVersionID, now, id,
		); err != nil {
			if isUniqueViolation(err) {
				return nil, ErrNameTaken
			}
			return nil, fmt.Errorf("personas.Update: %w", err)
		}
		currentVersion++
	} else {
		if _, err := tx.Exec(ctx,
			`UPDATE personas SET name=$1, description=$2, updated_at=$3 WHERE id=$4`,
			name, description, now, id,
		); err != nil {
			if isUniqueViolation(err) {
				return nil, ErrNameTaken
			}
			return nil, fmt.Errorf("personas.Update: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("personas.Update: %w", err)
	}

	return &Persona{
		ID:             id,
		Name:           name,
		Description:    description,
		CurrentVersion: currentVersion,
		Text:           newText,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
	}, nil
}

// ListVersions returns metadata for every version of a persona, newest first.
// Returns an empty slice if the persona has no versions (or doesn't exist).
func (s *Store) ListVersions(ctx context.Context, personaID string) ([]VersionSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT persona_id, version_number, created_at
		FROM persona_versions
		WHERE persona_id = $1
		ORDER BY version_number DESC
	`, personaID)
	if err != nil {
		return nil, fmt.Errorf("personas.ListVersions: %w", err)
	}
	defer rows.Close()
	var out []VersionSummary
	for rows.Next() {
		var v VersionSummary
		if err := rows.Scan(&v.PersonaID, &v.VersionNumber, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("personas.ListVersions: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetVersion returns one specific version with its text.
func (s *Store) GetVersion(ctx context.Context, personaID string, n int) (*Version, error) {
	var v Version
	err := s.pool.QueryRow(ctx, `
		SELECT persona_id, version_number, text, created_at
		FROM persona_versions
		WHERE persona_id = $1 AND version_number = $2
	`, personaID, n).Scan(&v.PersonaID, &v.VersionNumber, &v.Text, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("personas.GetVersion: %w", err)
	}
	return &v, nil
}

// Rollback creates a new version whose text is copied from the version
// at versionNumber, and sets current_version_id to it.
func (s *Store) Rollback(ctx context.Context, personaID string, versionNumber int) (*Persona, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("personas.Rollback: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sourceText string
	if err := tx.QueryRow(ctx,
		`SELECT text FROM persona_versions WHERE persona_id=$1 AND version_number=$2`,
		personaID, versionNumber,
	).Scan(&sourceText); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("personas.Rollback: %w", err)
	}

	var currentVersion int
	if err := tx.QueryRow(ctx,
		`SELECT pv.version_number
		 FROM personas p JOIN persona_versions pv ON pv.id = p.current_version_id
		 WHERE p.id = $1
		 FOR UPDATE`,
		personaID,
	).Scan(&currentVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("personas.Rollback: %w", err)
	}

	now := time.Now().UTC()
	var newVersionID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO persona_versions (persona_id, version_number, text, created_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		personaID, currentVersion+1, sourceText, now,
	).Scan(&newVersionID); err != nil {
		return nil, fmt.Errorf("personas.Rollback: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE personas SET current_version_id=$1, updated_at=$2 WHERE id=$3`,
		newVersionID, now, personaID,
	); err != nil {
		return nil, fmt.Errorf("personas.Rollback: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("personas.Rollback: %w", err)
	}

	return s.Get(ctx, personaID)
}

// Delete removes a persona. Cascades to its versions via the
// ON DELETE CASCADE on the persona_versions foreign key.
// Returns ErrNotFound if no row was removed.
func (s *Store) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM personas WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("personas.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

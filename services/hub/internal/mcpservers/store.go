package mcpservers

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("mcp server not found")
var ErrAlreadyExists = errors.New("mcp server with this name already exists")

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const selectColumns = `name, display_name, description, url, token_from_env, managed_by, created_at, updated_at`

func (s *Store) Create(ctx context.Context, m *MCPServer) error {
	managedBy := m.ManagedBy
	if managedBy == "" {
		managedBy = "user"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mcp_servers (name, display_name, description, url, token_from_env, managed_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, m.Name, m.DisplayName, m.Description, m.URL, m.TokenFromEnv, managedBy)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrAlreadyExists
		}
		return fmt.Errorf("insert: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, name string) (*MCPServer, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+selectColumns+` FROM mcp_servers WHERE name = $1`, name)
	m, err := scanRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mcpservers.Get: %w", err)
	}
	return m, nil
}

func (s *Store) List(ctx context.Context) ([]*MCPServer, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+selectColumns+` FROM mcp_servers ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("mcpservers.List: %w", err)
	}
	defer rows.Close()
	var out []*MCPServer
	for rows.Next() {
		m, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("mcpservers.List: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Update(ctx context.Context, m *MCPServer) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE mcp_servers SET
			display_name   = $2,
			description    = $3,
			url            = $4,
			token_from_env = $5,
			updated_at     = now()
		WHERE name = $1
	`, m.Name, m.DisplayName, m.Description, m.URL, m.TokenFromEnv)
	// managed_by is intentionally NOT mutated here — that's only changed via Upsert.
	if err != nil {
		return fmt.Errorf("mcpservers.Update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, name string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("mcpservers.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

// Upsert inserts or fully replaces the row by name. Used by the seed loader.
func (s *Store) Upsert(ctx context.Context, m *MCPServer) error {
	managedBy := m.ManagedBy
	if managedBy == "" {
		managedBy = "user"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mcp_servers (name, display_name, description, url, token_from_env, managed_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (name) DO UPDATE SET
			display_name   = EXCLUDED.display_name,
			description    = EXCLUDED.description,
			url            = EXCLUDED.url,
			token_from_env = EXCLUDED.token_from_env,
			managed_by     = EXCLUDED.managed_by,
			updated_at     = now()
	`, m.Name, m.DisplayName, m.Description, m.URL, m.TokenFromEnv, managedBy)
	if err != nil {
		return fmt.Errorf("mcpservers.Upsert: %w", err)
	}
	return nil
}

func scanRow(s scanner) (*MCPServer, error) {
	m := &MCPServer{}
	if err := s.Scan(&m.Name, &m.DisplayName, &m.Description, &m.URL, &m.TokenFromEnv, &m.ManagedBy, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	return m, nil
}

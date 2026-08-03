package db

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate runs all up migrations against the given DSN. It is idempotent —
// re-running with no new files is a no-op. Safe to call at every startup.
func Migrate(ctx context.Context, dsn string) error {
	if !looksLikeURL(dsn) {
		return fmt.Errorf("migrate: DSN must be in URL form (postgres://...), got %q", dsn)
	}
	src, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("migrations source: %w", err)
	}
	// The pgx/v5 driver registers itself under "pgx5"; use that scheme.
	pgxDSN := "pgx5://" + trimScheme(dsn)
	m, err := migrate.NewWithSourceInstance("iofs", src, pgxDSN)
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// trimScheme strips an existing "postgres://" or "postgresql://" prefix so we
// can replace it with "pgx5://" for golang-migrate's driver name.
func trimScheme(dsn string) string {
	for _, prefix := range []string{"postgres://", "postgresql://", "pgx5://"} {
		if len(dsn) >= len(prefix) && dsn[:len(prefix)] == prefix {
			return dsn[len(prefix):]
		}
	}
	return dsn
}

// looksLikeURL returns true if dsn starts with a recognized scheme prefix.
// We require URL-form DSNs because trimScheme (and golang-migrate's source
// driver detection) only handles those.
func looksLikeURL(dsn string) bool {
	for _, prefix := range []string{"postgres://", "postgresql://", "pgx5://"} {
		if strings.HasPrefix(dsn, prefix) {
			return true
		}
	}
	return false
}

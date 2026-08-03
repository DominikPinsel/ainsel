package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
)

func TestMigrateUpCreatesMcpServersTable(t *testing.T) {
	dsn, stop := startPostgres(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.Migrate(ctx, dsn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM mcp_servers`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("expected empty table, got %d rows", n)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	dsn, stop := startPostgres(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.Migrate(ctx, dsn); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := db.Migrate(ctx, dsn); err != nil {
		t.Fatalf("second Migrate (should be no-op): %v", err)
	}
}

func TestMigrateRejectsKeyValueDSN(t *testing.T) {
	err := db.Migrate(context.Background(), "host=localhost user=test")
	if err == nil || !strings.Contains(err.Error(), "URL form") {
		t.Errorf("expected URL-form rejection error, got: %v", err)
	}
}

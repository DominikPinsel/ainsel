// Package db owns the Postgres connection pool used by hub-backend.
// It is intentionally small: future hub-DB features import this package
// for a *pgxpool.Pool instead of opening their own connections.
package db

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Environment variables for pool sizing.
const (
	// EnvMaxConns overrides the maximum number of connections in the pool.
	EnvMaxConns = "HUB_DB_MAX_CONNS"
	// EnvMinConns overrides the minimum number of idle connections kept open.
	EnvMinConns = "HUB_DB_MIN_CONNS"

	defaultMaxConns = 10
	defaultMinConns = 1
)

// Open returns a connection pool against the DSN.
//
// Pool sizing: the event-queue long-poll (eventqueue.Store.WaitForTask)
// holds ONE dedicated connection per idle agent (LISTEN agent_tasks) for
// the whole poll window. MaxConns must therefore exceed the number of
// concurrently idle agent replicas while still leaving headroom for API,
// ingest, and background queries — otherwise those queries starve waiting
// for a free connection and requests time out. Size via HUB_DB_MAX_CONNS /
// HUB_DB_MIN_CONNS when running more than a handful of agents.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = envInt(EnvMaxConns, defaultMaxConns)
	cfg.MinConns = envInt(EnvMinConns, defaultMinConns)
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new pool: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// envInt returns the value of env parsed as a positive integer, falling
// back to def when env is unset, empty, unparsable, or less than 1.
func envInt(env string, def int32) int32 {
	v := os.Getenv(env)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return int32(n)
}

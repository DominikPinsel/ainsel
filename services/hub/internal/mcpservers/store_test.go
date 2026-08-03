package mcpservers_test

import (
	"context"
	"errors"
	"testing"

	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/mcpservers"
)

func setup(t *testing.T) *mcpservers.Store {
	t.Helper()
	ctx := context.Background()
	c, err := pgcontainer.Run(ctx, "postgres:17-alpine",
		pgcontainer.WithDatabase("test"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		pgcontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	dsn, _ := c.ConnectionString(ctx, "sslmode=disable")
	if err := db.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
	return mcpservers.NewStore(pool)
}

func TestStoreCreateThenGet(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	in := &mcpservers.MCPServer{
		Name:         "web-tools",
		DisplayName:  "Web Tools",
		URL:          "https://mcp.example.com/sse",
		TokenFromEnv: "WEB_TOOLS_TOKEN", // #nosec G101 -- test data, env var name not a credential
	}
	if err := s.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(ctx, "web-tools")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.URL != "https://mcp.example.com/sse" {
		t.Errorf("URL: %s", got.URL)
	}
	if got.TokenFromEnv != "WEB_TOOLS_TOKEN" {
		t.Errorf("TokenFromEnv: %s", got.TokenFromEnv)
	}
}

func TestStoreDuplicateName(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	m := &mcpservers.MCPServer{Name: "dup", DisplayName: "Dup", URL: "http://x"}
	if err := s.Create(ctx, m); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	err := s.Create(ctx, m)
	if !errors.Is(err, mcpservers.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestStoreGetNotFound(t *testing.T) {
	s := setup(t)
	_, err := s.Get(context.Background(), "missing")
	if !errors.Is(err, mcpservers.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreUpdate(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	_ = s.Create(ctx, &mcpservers.MCPServer{Name: "a", DisplayName: "A", URL: "http://old"})
	if err := s.Update(ctx, &mcpservers.MCPServer{Name: "a", DisplayName: "A2", URL: "http://new", TokenFromEnv: "FOO_TOKEN"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get(ctx, "a")
	if got.URL != "http://new" {
		t.Errorf("URL after update: %s", got.URL)
	}
}

func TestStoreDelete(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	_ = s.Create(ctx, &mcpservers.MCPServer{Name: "del", DisplayName: "Del", URL: "http://x"})
	if err := s.Delete(ctx, "del"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := s.Get(ctx, "del")
	if !errors.Is(err, mcpservers.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

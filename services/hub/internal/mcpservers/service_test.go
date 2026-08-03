package mcpservers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/mcpservers"
)

func TestServiceCreateAndGet(t *testing.T) {
	st := setup(t)
	svc := mcpservers.NewService(st)

	in := &mcpservers.MCPServer{Name: "s1", DisplayName: "S1", URL: "http://s1"}
	if err := svc.Create(context.Background(), in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := svc.Get(context.Background(), "s1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.URL != "http://s1" {
		t.Errorf("URL: %s", got.URL)
	}
}

func TestServiceDeleteNotFound(t *testing.T) {
	st := setup(t)
	svc := mcpservers.NewService(st)
	err := svc.Delete(context.Background(), "ghost")
	if !errors.Is(err, mcpservers.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestServiceDeleteRefusesHelmManaged(t *testing.T) {
	st := setup(t)
	svc := mcpservers.NewService(st)
	ctx := context.Background()
	_ = svc.Upsert(ctx, &mcpservers.MCPServer{Name: "example-mcp", DisplayName: "Example MCP", URL: "http://example-mcp", ManagedBy: "helm"})
	err := svc.Delete(ctx, "example-mcp")
	if !errors.Is(err, mcpservers.ErrManaged) {
		t.Errorf("expected ErrManaged, got %v", err)
	}
}

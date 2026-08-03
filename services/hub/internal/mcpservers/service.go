package mcpservers

import (
	"context"
	"errors"
)

// ErrManaged is returned by Delete when the row is owned by helm.
var ErrManaged = errors.New("mcp server is managed and cannot be deleted")

// Service wraps Store with business logic.
type Service struct {
	store *Store
}

func NewService(s *Store) *Service {
	return &Service{store: s}
}

func (s *Service) Create(ctx context.Context, m *MCPServer) error {
	return s.store.Create(ctx, m)
}

func (s *Service) Get(ctx context.Context, name string) (*MCPServer, error) {
	return s.store.Get(ctx, name)
}

func (s *Service) List(ctx context.Context) ([]*MCPServer, error) {
	return s.store.List(ctx)
}

func (s *Service) Update(ctx context.Context, m *MCPServer) error {
	return s.store.Update(ctx, m)
}

// Delete removes a row. Returns ErrManaged when the row is owned by helm.
func (s *Service) Delete(ctx context.Context, name string) error {
	row, err := s.store.Get(ctx, name)
	if err != nil {
		return err
	}
	if row.ManagedBy != "user" {
		return ErrManaged
	}
	return s.store.Delete(ctx, name)
}

// Upsert is used by the seed loader.
func (s *Service) Upsert(ctx context.Context, m *MCPServer) error {
	return s.store.Upsert(ctx, m)
}

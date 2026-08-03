package usertokens_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/usertokens"
)

func TestCreate(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	tok, plaintext, err := store.Create(ctx, "user1", "my laptop", time.Now().Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tok.ID == "" {
		t.Error("expected ID")
	}
	if tok.UserID != "user1" {
		t.Errorf("UserID: got %q", tok.UserID)
	}
	if tok.Name != "my laptop" {
		t.Errorf("Name: got %q", tok.Name)
	}
	if tok.RevokedAt != nil {
		t.Error("expected nil RevokedAt")
	}
	if len(plaintext) < 10 {
		t.Errorf("plaintext too short: %q", plaintext)
	}
	if plaintext[:7] != "ainsel_" {
		t.Errorf("plaintext must start with ainsel_: %q", plaintext)
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	exp := time.Now().Add(30 * 24 * time.Hour)
	if _, _, err := store.Create(ctx, "user1", "tok-a", exp); err != nil {
		t.Fatalf("Create tok-a: %v", err)
	}
	if _, _, err := store.Create(ctx, "user1", "tok-b", exp); err != nil {
		t.Fatalf("Create tok-b: %v", err)
	}
	if _, _, err := store.Create(ctx, "user2", "tok-c", exp); err != nil {
		t.Fatalf("Create tok-c: %v", err)
	}

	toks, err := store.List(ctx, "user1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(toks) != 2 {
		t.Fatalf("expected 2 tokens for user1, got %d", len(toks))
	}
}

func TestRevoke(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	tok, _, _ := store.Create(ctx, "user1", "tok", time.Now().Add(30*24*time.Hour))

	if err := store.Revoke(ctx, tok.ID, "user1"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// second revoke returns not found
	err := store.Revoke(ctx, tok.ID, "user1")
	if !errors.Is(err, usertokens.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRevokeWrongUser(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	tok, _, _ := store.Create(ctx, "user1", "tok", time.Now().Add(30*24*time.Hour))
	err := store.Revoke(ctx, tok.ID, "user2")
	if !errors.Is(err, usertokens.ErrNotFound) {
		t.Errorf("expected ErrNotFound for wrong user, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, plaintext, _ := store.Create(ctx, "user1", "tok", time.Now().Add(30*24*time.Hour))

	got, err := store.Validate(ctx, plaintext)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.UserID != "user1" {
		t.Errorf("UserID: got %q", got.UserID)
	}
	if got.LastUsedAt == nil {
		t.Error("expected LastUsedAt to be set")
	}
}

func TestValidateExpired(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, plaintext, _ := store.Create(ctx, "user1", "tok", time.Now().Add(-1*time.Second))

	_, err := store.Validate(ctx, plaintext)
	if !errors.Is(err, usertokens.ErrNotFound) {
		t.Errorf("expected ErrNotFound for expired token, got %v", err)
	}
}

func TestValidateRevoked(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	tok, plaintext, err := store.Create(ctx, "user1", "tok", time.Now().Add(30*24*time.Hour))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Revoke(ctx, tok.ID, "user1"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	_, err = store.Validate(ctx, plaintext)
	if !errors.Is(err, usertokens.ErrNotFound) {
		t.Errorf("expected ErrNotFound for revoked token, got %v", err)
	}
}

func TestValidateUnknown(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, err := store.Validate(ctx, "ainsel_notarealtoken")
	if !errors.Is(err, usertokens.ErrNotFound) {
		t.Errorf("expected ErrNotFound for unknown token, got %v", err)
	}
}

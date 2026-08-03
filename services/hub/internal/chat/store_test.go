package chat_test

import (
	"context"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/chat"
)

// mustCreate creates a session and fails the test on error.
func mustCreate(t *testing.T, s *chat.Store, agent, user string) *chat.Session {
	t.Helper()
	sess, err := s.CreateSession(context.Background(), agent, user)
	if err != nil {
		t.Fatalf("CreateSession(%s, %s): %v", agent, user, err)
	}
	return sess
}

// mustAddMessage adds a message and fails the test on error.
func mustAddMessage(t *testing.T, s *chat.Store, sessionID, role, content string, tokens int) *chat.Message {
	t.Helper()
	msg, err := s.AddMessage(context.Background(), sessionID, role, content, tokens)
	if err != nil {
		t.Fatalf("AddMessage(%s, %s): %v", sessionID, role, err)
	}
	return msg
}

func TestStoreCreateAndGetSession(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	sess, err := store.CreateSession(ctx, "developer", "user-123")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.Name != sess.ID {
		t.Fatalf("expected Name to default to ID %q, got %q", sess.ID, sess.Name)
	}
	if sess.AgentName != "developer" || sess.UserID != "user-123" {
		t.Fatalf("unexpected session fields: %+v", sess)
	}

	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ID != sess.ID || got.AgentName != "developer" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Name != sess.ID {
		t.Fatalf("expected Name to be %q after round-trip, got %q", sess.ID, got.Name)
	}
	if len(got.Messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(got.Messages))
	}
}

func TestStoreGetSessionNotFound(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, err := store.GetSession(ctx, "missing")
	if err != chat.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestStoreListSessionsByAgent(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	mustCreate(t, store, "developer", "user-1")
	mustCreate(t, store, "reviewer", "user-1")
	mustCreate(t, store, "developer", "user-2")

	sessions, err := store.ListSessions(ctx, chat.ListSessionsOptions{AgentName: "developer"})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 developer sessions, got %d", len(sessions))
	}
	for _, s := range sessions {
		if s.AgentName != "developer" {
			t.Fatalf("expected all developer, got %s", s.AgentName)
		}
		if s.Name != s.ID {
			t.Fatalf("expected Name to default to ID, got %q for %q", s.Name, s.ID)
		}
	}
}

func TestStoreListSessionsByUser(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	mustCreate(t, store, "developer", "user-1")
	mustCreate(t, store, "reviewer", "user-1")
	mustCreate(t, store, "developer", "user-2")

	sessions, err := store.ListSessions(ctx, chat.ListSessionsOptions{UserID: "user-1"})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions for user-1, got %d", len(sessions))
	}
}

func TestStoreUpdateSessionName(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	sess, err := store.CreateSession(ctx, "developer", "user-1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.Name != sess.ID {
		t.Fatalf("expected initial Name == ID, got %q", sess.Name)
	}

	// Rename the session.
	newName := "My Chat About Go"
	if err := store.UpdateSessionName(ctx, sess.ID, newName); err != nil {
		t.Fatalf("UpdateSessionName: %v", err)
	}

	// Verify via GetSession.
	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Name != newName {
		t.Fatalf("expected Name %q after update, got %q", newName, got.Name)
	}

	// Verify via ListSessions.
	sessions, err := store.ListSessions(ctx, chat.ListSessionsOptions{AgentName: "developer"})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Name != newName {
		t.Fatalf("expected Name %q in list, got %q", newName, sessions[0].Name)
	}
}

func TestStoreUpdateSessionNameNotFound(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	err := store.UpdateSessionName(ctx, "missing", "New Name")
	if err != chat.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestStoreAddMessageAndGetHistory(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	sess, err := store.CreateSession(ctx, "developer", "user-1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	msg1, err := store.AddMessage(ctx, sess.ID, chat.RoleUser, "Hello", 0)
	if err != nil {
		t.Fatalf("AddMessage user: %v", err)
	}
	msg2, err := store.AddMessage(ctx, sess.ID, chat.RoleAssistant, "Hi there!", 42)
	if err != nil {
		t.Fatalf("AddMessage assistant: %v", err)
	}

	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got.Messages))
	}
	if got.Messages[0].Content != "Hello" || got.Messages[0].Role != chat.RoleUser {
		t.Fatalf("unexpected first message: %+v", got.Messages[0])
	}
	if got.Messages[1].Content != "Hi there!" || got.Messages[1].Role != chat.RoleAssistant {
		t.Fatalf("unexpected second message: %+v", got.Messages[1])
	}
	if got.Messages[1].Tokens != 42 {
		t.Fatalf("expected 42 tokens, got %d", got.Messages[1].Tokens)
	}
	// Messages should be ordered oldest first.
	if !got.Messages[0].CreatedAt.Before(got.Messages[1].CreatedAt) {
		t.Fatal("expected messages ordered oldest-first")
	}
	_ = msg1
	_ = msg2
}

func TestStoreAddMessageUpdatesSessionTimestamp(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	sess, _ := store.CreateSession(ctx, "developer", "user-1")
	originalUpdated := sess.UpdatedAt

	mustAddMessage(t, store, sess.ID, chat.RoleUser, "test", 0)

	got, _ := store.GetSession(ctx, sess.ID)
	if !got.UpdatedAt.After(originalUpdated) {
		t.Fatalf("expected updated_at to advance; was %v, now %v", originalUpdated, got.UpdatedAt)
	}
}

func TestStoreDeleteSession(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	sess, _ := store.CreateSession(ctx, "developer", "user-1")
	mustAddMessage(t, store, sess.ID, chat.RoleUser, "hello", 0)

	if err := store.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, err := store.GetSession(ctx, sess.ID)
	if err != chat.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound after delete, got %v", err)
	}
}

func TestStoreDeleteSessionNotFound(t *testing.T) {
	ctx := context.Background()
	store, cleanup := newTestStore(t)
	defer cleanup()

	err := store.DeleteSession(ctx, "missing")
	if err != chat.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}
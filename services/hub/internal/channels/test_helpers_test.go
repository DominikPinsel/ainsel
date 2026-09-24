package channels

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
)

// The channel registry is thin SQL over tables the migrations own, so these
// tests run against a real Postgres. TEST_DB_URL points at one; without it the
// tests skip, matching the convention the queue and trigger stores use.

var (
	testPoolOnce  sync.Once
	testPoolValue *pgxpool.Pool
	testPoolErr   error
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DB_URL")
	if dbURL == "" {
		t.Skip("TEST_DB_URL not set, skipping integration test")
	}
	testPoolOnce.Do(func() {
		ctx := context.Background()
		if err := db.Migrate(ctx, dbURL); err != nil {
			testPoolErr = err
			return
		}
		pool, err := db.Open(ctx, dbURL)
		if err != nil {
			testPoolErr = err
			return
		}
		testPoolValue = pool
	})
	if testPoolErr != nil {
		t.Fatalf("test db: %v", testPoolErr)
	}
	cleanChannels(t)
	return testPoolValue
}

// cleanChannels removes the rows these tests create. Everything they insert is
// prefixed "ch-test-", so the sweep cannot touch another package's fixtures.
func cleanChannels(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	pool := testPoolValue
	for _, stmt := range []string{
		`DELETE FROM channel_bridges`,
		`DELETE FROM agent_tasks WHERE event_id LIKE 'ch-test-%'`,
		`DELETE FROM events WHERE id LIKE 'ch-test-%'`,
		`DELETE FROM channels WHERE name LIKE 'ch-test-%'`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("clean: %v", err)
		}
	}
}

var nameSeq int

// uniqueName builds a channel name that is distinct per call, so tests can run
// in any order without colliding on the (kind, entity_ref) key.
func uniqueName(prefix string) string {
	nameSeq++
	return "ch-test-" + prefix + "-" + time.Now().UTC().Format("150405") + "-" + itoa(nameSeq)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// stores returns the channel store and the event queue over the test database.
func stores(t *testing.T) (*Store, *eventqueue.Store) {
	t.Helper()
	pool := testPool(t)
	return NewStore(pool), eventqueue.NewStore(pool)
}

// seed provisions a channel of the given kind for ref and returns its id.
func seed(t *testing.T, s *Store, kind Kind, ref string) string {
	t.Helper()
	ch, err := s.Ensure(context.Background(), kind, ref, ref, DefaultDescription(kind, ref))
	if err != nil {
		t.Fatalf("seed %s/%s: %v", kind, ref, err)
	}
	return ch.ID
}

// seedCustom creates a user-authored channel and returns its id.
func seedCustom(t *testing.T, s *Store) string {
	t.Helper()
	name := uniqueName("custom")
	ch, err := s.Create(context.Background(), name, "grouping channel")
	if err != nil {
		t.Fatalf("seed custom: %v", err)
	}
	return ch.ID
}

// insertEvent stores an event born in channelID ("" leaves it unstamped).
func insertEvent(t *testing.T, eq *eventqueue.Store, id, connector, channelID string) {
	t.Helper()
	ctx := context.Background()
	if err := eq.InsertEvent(ctx, eventqueue.Event{
		ID:        id,
		Connector: connector,
		ChannelID: channelID,
		Headers:   json.RawMessage(`{"type":"test.event"}`),
		Data:      json.RawMessage(`{"n":1}`),
		Raw:       `{"n":1}`,
	}); err != nil {
		t.Fatalf("insert event %s: %v", id, err)
	}
}

// insertTask records a delivery of an event to an agent.
func insertTask(t *testing.T, eq *eventqueue.Store, eventID, agentName string) {
	t.Helper()
	ctx := context.Background()
	if err := eq.EnqueueTask(ctx, eventqueue.Task{
		EventID:     eventID,
		AgentName:   agentName,
		TriggerName: "ch-test-trigger",
		Headers:     json.RawMessage(`{}`),
		Payload:     json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("insert task %s/%s: %v", eventID, agentName, err)
	}
}

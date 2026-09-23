package channels

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

// fakeRegistry stands in for the Kubernetes connector and agent registries.
type fakeRegistry struct {
	agents     []Entity
	connectors []Entity
	err        error
}

func (f fakeRegistry) ListAgents(ctx context.Context) ([]Entity, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.agents, nil
}

func (f fakeRegistry) ListConnectors(ctx context.Context) ([]Entity, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.connectors, nil
}

func readEventChannel(t *testing.T, eq *eventqueue.Store, id string) (string, bool) {
	t.Helper()
	ctx := context.Background()
	var channelID *string
	err := eq.Pool().QueryRow(ctx, `SELECT channel_id FROM events WHERE id = $1`, id).Scan(&channelID)
	if err != nil {
		t.Fatalf("read event %s: %v", id, err)
	}
	if channelID == nil {
		return "", false
	}
	return *channelID, true
}

func TestSyncProvisionsFromRegistriesAndMarksOrphans(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()

	connectorRef := uniqueName("sync-conn")
	agentRef := uniqueName("sync-agent")
	rec := NewReconciler(s, eq, fakeRegistry{
		connectors: []Entity{{Ref: connectorRef, Name: "Forgejo", Description: "Issues and merges"}},
		agents:     []Entity{{Ref: agentRef, Name: agentRef}},
	})

	res, err := rec.Sync(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Provisioned < 2 {
		t.Fatalf("expected both entities provisioned, got %+v", res)
	}

	ch, err := s.GetByEntity(ctx, KindConnector, connectorRef)
	if err != nil {
		t.Fatalf("get connector channel: %v", err)
	}
	// A CR's own description wins over the kind's default, so the channel reads
	// like the thing it came from.
	if ch.Description != "Issues and merges" {
		t.Errorf("description = %q, want the CR's", ch.Description)
	}
	if ch.Name != "Forgejo" {
		t.Errorf("name = %q, want the CR's display name", ch.Name)
	}
	if ch.Orphaned {
		t.Error("a listed entity must not be orphaned")
	}
	// An agent with no description of its own gets the inbox wording.
	inbox, err := s.GetByEntity(ctx, KindAgent, agentRef)
	if err != nil {
		t.Fatalf("get agent channel: %v", err)
	}
	if inbox.Description == "" {
		t.Error("provisioned channel should carry a default description")
	}

	// Sync is idempotent: running it again provisions the same set and creates
	// no duplicates, which is what lets it sit on a ticker.
	before := countChannels(t, s)
	if _, err := rec.Sync(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if after := countChannels(t, s); after != before {
		t.Fatalf("sync created duplicates: %d → %d", before, after)
	}

	// Drop the connector from the registry: its channel survives (event history
	// points at it) but is flagged.
	rec2 := NewReconciler(s, eq, fakeRegistry{agents: []Entity{{Ref: agentRef, Name: agentRef}}})
	res, err = rec2.Sync(ctx)
	if err != nil {
		t.Fatalf("sync after deletion: %v", err)
	}
	if res.Orphaned == 0 {
		t.Fatal("expected the removed connector channel to be orphaned")
	}
	if ch, _ = s.GetByEntity(ctx, KindConnector, connectorRef); ch == nil || !ch.Orphaned {
		t.Fatalf("connector channel should survive as an orphan: %+v", ch)
	}
	if ch, _ := s.GetByEntity(ctx, KindAgent, agentRef); ch.Orphaned {
		t.Error("the still-listed agent channel must not be orphaned")
	}
}

func countChannels(t *testing.T, s *Store) int {
	t.Helper()
	rows, err := s.List(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return len(rows)
}

func TestSyncStampsHistoryFromLabels(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()

	connectorRef := uniqueName("stamp-conn")
	agentRef := uniqueName("stamp-agent")
	goneRef := uniqueName("stamp-gone")

	// History written before channels existed: two events from a live connector,
	// one from a connector that has since been deleted, and one cron tick that
	// was delivered to an agent.
	insertEvent(t, eq, "ch-test-stamp-1", connectorRef, "")
	insertEvent(t, eq, "ch-test-stamp-2", connectorRef, "")
	insertEvent(t, eq, "ch-test-stamp-3", goneRef, "")
	insertEvent(t, eq, "ch-test-stamp-4", ainselapishared.SourceCron, "")
	insertTask(t, eq, "ch-test-stamp-4", agentRef)

	// Unstamped sources are what the reconciler works from — including the label
	// no registry entry answers to any more.
	labels, err := eq.UnstampedSourceLabels(ctx)
	if err != nil {
		t.Fatalf("unstamped labels: %v", err)
	}
	if !contains(labels, connectorRef) || !contains(labels, goneRef) {
		t.Fatalf("unstamped source labels = %v", labels)
	}
	if !contains(labels, ainselapishared.SourceCron) {
		t.Fatalf("cron events are unstamped history too; the query should list the label, got %v", labels)
	}
	agents, err := eq.UnstampedInboxAgents(ctx, DirectSourceLabels)
	if err != nil {
		t.Fatalf("unstamped inbox agents: %v", err)
	}
	if !contains(agents, agentRef) {
		t.Fatalf("expected the cron recipient %q in %v", agentRef, agents)
	}

	rec := NewReconciler(s, eq, fakeRegistry{
		connectors: []Entity{{Ref: connectorRef, Name: connectorRef}},
		agents:     []Entity{{Ref: agentRef, Name: agentRef}},
	})
	res, err := rec.Sync(ctx)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.StampedEvents != 4 {
		t.Fatalf("stamped %d events, want 4", res.StampedEvents)
	}

	connChannel, err := s.GetByEntity(ctx, KindConnector, connectorRef)
	if err != nil {
		t.Fatalf("connector channel: %v", err)
	}
	for _, id := range []string{"ch-test-stamp-1", "ch-test-stamp-2"} {
		if got, ok := readEventChannel(t, eq, id); !ok || got != connChannel.ID {
			t.Errorf("event %s born in %q, want %s", id, got, connChannel.ID)
		}
	}
	// The deleted connector still owns its history: a channel was created from
	// the label and immediately orphaned.
	gone, err := s.GetByEntity(ctx, KindConnector, goneRef)
	if err != nil {
		t.Fatalf("channel for deleted connector: %v", err)
	}
	if !gone.Orphaned {
		t.Error("a channel created from history alone is not confirmed by the registry")
	}
	if got, ok := readEventChannel(t, eq, "ch-test-stamp-3"); !ok || got != gone.ID {
		t.Errorf("orphaned connector history not stamped: %q", got)
	}
	// The pseudo-sources are answered from deliveries, never from their label:
	// "cron" must not become a connector stream just because events carry it.
	if _, err := s.GetByEntity(ctx, KindConnector, ainselapishared.SourceCron); err != ErrNotFound {
		t.Errorf("cron got a connector channel: %v", err)
	}

	// The cron tick was born in its agent's inbox.
	inbox, err := s.GetByEntity(ctx, KindAgent, agentRef)
	if err != nil {
		t.Fatalf("agent channel: %v", err)
	}
	if got, ok := readEventChannel(t, eq, "ch-test-stamp-4"); !ok || got != inbox.ID {
		t.Errorf("cron event born in %q, want the inbox %s", got, inbox.ID)
	}

	// Stamping is one-shot: a second pass changes nothing, so a tick cannot
	// rewrite a channel that was later re-provisioned.
	again, err := rec.Sync(ctx)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if again.StampedEvents != 0 {
		t.Fatalf("second sync re-stamped %d events", again.StampedEvents)
	}
}

func TestSyncDoesNotStealAnotherChannelsEvents(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()

	// An event that already has a birth channel must survive stamping untouched
	// — it was routed by the live path, which knows better than this repair.
	ref := uniqueName("owned")
	ch := seed(t, s, KindConnector, ref)
	insertEvent(t, eq, "ch-test-owned-1", ref, ch)
	unowned := "ch-test-unowned"
	insertEvent(t, eq, unowned, ref, "")

	rec := NewReconciler(s, eq, fakeRegistry{connectors: []Entity{{Ref: ref, Name: ref}}})
	if _, err := rec.Sync(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}
	events, err := eq.QueryEvents(ctx, eventqueue.EventFilter{Connector: ref}, 10, 0)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected both events, got %d", len(events))
	}
	byID := map[string]eventqueue.Event{}
	for _, e := range events {
		byID[e.ID] = e
	}
	if byID["ch-test-owned-1"].ChannelID != ch {
		t.Error("an already-stamped event was rewritten")
	}
	if byID[unowned].ChannelID == "" {
		t.Error("the unstamped event was not adopted")
	}
}

func TestSyncReportsRegistryFailure(t *testing.T) {
	s, eq := stores(t)
	rec := NewReconciler(s, eq, fakeRegistry{err: context.DeadlineExceeded})
	if _, err := rec.Sync(context.Background()); err == nil {
		t.Fatal("a registry that cannot be read must fail the sync, not empty the registry")
	}
}

// TestIngestProvisioningPath covers the guarantee the ingest handler relies on:
// a connector label never seen before is provisioned on the spot, so an event is
// never stored without a home.
func TestIngestProvisioningPath(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()
	svc := NewService(s, eq, fakeTriggers{})

	label := uniqueName("ingest")
	id, err := svc.ConnectorChannelFor(ctx, label)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := eq.InsertEvent(ctx, eventqueue.Event{
		ID:        "ch-test-ingest-1",
		Connector: label,
		ChannelID: id,
		Headers:   json.RawMessage(`{"type":"x"}`),
		Data:      json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got, ok := readEventChannel(t, eq, "ch-test-ingest-1"); !ok || got != id {
		t.Fatalf("event not stamped: %q %v", got, ok)
	}
	// The event and its channel agree on which stream it belongs to.
	filtered, _, err := svc.Events(ctx, id, EventQuery{Since: time.Now().UTC().Add(-time.Hour)})
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "ch-test-ingest-1" {
		t.Fatalf("the event should be readable through its channel: %+v", filtered)
	}
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

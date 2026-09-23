package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/channels"
	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
)

// ingestEvent posts one event body through the internal ingest endpoint the
// webhook-receiver uses.
func ingestEvent(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/internal/events", strings.NewReader(body))
	req.Header.Set("X-Internal-Token", "test-secret")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestIngestStampsBirthChannel(t *testing.T) {
	s, cs, eq, _ := channelTestServer(t)
	s.internalValidateSecret = "test-secret"
	s.mux.HandleFunc("/api/internal/events", s.handleIngestEvent)

	label := "ingest-src"
	body := `{"id":"ingest-1","connector":"` + label + `","headers":{"type":"issue_open"},"data":{"n":1},"raw":"{}"}`
	rec := ingestEvent(t, s, body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("ingest: %d %s", rec.Code, rec.Body.String())
	}

	// The channel was provisioned on first sight: an event can never be stored
	// without a home, even before the reconcile loop has seen the connector.
	ch, err := cs.GetByEntity(context.Background(), channels.KindConnector, label)
	if err != nil {
		t.Fatalf("channel for %q: %v", label, err)
	}
	events, err := eq.QueryEvents(context.Background(), eventqueue.EventFilter{Connector: label}, 10, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("query: %v %d", err, len(events))
	}
	if events[0].ChannelID != ch.ID {
		t.Fatalf("event born in %q, want %q", events[0].ChannelID, ch.ID)
	}

	// A second event on the same stream reuses the channel rather than forking a
	// new one.
	if rec = ingestEvent(t, s, strings.Replace(body, "ingest-1", "ingest-2", 1)); rec.Code != http.StatusAccepted {
		t.Fatalf("second ingest: %d", rec.Code)
	}
	again, err := cs.GetByEntity(context.Background(), channels.KindConnector, label)
	if err != nil || again.ID != ch.ID {
		t.Fatalf("channel identity changed between ingests: %v %+v", err, again)
	}
}

func TestIngestIgnoresClientSuppliedChannel(t *testing.T) {
	s, cs, eq, _ := channelTestServer(t)
	s.internalValidateSecret = "test-secret"
	s.mux.HandleFunc("/api/internal/events", s.handleIngestEvent)

	victim := seedChannel(t, cs, channels.KindConnector, "victim-connector")

	// A publisher cannot choose which stream its events land in by naming one:
	// the birth channel follows the connector label, which is what the receiver
	// pod is authenticated to write.
	body := `{"id":"forge-1","connector":"attacker-connector","channelId":"` + victim + `","headers":{},"data":{},"raw":"{}"}`
	if rec := ingestEvent(t, s, body); rec.Code != http.StatusAccepted {
		t.Fatalf("ingest: %d %s", rec.Code, rec.Body.String())
	}
	events, err := eq.QueryEvents(context.Background(), eventqueue.EventFilter{Channel: victim}, 10, 0)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("a publisher wrote into someone else's channel: %+v", events)
	}
	mine, err := cs.GetByEntity(context.Background(), channels.KindConnector, "attacker-connector")
	if err != nil {
		t.Fatalf("attacker channel should have been provisioned: %v", err)
	}
	events, _ = eq.QueryEvents(context.Background(), eventqueue.EventFilter{Channel: mine.ID}, 10, 0)
	if len(events) != 1 || events[0].ID != "forge-1" {
		t.Fatalf("event not stamped with its own channel: %+v", events)
	}
}

// TestIngestStoresEventWhenRegistryFails keeps the failure mode honest: a broken
// channel registry must not turn into rejected webhooks, because the connector
// would retry an event it already accepted.
func TestIngestStoresEventWhenRegistryFails(t *testing.T) {
	s, _, eq, _ := channelTestServer(t)
	s.internalValidateSecret = "test-secret"
	s.mux.HandleFunc("/api/internal/events", s.handleIngestEvent)
	// Simulate the registry being unreachable by dropping the service.
	s.channelSvc = nil

	body := `{"id":"degraded-1","connector":"any-connector","headers":{},"data":{},"raw":"{}"}`
	if rec := ingestEvent(t, s, body); rec.Code != http.StatusAccepted {
		t.Fatalf("ingest with no channel service: %d %s", rec.Code, rec.Body.String())
	}
	events, err := eq.QueryEvents(context.Background(), eventqueue.EventFilter{Connector: "any-connector"}, 10, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("event should still be stored: %v %+v", err, events)
	}
	if events[0].ChannelID != "" {
		t.Errorf("an unresolvable stream leaves the event unstamped, got %q", events[0].ChannelID)
	}
	// The event is still readable as history; once the registry comes back the
	// reconciler adopts unstamped events by label (see TestSyncStampsHistory).
}

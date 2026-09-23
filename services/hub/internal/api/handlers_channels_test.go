package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/DominikPinsel/ainsel/services/hub/internal/channels"
	"github.com/DominikPinsel/ainsel/services/hub/internal/db"
	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
)

// channelTestServer boots a Postgres testcontainer, wires the channel registry
// over it, and returns the server plus the stores a test needs to seed. It skips
// when Docker is unavailable, matching the other integration suites.
func channelTestServer(t *testing.T) (*Server, *channels.Store, *eventqueue.Store, *triggers.Store) {
	t.Helper()
	ctx := context.Background()

	c, err := pgcontainer.Run(ctx, "postgres:17-alpine",
		pgcontainer.WithDatabase("ainsel_test"),
		pgcontainer.WithUsername("test"),
		pgcontainer.WithPassword("test"),
		pgcontainer.BasicWaitStrategies(),
	)
	if err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("connection string: %v", err)
	}
	if err := db.Migrate(ctx, dsn); err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		_ = c.Terminate(ctx)
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_ = c.Terminate(context.Background())
	})

	eq := eventqueue.NewStore(pool)
	ts := triggers.NewStore(pool)
	cs := channels.NewStore(pool)
	svc := channels.NewService(cs, eq, ts)

	s := testServer(t)
	s.SetEventQueue(eq)
	s.SetTriggerStore(ts)
	s.SetChannelService(svc, channels.NewTransfer(svc, eq, invocations.NewMemoryStore(100)))
	return s, cs, eq, ts
}

// doRawJSON issues a request with a literal body string, so a test can send a
// malformed payload the JSON encoder would never produce.
func doRawJSON(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// seedChannel provisions a channel of a kind through the store.
func seedChannel(t *testing.T, s *channels.Store, kind channels.Kind, ref string) string {
	t.Helper()
	ch, err := s.Ensure(context.Background(), kind, ref, ref, channels.DefaultDescription(kind, ref))
	if err != nil {
		t.Fatalf("seed %s/%s: %v", kind, ref, err)
	}
	return ch.ID
}

func TestChannelsAPI_ListShowsProvisionedChannels(t *testing.T) {
	s, cs, _, _ := channelTestServer(t)

	srcRef := "api-src"
	connector := seedChannel(t, cs, channels.KindConnector, srcRef)
	inbox := seedChannel(t, cs, channels.KindAgent, "api-agent")

	rec := doRawJSON(t, s, http.MethodGet, "/api/v1/channels", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items      []channels.View `json:"items"`
		Total      int             `json:"total"`
		Page       int             `json:"page"`
		PageSize   int             `json:"pageSize"`
		TotalPages int             `json:"totalPages"`
		Window     string          `json:"window"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("expected both channels, got %d / %d", resp.Total, len(resp.Items))
	}
	byID := map[string]channels.View{}
	for _, v := range resp.Items {
		byID[v.ID] = v
	}
	if byID[connector].Kind != channels.KindConnector {
		t.Errorf("connector channel kind = %q", byID[connector].Kind)
	}
	if byID[inbox].Kind != channels.KindAgent {
		t.Errorf("agent channel kind = %q", byID[inbox].Kind)
	}
	if resp.Window == "" {
		t.Error("the list must report the window its rates cover")
	}

	// The kind filter is answered in SQL, not by trimming the page.
	rec = doRawJSON(t, s, http.MethodGet, "/api/v1/channels?kind=connector", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	if resp.Total != 1 || resp.Items[0].ID != connector {
		t.Fatalf("kind filter: %+v", resp.Items)
	}

	rec = doRawJSON(t, s, http.MethodGet, "/api/v1/channels?kind=bogus", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid kind should be 400, got %d", rec.Code)
	}
}

func TestChannelsAPI_CreateUpdateDeleteCustomChannel(t *testing.T) {
	s, cs, _, _ := channelTestServer(t)

	rec := doRawJSON(t, s, http.MethodPost, "/api/v1/channels", `{"name":"Nightly bundle","description":"everything after dark"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created channels.Channel
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Kind != channels.KindCustom || created.EntityRef != "" {
		t.Fatalf("created channel: %+v", created)
	}

	rec = doRawJSON(t, s, http.MethodGet, "/api/v1/channels/"+created.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var detail channels.Detail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Name != "Nightly bundle" {
		t.Errorf("detail name = %q", detail.Name)
	}

	rec = doRawJSON(t, s, http.MethodPut, "/api/v1/channels/"+created.ID, `{"name":"Renamed bundle"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}

	// A provisioned channel is not editable through this API: its entity owns
	// the label.
	connector := seedChannel(t, cs, channels.KindConnector, "api-guarded")
	rec = doRawJSON(t, s, http.MethodPut, "/api/v1/channels/"+connector, `{"name":"hijack"}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("renaming a provisioned channel: got %d, want 409", rec.Code)
	}
	rec = doRawJSON(t, s, http.MethodDelete, "/api/v1/channels/"+connector, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("deleting a provisioned channel: got %d, want 409", rec.Code)
	}

	// Unknown ids are a 404, not an invented channel.
	if rec = doRawJSON(t, s, http.MethodGet, "/api/v1/channels/ch-does-not-exist", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown channel: got %d, want 404", rec.Code)
	}

	rec = doRawJSON(t, s, http.MethodDelete, "/api/v1/channels/"+created.ID, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec = doRawJSON(t, s, http.MethodGet, "/api/v1/channels/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("deleted channel still readable: %d", rec.Code)
	}
}

func TestChannelsAPI_BridgeLifecycle(t *testing.T) {
	s, cs, _, _ := channelTestServer(t)
	ctx := context.Background()

	connector := seedChannel(t, cs, channels.KindConnector, "api-bridge-src")
	inbox := seedChannel(t, cs, channels.KindAgent, "api-bridge-dst")
	group := mustCreateChannel(t, s, "bundle")

	// A bridge needs a custom endpoint on at least one side.
	rec := doRawJSON(t, s, http.MethodPost, "/api/v1/channels/"+connector+"/bridges", `{"to":"`+inbox+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("connector→agent bridge: got %d %s, want 409", rec.Code, rec.Body.String())
	}

	rec = doRawJSON(t, s, http.MethodPost, "/api/v1/channels/"+connector+"/bridges", `{"to":"`+group+`","name":"everything"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("attach: %d %s", rec.Code, rec.Body.String())
	}
	var bridge channels.Bridge
	if err := json.Unmarshal(rec.Body.Bytes(), &bridge); err != nil {
		t.Fatalf("decode bridge: %v", err)
	}
	if bridge.FromChannel != connector || bridge.ToChannel != group || bridge.Name != "everything" {
		t.Fatalf("bridge: %+v", bridge)
	}

	if _, err := s.channelSvc.Attach(ctx, group, inbox, ""); err != nil {
		t.Fatalf("attach inbox leg: %v", err)
	}

	// The detail view reports both directions, so the console can draw them.
	rec = doRawJSON(t, s, http.MethodGet, "/api/v1/channels/"+group, "")
	var detail channels.Detail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Incoming) != 1 || detail.Incoming[0].FromChannel != connector {
		t.Errorf("incoming edges: %+v", detail.Incoming)
	}
	if len(detail.Outgoing) != 1 || detail.Outgoing[0].ToChannel != inbox {
		t.Errorf("outgoing edges: %+v", detail.Outgoing)
	}

	// A channel with edges crossing it cannot be deleted.
	rec = doRawJSON(t, s, http.MethodDelete, "/api/v1/channels/"+group, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("delete in use: got %d, want 409", rec.Code)
	}

	// An edge may be detached from either of its endpoints — and only from one
	// of them: a channel that is not party to the bridge cannot delete it.
	wrongSide := doRawJSON(t, s, http.MethodDelete, "/api/v1/channels/"+inbox+"/bridges/"+bridge.ID, "")
	if wrongSide.Code != http.StatusNotFound {
		t.Fatalf("detach from a non-endpoint: got %d, want 404", wrongSide.Code)
	}
	if rec = doRawJSON(t, s, http.MethodDelete, "/api/v1/channels/"+group+"/bridges/"+bridge.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("detach from the far endpoint: %d %s", rec.Code, rec.Body.String())
	}
	if rec = doRawJSON(t, s, http.MethodDelete, "/api/v1/channels/"+group+"/bridges/"+bridge.ID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("double detach: got %d, want 404", rec.Code)
	}
	// The grouping channel still has the other leg attached, so it stays in use
	// until both are gone.
	if rec = doRawJSON(t, s, http.MethodDelete, "/api/v1/channels/"+group, ""); rec.Code != http.StatusConflict {
		t.Errorf("delete with one leg left: got %d, want 409", rec.Code)
	}
	remaining, err := s.channelSvc.Bridges(ctx, group)
	if err != nil || len(remaining) != 1 {
		t.Fatalf("expected one remaining edge on the group, got %+v (%v)", remaining, err)
	}
	if rec = doRawJSON(t, s, http.MethodDelete, "/api/v1/channels/"+group+"/bridges/"+remaining[0].ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("detach remaining leg: %d %s", rec.Code, rec.Body.String())
	}
	if rec = doRawJSON(t, s, http.MethodDelete, "/api/v1/channels/"+group, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete now-empty group: %d %s", rec.Code, rec.Body.String())
	}
}

func mustCreateChannel(t *testing.T, s *Server, name string) string {
	t.Helper()
	rec := doRawJSON(t, s, http.MethodPost, "/api/v1/channels", `{"name":"`+name+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: %d %s", name, rec.Code, rec.Body.String())
	}
	var ch channels.Channel
	if err := json.Unmarshal(rec.Body.Bytes(), &ch); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return ch.ID
}

func TestChannelsAPI_SubscriptionsMergeBothRegistries(t *testing.T) {
	s, cs, _, ts := channelTestServer(t)
	ctx := context.Background()

	connectorRef, agentRef := "api-sub-src", "api-sub-agent"
	connector := seedChannel(t, cs, channels.KindConnector, connectorRef)
	inbox := seedChannel(t, cs, channels.KindAgent, agentRef)
	group := mustCreateChannel(t, s, "sub bundle")

	// A trigger edge, created through the trigger registry rather than here.
	trigger := &triggers.Trigger{
		ID:             "tg-api-sub",
		DisplayName:    "on issue",
		AgentRef:       agentRef,
		ConnectorRef:   connectorRef,
		AgentValid:     true,
		ConnectorValid: true,
	}
	if err := ts.CreateTrigger(ctx, trigger); err != nil {
		t.Fatalf("seed trigger: %v", err)
	}
	if _, err := s.channelSvc.Attach(ctx, connector, group, ""); err != nil {
		t.Fatalf("attach: %v", err)
	}

	rec := doRawJSON(t, s, http.MethodGet, "/api/v1/channel-subscriptions", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("subscriptions: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []channels.Subscription `json:"items"`
		Total int                     `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("expected one trigger edge and one bridge edge, got %+v", resp.Items)
	}
	sources := map[string]bool{}
	for _, sub := range resp.Items {
		sources[sub.Source] = true
		if sub.FromChannel == "" || sub.ToChannel == "" {
			t.Errorf("edge lost its endpoints: %+v", sub)
		}
		// The trigger edge is resolved through the channel registry, so the
		// join lands on the provisioned ids rather than the raw refs.
		if sub.Source == channels.SourceTrigger && (sub.FromChannel != connector || sub.ToChannel != inbox) {
			t.Errorf("trigger edge did not resolve to the provisioned channels: %+v", sub)
		}
		if sub.Source == channels.SourceTrigger && sub.Name != "on issue" {
			t.Errorf("trigger edge should carry its display name, got %q", sub.Name)
		}
	}
	if !sources[channels.SourceTrigger] || !sources[channels.SourceBridge] {
		t.Errorf("expected both edge kinds, got %+v", resp.Items)
	}
}

func TestChannelsAPI_TimelineReflectsBirthAndDelivery(t *testing.T) {
	s, cs, eq, _ := channelTestServer(t)
	ctx := context.Background()

	srcRef := "api-tl-src"
	connector := seedChannel(t, cs, channels.KindConnector, srcRef)
	inboxRef := "api-tl-agent"
	inbox := seedChannel(t, cs, channels.KindAgent, inboxRef)
	group := mustCreateChannel(t, s, "tl bundle")

	insertAPIEvent(t, eq, "api-tl-1", srcRef, connector)
	insertAPIEvent(t, eq, "api-tl-2", "other-connector", "")
	insertAPITask(t, eq, "api-tl-2", inboxRef)

	// Connector timeline: only what was born there.
	events := timelineIDs(t, s, connector)
	if len(events) != 1 || events[0] != "api-tl-1" {
		t.Fatalf("connector timeline = %v", events)
	}
	// Inbox timeline: what was delivered, wherever it was born.
	events = timelineIDs(t, s, inbox)
	if len(events) != 1 || events[0] != "api-tl-2" {
		t.Fatalf("inbox timeline = %v", events)
	}
	// A grouping channel with nothing attached is empty.
	if events = timelineIDs(t, s, group); len(events) != 0 {
		t.Fatalf("empty group timeline = %v", events)
	}
	if _, err := s.channelSvc.Attach(ctx, connector, group, ""); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if events = timelineIDs(t, s, group); len(events) != 1 || events[0] != "api-tl-1" {
		t.Fatalf("group timeline after attach = %v", events)
	}
}

func timelineIDs(t *testing.T, s *Server, id string) []string {
	t.Helper()
	rec := doRawJSON(t, s, http.MethodGet, "/api/v1/channels/"+id+"/events", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("timeline %s: %d %s", id, rec.Code, rec.Body.String())
	}
	var resp struct {
		Events []struct {
			ID string `json:"id"`
		} `json:"events"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	out := make([]string, 0, len(resp.Events))
	for _, e := range resp.Events {
		out = append(out, e.ID)
	}
	return out
}

func insertAPIEvent(t *testing.T, eq *eventqueue.Store, id, connector, channelID string) {
	t.Helper()
	err := eq.InsertEvent(context.Background(), eventqueue.Event{
		ID:        id,
		Connector: connector,
		ChannelID: channelID,
		Headers:   json.RawMessage(`{"type":"api.test"}`),
		Data:      json.RawMessage(`{"n":1}`),
		Raw:       `{"n":1}`,
	})
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
}

func insertAPITask(t *testing.T, eq *eventqueue.Store, eventID, agent string) {
	t.Helper()
	err := eq.EnqueueTask(context.Background(), eventqueue.Task{
		EventID:     eventID,
		AgentName:   agent,
		TriggerName: "api-trigger",
		Headers:     json.RawMessage(`{}`),
		Payload:     json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("enqueue task: %v", err)
	}
}

func TestChannelsAPI_WindowBoundary(t *testing.T) {
	s, cs, eq, _ := channelTestServer(t)

	srcRef := "api-window-src"
	connector := seedChannel(t, cs, channels.KindConnector, srcRef)
	// One event "now", one well outside a short window.
	insertAPIEvent(t, eq, "api-window-1", srcRef, connector)
	if _, err := eq.Pool().Exec(context.Background(),
		`UPDATE events SET received_at = now() - interval '40 hours' WHERE id = 'api-window-1'`); err != nil {
		t.Fatalf("age event: %v", err)
	}
	insertAPIEvent(t, eq, "api-window-2", srcRef, connector)

	// Default window is 24h: the aged event is outside it.
	rec := doRawJSON(t, s, http.MethodGet, "/api/v1/channels?kind=connector", "")
	var resp struct {
		Items []channels.View `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Counts.Events != 1 {
		t.Fatalf("default window counts: %+v", resp.Items)
	}

	// Asking for a wider window moves the boundary — the same span the timeline
	// query uses, so counts and events cannot disagree.
	since := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	rec = doRawJSON(t, s, http.MethodGet, "/api/v1/channels?kind=connector&since="+since, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Items[0].Counts.Events != 2 {
		t.Fatalf("wide window counts: %+v", resp.Items)
	}

	rec = doRawJSON(t, s, http.MethodGet, "/api/v1/channels?since=not-a-time", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad since should be 400, got %d", rec.Code)
	}
}

func TestChannelsAPI_MethodAndPathGuards(t *testing.T) {
	s, _, _, _ := channelTestServer(t)

	if rec := doRawJSON(t, s, http.MethodPatch, "/api/v1/channels", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("PATCH on the collection: got %d, want 405", rec.Code)
	}
	if rec := doRawJSON(t, s, http.MethodPost, "/api/v1/channels", `{"name":"  "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("blank name: got %d %s, want 400", rec.Code, rec.Body.String())
	}
	if rec := doRawJSON(t, s, http.MethodPost, "/api/v1/channels", `{"name":"x",,}`); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed body: got %d, want 400", rec.Code)
	}
	if rec := doRawJSON(t, s, http.MethodGet, "/api/v1/channels/", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("missing id: got %d, want 400", rec.Code)
	}
	if rec := doRawJSON(t, s, http.MethodGet, "/api/v1/channels/ch-unknown/nested", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown sub-resource: got %d, want 404", rec.Code)
	}
}

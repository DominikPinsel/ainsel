package channels

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
)

// fakeTriggers stands in for the trigger registry: the service composes its
// edges with the bridges at read time, and building real Trigger CRs to prove a
// join would test Kubernetes, not this package.
type fakeTriggers struct {
	rows []triggers.Trigger
	err  error
}

func (f fakeTriggers) ListTriggers(ctx context.Context, agentRef, connectorRef string) ([]triggers.Trigger, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []triggers.Trigger
	for _, t := range f.rows {
		if agentRef != "" && t.AgentRef != agentRef {
			continue
		}
		if connectorRef != "" && t.ConnectorRef != connectorRef {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func TestSubscriptionsComposesTriggersAndBridges(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()

	connectorRef := uniqueName("compose-src")
	agentRef := uniqueName("compose-dst")
	src := seed(t, s, KindConnector, connectorRef)
	dst := seed(t, s, KindAgent, agentRef)
	group := seedCustom(t, s)

	reg := fakeTriggers{rows: []triggers.Trigger{{
		ID:           "tg-test-1",
		DisplayName:  "on-issue",
		AgentRef:     agentRef,
		ConnectorRef: connectorRef,
	}}}
	svc := NewService(s, eq, reg)

	if _, err := s.CreateBridge(ctx, src, group, "everything else"); err != nil {
		t.Fatalf("attach: %v", err)
	}

	subs, err := svc.Subscriptions(ctx)
	if err != nil {
		t.Fatalf("subscriptions: %v", err)
	}
	bySource := map[string][]Subscription{}
	for _, sub := range subs {
		bySource[sub.Source] = append(bySource[sub.Source], sub)
	}

	// The trigger edge is resolved to channel ids, and keeps its owner's ref so
	// a caller can jump back to the trigger itself.
	triggerEdges := bySource[SourceTrigger]
	if len(triggerEdges) != 1 {
		t.Fatalf("expected one trigger edge, got %+v", triggerEdges)
	}
	if triggerEdges[0].FromChannel != src || triggerEdges[0].ToChannel != dst {
		t.Errorf("trigger edge endpoints: %+v", triggerEdges[0])
	}
	if triggerEdges[0].RefID != "tg-test-1" || triggerEdges[0].Name != "on-issue" {
		t.Errorf("trigger edge lost its origin: %+v", triggerEdges[0])
	}
	bridgeEdges := bySource[SourceBridge]
	if len(bridgeEdges) != 1 || bridgeEdges[0].FromChannel != src || bridgeEdges[0].ToChannel != group {
		t.Errorf("bridge edge: %+v", bridgeEdges)
	}
	if bridgeEdges[0].Name != "everything else" {
		t.Errorf("bridge label lost: %+v", bridgeEdges[0])
	}
}

func TestSubscriptionsKeepsDanglingTriggerEndpoints(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()

	// A trigger whose connector channel was never provisioned — a CR deleted
	// while its trigger row survived. The edge must still be reported, with the
	// raw refs, or the graph silently loses a subscription.
	reg := fakeTriggers{rows: []triggers.Trigger{{
		ID: "tg-test-dangling", DisplayName: "dangling", AgentRef: "no-such-agent", ConnectorRef: "no-such-connector",
	}}}
	svc := NewService(s, eq, reg)
	subs, err := svc.Subscriptions(ctx)
	if err != nil {
		t.Fatalf("subscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected the dangling edge, got %+v", subs)
	}
	if subs[0].FromChannel != "" || subs[0].ToChannel != "" {
		t.Errorf("a dangling edge must not claim a channel: %+v", subs[0])
	}
	if subs[0].FromRef != "no-such-connector" || subs[0].ToRef != "no-such-agent" {
		t.Errorf("raw refs lost: %+v", subs[0])
	}
}

func TestCountsSeparateBirthsDeliveriesAndFailures(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()

	srcRef := uniqueName("count-src")
	dstRef := uniqueName("count-dst")
	src := seed(t, s, KindConnector, srcRef)
	dst := seed(t, s, KindAgent, dstRef)
	group := seedCustom(t, s)

	// src → group → dst, so one event is born in src, passes through group and
	// is delivered to dst.
	if _, err := s.CreateBridge(ctx, src, group, ""); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, err := s.CreateBridge(ctx, group, dst, ""); err != nil {
		t.Fatalf("attach: %v", err)
	}

	// Two events arrive at src. One is delivered to dst and succeeds; the other
	// matches nothing.
	insertEvent(t, eq, "ch-test-evt-1", srcRef, src)
	insertEvent(t, eq, "ch-test-evt-2", srcRef, src)
	insertTask(t, eq, "ch-test-evt-1", dstRef)

	// The successful task is still pending, which counts as neither failure nor
	// success; add a third event whose delivery fails.
	insertEvent(t, eq, "ch-test-evt-3", srcRef, src)
	insertTask(t, eq, "ch-test-evt-3", dstRef)
	failUnroutedTaskStatus(t, eq, "ch-test-evt-3", dstRef)

	svc := NewService(s, eq, fakeTriggers{})
	since := time.Now().UTC().Add(-time.Hour)

	views, err := svc.List(ctx, "", since)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byID := map[string]View{}
	for _, v := range views {
		byID[v.ID] = v
	}

	// The connector channel counts its own births; nothing was routed at
	// trigger level, so the unmatched figure covers the events no delivery
	// followed.
	srcView := byID[src]
	if srcView.Counts.Events != 3 {
		t.Errorf("connector births = %d, want 3", srcView.Counts.Events)
	}
	// The grouping channel sees everything that crossed it.
	groupView := byID[group]
	if groupView.Counts.Events != 3 {
		t.Errorf("grouping channel count = %d, want 3", groupView.Counts.Events)
	}
	// The inbox counts what was delivered to it, and the failed task shows up as
	// a failure rather than changing the event count.
	dstView := byID[dst]
	if dstView.Counts.Events != 2 {
		t.Errorf("inbox deliveries = %d, want 2", dstView.Counts.Events)
	}
	if dstView.Counts.Failed != 1 {
		t.Errorf("inbox failures = %d, want 1", dstView.Counts.Failed)
	}
	// Edge fan-out is reported per channel: src feeds two edges, dst none.
	if srcView.Bridges != 1 || dstView.Bridges != 1 {
		t.Errorf("bridge degrees: src=%d dst=%d, want 1 and 1", srcView.Bridges, dstView.Bridges)
	}
}

// failUnroutedTaskStatus marks the task for an event/agent pair failed, the way
// the runner does when an agent run ends badly.
func failUnroutedTaskStatus(t *testing.T, eq *eventqueue.Store, eventID, agent string) {
	t.Helper()
	ctx := context.Background()
	if _, err := eq.Pool().Exec(ctx,
		`UPDATE agent_tasks SET status = 'failed', completed_at = now(), error = 'test failure'
		  WHERE event_id = $1 AND agent_name = $2`, eventID, agent); err != nil {
		t.Fatalf("fail task: %v", err)
	}
}

func TestTimelineFilterPerKind(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()

	srcRef := uniqueName("tl-src")
	dstRef := uniqueName("tl-dst")
	src := seed(t, s, KindConnector, srcRef)
	dst := seed(t, s, KindAgent, dstRef)
	group := seedCustom(t, s)
	other := seed(t, s, KindConnector, uniqueName("tl-other"))

	insertEvent(t, eq, "ch-test-tl-1", srcRef, src)
	insertEvent(t, eq, "ch-test-tl-2", other, other)
	// An event delivered to dst but born elsewhere belongs in dst's timeline and
	// in no birth-count query.
	insertEvent(t, eq, "ch-test-tl-3", other, other)
	insertTask(t, eq, "ch-test-tl-3", dstRef)

	svc := NewService(s, eq, fakeTriggers{})

	// A connector timeline holds its own births only.
	events, total, err := svc.Events(ctx, src, EventQuery{})
	if err != nil {
		t.Fatalf("connector timeline: %v", err)
	}
	if total != 1 || events[0].ID != "ch-test-tl-1" {
		t.Fatalf("connector timeline = %d %+v", total, ids(events))
	}

	// An inbox timeline holds what was delivered to it, wherever it was born.
	events, total, err = svc.Events(ctx, dst, EventQuery{})
	if err != nil {
		t.Fatalf("inbox timeline: %v", err)
	}
	if total != 1 || events[0].ID != "ch-test-tl-3" {
		t.Fatalf("inbox timeline = %d %+v", total, ids(events))
	}

	// A grouping channel with nothing attached is empty, not an error.
	events, total, err = svc.Events(ctx, group, EventQuery{})
	if err != nil {
		t.Fatalf("group timeline: %v", err)
	}
	if total != 0 || len(events) != 0 {
		t.Fatalf("empty group timeline = %d %+v", total, ids(events))
	}

	// Once attached, it reports the union of the channels reaching it.
	if _, err := s.CreateBridge(ctx, src, group, ""); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, err := s.CreateBridge(ctx, other, group, ""); err != nil {
		t.Fatalf("attach: %v", err)
	}
	events, total, err = svc.Events(ctx, group, EventQuery{})
	if err != nil {
		t.Fatalf("group timeline after attach: %v", err)
	}
	// tl-1 was born in src and tl-2/tl-3 in other; both feed the group, so the
	// grouping timeline is the union of what reaches it.
	if total != 3 {
		t.Fatalf("group timeline = %d %+v", total, ids(events))
	}

	// Filters pass through: an unknown subject pattern matches nothing.
	_, total, err = svc.Events(ctx, group, EventQuery{Subject: "nope.nope"})
	if err != nil {
		t.Fatalf("filtered timeline: %v", err)
	}
	if total != 0 {
		t.Fatalf("subject filter leaked %d events", total)
	}
}

func ids(events []eventqueue.Event) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.ID)
	}
	return out
}

func TestProvisioningHelpers(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()
	svc := NewService(s, eq, fakeTriggers{})

	label := uniqueName("provision")
	id, err := svc.ConnectorChannelFor(ctx, label)
	if err != nil {
		t.Fatalf("connector channel: %v", err)
	}
	if id == "" {
		t.Fatal("expected a channel id")
	}
	again, err := svc.ConnectorChannelFor(ctx, label)
	if err != nil || again != id {
		t.Fatalf("provisioning is not stable: %s vs %s (%v)", id, again, err)
	}

	inbox, err := svc.InboxChannelFor(ctx, "agent-"+label)
	if err != nil {
		t.Fatalf("inbox channel: %v", err)
	}
	if inbox == id {
		t.Error("an agent inbox and a connector stream with the same ref must differ")
	}
	ch, err := svc.EntityChannel(ctx, KindAgent, "agent-"+label)
	if err != nil || ch == nil {
		t.Fatalf("entity channel: %v", err)
	}
	if !strings.Contains(ch.Description, "inbox") {
		t.Errorf("provisioned channel should carry its kind's description, got %q", ch.Description)
	}
}

func TestCustomChannelLifecycle(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()
	svc := NewService(s, eq, fakeTriggers{})

	if _, err := svc.CreateCustom(ctx, "  ", ""); err == nil {
		t.Fatal("a blank name must be refused")
	}
	ch, err := svc.CreateCustom(ctx, uniqueName("group"), "bundles the nightly streams")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ch.Kind != KindCustom || ch.EntityRef != "" {
		t.Fatalf("unexpected custom channel: %+v", ch)
	}

	name := "renamed"
	updated, err := svc.Update(ctx, ch.ID, &name, nil)
	if err != nil || updated.Name != name {
		t.Fatalf("update: %v %+v", err, updated)
	}
	// A provisioned channel refuses the same edit.
	prov := seed(t, s, KindConnector, uniqueName("group-src"))
	if _, err := svc.Update(ctx, prov, &name, nil); err != ErrNotCustom {
		t.Fatalf("update provisioned: got %v, want ErrNotCustom", err)
	}

	// Attaching an edge makes the channel in use.
	if _, err := svc.Attach(ctx, prov, ch.ID, ""); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := svc.Delete(ctx, ch.ID); err != ErrInUse {
		t.Fatalf("delete in use: got %v, want ErrInUse", err)
	}
	bridges, err := svc.Bridges(ctx, ch.ID)
	if err != nil || len(bridges) != 1 {
		t.Fatalf("bridges: %v %+v", err, bridges)
	}
	if err := svc.Detach(ctx, bridges[0].ID); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if err := svc.Delete(ctx, ch.ID); err != nil {
		t.Fatalf("delete after detach: %v", err)
	}
	if _, err := svc.Channel(ctx, ch.ID); err != ErrNotFound {
		t.Fatalf("channel survived deletion: %v", err)
	}
}

func TestDeliveriesSkipTriggerMatchedAgents(t *testing.T) {
	s, eq := stores(t)
	ctx := context.Background()

	src := seed(t, s, KindConnector, uniqueName("skip-src"))
	group := seedCustom(t, s)
	first := seed(t, s, KindAgent, "first-"+uniqueName("skip"))
	second := seed(t, s, KindAgent, "second-"+uniqueName("skip"))

	for _, edge := range [][2]string{{src, group}, {group, first}, {group, second}} {
		if _, err := s.CreateBridge(ctx, edge[0], edge[1], ""); err != nil {
			t.Fatalf("attach: %v", err)
		}
	}
	svc := NewService(s, eq, fakeTriggers{})

	all, err := svc.Deliveries(ctx, src, nil)
	if err != nil || len(all) != 2 {
		t.Fatalf("deliveries: %v %+v", err, all)
	}
	filtered, err := svc.Deliveries(ctx, src, []string{all[0].AgentName})
	if err != nil {
		t.Fatalf("filtered deliveries: %v", err)
	}
	if len(filtered) != 1 || filtered[0].AgentName == all[0].AgentName {
		t.Fatalf("already-delivered agent survived filtering: %+v", filtered)
	}

	// An event with no birth channel cannot be transferred.
	none, err := svc.Deliveries(ctx, "", nil)
	if err != nil || none != nil {
		t.Fatalf("empty birth channel: %v %+v", err, none)
	}
}

func TestSubscriptionsFailsLoudlyWhenRegistryUnreadable(t *testing.T) {
	s, eq := stores(t)
	svc := NewService(s, eq, fakeTriggers{err: context.DeadlineExceeded})
	if _, err := svc.Subscriptions(context.Background()); err == nil {
		t.Fatal("a trigger registry that errors must not read as an empty graph")
	}
}

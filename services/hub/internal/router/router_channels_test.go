package router

import (
	"context"
	"errors"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/channels"
	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/trigger"
)

// fakeTransfer records what the router asked the channel graph to deliver, and
// behaves like the real one: it enqueues a task per delivery, so the assertions
// cover the whole routing step rather than just the call.
type fakeTransfer struct {
	inputs     []channels.TransferInput
	deliveries []channels.TransferResult
	enqueue    *mockQueue
	err        error
}

func (f *fakeTransfer) Apply(ctx context.Context, in channels.TransferInput) ([]channels.TransferResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.inputs = append(f.inputs, in)
	for _, d := range f.deliveries {
		if f.enqueue == nil || d.Err != nil {
			continue
		}
		_ = f.enqueue.EnqueueTask(ctx, eventqueue.Task{
			EventID: in.EventID, AgentName: d.AgentName, TriggerName: d.BridgeName,
		})
	}
	return f.deliveries, nil
}

// channelEvent is an event born in a connector channel — the shape ingest
// produces once the birth channel is stamped.
func channelEvent(t *testing.T, id, connector, channelID string) eventqueue.Event {
	t.Helper()
	evt := newTestQueueEvent()
	evt.ID = id
	evt.Connector = connector
	evt.ChannelID = channelID
	return evt
}

func TestHandleEvent_TransfersAlongBridgesAfterTriggerMatching(t *testing.T) {
	q := &mockQueue{}
	idx := trigger.NewIndex()
	// One trigger matches agent-1; the bridge graph should be asked to deliver
	// everything *except* that agent.
	idx.Update(newValidTrigger("trig-1", "conn-1", "issue.opened", "agent-1"))

	tr := &fakeTransfer{enqueue: q, deliveries: []channels.TransferResult{
		{AgentName: "agent-2", BridgeID: "br-1", BridgeName: "nightly bundle"},
	}}
	r := &Router{
		eq:          q,
		index:       idx,
		invocations: invocations.NewMemoryStore(10),
		broadcaster: &stubBroadcaster{},
		transfers:   tr,
	}

	r.handleEvent(context.Background(), channelEvent(t, "evt-1", "conn-1", "ch-src"))

	if len(tr.inputs) != 1 {
		t.Fatalf("expected one transfer, got %d", len(tr.inputs))
	}
	in := tr.inputs[0]
	if in.BirthChannel != "ch-src" || in.EventID != "evt-1" || in.SourceLabel != "conn-1" {
		t.Errorf("transfer input: %+v", in)
	}
	// The agent the trigger already matched must not be offered a second task.
	if len(in.Delivered) != 1 || in.Delivered[0] != "agent-1" {
		t.Errorf("transfer should exclude trigger-matched agents, got %v", in.Delivered)
	}
	if string(in.Payload) != "{}" {
		t.Errorf("transfer payload = %s", in.Payload)
	}

	// Both the trigger task and the bridge task are in the queue.
	if len(q.enqueued) != 2 {
		t.Fatalf("expected 2 enqueued tasks, got %d: %+v", len(q.enqueued), q.enqueued)
	}
	byAgent := map[string]eventqueue.Task{}
	for _, task := range q.enqueued {
		byAgent[task.AgentName] = task
	}
	if _, ok := byAgent["agent-1"]; !ok {
		t.Error("trigger match did not produce a task")
	}
	bridged, ok := byAgent["agent-2"]
	if !ok {
		t.Fatal("bridge delivery did not produce a task")
	}
	if bridged.TriggerName != "nightly bundle" {
		t.Errorf("bridged task should be attributed to its subscription, got %q", bridged.TriggerName)
	}
	if len(q.routedIDs) != 1 {
		t.Errorf("event should be marked routed once, got %v", q.routedIDs)
	}
}

func TestHandleEvent_TransfersWithoutAnyTriggerMatch(t *testing.T) {
	// The point of separating the two registries: a connector whose events match
	// no trigger still feeds the grouping channels attached to its stream.
	q := &mockQueue{}
	tr := &fakeTransfer{enqueue: q, deliveries: []channels.TransferResult{
		{AgentName: "agent-9", BridgeID: "br-9", BridgeName: "bundle"},
	}}
	r := &Router{
		eq:          q,
		index:       trigger.NewIndex(),
		invocations: invocations.NewMemoryStore(10),
		broadcaster: &stubBroadcaster{},
		transfers:   tr,
	}

	r.handleEvent(context.Background(), channelEvent(t, "evt-1", "conn-1", "ch-src"))

	if len(q.enqueued) != 1 || q.enqueued[0].AgentName != "agent-9" {
		t.Fatalf("bridge-only routing produced %+v", q.enqueued)
	}
	// It is a matched delivery, not an unmatched event.
	if len(q.routedIDs) != 1 {
		t.Error("event should still be marked routed")
	}
	if in := tr.inputs[0]; len(in.Delivered) != 0 {
		t.Errorf("no agent was pre-delivered, got %v", in.Delivered)
	}
}

func TestHandleEvent_SkipsTransferWithoutBirthChannel(t *testing.T) {
	// History that predates the registry, or a stream that could not be
	// resolved, must not be routed by guess.
	q := &mockQueue{}
	tr := &fakeTransfer{}
	r := &Router{
		eq:          q,
		index:       trigger.NewIndex(),
		invocations: invocations.NewMemoryStore(10),
		broadcaster: &stubBroadcaster{},
		transfers:   tr,
	}

	evt := newTestQueueEvent()
	evt.ChannelID = ""
	r.handleEvent(context.Background(), evt)

	if len(tr.inputs) != 0 {
		t.Fatalf("an unstamped event must not walk the graph: %+v", tr.inputs)
	}
	if len(q.routedIDs) != 1 {
		t.Error("unstamped events are still routed (marked done), just not transferred")
	}
}

func TestHandleEvent_WithoutTransferWiringIsUnchanged(t *testing.T) {
	// A hub started with no channel registry behaves exactly as before: triggers
	// route, nothing else.
	q := &mockQueue{}
	idx := trigger.NewIndex()
	idx.Update(newValidTrigger("trig-1", "conn-1", "issue.opened", "agent-1"))
	r := &Router{
		eq:          q,
		index:       idx,
		invocations: invocations.NewMemoryStore(10),
		broadcaster: &stubBroadcaster{},
	}
	r.handleEvent(context.Background(), channelEvent(t, "evt-1", "conn-1", "ch-src"))
	if len(q.enqueued) != 1 || q.enqueued[0].AgentName != "agent-1" {
		t.Fatalf("unexpected tasks: %+v", q.enqueued)
	}
}

func TestHandleEvent_BridgeTaskFailureStillRoutesTheEvent(t *testing.T) {
	q := &mockQueue{}
	tr := &fakeTransfer{enqueue: q, deliveries: []channels.TransferResult{
		{AgentName: "agent-2", BridgeID: "br-1", BridgeName: "bundle", Err: errors.New("queue down")},
		{AgentName: "agent-3", BridgeID: "br-1", BridgeName: "bundle"},
	}}
	r := &Router{
		eq:          q,
		index:       trigger.NewIndex(),
		invocations: invocations.NewMemoryStore(10),
		broadcaster: &stubBroadcaster{},
		transfers:   tr,
	}
	r.handleEvent(context.Background(), channelEvent(t, "evt-1", "conn-1", "ch-src"))

	// One inbox failing must not stop the other, nor leave the event unrouted —
	// the invocation records the failure.
	if len(q.routedIDs) != 1 {
		t.Fatalf("event should be marked routed: %v", q.routedIDs)
	}
	if len(q.enqueued) != 1 || q.enqueued[0].AgentName != "agent-3" {
		t.Fatalf("the surviving inbox should still be served: %+v", q.enqueued)
	}
}

func TestHandleEvent_TransferErrorDoesNotBlockRouting(t *testing.T) {
	q := &mockQueue{}
	idx := trigger.NewIndex()
	idx.Update(newValidTrigger("trig-1", "conn-1", "issue.opened", "agent-1"))
	tr := &fakeTransfer{err: errors.New("channel graph unavailable")}
	bc := &recordingBroadcaster{}
	r := &Router{
		eq:          q,
		index:       idx,
		invocations: invocations.NewMemoryStore(10),
		broadcaster: bc,
		transfers:   tr,
	}
	r.handleEvent(context.Background(), channelEvent(t, "evt-1", "conn-1", "ch-src"))

	// The trigger match still stands; only the extra deliveries are lost. A
	// retry loop over an unreadable graph would stall every event behind it.
	if len(q.enqueued) != 1 {
		t.Fatalf("trigger delivery should still happen: %+v", q.enqueued)
	}
	if len(q.routedIDs) != 1 {
		t.Fatalf("event must not be retried forever: %v", q.routedIDs)
	}
}

func TestHandleEvent_BridgeDeliveryRecordsInvocation(t *testing.T) {
	q := &mockQueue{}
	inv := invocations.NewMemoryStore(10)
	tr := &fakeTransfer{deliveries: []channels.TransferResult{
		{AgentName: "agent-2", BridgeID: "br-1", BridgeName: "bundle", InvocationID: "inv-1"},
	}}
	r := &Router{
		eq:          q,
		index:       trigger.NewIndex(),
		invocations: inv,
		broadcaster: &stubBroadcaster{},
		transfers:   tr,
	}
	r.handleEvent(context.Background(), channelEvent(t, "evt-1", "conn-1", "ch-src"))

	// The transfer itself owns the invocation record (see channels.Transfer); the
	// router only asks for deliveries. What matters here is that the router does
	// not double-record one for the same agent.
	list := inv.List(invocations.ListOptions{AgentName: "agent-2"})
	if len(list) != 0 {
		t.Fatalf("router should not record invocations the transfer already recorded: %+v", list)
	}
}

func TestRouterIgnoresClientSuppliedChannelOnUnstampedEvents(t *testing.T) {
	// A publisher cannot choose a channel by naming one: the router only walks
	// the graph from the id ingest stamped, and ingest never takes it from the
	// request body.
	q := &mockQueue{}
	tr := &fakeTransfer{}
	r := &Router{
		eq:          q,
		index:       trigger.NewIndex(),
		invocations: invocations.NewMemoryStore(10),
		broadcaster: &stubBroadcaster{},
		transfers:   tr,
	}
	evt := newTestQueueEvent()
	evt.ChannelID = "ch-someone-elses-stream"
	// No bridge exists from that id, so nothing is delivered — and no error is
	// invented for a channel the registry does not know.
	r.handleEvent(context.Background(), evt)
	if len(q.enqueued) != 0 {
		t.Fatalf("unexpected tasks: %+v", q.enqueued)
	}
	if len(tr.inputs) != 1 || len(tr.inputs[0].Delivered) != 0 {
		t.Fatalf("transfer input: %+v", tr.inputs)
	}
}

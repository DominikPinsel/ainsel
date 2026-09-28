package channels

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
)

// recordedQueue captures the tasks a transfer enqueues, so the assertions are
// about what an agent would actually be handed.
type recordedQueue struct {
	tasks []eventqueue.Task
	err   error
}

func (q *recordedQueue) EnqueueTask(ctx context.Context, task eventqueue.Task) error {
	if q.err != nil {
		return q.err
	}
	q.tasks = append(q.tasks, task)
	return nil
}

// taskHeaders decodes a captured task's headers.
func taskHeaders(t *testing.T, task eventqueue.Task) map[string]string {
	t.Helper()
	var h map[string]string
	if err := json.Unmarshal(task.Headers, &h); err != nil {
		t.Fatalf("task headers: %v", err)
	}
	return h
}

func newTestTransfer(t *testing.T) (*Transfer, *Service, *Store, *recordedQueue, *invocations.MemoryStore) {
	t.Helper()
	s, eq := stores(t)
	svc := NewService(s, eq, fakeTriggers{})
	queue := &recordedQueue{}
	inv := invocations.NewMemoryStore(64)
	return NewTransfer(svc, queue, inv), svc, s, queue, inv
}

func TestTransferEnqueuesEveryReachedInbox(t *testing.T) {
	tr, _, s, queue, inv := newTestTransfer(t)
	ctx := context.Background()

	src := seed(t, s, KindConnector, uniqueName("tr-src"))
	group := seedCustom(t, s)
	agentRef := "tr-agent-a"
	dst := seed(t, s, KindAgent, agentRef)

	if _, err := s.CreateBridge(ctx, src, group, "nightly bundle"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, err := s.CreateBridge(ctx, group, dst, ""); err != nil {
		t.Fatalf("attach: %v", err)
	}

	payload := json.RawMessage(`{"issue":7}`)
	results, err := tr.Apply(ctx, TransferInput{
		EventID:      "ch-test-tr-1",
		BirthChannel: src,
		SourceLabel:  "forgejo",
		Headers:      map[string]string{"type": "issue_open", "X-GitHub-Event": "issues"},
		Payload:      payload,
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one transfer, got %+v", results)
	}
	if len(queue.tasks) != 1 {
		t.Fatalf("expected one queued task, got %+v", queue.tasks)
	}

	task := queue.tasks[0]
	if task.EventID != "ch-test-tr-1" || task.AgentName != agentRef {
		t.Fatalf("task routed wrong: %+v", task)
	}
	// The task carries the subscription that moved it, so the run is attributable
	// in the console and in the invocation record.
	if task.TriggerName == "" {
		t.Error("transferred task must name its subscription")
	}
	h := taskHeaders(t, task)
	if h[HeaderTriggerName] != task.TriggerName {
		t.Errorf("trigger header mismatch: %v", h)
	}
	if h[HeaderChannelID] != dst || h[HeaderOriginChannel] != src {
		t.Errorf("channel headers wrong: %v", h)
	}
	if h[HeaderChannelBridge] == "" || h[HeaderInvocationID] == "" {
		t.Errorf("bridge/invocation headers missing: %v", h)
	}
	// The original event headers ride along, and the canonical type is stamped
	// the same way the router stamps it — provider headers win — so the runner
	// sees the same event a trigger match would have delivered.
	if h["X-GitHub-Event"] != "issues" || h["type"] != "issues" {
		t.Errorf("event headers not propagated: %v", h)
	}
	if string(task.Payload) != string(payload) {
		t.Errorf("payload = %s, want %s", task.Payload, payload)
	}

	// Every transfer is recorded as an invocation against its subscription.
	list := inv.List(invocations.ListOptions{AgentName: agentRef})
	if len(list) != 1 {
		t.Fatalf("expected one invocation, got %+v", list)
	}
	if list[0].TriggerName != task.TriggerName || list[0].EventID != "ch-test-tr-1" {
		t.Errorf("invocation misattributed: %+v", list[0])
	}
	if list[0].Connector != "forgejo" {
		t.Errorf("invocation should record the source label, got %q", list[0].Connector)
	}
}

func TestTransferHonoursAlreadyDeliveredAgents(t *testing.T) {
	tr, _, s, queue, _ := newTestTransfer(t)
	ctx := context.Background()

	src := seed(t, s, KindConnector, uniqueName("dup-src"))
	group := seedCustom(t, s)
	matched := "dup-matched"
	other := "dup-other"
	matchedInbox := seed(t, s, KindAgent, matched)
	otherInbox := seed(t, s, KindAgent, other)
	for _, edge := range [][2]string{{src, group}, {group, matchedInbox}, {group, otherInbox}} {
		if _, err := s.CreateBridge(ctx, edge[0], edge[1], ""); err != nil {
			t.Fatalf("attach: %v", err)
		}
	}

	// A trigger already delivered to `matched`; the bridge must not queue it a
	// second time.
	results, err := tr.Apply(ctx, TransferInput{
		EventID:      "ch-test-dup-1",
		BirthChannel: src,
		Headers:      map[string]string{"type": "issue_open"},
		Payload:      json.RawMessage(`{}`),
		Delivered:    []string{matched},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(results) != 1 || results[0].AgentName != other {
		t.Fatalf("already-delivered agent was transferred anyway: %+v", results)
	}
	if len(queue.tasks) != 1 || queue.tasks[0].AgentName != other {
		t.Fatalf("queued tasks: %+v", queue.tasks)
	}
}

func TestTransferWithoutBirthChannelIsInert(t *testing.T) {
	tr, _, _, queue, inv := newTestTransfer(t)
	// An event stamped before channels existed, or one whose stream could not be
	// resolved, must not be routed by guess.
	results, err := tr.Apply(context.Background(), TransferInput{EventID: "ch-test-none"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(results) != 0 || len(queue.tasks) != 0 {
		t.Fatalf("unexpected transfers: %+v %+v", results, queue.tasks)
	}
	if len(inv.List(invocations.ListOptions{})) != 0 {
		t.Fatal("an inert transfer must not record invocations")
	}
}

func TestTransferRecordsFailureAndContinues(t *testing.T) {
	tr, _, s, _, inv := newTestTransfer(t)
	ctx := context.Background()

	src := seed(t, s, KindConnector, uniqueName("fail-src"))
	group := seedCustom(t, s)
	first, second := "fail-a", "fail-b"
	firstInbox, secondInbox := seed(t, s, KindAgent, first), seed(t, s, KindAgent, second)
	for _, edge := range [][2]string{{src, group}, {group, firstInbox}, {group, secondInbox}} {
		if _, err := s.CreateBridge(ctx, edge[0], edge[1], ""); err != nil {
			t.Fatalf("attach: %v", err)
		}
	}
	// Every enqueue fails: a partial fan-out is still the truth about what the
	// agent got, so one bad inbox must not stop the other.
	tr.eq = &recordedQueue{err: errors.New("queue down")}

	results, err := tr.Apply(ctx, TransferInput{
		EventID:      "ch-test-fail-1",
		BirthChannel: src,
		Headers:      map[string]string{"type": "issue_open"},
		Payload:      json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("apply should not fail the batch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected two attempted deliveries, got %+v", results)
	}
	for _, r := range results {
		if r.Err == nil {
			t.Errorf("delivery %s should report the enqueue error", r.AgentName)
		}
	}
	// Each failed delivery is closed out as a failure against its invocation.
	for _, agent := range []string{first, second} {
		list := inv.List(invocations.ListOptions{AgentName: agent})
		if len(list) != 1 {
			t.Fatalf("agent %s: expected one invocation, got %+v", agent, list)
		}
		if list[0].Status != invocations.StatusFailure {
			t.Errorf("agent %s: invocation status = %q, want failure", agent, list[0].Status)
		}
	}
}

func TestTransferIsNilSafe(t *testing.T) {
	// The cron and chat paths hold an optional transfer; a nil one means "no
	// bridges", not a crash.
	var tr *Transfer
	results, err := tr.Apply(context.Background(), TransferInput{EventID: "x", BirthChannel: "ch-y"})
	if err != nil || results != nil {
		t.Fatalf("nil transfer: %v %+v", err, results)
	}
}

// A transfer must not invent a delivery for a channel that only points at
// grouping channels — the graph can end in a group with nothing attached.
func TestTransferStopsAtGroupingChannels(t *testing.T) {
	tr, _, s, queue, _ := newTestTransfer(t)
	ctx := context.Background()

	src := seed(t, s, KindConnector, uniqueName("leaf-src"))
	group := seedCustom(t, s)
	if _, err := s.CreateBridge(ctx, src, group, ""); err != nil {
		t.Fatalf("attach: %v", err)
	}
	results, err := tr.Apply(ctx, TransferInput{
		EventID:      "ch-test-leaf-1",
		BirthChannel: src,
		Payload:      json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(results) != 0 || len(queue.tasks) != 0 {
		t.Fatalf("a group with no inbox attached must receive no tasks: %+v", queue.tasks)
	}
}

package channels

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
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

	if _, err := s.CreateBridge(ctx, src, group, "nightly bundle", nil); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if _, err := s.CreateBridge(ctx, group, dst, "", nil); err != nil {
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
		if _, err := s.CreateBridge(ctx, edge[0], edge[1], "", nil); err != nil {
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
		if _, err := s.CreateBridge(ctx, edge[0], edge[1], "", nil); err != nil {
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
	if _, err := s.CreateBridge(ctx, src, group, "", nil); err != nil {
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

// Bridge filters gate a transfer the way trigger filters gate a connector:
// groups OR, filters inside a group AND. A push event that matches neither
// group of a reviewer bridge must cost the graph a walk and nothing else.
func TestTransferBridgeFilters(t *testing.T) {
	tr, _, s, queue, inv := newTestTransfer(t)
	ctx := context.Background()

	src := seed(t, s, KindConnector, uniqueName("fltr-src"))
	group := seedCustom(t, s)
	agentRef := uniqueName("fltr-agent")
	dst := seed(t, s, KindAgent, agentRef)

	// A reviewer-shaped gate: pulls, or a comment mentioning review-agent, but
	// not the agent's own comments — mirroring the pre-channels trigger set.
	reviewerFilters := [][]ainselapishared.Filter{
		{
			{Field: "type", Op: "in", Values: []string{"pull_request"}},
		},
		{
			{Field: "type", Op: "eq", Value: "issue_comment"},
			{Field: "comment.body", Op: "contains", Value: "@review-agent"},
			{Field: "comment.user.login", Op: "not-in", Values: []string{"dev-agent", "review-agent"}},
		},
	}
	if _, err := s.CreateBridge(ctx, src, group, "", nil); err != nil {
		t.Fatalf("attach gather: %v", err)
	}
	if _, err := s.CreateBridge(ctx, group, dst, "reviewer only", reviewerFilters); err != nil {
		t.Fatalf("attach dispatch: %v", err)
	}

	// A push matches no group.
	res, err := tr.Apply(ctx, TransferInput{
		EventID:      "ch-test-fltr-push",
		BirthChannel: src,
		Headers:      map[string]string{"X-Forgejo-Event": "push"},
		Payload:      json.RawMessage(`{"ref":"refs/heads/main"}`),
	})
	if err != nil {
		t.Fatalf("apply push: %v", err)
	}
	if len(res) != 0 || len(queue.tasks) != 0 || inv.Len() != 0 {
		t.Fatalf("push must be filtered out of the reviewer bridge: results=%+v tasks=%d invocations=%d",
			res, len(queue.tasks), inv.Len())
	}

	// A pull_request event matches the first group and is delivered.
	res, err = tr.Apply(ctx, TransferInput{
		EventID:      "ch-test-fltr-pr",
		BirthChannel: src,
		Headers:      map[string]string{"X-Forgejo-Event": "pull_request"},
		Payload:      json.RawMessage(`{"action":"opened","number":7}`),
	})
	if err != nil {
		t.Fatalf("apply pr: %v", err)
	}
	if len(res) != 1 || len(queue.tasks) != 1 || inv.Len() != 1 {
		t.Fatalf("pull_request must pass the gate: results=%+v tasks=%d invocations=%d",
			res, len(queue.tasks), inv.Len())
	}
	if got := taskHeaders(t, queue.tasks[0])["type"]; got != "pull_request" {
		t.Fatalf("delivered task should carry the canonical type, got %q", got)
	}

	// A mention comment matches the second group.
	res, err = tr.Apply(ctx, TransferInput{
		EventID:      "ch-test-fltr-mention",
		BirthChannel: src,
		Headers:      map[string]string{"X-Forgejo-Event": "issue_comment"},
		Payload:      json.RawMessage(`{"action":"created","comment":{"body":"@review-agent please look","user":{"login":"dpinsel"}}}`),
	})
	if err != nil {
		t.Fatalf("apply mention: %v", err)
	}
	if len(res) != 1 || len(queue.tasks) != 2 || inv.Len() != 2 {
		t.Fatalf("mention group must pass: results=%+v tasks=%d invocations=%d",
			res, len(queue.tasks), inv.Len())
	}

	// A comment by the agent itself is excluded from the mention group, and
	// a plain comment without the mention reaches no inbox.
	for _, evt := range []struct {
		id   string
		body string
		user string
	}{
		{"ch-test-fltr-self", "@review-agent ack", "review-agent"},
		{"ch-test-fltr-plain", "looks good overall", "dpinsel"},
	} {
		res, err = tr.Apply(ctx, TransferInput{
			EventID:      evt.id,
			BirthChannel: src,
			Headers:      map[string]string{"X-Forgejo-Event": "issue_comment"},
			Payload:      json.RawMessage(`{"action":"created","comment":{"body":"` + evt.body + `","user":{"login":"` + evt.user + `"}}}`),
		})
		if err != nil {
			t.Fatalf("apply %s: %v", evt.id, err)
		}
		if len(res) != 0 || inv.Len() != 2 {
			t.Fatalf("%s must be filtered out: results=%+v invocations=%d", evt.id, res, inv.Len())
		}
	}
}

// Bridge filters ride the delivery they gate: an event whose walk ends on a
// filtered edge must see exactly those filters. An unfiltered bridge stays
// unconditional — the shape pre-filter deployments created.
func TestBridgeFiltersRideTheDelivery(t *testing.T) {
	_, _, s, _, _ := newTestTransfer(t)
	ctx := context.Background()

	src := seed(t, s, KindConnector, uniqueName("ride-src"))
	group := seedCustom(t, s)
	dst := seed(t, s, KindAgent, uniqueName("ride-agent"))

	pushOnly := [][]ainselapishared.Filter{
		{{Field: "type", Op: "eq", Value: "push"}},
	}
	if _, err := s.CreateBridge(ctx, src, group, "", nil); err != nil {
		t.Fatalf("attach gather: %v", err)
	}
	b, err := s.CreateBridge(ctx, group, dst, "push only", pushOnly)
	if err != nil {
		t.Fatalf("attach dispatch: %v", err)
	}
	if len(b.Filters) != 1 || len(b.Filters[0]) != 1 || b.Filters[0][0].Field != "type" {
		t.Fatalf("CreateBridge returned %+v, want the filter set stored", b.Filters)
	}
	got, err := s.GetBridge(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetBridge: %v", err)
	}
	if len(got.Filters) != 1 || got.Filters[0][0].Value != "push" {
		t.Fatalf("filters did not round-trip through the store: %+v", got.Filters)
	}

	deliveries, err := s.Deliveries(ctx, src)
	if err != nil {
		t.Fatalf("Deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected one delivery, got %+v", deliveries)
	}
	if len(deliveries[0].BridgeFilters) != 1 || deliveries[0].BridgeFilters[0][0].Value != "push" {
		t.Fatalf("delivery should carry the gate it rides: %+v", deliveries[0].BridgeFilters)
	}

	// An edge with no filters stays unconditional in every read: a second
	// inbox attached without a gate gets its delivery with no filter set.
	dst2 := seed(t, s, KindAgent, uniqueName("ride-agent-2"))
	if _, err := s.CreateBridge(ctx, group, dst2, "", nil); err != nil {
		t.Fatalf("attach plain dispatch: %v", err)
	}
	deliveries, err = s.Deliveries(ctx, src)
	if err != nil {
		t.Fatalf("Deliveries after plain attach: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("expected two deliveries, got %+v", deliveries)
	}
	for _, d := range deliveries {
		if d.AgentChannel == dst2 && d.BridgeFilters != nil {
			t.Fatalf("unconditional bridge delivered a gate: %+v", d.BridgeFilters)
		}
		if d.AgentChannel == dst && len(d.BridgeFilters) != 1 {
			t.Fatalf("filtered bridge lost its gate: %+v", d.BridgeFilters)
		}
	}
}

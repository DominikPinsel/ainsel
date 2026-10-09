package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
)

// fakeEventQueue implements the two narrow *eventqueue.Store surfaces
// recordRetryInvocation depends on, without needing a PostgreSQL pool.
type fakeEventQueue struct {
	updatedTaskID    int64
	updatedInvID     string
	updatedHeaders   json.RawMessage
	updateErr        error
	connectors       map[string]string // eventID -> connector
	connectorMissing bool              // pretend the event row was pruned
	connectorErr     bool
}

func (f *fakeEventQueue) UpdateTaskInvocation(_ context.Context, taskID int64, invocationID string, headers json.RawMessage) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updatedTaskID = taskID
	f.updatedInvID = invocationID
	f.updatedHeaders = headers
	return nil
}

func (f *fakeEventQueue) EventConnector(_ context.Context, eventID string) (string, bool, error) {
	if f.connectorErr {
		return "", false, context.DeadlineExceeded
	}
	if f.connectorMissing {
		return "", false, nil
	}
	conn, ok := f.connectors[eventID]
	return conn, ok, nil
}

func retryTestTask() *eventqueue.Task {
	headers := json.RawMessage(`{"type":"issue_comment","X-Invocation-ID":"inv-first"}`)
	return &eventqueue.Task{
		ID:           1486,
		EventID:      "evt_test1",
		AgentName:    "a-b626d516",
		TriggerName:  "t-2a229c25",
		InvocationID: "inv-first",
		Headers:      headers,
		Payload:      json.RawMessage(`{"issue":328}`),
		Attempts:     2,
	}
}

// seedStore inserts one terminal 'failure' invocation for inv-first, mirroring
// what the nack handler left behind when the first attempt gave up.
func retrySeedStore(t *testing.T) invocations.Store {
	t.Helper()
	store := invocations.NewMemoryStore(100)
	store.Record(invocations.Invocation{
		ID:          "inv-first",
		AgentName:   "a-b626d516",
		TriggerName: "t-2a229c25",
		EventID:     "evt_test1",
		Connector:   "c-2cfb8617",
	})
	if !store.Complete("inv-first", invocations.StatusFailure, "Turn timed out after 1800000ms", time.Time{}) {
		t.Fatal("seeding inv-first completion failed")
	}
	return store
}

// A retry claim of a task whose first attempt failed records a fresh running
// invocation, repoints the task at it, and rewrites the cached header — while
// the closed first-attempt row stays untouched as history.
func TestRecordRetryInvocationSupersedesTerminalInvocation(t *testing.T) {
	rec := retrySeedStore(t)
	eq := &fakeEventQueue{connectors: map[string]string{"evt_test1": "c-2cfb8617"}}
	task := retryTestTask()

	if err := recordRetryInvocation(context.Background(), rec, eq, eq, task); err != nil {
		t.Fatalf("recordRetryInvocation: %v", err)
	}

	if task.InvocationID == "inv-first" || task.InvocationID == "" {
		t.Fatalf("task not repointed to a fresh invocation, id = %q", task.InvocationID)
	}
	if eq.updatedTaskID != 1486 || eq.updatedInvID != task.InvocationID {
		t.Errorf("store updated to task=%d inv=%q, want task=1486 inv=%q", eq.updatedTaskID, eq.updatedInvID, task.InvocationID)
	}

	// The fresh row is running and labelled like the original.
	fresh, ok := rec.Get(task.InvocationID)
	if !ok {
		t.Fatalf("fresh invocation %q not persisted", task.InvocationID)
	}
	if fresh.Status != invocations.StatusRunning {
		t.Errorf("fresh status = %q, want running", fresh.Status)
	}
	if fresh.AgentName != "a-b626d516" || fresh.TriggerName != "t-2a229c25" || fresh.EventID != "evt_test1" || fresh.Connector != "c-2cfb8617" {
		t.Errorf("fresh invocation fields wrong: %+v", fresh)
	}

	// The first attempt's row keeps its failure status and its error.
	orig, ok := rec.Get("inv-first")
	if !ok || !orig.IsTerminal() {
		t.Fatalf("first-attempt row must stay terminal, got ok=%v", ok)
	}

	// The header the runner sees carries the new id.
	var hdr map[string]any
	if err := json.Unmarshal(task.Headers, &hdr); err != nil {
		t.Fatalf("re-written headers not valid JSON: %v", err)
	}
	if hdr[headerInvocationID] != task.InvocationID {
		t.Errorf("header X-Invocation-ID = %v, want %q", hdr[headerInvocationID], task.InvocationID)
	}
	if hdr["type"] != "issue_comment" {
		t.Errorf("existing headers must survive the rewrite, got %v", hdr)
	}
}

func TestRecordRetryInvocationSkipsFirstClaim(t *testing.T) {
	rec := retrySeedStore(t)
	eq := &fakeEventQueue{connectors: map[string]string{"evt_test1": "c-2cfb8617"}}
	task := retryTestTask()
	task.Attempts = 1

	if err := recordRetryInvocation(context.Background(), rec, eq, eq, task); err != nil {
		t.Fatalf("recordRetryInvocation: %v", err)
	}
	if task.InvocationID != "inv-first" || eq.updatedTaskID != 0 {
		t.Errorf("first claim must be a no-op, got inv=%q store updated=%d", task.InvocationID, eq.updatedTaskID)
	}
}

func TestRecordRetryInvocationKeepsIdWhenRowMissing(t *testing.T) {
	// The capacity-pruned case: no invocation row exists for inv-first, so
	// there is nothing meaningful to supersede — keep the id the task has.
	rec := invocations.NewMemoryStore(100)
	eq := &fakeEventQueue{connectorMissing: true}
	task := retryTestTask()

	if err := recordRetryInvocation(context.Background(), rec, eq, eq, task); err != nil {
		t.Fatalf("recordRetryInvocation: %v", err)
	}
	if task.InvocationID != "inv-first" || eq.updatedTaskID != 0 {
		t.Errorf("missing row must be a no-op, got inv=%q store updated=%d", task.InvocationID, eq.updatedTaskID)
	}
}

func TestRecordRetryInvocationKeepsIdWhenStillRunning(t *testing.T) {
	rec := invocations.NewMemoryStore(100)
	rec.Record(invocations.Invocation{ID: "inv-first", AgentName: "a-b626d516", TriggerName: "t-2a229c25", EventID: "evt_test1"})
	eq := &fakeEventQueue{}
	task := retryTestTask()

	if err := recordRetryInvocation(context.Background(), rec, eq, eq, task); err != nil {
		t.Fatalf("recordRetryInvocation: %v", err)
	}
	if task.InvocationID != "inv-first" || eq.updatedTaskID != 0 {
		t.Errorf("running row must not be superseded, got inv=%q store updated=%d", task.InvocationID, eq.updatedTaskID)
	}
}

// A failed connector lookup degrades the label, not the visibility: the
// attempt is still recorded, with an empty connector.
func TestRecordRetryInvocationConnectorLookupError(t *testing.T) {
	rec := retrySeedStore(t)
	eq := &fakeEventQueue{connectorErr: true}
	task := retryTestTask()

	if err := recordRetryInvocation(context.Background(), rec, eq, eq, task); err != nil {
		t.Fatalf("recordRetryInvocation with connector error: %v", err)
	}
	fresh, ok := rec.Get(task.InvocationID)
	if !ok || fresh.Connector != "" {
		t.Errorf("fresh invocation connector = %q ok=%v, want \"\" and true", fresh.Connector, ok)
	}
}

func TestRecordRetryInvocationTaskWithoutInvocation(t *testing.T) {
	// Tasks enqueued without a recorded invocation (channel transfers, chat)
	// get their first one on a retry claim.
	rec := invocations.NewMemoryStore(100)
	eq := &fakeEventQueue{connectors: map[string]string{"evt_test1": "chat"}}
	task := retryTestTask()
	task.InvocationID = ""

	if err := recordRetryInvocation(context.Background(), rec, eq, eq, task); err != nil {
		t.Fatalf("recordRetryInvocation: %v", err)
	}
	if task.InvocationID == "" || eq.updatedTaskID != 1486 {
		t.Fatalf("task without invocation must gain one, got id=%q updated=%d", task.InvocationID, eq.updatedTaskID)
	}
	fresh, _ := rec.Get(task.InvocationID)
	if fresh.Connector != "chat" {
		t.Errorf("fresh connector = %q, want chat", fresh.Connector)
	}
}

func TestRecordRetryInvocationUpdaterFailureLeavesTaskAlone(t *testing.T) {
	rec := retrySeedStore(t)
	eq := &fakeEventQueue{updateErr: context.DeadlineExceeded}
	task := retryTestTask()

	if err := recordRetryInvocation(context.Background(), rec, eq, eq, task); err == nil {
		t.Fatal("expected the updater error to propagate")
	}
	if task.InvocationID != "inv-first" {
		t.Errorf("failed repoint must not mutate the returned task, inv=%q", task.InvocationID)
	}
}

func TestRewriteInvocationHeaderEdgeCases(t *testing.T) {
	if got := rewriteInvocationHeader(nil, "inv-x"); got != nil {
		t.Errorf("nil headers must pass through, got %s", got)
	}
	broken := json.RawMessage(`{not json`)
	if got := rewriteInvocationHeader(broken, "inv-x"); string(got) != string(broken) {
		t.Errorf("unparsable headers must pass through, got %s", got)
	}
}

package queuesignal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
)

// fakeCounter answers queue measurements from a map and records what was asked
// for, so tests can assert both the decision and the read.
type fakeCounter struct {
	mu       sync.Mutex
	depths   map[string]eventqueue.QueueDepth
	byAgent  map[string]error
	allCalls int
	seen     []string
}

func newCounter() *fakeCounter {
	return &fakeCounter{depths: map[string]eventqueue.QueueDepth{}, byAgent: map[string]error{}}
}

func (f *fakeCounter) set(agent string, pending, active int32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.depths[agent] = eventqueue.QueueDepth{Pending: pending, Active: active}
}

func (f *fakeCounter) failOn(agent string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byAgent[agent] = err
}

func (f *fakeCounter) QueueCounts(_ context.Context, agentName string) (eventqueue.QueueDepth, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = append(f.seen, agentName)
	if err, ok := f.byAgent[agentName]; ok {
		return eventqueue.QueueDepth{}, err
	}
	return f.depths[agentName], nil
}

func (f *fakeCounter) AllQueueCounts(context.Context) (map[string]eventqueue.QueueDepth, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allCalls++
	out := map[string]eventqueue.QueueDepth{}
	for name, d := range f.depths {
		if d.Pending != 0 || d.Active != 0 {
			out[name] = d
		}
	}
	return out, nil
}

// fakePatcher records published signals and can be told to fail.
type fakePatcher struct {
	mu       sync.Mutex
	patches  []published
	failWith error
}

type published struct {
	agent string
	sig   Signal
}

func (f *fakePatcher) Patch(_ context.Context, agentName string, sig Signal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return f.failWith
	}
	f.patches = append(f.patches, published{agent: agentName, sig: sig})
	return nil
}

func (f *fakePatcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.patches)
}

func (f *fakePatcher) last(t *testing.T) published {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.patches) == 0 {
		t.Fatal("no signal was published")
	}
	return f.patches[len(f.patches)-1]
}

func (f *fakePatcher) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.patches = nil
}

func TestPublishOne_FirstMeasurementIsPublishedImmediately(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher, WithLogger(discardLogger()))

	counter.set("busy", 2, 1)
	done, err := p.publishOne(context.Background(), "busy", false)
	if err != nil {
		t.Fatalf("publishOne: %v", err)
	}
	if !done {
		t.Error("publishOne() = not done; a first observation must be published, not deferred")
	}
	got := patcher.last(t).sig
	if got.Pending != 2 || got.Active != 1 {
		t.Errorf("published %+v, want pending 2 active 1", got)
	}
	if got.ObservedAt.IsZero() {
		t.Error("published signal has no observation timestamp; the operator cannot tell empty from stale")
	}
	if got.LastInvocation.IsZero() {
		t.Error("first observation of queued work must advance the idle clock")
	}
}

// Growth inside an already-busy queue changes only how many pods the operator
// wants, not whether the agent should be awake at all, so it is the case the
// debounce exists for: a burst of fifty arrivals must not become fifty status
// writes. What matters is that the newest count still goes out afterwards
// instead of being dropped -- see the second half of this test.
func TestPublishOne_WorkArrivingBeatsTheDebounce(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher,
		WithDebounce(time.Hour), // would swallow the update if treated as ordinary churn
		WithLogger(discardLogger()))

	counter.set("a", 1, 0)
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	patcher.reset()

	counter.set("a", 2, 0)
	done, err := p.publishOne(context.Background(), "a", false)
	if err != nil {
		t.Fatalf("publishOne: %v", err)
	}
	if done {
		t.Error("queue growth published straight through the debounce; a burst of arrivals becomes a write per arrival")
	}
	if patcher.count() != 0 {
		t.Errorf("published %d times, want the growth debounced", patcher.count())
	}

	// Deferred, never discarded: once the window passes the newest count is written.
	p.mu.Lock()
	p.nextPub["a"] = time.Time{}
	p.mu.Unlock()
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("publishOne after debounce: %v", err)
	}
	if got := patcher.last(t).sig; got.Pending != 2 {
		t.Errorf("pending = %d, want the deferred value 2", got.Pending)
	}
}

// The mirror image: a queue draining to empty is the only permission the
// operator gets to scale pods away, so it must not be debounced either.
func TestPublishOne_DrainingToEmptyBeatsTheDebounce(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher, WithDebounce(time.Hour), WithLogger(discardLogger()))

	counter.set("a", 0, 3)
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	patcher.reset()

	counter.set("a", 0, 0)
	done, err := p.publishOne(context.Background(), "a", false)
	if err != nil {
		t.Fatalf("publishOne: %v", err)
	}
	if !done {
		t.Error("an emptying queue must publish now; the agent cannot go dormant on a delayed signal")
	}
	if got := patcher.last(t).sig; !got.Empty() {
		t.Errorf("published %+v, want an empty queue", got)
	}
}

// Churn inside the same state is the debounced case, and a suppressed update
// must stay queued rather than being silently dropped.
func TestPublishOne_DebouncedUpdateStaysPending(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher, WithDebounce(time.Hour), WithLogger(discardLogger()))

	counter.set("a", 5, 0)
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	patcher.reset()

	// Active unchanged, queue still non-empty, pending shrank: ordinary churn.
	counter.set("a", 4, 0)
	done, err := p.publishOne(context.Background(), "a", false)
	if err != nil {
		t.Fatalf("publishOne: %v", err)
	}
	if done {
		t.Error("publishOne() reported done while the update was debounced; the new count would never be published")
	}
	if patcher.count() != 0 {
		t.Errorf("published %d times during debounce, want 0", patcher.count())
	}

	// Once the window passes, the suppressed value must go out.
	p.mu.Lock()
	p.nextPub["a"] = time.Time{}
	p.mu.Unlock()
	done, err = p.publishOne(context.Background(), "a", false)
	if err != nil {
		t.Fatalf("publishOne after debounce: %v", err)
	}
	if !done {
		t.Error("publishOne() still not done after the debounce window elapsed")
	}
	if got := patcher.last(t).sig; got.Pending != 4 {
		t.Errorf("pending = %d, want the debounced value 4", got.Pending)
	}
}

// A claim starting or finishing is the drain signal: the operator will not scale
// a pod away while tasks are claimed, so a stale active count would keep an idle
// agent warm (or, going the other way, delay the scale-down decision by a full
// debounce window). Immediate even though the queue never became empty.
func TestPublishOne_ClaimTransitionsAreImmediate(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher, WithDebounce(time.Hour), WithLogger(discardLogger()))

	counter.set("a", 5, 0)
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	patcher.reset()

	// A pod took a task: still non-empty both before and after.
	counter.set("a", 4, 1)
	done, err := p.publishOne(context.Background(), "a", false)
	if err != nil {
		t.Fatalf("publishOne: %v", err)
	}
	if !done {
		t.Error("a claim transition was debounced; the operator's drain rule is working off a stale active count")
	}
	if got := patcher.last(t).sig; got.Active != 1 || got.Pending != 4 {
		t.Errorf("published %+v, want pending 4 active 1", got)
	}
}

// New work must push the idle clock forward, or quiet is measured from the first
// task ever enqueued: an agent that drains would be judged idle immediately after
// finishing and go straight back to sleep, and the next event would pay a cold
// boot again.
func TestPublishOne_NewWorkResetsTheIdleClock(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher, WithDebounce(time.Hour), WithLogger(discardLogger()))

	counter.set("a", 1, 0)
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	first := patcher.last(t).sig.LastInvocation
	if first.IsZero() {
		t.Fatal("first observation left the idle clock unset")
	}
	patcher.reset()

	// The agent works through the queue, then something new arrives.
	counter.set("a", 0, 1)
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("drain publish: %v", err)
	}
	patcher.reset()

	counter.set("a", 2, 0)
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("reburst publish: %v", err)
	}
	sig := patcher.last(t).sig
	if sig.LastInvocation.IsZero() {
		t.Fatal("work arriving did not advance the idle clock; a drained agent would be judged quiet from its first-ever task")
	}
	if !sig.LastInvocation.After(first) {
		t.Errorf("idle clock %v did not move past %v", sig.LastInvocation, first)
	}

	// Draining without new work must leave the clock where it is.
	patcher.reset()
	counter.set("a", 0, 0)
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("empty publish: %v", err)
	}
	if got := patcher.last(t).sig; !got.LastInvocation.IsZero() {
		t.Errorf("an emptying queue advanced the idle clock to %v", got.LastInvocation)
	}
}

func TestPublishOne_IdenticalCountsNeedNoWrite(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher, WithLogger(discardLogger()))

	counter.set("a", 7, 2)
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	patcher.reset()

	done, err := p.publishOne(context.Background(), "a", false)
	if err != nil {
		t.Fatalf("publishOne: %v", err)
	}
	if !done {
		t.Error("nothing changed, so the agent should be considered settled")
	}
	if patcher.count() != 0 {
		t.Errorf("published %d times with no change, want 0", patcher.count())
	}
}

// The sweep republishes whether or not anything changed, which is what keeps the
// observation timestamp from going stale when the hub is alive but idle.
func TestPublishOne_ForceRepublishesUnchangedCounts(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher, WithLogger(discardLogger()))

	counter.set("a", 7, 2)
	if _, err := p.publishOne(context.Background(), "a", false); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	first := patcher.last(t).sig.ObservedAt
	patcher.reset()

	time.Sleep(time.Millisecond)
	done, err := p.publishOne(context.Background(), "a", true)
	if err != nil {
		t.Fatalf("publishOne forced: %v", err)
	}
	if !done {
		t.Error("forced publish reported not done")
	}
	if patcher.count() != 1 {
		t.Fatalf("published %d times, want 1 from the sweep", patcher.count())
	}
	got := patcher.last(t).sig
	if !got.ObservedAt.After(first) {
		t.Errorf("observation timestamp %v did not advance past %v", got.ObservedAt, first)
	}
	if !got.LastInvocation.IsZero() {
		t.Errorf("a heartbeat republish advanced the idle clock to %v: nothing new arrived, so the agent must still be judged quiet from its last real task", got.LastInvocation)
	}
}

func TestPublishOne_MissingAgentIsForgotten(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	patcher.failWith = apierrors.NewNotFound(schema.GroupResource{
		Group:    "ainsel.dev",
		Resource: "agents",
	}, "gone")
	p := New(counter, patcher, WithLogger(discardLogger()))

	counter.set("gone", 3, 0)
	done, err := p.publishOne(context.Background(), "gone", false)
	if err != nil {
		t.Fatalf("publishOne for a deleted agent: %v", err)
	}
	if !done {
		t.Error("a deleted agent must be dropped, not retried forever")
	}
	if _, ok := p.Published("gone"); ok {
		t.Error("publisher still remembers a deleted agent; the sweep would resurrect it")
	}
}

func TestPublishOne_PatchFailureIsRetried(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	patcher.failWith = errors.New("apiserver unavailable")
	p := New(counter, patcher, WithLogger(discardLogger()))

	counter.set("a", 1, 0)
	if _, err := p.publishOne(context.Background(), "a", false); err == nil {
		t.Fatal("publishOne() = nil, want the patch error surfaced for retry")
	}
	if _, ok := p.Published("a"); ok {
		t.Error("publisher recorded a signal it never wrote; debounce would now skip it")
	}
}

func TestPublishOne_CountFailurePropagates(t *testing.T) {
	counter := newCounter()
	counter.failOn("a", errors.New("db down"))
	p := New(counter, &fakePatcher{}, WithLogger(discardLogger()))

	done, err := p.publishOne(context.Background(), "a", false)
	if err == nil {
		t.Fatal("publishOne() = nil, want the measurement error")
	}
	if done {
		t.Error("publishOne() = done after a failed measurement; the agent would stop being watched")
	}
}

func TestObserveDoesNoIOLikePublishing(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher, WithLogger(discardLogger()))

	p.Observe("a")
	if patcher.count() != 0 {
		t.Error("Observe published synchronously; it runs on the request path and must only mark")
	}
	p.mu.Lock()
	dirty := p.dirty["a"]
	p.mu.Unlock()
	if !dirty {
		t.Error("Observe did not mark the agent for the publish loop")
	}
}

// After a hub restart, in-memory history is gone while tasks may have piled up
// meanwhile. The first round must find them, or those agents stay asleep with
// work waiting.
func TestRunRepublishesQueuedWorkWithNoObservations(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher,
		WithTick(5*time.Millisecond),
		WithSweep(5*time.Millisecond),
		WithLogger(discardLogger()))

	counter.set("orphan", 2, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = p.Run(ctx) }()

	deadline := time.After(2 * time.Second)
	for {
		if patcher.count() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Run published nothing for an agent with queued work and no observations")
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}
	if sig, ok := p.Published("orphan"); !ok || sig.Pending != 2 {
		t.Errorf("published state = %+v (found %v), want pending 2 for orphan", sig, ok)
	}
}

// An agent that drained while the hub was down must also be corrected, or it
// stays marked busy.
func TestMarkAllCatchesDrainedAgents(t *testing.T) {
	counter, patcher := newCounter(), &fakePatcher{}
	p := New(counter, patcher, WithLogger(discardLogger()))

	// Pretend we previously said "a" was busy, and it has since finished.
	p.mu.Lock()
	p.last["a"] = Signal{Pending: 0, Active: 2}
	p.mu.Unlock()

	p.markAll(context.Background())

	p.mu.Lock()
	dirty := p.dirty["a"]
	p.mu.Unlock()
	if !dirty {
		t.Error("markAll missed an agent published busy that now has no rows; it would never go dormant")
	}
}

func TestMergePatchBodyWritesZerosExplicitly(t *testing.T) {
	body, err := mergePatchBody(Signal{
		Pending:        0,
		Active:         0,
		ObservedAt:     time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
		LastInvocation: time.Date(2026, 3, 4, 5, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("mergePatchBody: %v", err)
	}
	var doc struct {
		Status map[string]json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("patch body is not valid JSON: %v", err)
	}
	// The counts are omitempty in Go, so a zero written through a marshalled
	// struct would vanish and an agent that drained would never be told so.
	for _, key := range []string{"pendingTasks", "activeTasks", "queueObservedAt"} {
		if _, ok := doc.Status[key]; !ok {
			t.Errorf("patch has no %q key; a drain to zero must be expressible: %s", key, body)
		}
	}
	if string(doc.Status["pendingTasks"]) != "0" {
		t.Errorf("pendingTasks = %s, want a literal 0", doc.Status["pendingTasks"])
	}
}

func TestMergePatchBodyLeavesIdleClockAloneWhenUnset(t *testing.T) {
	body, err := mergePatchBody(Signal{ObservedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("mergePatchBody: %v", err)
	}
	if strings.Contains(string(body), "lastInvocation") {
		t.Errorf("patch sets lastInvocation with no new work; the agent could never be judged quiet: %s", body)
	}
}

// mergePatchBody must be the exact bytes K8sPatcher sends, so the two cannot
// drift apart.
func TestK8sPatcherUsesMergePatchBody(t *testing.T) {
	want, err := mergePatchBody(Signal{Pending: 1, Active: 1, ObservedAt: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("mergePatchBody: %v", err)
	}
	if !json.Valid(want) {
		t.Fatal("mergePatchBody produced invalid JSON")
	}
	if !strings.Contains(string(want), `"pendingTasks":1`) {
		t.Errorf("patch missing pendingTasks: %s", want)
	}
}

// discardLogger keeps the publisher's retry warnings out of test output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

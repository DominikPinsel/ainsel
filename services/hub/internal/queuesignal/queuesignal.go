// Package queuesignal publishes per-agent queue depth onto Agent status, so the
// agent operator can scale pods from real work instead of a standing replica
// count.
//
// The hub owns the task store, so it is the only component that knows whether an
// agent has work waiting. The operator owns the Deployment. This package is the
// seam between them: it watches queue transitions, re-measures the affected
// agent's queue, and writes the four queue fields on Agent status.
//
// Writes are edge-triggered with a debounce in between. A queue going from empty
// to non-empty is published on the next tick, because that is the case where
// latency is visible to a human: a chat message or webhook is sitting behind a
// pod that still has to be created, scheduled and booted. Movement inside a
// non-empty queue is debounced, because the number only matters near the replica
// ceiling and a burst of fifty events is fifty measurements of the same "yes,
// there is work".
//
// Every published signal carries an observation timestamp, and a periodic sweep
// republishes known agents whether or not anything changed. That heartbeat is
// what lets the operator tell "this queue is empty" from "nobody has looked at
// this queue since the hub died", and treat the latter as unknown rather than as
// permission to sleep.
package queuesignal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	v1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

// Defaults for New. Tick is the wake-latency budget: an enqueued task waits at
// most one tick to become visible to the operator, against a pod boot that takes
// seconds. Debounce caps write frequency for agents with continuously busy
// queues. Sweep is the heartbeat interval, and therefore sets the floor for how
// long the operator may wait before calling a published signal stale.
const (
	DefaultTick     = 250 * time.Millisecond
	DefaultDebounce = 2 * time.Second
	DefaultSweep    = 60 * time.Second
)

// Signal is what gets written to Agent status.
type Signal struct {
	Pending int32
	Active  int32
	// ObservedAt stamps when Pending/Active were measured. The operator uses it
	// to tell "queue is empty" from "nobody has looked lately".
	ObservedAt time.Time
	// LastInvocation, when non-zero, advances the agent's idle clock. It is set
	// only when work arrived, so it means "the last time this agent was handed
	// something to do", and scale-down measures quiet against it.
	LastInvocation time.Time
}

// Empty reports whether an agent has no work waiting or in flight.
func (s Signal) Empty() bool { return s.Pending == 0 && s.Active == 0 }

// sameCounts reports whether a re-measurement says the same thing as the last
// published signal, ignoring timestamps.
func sameCounts(a, b Signal) bool { return a.Pending == b.Pending && a.Active == b.Active }

// Counter measures queue depth. Satisfied by *eventqueue.Store.
type Counter interface {
	QueueCounts(ctx context.Context, agentName string) (eventqueue.QueueDepth, error)
	AllQueueCounts(ctx context.Context) (map[string]eventqueue.QueueDepth, error)
}

// Patcher writes a signal to one agent. Split out from the k8s implementation so
// the publisher's decisions can be tested without a cluster.
type Patcher interface {
	Patch(ctx context.Context, agentName string, sig Signal) error
}

// Publisher coalesces queue-change observations into status writes.
//
// Create with New, start with Run. Observe is safe to call from any goroutine and
// never blocks, which is what makes it usable from the hub's request paths.
type Publisher struct {
	counters Counter
	patcher  Patcher
	tick     time.Duration
	debounce time.Duration
	sweep    time.Duration
	log      *slog.Logger

	mu      sync.Mutex
	dirty   map[string]bool      // agents with unpublished changes
	last    map[string]Signal    // last signal written per agent
	nextPub map[string]time.Time // earliest time a non-edge update may be written
}

// New builds a Publisher. A nil logger becomes slog.Default().
func New(counters Counter, patcher Patcher, opts ...Option) *Publisher {
	p := &Publisher{
		counters: counters,
		patcher:  patcher,
		tick:     DefaultTick,
		debounce: DefaultDebounce,
		sweep:    DefaultSweep,
		log:      slog.Default(),
		dirty:    map[string]bool{},
		last:     map[string]Signal{},
		nextPub:  map[string]time.Time{},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Option tunes a Publisher for tests or unusual deployments.
type Option func(*Publisher)

// WithTick sets the publish loop interval (wake-latency budget).
func WithTick(d time.Duration) Option { return func(p *Publisher) { p.tick = d } }

// WithDebounce sets the minimum interval between non-edge updates for one agent.
func WithDebounce(d time.Duration) Option { return func(p *Publisher) { p.debounce = d } }

// WithSweep sets how often known agents are re-measured and republished, which
// repairs failed writes, catches changes the store did not report, and keeps the
// observation timestamp fresh.
func WithSweep(d time.Duration) Option { return func(p *Publisher) { p.sweep = d } }

// WithLogger directs the publisher's logs.
func WithLogger(l *slog.Logger) Option { return func(p *Publisher) { p.log = l } }

// Observe records that an agent's queue may have changed. It never blocks and
// never performs I/O; it is safe to call inline from a request handler.
func (p *Publisher) Observe(agentName string) {
	if agentName == "" {
		return
	}
	p.mu.Lock()
	p.dirty[agentName] = true
	p.mu.Unlock()
}

// Run drives the publish loop until ctx is cancelled.
//
// The first sweep after startup republishes everything the queue currently
// holds: last-written state lives in memory, so agents whose tasks arrived while
// the hub was down would otherwise stay invisible to the operator.
func (p *Publisher) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.tick)
	defer ticker.Stop()
	sweep := time.NewTicker(p.sweep)
	defer sweep.Stop()

	p.log.Info("queue signal publisher started",
		"tick", p.tick.String(), "debounce", p.debounce.String(), "sweep", p.sweep.String())

	// Immediate first sweep rather than waiting out the sweep interval.
	p.markAll(ctx)
	p.publishRound(ctx, false)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			p.publishRound(ctx, false)
		case <-sweep.C:
			p.markAll(ctx)
			p.publishRound(ctx, true)
		}
	}
}

// markAll measures the queue and marks agents whose published state disagrees,
// plus every agent with work in flight or waiting.
func (p *Publisher) markAll(ctx context.Context) {
	counts, err := p.counters.AllQueueCounts(ctx)
	if err != nil {
		p.log.Error("queue signal: measurement failed", "error", err)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for name := range counts {
		p.dirty[name] = true
	}
	// Anything we last said was busy but is absent from the busy set has drained
	// without telling us, so it needs a correction too.
	for name, sig := range p.last {
		if !sig.Empty() {
			if _, ok := counts[name]; !ok {
				p.dirty[name] = true
			}
		}
	}
}

// publishRound re-measures and writes the marked agents. When force is set, the
// debounce is bypassed so the sweep can keep observation timestamps fresh.
func (p *Publisher) publishRound(ctx context.Context, force bool) {
	p.mu.Lock()
	names := make([]string, 0, len(p.dirty))
	for name := range p.dirty {
		names = append(names, name)
	}
	p.dirty = map[string]bool{}
	p.mu.Unlock()

	for _, name := range names {
		done, err := p.publishOne(ctx, name, force)
		if err != nil {
			p.log.Warn("queue signal: publish failed, will retry", "agent", name, "error", err)
			p.mu.Lock()
			p.dirty[name] = true // retry on the next tick, throttled by debounce
			p.mu.Unlock()
			continue
		}
		if !done {
			p.mu.Lock()
			p.dirty[name] = true // still debounced; not forgotten
			p.mu.Unlock()
		}
	}
}

// publishOne measures one agent and writes it if the debounce rules permit.
// done reports whether the agent has nothing left to say.
func (p *Publisher) publishOne(ctx context.Context, name string, force bool) (bool, error) {
	depth, err := p.counters.QueueCounts(ctx, name)
	if err != nil {
		return false, fmt.Errorf("count: %w", err)
	}

	now := time.Now()
	p.mu.Lock()
	prev, hadPrev := p.last[name]
	sig := Signal{Pending: depth.Pending, Active: depth.Active, ObservedAt: now}
	// Work arrived: advance the idle clock, so an agent cannot be judged quiet
	// while tasks are queued for it.
	if !hadPrev || depth.Pending > prev.Pending {
		sig.LastInvocation = now
	}
	// An edge — the queue emptying or filling, or a claim starting or finishing —
	// must be published immediately. Anything else waits for the debounce window,
	// and identical counts need no write at all.
	edge := !hadPrev || prev.Empty() != sig.Empty() || prev.Active != sig.Active
	switch {
	case hadPrev && sameCounts(prev, sig) && !force && !edge:
		p.mu.Unlock()
		return true, nil
	case !edge && !force && now.Before(p.nextPub[name]):
		p.mu.Unlock()
		return false, nil // debounced, retried on a later round
	}
	p.nextPub[name] = now.Add(p.debounce)
	p.mu.Unlock()

	if err := p.patcher.Patch(ctx, name, sig); err != nil {
		if apierrors.IsNotFound(err) {
			// No Agent CR to publish to: deleted, or a task routed to a name that
			// never existed. Forget it, so the sweep stops resurrecting it.
			p.mu.Lock()
			delete(p.last, name)
			delete(p.nextPub, name)
			p.mu.Unlock()
			p.log.Debug("queue signal: agent not found, dropping", "agent", name)
			return true, nil
		}
		return false, fmt.Errorf("patch: %w", err)
	}

	p.mu.Lock()
	p.last[name] = sig
	p.mu.Unlock()
	return true, nil
}

// Published reports the last signal written for an agent. Exposed for tests and
// for the hub's own diagnostics.
func (p *Publisher) Published(name string) (Signal, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sig, ok := p.last[name]
	return sig, ok
}

// mergePatchBody renders the JSON merge patch for one agent's queue fields.
//
// The keys are named explicitly rather than marshalling a struct: the counts are
// omitempty in the Go type, so a zero would drop out of the document and an agent
// whose queue had drained would never be told it was empty.
func mergePatchBody(sig Signal) ([]byte, error) {
	status := map[string]any{
		"pendingTasks":    sig.Pending,
		"activeTasks":     sig.Active,
		"queueObservedAt": metav1.NewTime(sig.ObservedAt.UTC()),
	}
	if !sig.LastInvocation.IsZero() {
		status["lastInvocation"] = metav1.NewTime(sig.LastInvocation.UTC())
	}
	body, err := json.Marshal(map[string]any{"status": status})
	if err != nil {
		return nil, fmt.Errorf("queuesignal: marshal status patch: %w", err)
	}
	return body, nil
}

// K8sPatcher writes queue signals onto Agent status subresources.
//
// It sends a raw JSON merge patch naming exactly the fields the hub owns, rather
// than a read-modify-update, for two reasons. Both the hub and the operator write
// status, and an update would let either clobber the other's fields. And the
// counts are omitempty in the Go struct, so a marshalled zero would vanish from
// the document and an agent whose queue drained would never be told so.
type K8sPatcher struct {
	client    client.Client
	namespace string
	log       *slog.Logger
}

// NewK8sPatcher builds a patcher writing to agents in the given namespace.
func NewK8sPatcher(c client.Client, namespace string) *K8sPatcher {
	return &K8sPatcher{client: c, namespace: namespace, log: slog.Default()}
}

// WithLogger returns the patcher, logging to l.
func (k *K8sPatcher) WithLogger(l *slog.Logger) *K8sPatcher {
	k.log = l
	return k
}

// Patch writes sig to the named agent's status.
func (k *K8sPatcher) Patch(ctx context.Context, agentName string, sig Signal) error {
	body, err := mergePatchBody(sig)
	if err != nil {
		return err
	}

	obj := &v1alpha1.Agent{}
	obj.SetName(agentName)
	obj.SetNamespace(k.namespace)
	if err := k.client.Status().Patch(ctx, obj, client.RawPatch(types.MergePatchType, body)); err != nil {
		if apierrors.IsNotFound(err) {
			return err // unwrapped, so IsNotFound still sees it upstream
		}
		return fmt.Errorf("queuesignal: patch agent %q status: %w", agentName, err)
	}
	return nil
}

package cron

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/metrics"
	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
	cronparse "github.com/DominikPinsel/ainsel/shared/api/cron"
)

// ConnectorName is the canonical connector value stamped on cron-emitted
// events. The agent runtime recognises it and renders the event's prompt
// verbatim instead of the forgejo event template.
const ConnectorName = "cron"

// Header names propagated to agents (mirrors the router so the agent runtime
// and invocation tracking treat cron fires identically to webhook events).
const (
	headerTriggerName  = "X-Trigger-Name"
	headerInvocationID = "X-Invocation-ID"
)

// fireFn is the signature of the function used to publish a cron event to an
// agent's task queue. Extracted so tests can capture publishes without a
// live database connection.
type fireFn func(agentRef, triggerName, invocationID string, event ainselapishared.Event) error

// entry is the in-memory representation of a reconciled CronTrigger.
type entry struct {
	id        string // trigger id (used as key and trigger name)
	agentRef  string
	prompt    string
	schedule  *cronparse.Schedule
	enabled   bool
	lastFired time.Time
	nextRun   time.Time
}

// Emitter watches CronTrigger resources (via informer callbacks) and fires
// them on schedule, inserting a synthetic event and enqueuing a task for the
// target agent. It is the cron equivalent of the webhook-driven router.
type Emitter struct {
	mu       sync.RWMutex
	entries  map[string]*entry
	eq       *eventqueue.Store
	invStore *invocations.Store
	now      func() time.Time
	fire     fireFn
}

// New creates an Emitter backed by the given event queue store and invocation
// store.
func New(eq *eventqueue.Store, invStore *invocations.Store) *Emitter {
	e := &Emitter{
		entries:  make(map[string]*entry),
		eq:       eq,
		invStore: invStore,
		now:      time.Now,
	}
	e.fire = e.publish
	return e
}

// Upsert adds or replaces a CronTrigger in the schedule.
func (e *Emitter) Upsert(ct *triggers.CronTrigger) {
	if ct == nil {
		return
	}
	id := ct.ID

	sched, err := cronparse.Parse(ct.Schedule)
	if err != nil {
		slog.Warn("cron trigger has invalid schedule, skipping", "trigger", id, "schedule", ct.Schedule, "error", err)
		e.mu.Lock()
		delete(e.entries, id)
		e.mu.Unlock()
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	var lastFired time.Time
	if prev, ok := e.entries[id]; ok {
		lastFired = prev.lastFired
	} else {
		lastFired = e.now()
	}
	nextRun := sched.Next(lastFired)
	e.entries[id] = &entry{
		id:        id,
		agentRef:  ct.AgentRef,
		prompt:    ct.Prompt,
		schedule:  sched,
		enabled:   ct.Enabled,
		lastFired: lastFired,
		nextRun:   nextRun,
	}
	slog.Info("cron trigger upserted", "trigger", id, "agent", ct.AgentRef, "schedule", ct.Schedule, "nextRun", nextRun)
}

// Delete removes a CronTrigger from the schedule.
func (e *Emitter) Delete(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.entries, key)
	slog.Info("cron trigger removed", "trigger", key)
}

// Run starts the scheduling loop, blocking until ctx is cancelled.
func (e *Emitter) Run(ctx context.Context) error {
	slog.Info("cron emitter started")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			e.tick(now)
		}
	}
}

// tick evaluates all entries against the current time and fires any that are due.
func (e *Emitter) tick(now time.Time) {
	e.mu.RLock()
	due := make([]*entry, 0)
	for _, en := range e.entries {
		if !en.enabled {
			continue
		}
		next := en.schedule.Next(en.lastFired)
		if next.IsZero() {
			continue
		}
		if !next.After(now) {
			due = append(due, en)
		}
	}
	e.mu.RUnlock()

	for _, en := range due {
		e.fireEntry(en)
	}
}

// fireEntry publishes a cron event for the given entry and advances its
// lastFired/nextRun.
func (e *Emitter) fireEntry(en *entry) {
	fireTime := en.schedule.Next(en.lastFired)
	if fireTime.IsZero() {
		return
	}

	// Build the canonical event.
	data, _ := json.Marshal(map[string]string{
		"cronTrigger": en.id,
		"prompt":      en.prompt,
	})
	evt := ainselapishared.Event{
		ID:        newEventID(),
		Version:   "1",
		Connector: ConnectorName,
		Timestamp: fireTime,
		Headers:   map[string]string{"type": ConnectorName},
		Data:      data,
	}

	var invID string
	if e.invStore != nil {
		rec := e.invStore.Record(invocations.Invocation{
			AgentName:   en.agentRef,
			TriggerName: en.id,
			EventID:     evt.ID,
			Connector:   ConnectorName,
		})
		invID = rec.ID
	}

	if err := e.fire(en.agentRef, en.id, invID, evt); err != nil {
		slog.Error("cron fire publish failed", "trigger", en.id, "agent", en.agentRef, "error", err)
		metrics.RoutingErrors.Inc()
		if e.invStore != nil && invID != "" {
			e.invStore.Complete(invID, invocations.StatusFailure, "cron publish failed: "+err.Error(), time.Time{})
		}
		return
	}

	metrics.CronFires.Inc()
	slog.Info("cron fire routed to agent",
		"trigger", en.id,
		"agent", en.agentRef,
		"invocation_id", invID,
		"fire_time", fireTime.Format(time.RFC3339),
	)

	e.mu.Lock()
	if cur, ok := e.entries[en.id]; ok {
		cur.lastFired = fireTime
		cur.nextRun = cur.schedule.Next(fireTime)
	}
	e.mu.Unlock()
}

// publish inserts the cron event into the events table and enqueues a task
// for the target agent.
func (e *Emitter) publish(agentRef, triggerName, invocationID string, evt ainselapishared.Event) error {
	ctx := context.Background()

	// Insert the synthetic event.
	headersJSON, _ := json.Marshal(evt.Headers)
	if err := e.eq.InsertEvent(ctx, eventqueue.Event{
		ID:        evt.ID,
		Connector: evt.Connector,
		Headers:   headersJSON,
		Data:      evt.Data,
		Raw:       string(evt.Data),
	}); err != nil {
		return fmt.Errorf("insert cron event: %w", err)
	}

	// Mark it as routed immediately (cron events bypass the router).
	if err := e.eq.MarkRouted(ctx, evt.ID); err != nil {
		return fmt.Errorf("mark cron event routed: %w", err)
	}

	// Enqueue the task for the agent.
	taskHeaders := map[string]string{
		"type":            ConnectorName,
		headerTriggerName: triggerName,
	}
	if invocationID != "" {
		taskHeaders[headerInvocationID] = invocationID
	}
	taskHeadersJSON, _ := json.Marshal(taskHeaders)

	if err := e.eq.EnqueueTask(ctx, eventqueue.Task{
		EventID:      evt.ID,
		AgentName:    agentRef,
		TriggerName:  triggerName,
		InvocationID: invocationID,
		Headers:      taskHeadersJSON,
		Payload:      evt.Data,
	}); err != nil {
		return fmt.Errorf("enqueue cron task for agent %s: %w", agentRef, err)
	}
	return nil
}

// SetNow replaces the clock used to compute initial next-run times. For tests.
func (e *Emitter) SetNow(fn func() time.Time) { e.now = fn }

// SetFire replaces the publish function. For tests.
func (e *Emitter) SetFire(fn fireFn) { e.fire = fn }

// EntryCount returns the number of scheduled entries. For tests/inspection.
func (e *Emitter) EntryCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.entries)
}

// Keys returns the IDs of all cron triggers currently in the emitter.
func (e *Emitter) Keys() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	keys := make([]string, 0, len(e.entries))
	for k := range e.entries {
		keys = append(keys, k)
	}
	return keys
}

// newEventID returns a short random id for a cron event.
func newEventID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "cron-" + hex.EncodeToString(b)
}

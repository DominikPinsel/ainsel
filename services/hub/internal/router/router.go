package router

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/metrics"
	"github.com/DominikPinsel/ainsel/services/hub/internal/trigger"
	"github.com/DominikPinsel/ainsel/services/hub/internal/types"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

// Header names used to propagate routing metadata to agents.
const (
	headerTriggerName  = "X-Trigger-Name"
	headerInvocationID = "X-Invocation-ID"
)

// Broadcaster is implemented by the API server to push events to WebSocket clients.
type Broadcaster interface {
	BroadcastError(types.ErrorEntry)
	BroadcastEvent(types.ActivityEntry)
	BroadcastStats(ctx context.Context)
}

// Queue is the subset of eventqueue.Store used by the Router.
// Extracted as an interface for unit testing.
type Queue interface {
	FetchUnrouted(ctx context.Context, limit int) ([]eventqueue.Event, error)
	MarkRouted(ctx context.Context, eventID string) error
	EnqueueTask(ctx context.Context, task eventqueue.Task) error
}

// Router polls the PostgreSQL events table for unrouted events, matches them
// against triggers, and enqueues agent tasks. It replaces the former
// JetStream consumer.
type Router struct {
	index       *trigger.Index
	eq          Queue
	broadcaster Broadcaster
	invocations *invocations.Store
	pollInterval time.Duration
}

// New creates a Router backed by the PostgreSQL event queue.
func New(eq Queue, index *trigger.Index, broadcaster Broadcaster, invStore *invocations.Store) *Router {
	return &Router{
		index:        index,
		eq:           eq,
		broadcaster:  broadcaster,
		invocations:  invStore,
		pollInterval: 2 * time.Second,
	}
}

// Run starts the event polling loop, blocking until ctx is cancelled.
func (r *Router) Run(ctx context.Context) error {
	slog.Info("router started, polling events table")

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.poll(ctx)
		}
	}
}

// poll fetches unrouted events and processes them.
func (r *Router) poll(ctx context.Context) {
	events, err := r.eq.FetchUnrouted(ctx, 10)
	if err != nil {
		slog.Error("router: fetch unrouted events", "error", err)
		return
	}

	for _, evt := range events {
		r.handleEvent(ctx, evt)
	}
}

// handleEvent processes a single event: match triggers, enqueue tasks, mark routed.
func (r *Router) handleEvent(ctx context.Context, evt eventqueue.Event) {
	metrics.EventsConsumed.Inc()

	// Reconstruct the shared Event struct for trigger matching.
	sharedEvt := ainselapishared.Event{
		ID:        evt.ID,
		Version:   "1",
		Connector: evt.Connector,
		Timestamp: time.Now().UTC(),
	}
	// Unmarshal headers and data for the filter engine.
	if evt.Headers != nil {
		_ = json.Unmarshal(evt.Headers, &sharedEvt.Headers)
	}
	if sharedEvt.Headers == nil {
		sharedEvt.Headers = make(map[string]string)
	}
	sharedEvt.Data = ainselapishared.RawJSON(evt.Data)
	sharedEvt.Raw = evt.Raw

	matches := r.index.Match(&sharedEvt)

	if len(matches) == 0 {
		slog.Debug("no matches for event",
			"eventType", sharedEvt.Headers["type"],
			"connector", evt.Connector,
		)
	}

	// Deduplicate by agent.
	matches = trigger.DeduplicateByAgent(matches)

	if len(matches) == 0 {
		slog.Info("activity_event",
			"log_type", "activity_event",
			"connector", evt.Connector,
			"status", "unmatched",
			"payload", string(evt.Data),
		)
		if r.broadcaster != nil {
			r.broadcaster.BroadcastEvent(types.ActivityEntry{
				Connector: evt.Connector,
				Status:    "unmatched",
				Payload:   evt.Data,
			})
			r.broadcaster.BroadcastStats(context.WithoutCancel(ctx))
		}
		_ = r.eq.MarkRouted(ctx, evt.ID)
		return
	}

	metrics.TriggersMatched.Add(float64(len(matches)))
	slog.Info("event matched", "type", sharedEvt.Headers["type"], "connector", evt.Connector, "matches", len(matches))

	entryMatches := make([]types.MatchResult, 0, len(matches))
	status := "matched"
	allEnqueued := true

	for _, m := range matches {
		// Record an invocation before enqueuing.
		var invID string
		if r.invocations != nil {
			rec := r.invocations.Record(invocations.Invocation{
				AgentName:   m.AgentRef,
				TriggerName: m.TriggerName,
				EventID:     evt.ID,
				Connector:   evt.Connector,
			})
			invID = rec.ID
		}

		// Build headers for the agent task.
		// Include the original event headers so the agent runner can
		// determine the event type (e.g. "issue_assign").
		taskHeaders := make(map[string]string, len(sharedEvt.Headers)+3)
		for k, v := range sharedEvt.Headers {
			taskHeaders[k] = v
		}
		// Add the canonical event type so the runner can use a single
		// platform-independent header instead of scanning for
		// provider-specific X-*-Event headers.
		if t := trigger.CanonicalEventType(sharedEvt.Headers); t != "" {
			taskHeaders["type"] = t
		}
		taskHeaders[headerTriggerName] = m.TriggerName
		if invID != "" {
			taskHeaders[headerInvocationID] = invID
		}
		taskHeadersJSON, _ := json.Marshal(taskHeaders)

		// The payload is the full event JSON the agent receives.
		payload := evt.Data

		task := eventqueue.Task{
			EventID:      evt.ID,
			AgentName:    m.AgentRef,
			TriggerName:  m.TriggerName,
			InvocationID: invID,
			Headers:      taskHeadersJSON,
			Payload:      payload,
		}

		if err := r.eq.EnqueueTask(ctx, task); err != nil {
			slog.Error("enqueue task for agent", "agent", m.AgentRef, "error", err)
			metrics.RoutingErrors.Inc()
			status = "error"
			allEnqueued = false

			if r.invocations != nil && invID != "" {
				r.invocations.Complete(invID, invocations.StatusFailure, "enqueue task failed: "+err.Error(), time.Time{})
			}

			slog.Info("error_event",
				"log_type", "error_event",
				"severity", "error",
				"source", "router",
				"error_message", "failed to route event to agent: "+err.Error(),
				"agent", m.AgentRef,
				"trigger", m.TriggerName,
				"eventType", sharedEvt.Headers["type"],
				"invocation_id", invID,
			)

			if r.broadcaster != nil {
				r.broadcaster.BroadcastError(types.ErrorEntry{
					Severity: "error",
					Source:   "router",
					Message:  "failed to route event to agent: " + err.Error(),
					Details:  map[string]interface{}{"agent": m.AgentRef, "trigger": m.TriggerName, "eventType": sharedEvt.Headers["type"], "invocationId": invID},
				})
			}
		} else {
			slog.Info("routed to agent",
				"agent", m.AgentRef,
				"trigger", m.TriggerName,
				"invocation_id", invID,
			)
			metrics.EventsRouted.Inc()
		}
		entryMatches = append(entryMatches, types.MatchResult{
			Trigger: m.TriggerName,
			Agent:   m.AgentRef,
		})
	}

	// Log activity event.
	matchesJSON, _ := json.Marshal(entryMatches)
	slog.Info("activity_event",
		"log_type", "activity_event",
		"connector", evt.Connector,
		"status", status,
		"matches", string(matchesJSON),
		"payload", string(evt.Data),
	)

	if r.broadcaster != nil {
		r.broadcaster.BroadcastEvent(types.ActivityEntry{
			Connector: evt.Connector,
			Status:    status,
			Matches:   entryMatches,
			Payload:   evt.Data,
		})
		r.broadcaster.BroadcastStats(context.WithoutCancel(ctx))
	}

	// Mark event as routed regardless of individual task failures.
	// Failed tasks are already marked in the invocation store.
	if allEnqueued {
		_ = r.eq.MarkRouted(ctx, evt.ID)
	} else {
		// Still mark as routed to avoid infinite retry loops.
		// Individual task failures are tracked via invocations.
		_ = r.eq.MarkRouted(ctx, evt.ID)
		slog.Warn("event routed with some task failures",
			"eventID", evt.ID,
			"connector", evt.Connector,
			"matches", len(matches),
		)
	}
}

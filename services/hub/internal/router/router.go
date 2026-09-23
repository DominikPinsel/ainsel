package router

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/channels"
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

// Transferer moves an event along the subscriptions stored on the channel
// graph. Implemented by *channels.Transfer; extracted as an interface so the
// router's fan-out is testable without a database.
type Transferer interface {
	Apply(ctx context.Context, in channels.TransferInput) ([]channels.TransferResult, error)
}

// Router polls the PostgreSQL events table for unrouted events, matches them
// against triggers, transfers them along bridge subscriptions, and enqueues
// agent tasks. It replaces the former JetStream consumer.
type Router struct {
	index        *trigger.Index
	eq           Queue
	broadcaster  Broadcaster
	invocations  invocations.Store
	transfers    Transferer
	pollInterval time.Duration
}

// New creates a Router backed by the PostgreSQL event queue. transfers may be
// nil, in which case events move only where a trigger matches them.
func New(eq Queue, index *trigger.Index, broadcaster Broadcaster, invStore invocations.Store, transfers Transferer) *Router {
	return &Router{
		index:        index,
		eq:           eq,
		broadcaster:  broadcaster,
		invocations:  invStore,
		transfers:    transfers,
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

// handleEvent processes a single event: match triggers, transfer along channel
// subscriptions, enqueue tasks, mark routed.
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

	// Deduplicate by agent.
	matches := trigger.DeduplicateByAgent(r.index.Match(&sharedEvt))
	if len(matches) == 0 {
		slog.Debug("no trigger matches for event",
			"eventType", sharedEvt.Headers["type"],
			"connector", evt.Connector,
		)
	}

	entryMatches, delivered, allEnqueued := r.routeViaTriggers(ctx, evt, sharedEvt, matches)

	// A trigger decides which inboxes a connector's events land in; a bridge
	// subscription decides which *other* channels the event is transferred to.
	// Both run for every event, so grouping channels feed the inboxes attached
	// to them whether or not a trigger also matched.
	bridgeMatches, bridgeOK := r.routeViaBridges(ctx, evt, sharedEvt.Headers, delivered)
	entryMatches = append(entryMatches, bridgeMatches...)
	allEnqueued = allEnqueued && bridgeOK

	if len(entryMatches) == 0 {
		slog.Info("activity_event",
			"log_type", "activity_event",
			"connector", evt.Connector,
			"channel", evt.ChannelID,
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

	status := "matched"
	if !allEnqueued {
		status = "error"
	}

	matchesJSON, _ := json.Marshal(entryMatches)
	slog.Info("activity_event",
		"log_type", "activity_event",
		"connector", evt.Connector,
		"channel", evt.ChannelID,
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

	// Mark event as routed regardless of individual task failures. Failed tasks
	// are already recorded on their invocations, and re-routing the whole event
	// would deliver duplicates to the inboxes that succeeded.
	_ = r.eq.MarkRouted(ctx, evt.ID)
	if !allEnqueued {
		slog.Warn("event routed with some task failures",
			"eventID", evt.ID,
			"connector", evt.Connector,
			"matches", len(entryMatches),
		)
	}
}

// routeViaTriggers enqueues one task per matched trigger. It returns the
// activity matches, the agents that received the event — the inboxes a bridge
// transfer must not fill twice — and whether every enqueue succeeded.
func (r *Router) routeViaTriggers(ctx context.Context, evt eventqueue.Event, sharedEvt ainselapishared.Event, matches []trigger.MatchResult) ([]types.MatchResult, []string, bool) {
	entryMatches := make([]types.MatchResult, 0, len(matches))
	delivered := make([]string, 0, len(matches))
	allEnqueued := true

	if len(matches) > 0 {
		metrics.TriggersMatched.Add(float64(len(matches)))
		slog.Info("event matched", "type", sharedEvt.Headers["type"], "connector", evt.Connector, "matches", len(matches))
	}

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
			delivered = append(delivered, m.AgentRef)
		}
		entryMatches = append(entryMatches, types.MatchResult{
			Trigger: m.TriggerName,
			Agent:   m.AgentRef,
		})
	}

	return entryMatches, delivered, allEnqueued
}

// routeViaBridges transfers the event into every inbox a bridge path reaches
// from the channel it was born in. Returns the activity matches it produced and
// whether all of them were enqueued.
func (r *Router) routeViaBridges(ctx context.Context, evt eventqueue.Event, headers map[string]string, delivered []string) ([]types.MatchResult, bool) {
	if r.transfers == nil || evt.ChannelID == "" {
		return nil, true
	}
	results, err := r.transfers.Apply(ctx, channels.TransferInput{
		EventID:      evt.ID,
		BirthChannel: evt.ChannelID,
		SourceLabel:  evt.Connector,
		Headers:      headers,
		Payload:      evt.Data,
		Delivered:    delivered,
	})
	if err != nil {
		// The event stays unroutable-but-seen: a broken channel graph must not
		// turn into a retry loop over the whole batch.
		slog.Error("channel transfer failed", "eventID", evt.ID, "channel", evt.ChannelID, "error", err)
		metrics.RoutingErrors.Inc()
		return nil, false
	}

	out := make([]types.MatchResult, 0, len(results))
	for _, res := range results {
		if res.Err != nil {
			metrics.RoutingErrors.Inc()
			continue
		}
		metrics.EventsRouted.Inc()
		slog.Info("transferred to agent inbox",
			"agent", res.AgentName,
			"bridge", res.BridgeName,
			"channel_origin", evt.ChannelID,
			"invocation_id", res.InvocationID,
		)
		out = append(out, types.MatchResult{Trigger: res.BridgeName, Agent: res.AgentName})
	}
	return out, true
}

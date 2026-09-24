package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/trigger"
)

// Header names propagated to agents by a transfer, mirroring the router so the
// runner and the invocation record treat a bridge delivery exactly like a
// trigger match.
const (
	HeaderTriggerName   = "X-Trigger-Name"
	HeaderInvocationID  = "X-Invocation-ID"
	HeaderChannelBridge = "X-Channel-Bridge"
	HeaderChannelID     = "X-Channel-ID"
	HeaderOriginChannel = "X-Channel-Origin"
)

// TaskQueue is the queue surface a transfer writes through.
type TaskQueue interface {
	EnqueueTask(ctx context.Context, task eventqueue.Task) error
}

// TransferInput describes the event whose copies a transfer produces.
type TransferInput struct {
	EventID string
	// BirthChannel is the channel the event was born in. Empty means the event
	// predates channels or its stream was never provisioned, and no transfer is
	// possible.
	BirthChannel string
	// SourceLabel is the event's connector label, recorded on invocations so a
	// run can be traced back to what produced it.
	SourceLabel string
	// Headers are the event headers; each delivery gets its own copy.
	Headers map[string]string
	// Payload is the event data delivered into each inbox.
	Payload json.RawMessage
	// Delivered lists agents that already received this event. A trigger match
	// and a bridge path must not produce two tasks for one inbox.
	Delivered []string
}

// TransferResult is the outcome of one attempted delivery.
type TransferResult struct {
	AgentName    string
	BridgeID     string
	BridgeName   string
	InvocationID string
	Err          error
}

// Transfer performs the subscriptions stored in this package: it walks the
// channel graph from an event's birth channel and publishes the event into
// every agent inbox a bridge path reaches.
//
// It is shared by every publisher — the router after trigger matching, and the
// cron and chat paths that deliver straight into an inbox — so an event cannot
// arrive in one place through a bridge and not another.
type Transfer struct {
	svc *Service
	eq  TaskQueue
	inv invocations.Store
}

// NewTransfer wires a Transfer over the channel service, the task queue and the
// invocation store. inv may be nil, in which case transferred runs are not
// tracked.
func NewTransfer(svc *Service, eq TaskQueue, inv invocations.Store) *Transfer {
	return &Transfer{svc: svc, eq: eq, inv: inv}
}

// Apply transfers one event along every enabled bridge path out of its birth
// channel. A failed delivery is reported per result rather than aborting the
// rest: a partial fan-out is still the truth about what the agent got.
func (t *Transfer) Apply(ctx context.Context, in TransferInput) ([]TransferResult, error) {
	if t == nil || t.svc == nil || in.BirthChannel == "" {
		return nil, nil
	}
	deliveries, err := t.svc.Deliveries(ctx, in.BirthChannel, in.Delivered)
	if err != nil {
		return nil, fmt.Errorf("channels: resolve transfers from %s: %w", in.BirthChannel, err)
	}

	results := make([]TransferResult, 0, len(deliveries))
	for _, d := range deliveries {
		res := TransferResult{AgentName: d.AgentName, BridgeID: d.BridgeID, BridgeName: d.BridgeName}

		var invID string
		if t.inv != nil {
			rec := t.inv.Record(invocations.Invocation{
				AgentName:   d.AgentName,
				TriggerName: d.BridgeName,
				EventID:     in.EventID,
				Connector:   in.SourceLabel,
			})
			invID = rec.ID
			res.InvocationID = invID
		}

		headers := transferHeaders(in, d, invID)
		headersJSON, err := json.Marshal(headers)
		if err != nil {
			return results, fmt.Errorf("channels: marshal transfer headers for %s: %w", d.AgentName, err)
		}
		if err := t.eq.EnqueueTask(ctx, eventqueue.Task{
			EventID:      in.EventID,
			AgentName:    d.AgentName,
			TriggerName:  d.BridgeName,
			InvocationID: invID,
			Headers:      headersJSON,
			Payload:      in.Payload,
		}); err != nil {
			res.Err = err
			slog.Error("bridge transfer failed",
				"event_id", in.EventID,
				"agent", d.AgentName,
				"bridge", d.BridgeID,
				"error", err,
			)
			if t.inv != nil && invID != "" {
				t.inv.Complete(invID, invocations.StatusFailure, "bridge transfer failed: "+err.Error(), time.Time{})
			}
		}
		results = append(results, res)
	}
	return results, nil
}

// transferHeaders builds the header set of one transferred task. The original
// event headers are copied so the runner can still derive the event type, and
// the canonical type is stamped explicitly the same way the router does it.
func transferHeaders(in TransferInput, d Delivery, invID string) map[string]string {
	headers := make(map[string]string, len(in.Headers)+5)
	for k, v := range in.Headers {
		headers[k] = v
	}
	if t := trigger.CanonicalEventType(in.Headers); t != "" {
		headers["type"] = t
	}
	headers[HeaderTriggerName] = d.BridgeName
	headers[HeaderChannelID] = d.AgentChannel
	headers[HeaderOriginChannel] = in.BirthChannel
	headers[HeaderChannelBridge] = d.BridgeID
	if invID != "" {
		headers[HeaderInvocationID] = invID
	}
	return headers
}

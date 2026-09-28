// Package channels owns the channel registry: the named streams events live
// in, the subscriptions that transfer events between them, and the traversal
// that moves an event from the channel it was born in into every channel a
// subscription reaches.
//
// A channel has no role — it neither produces nor consumes. Events are born in
// a channel and move between channels; an agent takes everything from its own
// channel. Identity is always the channel id, never the name: a connector and
// an agent may both be labelled "forgejo" and remain two distinct channels.
//
// Three kinds exist, each with its own lifecycle:
//
//	connector  provisioned per WebhookConnector CR, permanent
//	agent      provisioned per Agent CR — it IS that agent's inbox
//	custom     created by users to bundle subscriptions under one name
//
// Transfers come from two registries. A Trigger transfers one connector
// channel into one agent inbox and is owned by the trigger registry; a bridge
// attaches a channel to a custom grouping channel and is owned here. Triggers
// are never copied into this package — the trigger store stays the single
// source of truth for those edges, and the subscription view joins them at
// read time.
package channels

import (
	"time"

	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

// Kind is the provisioning class of a channel.
type Kind string

const (
	// KindConnector is the stream a connector publishes into.
	KindConnector Kind = "connector"
	// KindAgent is an agent's inbox: everything born or transferred here is
	// prompted to that agent.
	KindAgent Kind = "agent"
	// KindCustom is a user-created grouping channel.
	KindCustom Kind = "custom"
)

// Valid reports whether k is one of the three channel kinds.
func (k Kind) Valid() bool {
	switch k {
	case KindConnector, KindAgent, KindCustom:
		return true
	}
	return false
}

// Channel is a single named stream.
type Channel struct {
	ID string `json:"id"`
	// Kind is the provisioning class.
	Kind Kind `json:"kind"`
	// Name is the rename-able display label. Not unique.
	Name string `json:"name"`
	// Description is one line of intent.
	Description string `json:"description"`
	// EntityRef is the registry id this channel was provisioned for: the
	// WebhookConnector CR name (which is also the value events carry in
	// `connector`) or the Agent CR name. Empty for custom channels.
	EntityRef string `json:"entityRef,omitempty"`
	// Orphaned marks a channel whose registry entity is gone. Rows survive
	// deletion on purpose — event history points at them.
	Orphaned  bool      `json:"orphaned"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Bridge is a stored transfer between two channels. At least one endpoint is
// always a custom channel: a plain connector→agent edge is a trigger, and
// letting both exist for the same pair would put event routing in two places.
type Bridge struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	FromChannel string    `json:"fromChannel"`
	ToChannel   string    `json:"toChannel"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Subscription is one channel→channel edge, whatever registry owns it. Source
// is "trigger" or "bridge" and RefID is the corresponding record id.
// FromChannel/ToChannel are empty when the edge points at a stream that was
// never provisioned; the raw refs are kept so a caller can still label the
// dangling end.
type Subscription struct {
	Source      string `json:"source"`
	RefID       string `json:"refId"`
	Name        string `json:"name"`
	FromChannel string `json:"fromChannel,omitempty"`
	ToChannel   string `json:"toChannel,omitempty"`
	FromRef     string `json:"fromRef,omitempty"`
	ToRef       string `json:"toRef,omitempty"`
}

// Owners of the edges in the subscription view.
const (
	SourceTrigger = "trigger"
	SourceBridge  = "bridge"
)

// Counts are the traffic figures for one channel within a time window.
type Counts struct {
	// Events is what entered the channel in the window: births for connector
	// channels, everything that flowed through for custom channels, and
	// deliveries for agent channels.
	Events int `json:"events"`
	// Unmatched is the share of Events that no subscription transferred on.
	// Only connector channels can have unmatched events.
	Unmatched int `json:"unmatched"`
	// Failed counts deliveries that ended in a failed task.
	Failed int `json:"failed"`
}

// View is a channel plus its counts and edge fan-out — what the console list
// and the MCP tools consume.
type View struct {
	Channel
	Counts        Counts `json:"counts"`
	Bridges       int    `json:"bridges"`
	Subscriptions int    `json:"subscriptions"`
}

// Detail is one channel in full: its record, its counts, and every
// subscription that touches it in either direction.
type Detail struct {
	View
	Incoming []Subscription `json:"incoming"`
	Outgoing []Subscription `json:"outgoing"`
}

// Delivery is one transfer to perform: publish an event into an agent's inbox
// because a bridge path reached it from the channel the event was born in.
type Delivery struct {
	// AgentChannel is the destination inbox.
	AgentChannel string
	// AgentName is the Agent CR name to enqueue the task for.
	AgentName string
	// BridgeID and BridgeName identify the subscription that transferred the
	// event, for invocation records and task headers.
	BridgeID   string
	BridgeName string
}

// Entity is a registry entry the reconciler provisions channels from.
type Entity struct {
	// Ref is the CR name — the WebhookConnector or Agent metadata.name.
	Ref string
	// Name is the display label from the CR spec, falling back to Ref.
	Name string
	// Description is the CR's own description, used as the channel's.
	Description string
}

// EventQuery scopes a channel's event timeline.
type EventQuery struct {
	Since  time.Time
	Limit  int
	Offset int
	// Agent narrows the timeline to events delivered to one agent.
	Agent string
	// Status is a derived activity status: matched, unmatched or error.
	Status string
	// Subject is a "<connector>.<eventType>" pattern, as the events endpoint
	// accepts it.
	Subject string
}

// DirectSourceLabels are the connector labels of events the hub publishes
// itself. Their events are born in an agent's inbox, not in a connector
// channel, so history stamping treats them separately.
var DirectSourceLabels = []string{ainselapishared.SourceCron, ainselapishared.SourceChat}

// DefaultDescription is the one line of intent a channel carries when nothing
// better is known about it. Provisioned channels get it from their kind; a
// registry entity with its own description overrides it.
func DefaultDescription(kind Kind, label string) string {
	switch kind {
	case KindConnector:
		return "Where " + label + " events arrive"
	case KindAgent:
		return "The inbox agent " + label + " drains"
	case KindCustom:
		return "Grouping channel for " + label
	}
	return ""
}

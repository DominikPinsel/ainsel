package channels

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

// Registry is the source of entities channels are provisioned from: the agent
// and connector registries. Only the CR names matter here — a channel is keyed
// by its entity's identity, so a rename in the registry is a label change, not
// a new stream.
type Registry interface {
	ListAgents(ctx context.Context) ([]Entity, error)
	ListConnectors(ctx context.Context) ([]Entity, error)
}

// HistoryStamper is the part of the event queue the reconciler needs: attach
// stored events to the channel they were born in. Implemented by
// *eventqueue.Store, which owns those tables.
type HistoryStamper interface {
	UnstampedSourceLabels(ctx context.Context) ([]string, error)
	UnstampedInboxAgents(ctx context.Context, directLabels []string) ([]string, error)
	StampBirthChannels(ctx context.Context, stamps []eventqueue.ChannelStamp) (int64, error)
	StampInboxBirthChannels(ctx context.Context, directLabels []string, stamps []eventqueue.ChannelStamp) (int64, error)
}

// SyncResult reports what one reconcile pass did.
type SyncResult struct {
	Provisioned   int
	Orphaned      int64
	StampedEvents int64
}

// Reconciler keeps the channel registry in step with the entity registries.
//
// Channels are not created by an explicit act of provisioning: a connector and
// an agent each *are* a stream, so the registry has to follow the CRs. The
// reconciler also adopts history — events written before channels existed are
// attached to the stream they describe, which may mean creating that stream from
// a label the registry no longer knows (a connector deleted in the meantime).
type Reconciler struct {
	store    *Store
	eq       HistoryStamper
	registry Registry
}

// NewReconciler wires a Reconciler against the channel store, the event queue
// and an entity registry.
func NewReconciler(store *Store, eq HistoryStamper, registry Registry) *Reconciler {
	return &Reconciler{store: store, eq: eq, registry: registry}
}

// Sync provisions a channel for every registry entity, flags streams whose
// entity is gone, and stamps stored events with their birth channel.
//
// It is idempotent and cheap enough to run on a ticker: the writes are
// upserts plus one guarded UPDATE per stamping rule, and events already
// stamped are not touched again.
func (r *Reconciler) Sync(ctx context.Context) (SyncResult, error) {
	var res SyncResult

	agents, err := r.registry.ListAgents(ctx)
	if err != nil {
		return res, fmt.Errorf("channels.Sync: list agents: %w", err)
	}
	connectors, err := r.registry.ListConnectors(ctx)
	if err != nil {
		return res, fmt.Errorf("channels.Sync: list connectors: %w", err)
	}

	for _, c := range connectors {
		if _, err := r.store.Ensure(ctx, KindConnector, c.Ref, c.Name, entityDescription(KindConnector, c)); err != nil {
			return res, err
		}
		res.Provisioned++
	}
	for _, a := range agents {
		if _, err := r.store.Ensure(ctx, KindAgent, a.Ref, a.Name, entityDescription(KindAgent, a)); err != nil {
			return res, err
		}
		res.Provisioned++
	}

	// Streams that only history refers to: a connector label or an agent
	// inbox named by events stored before channels existed. These rows are
	// created un-named (the label is all we know) and are flagged orphaned
	// below, because the registry does not confirm them.
	unstampedLabels, err := r.eq.UnstampedSourceLabels(ctx)
	if err != nil {
		return res, err
	}
	for _, label := range unstampedLabels {
		if ainselapishared.IsDirectSource(label) {
			continue
		}
		if _, err := r.store.Ensure(ctx, KindConnector, label, label, DefaultDescription(KindConnector, label)); err != nil {
			return res, err
		}
		res.Provisioned++
	}
	inboxAgents, err := r.eq.UnstampedInboxAgents(ctx, DirectSourceLabels)
	if err != nil {
		return res, err
	}
	for _, name := range inboxAgents {
		if _, err := r.store.Ensure(ctx, KindAgent, name, name, DefaultDescription(KindAgent, name)); err != nil {
			return res, err
		}
		res.Provisioned++
	}

	// A channel of a provisioned kind is orphaned unless the registry still
	// lists its entity. Only run this after a successful listing — an empty
	// list means "everything is gone", which is exactly what it says.
	connectorRefs := entityRefs(connectors)
	agentRefs := entityRefs(agents)
	connectorOrphans, err := r.store.MarkOrphans(ctx, KindConnector, connectorRefs)
	if err != nil {
		return res, err
	}
	agentOrphans, err := r.store.MarkOrphans(ctx, KindAgent, agentRefs)
	if err != nil {
		return res, err
	}
	res.Orphaned = connectorOrphans + agentOrphans

	stamped, err := r.stampHistory(ctx)
	if err != nil {
		return res, err
	}
	res.StampedEvents = stamped

	if res.Provisioned > 0 || res.Orphaned > 0 || res.StampedEvents > 0 {
		slog.Info("channels reconciled",
			"provisioned", res.Provisioned,
			"orphaned", res.Orphaned,
			"events_stamped", res.StampedEvents,
		)
	}
	return res, nil
}

// stampHistory attaches events with no birth channel to the channel their
// label or delivery names.
func (r *Reconciler) stampHistory(ctx context.Context) (int64, error) {
	all, err := r.store.List(ctx, "")
	if err != nil {
		return 0, err
	}
	var connectorStamps, agentStamps []eventqueue.ChannelStamp
	for _, c := range all {
		if c.EntityRef == "" {
			continue
		}
		switch c.Kind {
		case KindConnector:
			connectorStamps = append(connectorStamps, eventqueue.ChannelStamp{Label: c.EntityRef, ChannelID: c.ID})
		case KindAgent:
			agentStamps = append(agentStamps, eventqueue.ChannelStamp{Label: c.EntityRef, ChannelID: c.ID})
		}
	}

	stamped, err := r.eq.StampBirthChannels(ctx, connectorStamps)
	if err != nil {
		return stamped, err
	}
	inboxStamped, err := r.eq.StampInboxBirthChannels(ctx, DirectSourceLabels, agentStamps)
	if err != nil {
		return stamped, err
	}
	return stamped + inboxStamped, nil
}

func entityRefs(entities []Entity) []string {
	refs := make([]string, 0, len(entities))
	for _, e := range entities {
		refs = append(refs, e.Ref)
	}
	return refs
}

// entityDescription prefers what the entity says about itself and falls back to
// the kind's default, so a renamed connector never keeps a stale sentence.
func entityDescription(kind Kind, e Entity) string {
	if e.Description != "" {
		return e.Description
	}
	return DefaultDescription(kind, e.Name)
}

package channels

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
)

// DefaultWindow is the traffic window reported alongside a channel when the
// caller does not ask for one.
const DefaultWindow = 24 * time.Hour

// QueueReader is the part of the event queue the service reads. Counts and
// timelines are answered from the events and agent_tasks tables, which the
// queue owns.
type QueueReader interface {
	BirthStats(ctx context.Context, since time.Time) ([]eventqueue.BirthStat, error)
	DeliveryStats(ctx context.Context, since time.Time) ([]eventqueue.DeliveryStat, error)
	QueryEvents(ctx context.Context, f eventqueue.EventFilter, limit, offset int) ([]eventqueue.Event, error)
	CountEvents(ctx context.Context, f eventqueue.EventFilter) (int, error)
}

// TriggerReader reads the trigger registry. A trigger *is* a subscription
// between a connector channel and an agent inbox, so the subscription view is
// answered from the registry rather than from a copy of it here.
type TriggerReader interface {
	ListTriggers(ctx context.Context, agentRef, connectorRef string) ([]triggers.Trigger, error)
}

// Service answers everything the console and the MCP server need about
// channels: their traffic, their edges, their timelines, and the mutations a
// user may make.
type Service struct {
	store    *Store
	eq       QueueReader
	triggers TriggerReader
}

// NewService wires a Service over the channel store, the event queue and the
// trigger registry.
func NewService(store *Store, eq QueueReader, ts TriggerReader) *Service {
	return &Service{store: store, eq: eq, triggers: ts}
}

// Store exposes the underlying channel store for the publishers that need to
// resolve a birth channel (ingest, cron, chat) without depending on the views.
func (s *Service) Store() *Store { return s.store }

// entityKey addresses a provisioned channel by its registry entity.
func entityKey(kind Kind, ref string) string { return string(kind) + "#" + ref }

// List returns every channel of one kind (all kinds when kind is empty) with
// its traffic counts. `since` is the lower bound of the event window — the same
// boundary the event-window endpoint reports, so a channel's rates and its
// timeline always cover the same span. A zero since means DefaultWindow.
func (s *Service) List(ctx context.Context, kind Kind, since time.Time) ([]View, error) {
	if kind != "" && !kind.Valid() {
		return nil, fmt.Errorf("unknown channel kind %q", kind)
	}
	rows, err := s.store.List(ctx, kind)
	if err != nil {
		return nil, err
	}
	subs, err := s.Subscriptions(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.countsFor(ctx, rows, subs, since)
	if err != nil {
		return nil, err
	}

	degree, bridgeDegree := degrees(subs)
	views := make([]View, 0, len(rows))
	for _, c := range rows {
		views = append(views, View{
			Channel:       c,
			Counts:        counts[c.ID],
			Bridges:       bridgeDegree[c.ID],
			Subscriptions: degree[c.ID],
		})
	}
	return views, nil
}

// Get returns one channel with its counts and both directions of edges.
func (s *Service) Get(ctx context.Context, id string, since time.Time) (*Detail, error) {
	ch, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	subs, err := s.Subscriptions(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.countsFor(ctx, []Channel{*ch}, subs, since)
	if err != nil {
		return nil, err
	}
	degree, bridgeDegree := degrees(subs)

	d := &Detail{
		View: View{
			Channel:       *ch,
			Counts:        counts[id],
			Bridges:       bridgeDegree[id],
			Subscriptions: degree[id],
		},
	}
	for _, sub := range subs {
		switch {
		case sub.ToChannel == id:
			d.Incoming = append(d.Incoming, sub)
		case sub.FromChannel == id:
			d.Outgoing = append(d.Outgoing, sub)
		}
	}
	return d, nil
}

// Subscriptions returns every channel→channel edge in the system: the trigger
// registry's transfers plus the bridges stored here. Edges whose endpoints were
// never provisioned keep their raw refs so a caller can still label them.
func (s *Service) Subscriptions(ctx context.Context) ([]Subscription, error) {
	rows, err := s.store.List(ctx, "")
	if err != nil {
		return nil, err
	}
	byEntity := make(map[string]string, len(rows))
	for _, c := range rows {
		if c.EntityRef != "" {
			byEntity[entityKey(c.Kind, c.EntityRef)] = c.ID
		}
	}

	var out []Subscription
	if s.triggers != nil {
		trs, err := s.triggers.ListTriggers(ctx, "", "")
		if err != nil {
			return nil, fmt.Errorf("channels: list trigger subscriptions: %w", err)
		}
		sort.Slice(trs, func(i, j int) bool { return trs[i].ID < trs[j].ID })
		for _, t := range trs {
			out = append(out, Subscription{
				Source:      SourceTrigger,
				RefID:       t.ID,
				Name:        firstNonEmpty(t.DisplayName, t.ID),
				FromChannel: byEntity[entityKey(KindConnector, t.ConnectorRef)],
				ToChannel:   byEntity[entityKey(KindAgent, t.AgentRef)],
				FromRef:     t.ConnectorRef,
				ToRef:       t.AgentRef,
			})
		}
	}

	bridges, err := s.store.ListBridges(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, b := range bridges {
		out = append(out, Subscription{
			Source:      SourceBridge,
			RefID:       b.ID,
			Name:        b.Name,
			FromChannel: b.FromChannel,
			ToChannel:   b.ToChannel,
		})
	}
	return out, nil
}

// countsFor fills the traffic figures for the given channels. One birth-count
// query and one delivery-count query answer the whole set, so a list of
// channels costs two aggregates rather than three queries per row.
func (s *Service) countsFor(ctx context.Context, rows []Channel, subs []Subscription, since time.Time) (map[string]Counts, error) {
	out := make(map[string]Counts, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	if since.IsZero() {
		since = time.Now().UTC().Add(-DefaultWindow)
	}
	since = since.UTC()

	births, err := s.eq.BirthStats(ctx, since)
	if err != nil {
		return nil, err
	}
	birthBy := make(map[string]eventqueue.BirthStat, len(births))
	for _, b := range births {
		birthBy[b.ChannelID] = b
	}
	deliveries, err := s.eq.DeliveryStats(ctx, since)
	if err != nil {
		return nil, err
	}
	deliveryBy := make(map[string]eventqueue.DeliveryStat, len(deliveries))
	for _, d := range deliveries {
		deliveryBy[d.AgentName] = d
	}

	// Grouping channels hold no deliveries of their own: their traffic is the
	// sum of the streams that reach them, so resolve each custom channel's
	// reverse closure once.
	closureOf := map[string][]string{}
	for _, c := range rows {
		if c.Kind == KindCustom {
			reachers, err := s.store.ReachingInto(ctx, c.ID)
			if err != nil {
				return nil, err
			}
			closureOf[c.ID] = reachers
		}
	}

	for _, c := range rows {
		var cnt Counts
		switch c.Kind {
		case KindConnector:
			b := birthBy[c.ID]
			cnt = Counts{Events: b.Events, Unmatched: b.Unmatched, Failed: b.Failed}
		case KindAgent:
			d := deliveryBy[c.EntityRef]
			// Delivery counts are keyed by agent name and shared by every
			// event the inbox received, whatever transferred it.
			cnt = Counts{Events: d.Events, Failed: d.Failed}
		case KindCustom:
			for _, id := range closureOf[c.ID] {
				b := birthBy[id]
				cnt.Events += b.Events
			}
		}
		out[c.ID] = cnt
	}
	return out, nil
}

// degrees counts edges touching each channel, overall and bridge-only.
func degrees(subs []Subscription) (degree, bridgeDegree map[string]int) {
	degree = map[string]int{}
	bridgeDegree = map[string]int{}
	for _, sub := range subs {
		for _, id := range []string{sub.FromChannel, sub.ToChannel} {
			if id == "" {
				continue
			}
			degree[id]++
			if sub.Source == SourceBridge {
				bridgeDegree[id]++
			}
		}
	}
	return degree, bridgeDegree
}

// Events returns the timeline of one channel: the events born in it, the
// events delivered to an agent's inbox, or the events that flowed through a
// grouping channel. The result is the same shape the events API returns, so a
// timeline and its counts cannot disagree.
func (s *Service) Events(ctx context.Context, id string, q EventQuery) ([]eventqueue.Event, int, error) {
	ch, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	filter, err := s.timelineFilter(ctx, *ch)
	if err != nil {
		return nil, 0, err
	}
	if !q.Since.IsZero() {
		filter.Since = q.Since
	}
	if filter.Agent == "" {
		filter.Agent = q.Agent
	}
	filter.Status = q.Status
	filter.Subject = q.Subject
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	events, err := s.eq.QueryEvents(ctx, filter, limit, q.Offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.eq.CountEvents(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// timelineFilter maps a channel onto the events it holds.
func (s *Service) timelineFilter(ctx context.Context, ch Channel) (eventqueue.EventFilter, error) {
	switch ch.Kind {
	case KindConnector:
		return eventqueue.EventFilter{Channel: ch.ID}, nil
	case KindAgent:
		// An inbox holds what was delivered to it — including events born
		// directly in it, which are delivered without crossing a subscription.
		return eventqueue.EventFilter{Agent: ch.EntityRef}, nil
	case KindCustom:
		reachers, err := s.store.ReachingInto(ctx, ch.ID)
		if err != nil {
			return eventqueue.EventFilter{}, err
		}
		return eventqueue.EventFilter{Channels: reachers}, nil
	}
	return eventqueue.EventFilter{}, errors.New("unknown channel kind")
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

// CreateCustom creates a user-authored grouping channel.
func (s *Service) CreateCustom(ctx context.Context, name, description string) (*Channel, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("name is required")
	}
	return s.store.Create(ctx, name, description)
}

// Update renames a custom channel or changes its description. Provisioned
// channels answer ErrNotCustom.
func (s *Service) Update(ctx context.Context, id string, name, description *string) (*Channel, error) {
	if name != nil && strings.TrimSpace(*name) == "" {
		return nil, errors.New("name cannot be empty")
	}
	return s.store.Update(ctx, id, name, description)
}

// Delete removes a custom channel that nothing is attached to.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

// Attach records a bridge from one channel into another and returns it.
func (s *Service) Attach(ctx context.Context, fromID, toID, name string) (*Bridge, error) {
	return s.store.CreateBridge(ctx, fromID, toID, name)
}

// Detach removes a bridge.
func (s *Service) Detach(ctx context.Context, bridgeID string) error {
	return s.store.DeleteBridge(ctx, bridgeID)
}

// Bridges returns the bridges attached to one channel (either end).
func (s *Service) Bridges(ctx context.Context, channelID string) ([]Bridge, error) {
	return s.store.ListBridges(ctx, channelID)
}

// ConnectorChannelFor returns the id of the channel events with this connector
// label are born in, provisioning it on first sight so an event always has a
// stream to be stamped with before it is stored.
func (s *Service) ConnectorChannelFor(ctx context.Context, label string) (string, error) {
	ch, err := s.store.Ensure(ctx, KindConnector, label, label, DefaultDescription(KindConnector, label))
	if err != nil {
		return "", err
	}
	return ch.ID, nil
}

// InboxChannelFor returns the id of an agent's inbox channel. Cron ticks and
// chat messages are born there rather than in a connector channel, because
// nothing publishes them — the hub does.
func (s *Service) InboxChannelFor(ctx context.Context, agentName string) (string, error) {
	ch, err := s.store.Ensure(ctx, KindAgent, agentName, agentName, DefaultDescription(KindAgent, agentName))
	if err != nil {
		return "", err
	}
	return ch.ID, nil
}

// EntityChannel returns the channel provisioned for a registry entity, if any.
func (s *Service) EntityChannel(ctx context.Context, kind Kind, entityRef string) (*Channel, error) {
	return s.store.GetByEntity(ctx, kind, entityRef)
}

// Deliveries returns the transfers to perform for an event born in the given
// channel, skipping agents the trigger registry already matched so an event is
// delivered once per inbox whatever the number of paths into it.
func (s *Service) Deliveries(ctx context.Context, birthChannel string, alreadyDelivered []string) ([]Delivery, error) {
	if birthChannel == "" {
		return nil, nil
	}
	deliveries, err := s.store.Deliveries(ctx, birthChannel)
	if err != nil {
		return nil, err
	}
	skip := make(map[string]bool, len(alreadyDelivered))
	for _, name := range alreadyDelivered {
		skip[name] = true
	}
	out := deliveries[:0]
	for _, d := range deliveries {
		if skip[d.AgentName] {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// Bridge returns one bridge by id.
func (s *Service) Bridge(ctx context.Context, bridgeID string) (*Bridge, error) {
	return s.store.GetBridge(ctx, bridgeID)
}

// Channel returns one channel by id.
func (s *Service) Channel(ctx context.Context, id string) (*Channel, error) {
	return s.store.Get(ctx, id)
}

// BridgesInto returns the bridges that transfer events into a channel.
func (s *Service) BridgesInto(ctx context.Context, id string) ([]Bridge, error) {
	all, err := s.store.ListBridges(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]Bridge, 0, len(all))
	for _, b := range all {
		if b.ToChannel == id {
			out = append(out, b)
		}
	}
	return out, nil
}

// ReachingInto returns the ids of channels whose events arrive at the given
// channel, following bridge paths of any length. Used to un-adopt nested
// channels when a grouping channel is deleted.
func (s *Service) ReachingInto(ctx context.Context, id string) ([]string, error) {
	return s.store.ReachingInto(ctx, id)
}

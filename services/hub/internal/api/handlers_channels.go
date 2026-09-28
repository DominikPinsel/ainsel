package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/channels"
	"github.com/DominikPinsel/ainsel/services/hub/internal/eventqueue"
)

// channelDefaultWindow is how far the per-channel rates on a list cover when the
// caller does not pass ?since=. The window's end is always now.
const channelDefaultWindow = 24 * time.Hour

// SetChannelService wires the channel registry and its bridge transfers into
// the server and registers the routes.
func (s *Server) SetChannelService(svc *channels.Service, transfer *channels.Transfer) {
	s.channelSvc = svc
	s.channelTransfer = transfer
	s.registerChannelRoutes()
}

func (s *Server) registerChannelRoutes() {
	// Collection routes.
	s.mux.HandleFunc("/api/v1/channels", s.handleChannels)
	s.mux.HandleFunc("/api/v1/channels/", s.handleChannelPath)

	// The graph's edges on their own, so a client can draw the subscriptions
	// without walking every channel.
	s.mux.HandleFunc("/api/v1/channel-subscriptions", s.handleChannelSubscriptions)
}

// handleChannelPath dispatches /api/v1/channels/{id} and its sub-resources:
//
//	{id}
//	{id}/events
//	{id}/bridges
//	{id}/bridges/{bridgeId}
func (s *Server) handleChannelPath(w http.ResponseWriter, r *http.Request) {
	rest := extractName(r.URL.Path, "/api/v1/channels/")
	if rest == "" {
		writeError(w, http.StatusBadRequest, "missing channel id")
		return
	}
	id, sub, found := strings.Cut(rest, "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing channel id")
		return
	}
	// The path channel is the subject of every route below, and its kind decides
	// which permission applies — so it is resolved once here rather than in each
	// handler that needs it.
	switch {
	case !found:
		switch r.Method {
		case http.MethodGet:
			s.getChannel(w, r, id)
		case http.MethodPut:
			s.updateChannel(w, r, id)
		case http.MethodDelete:
			s.deleteChannel(w, r, id)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case sub == "events":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.channelEvents(w, r, id)
	case sub == "bridges" || strings.HasPrefix(sub, "bridges/"):
		s.handleChannelBridges(w, r, id, strings.TrimPrefix(sub, "bridges"))
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

// handleChannelBridges dispatches the bridge sub-routes of one channel.
func (s *Server) handleChannelBridges(w http.ResponseWriter, r *http.Request, id, tail string) {
	bridgeID := strings.TrimPrefix(tail, "/")
	switch r.Method {
	case http.MethodPost:
		if bridgeID != "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		s.attachBridge(w, r, id)
	case http.MethodDelete:
		s.detachBridge(w, r, id, bridgeID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ---------------------------------------------------------------------------
// Authorization mapping
// ---------------------------------------------------------------------------

// channelAuthzTarget maps a channel onto the resource whose group owns it: a
// custom channel owns itself, while a connector's or an agent's channel follows
// its entity — so granting a connector grants reading its stream, and nobody
// has to permission a channel that no one created.
func channelAuthzTarget(ch *channels.Channel) (resourceType, name string) {
	switch ch.Kind {
	case channels.KindConnector:
		return "connector", ch.EntityRef
	case channels.KindAgent:
		return "agent", ch.EntityRef
	default:
		return "channel", ch.ID
	}
}

// requireChannelRead checks the caller may see one channel.
func (s *Server) requireChannelRead(w http.ResponseWriter, r *http.Request, ch *channels.Channel) bool {
	resourceType, name := channelAuthzTarget(ch)
	return s.requireRead(w, r, resourceType, name)
}

// requireChannelWrite checks the caller may change a channel's subscriptions.
// An edge moves events out of one stream and into another, so both endpoints
// need the write permission.
func (s *Server) requireChannelWrite(w http.ResponseWriter, r *http.Request, ch *channels.Channel) bool {
	resourceType, name := channelAuthzTarget(ch)
	return s.requireWrite(w, r, resourceType, name)
}

// visibleChannels is the subset of the given channels the caller may see. It
// answers a whole page in one lookup per resource type instead of one authz
// round trip per row.
func (s *Server) visibleChannels(r *http.Request, chans []channels.Channel) map[string]bool {
	if s.authzStore == nil || s.callerIsAdmin(r) {
		set := make(map[string]bool, len(chans))
		for _, ch := range chans {
			set[ch.ID] = true
		}
		return set
	}
	byType := map[string][]string{}
	keyOf := map[string]string{}
	for _, ch := range chans {
		resourceType, name := channelAuthzTarget(&ch)
		byType[resourceType] = append(byType[resourceType], name)
		keyOf[resourceType+"|"+name] = ch.ID
	}
	visible := make(map[string]bool, len(chans))
	for resourceType, names := range byType {
		for _, name := range s.filterByAccess(r, resourceType, names) {
			if id, ok := keyOf[resourceType+"|"+name]; ok {
				visible[id] = true
			}
		}
	}
	return visible
}

// channelSince reads the ?since= window boundary, defaulting to the last 24
// hours. Rates and the timeline are computed over the same span, so a channel's
// numbers and its events cannot disagree.
func channelSince(r *http.Request) (time.Time, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("since"))
	if raw == "" {
		return time.Now().UTC().Add(-channelDefaultWindow), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errors.New("invalid since: expected RFC 3339 timestamp")
	}
	return t.UTC(), nil
}

// ---------------------------------------------------------------------------
// Collection handlers
// ---------------------------------------------------------------------------

// handleChannels serves the collection routes: the list and, on POST, the
// creation of a grouping channel.
func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.createChannel(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.channelSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "channels not configured")
		return
	}
	since, err := channelSince(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kind := channels.Kind(strings.TrimSpace(r.URL.Query().Get("kind")))
	if kind != "" && !kind.Valid() {
		writeError(w, http.StatusBadRequest, "invalid kind: expected connector, agent or custom")
		return
	}

	views, err := s.channelSvc.List(r.Context(), kind, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	visible := s.visibleChannels(r, viewChannels(views))

	// Channels carry no natural order, so the list is sorted by activity and
	// then id — the same result every time, which a paged list must be.
	filtered := make([]channels.View, 0, len(views))
	for _, v := range views {
		if visible[v.ID] {
			filtered = append(filtered, v)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		ei, ej := filtered[i], filtered[j]
		if ei.Counts.Events != ej.Counts.Events {
			return ei.Counts.Events > ej.Counts.Events
		}
		return ei.ID < ej.ID
	})

	page, err := ParsePageParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	total := len(filtered)
	lo, hi := page.Slice(total)
	items := filtered[lo:hi]
	if items == nil {
		items = []channels.View{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":      items,
		"total":      total,
		"page":       page.Page,
		"pageSize":   page.PageSize,
		"totalPages": page.TotalPages(total),
		"window":     channelWindowLabel(since),
	})
}

// viewChannels flattens the channel records out of a list view, for the access
// filter which works on channels rather than views.
func viewChannels(views []channels.View) []channels.Channel {
	out := make([]channels.Channel, 0, len(views))
	for _, v := range views {
		out = append(out, v.Channel)
	}
	return out
}

// channelWindowLabel reports the window as a rounded duration, for the "per
// hour over the last 24h" caption the console shows.
func channelWindowLabel(since time.Time) string {
	d := time.Since(since)
	if d < 0 {
		d = 0
	}
	return d.Round(time.Minute).String()
}

// handleChannelSubscriptions returns every channel→channel edge: the transfers
// owned by the trigger registry alongside the bridges stored on channels.
func (s *Server) handleChannelSubscriptions(w http.ResponseWriter, r *http.Request) {
	if s.channelSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "channels not configured")
		return
	}
	subs, err := s.channelSvc.Subscriptions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// An edge reveals both of its endpoints, so it is only reported when the
	// caller can see each of them.
	refs := make(map[string]bool, len(subs)*2)
	for _, sub := range subs {
		if sub.FromChannel != "" {
			refs[sub.FromChannel] = true
		}
		if sub.ToChannel != "" {
			refs[sub.ToChannel] = true
		}
	}
	chans := make([]channels.Channel, 0, len(refs))
	for id := range refs {
		ch, err := s.channelSvc.Channel(r.Context(), id)
		if err != nil || ch == nil {
			continue // a dangling end is invisible, and the edge is filtered below
		}
		chans = append(chans, *ch)
	}
	visible := s.visibleChannels(r, chans)

	filtered := make([]channels.Subscription, 0, len(subs))
	for _, sub := range subs {
		if sub.FromChannel == "" || sub.ToChannel == "" {
			continue
		}
		if visible[sub.FromChannel] && visible[sub.ToChannel] {
			filtered = append(filtered, sub)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": filtered, "total": len(filtered)})
}

// ---------------------------------------------------------------------------
// Item handlers
// ---------------------------------------------------------------------------

// channelFromPath loads the channel named by a path id, writing the error
// response and reporting false when it cannot be used.
func (s *Server) channelFromPath(w http.ResponseWriter, r *http.Request, id string) (*channels.Channel, bool) {
	if s.channelSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "channels not configured")
		return nil, false
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "channel id required")
		return nil, false
	}
	ch, err := s.channelSvc.Channel(r.Context(), id)
	if err != nil {
		channelError(w, err)
		return nil, false
	}
	return ch, true
}

// handleGetChannel returns one channel with its counts and both directions of
// its subscriptions.
func (s *Server) getChannel(w http.ResponseWriter, r *http.Request, id string) {
	ch, ok := s.channelFromPath(w, r, id)
	if !ok {
		return
	}
	if !s.requireChannelRead(w, r, ch) {
		return
	}
	since, err := channelSince(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	detail, err := s.channelSvc.Get(r.Context(), ch.ID, since)
	if err != nil {
		channelError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

type channelCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	GroupID     string `json:"groupId,omitempty"`
}

type channelUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// handleCreateChannel creates a grouping channel. Provisioned channels are not
// creatable here: they exist because their connector or agent does.
func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	if s.channelSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "channels not configured")
		return
	}
	var req channelCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if s.authzChecker != nil {
		if req.GroupID == "" {
			writeError(w, http.StatusBadRequest, "groupId is required")
			return
		}
		if !s.requireGroupWrite(w, r, req.GroupID) {
			return
		}
	}

	ch, err := s.channelSvc.CreateCustom(r.Context(), req.Name, req.Description)
	if err != nil {
		channelError(w, err)
		return
	}
	if s.authzStore != nil && req.GroupID != "" {
		if err := s.authzStore.SetResourceGroup(r.Context(), "channel", ch.ID, req.GroupID, false); err != nil {
			slog.Warn("resource group write failed", "resource_type", "channel", "resource_id", ch.ID, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, ch)
}

// handleUpdateChannel renames a custom channel or changes its description. The
// id never changes: it is what events and bridges reference.
func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request, id string) {
	ch, ok := s.channelFromPath(w, r, id)
	if !ok {
		return
	}
	if !s.requireChannelWrite(w, r, ch) {
		return
	}
	var req channelUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	updated, err := s.channelSvc.Update(r.Context(), ch.ID, req.Name, req.Description)
	if err != nil {
		channelError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteChannel deletes a custom channel. It refuses while subscriptions
// still cross its boundary, so a stream cannot disappear underneath them.
func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request, id string) {
	ch, ok := s.channelFromPath(w, r, id)
	if !ok {
		return
	}
	if !s.requireChannelWrite(w, r, ch) {
		return
	}
	// Channels that were reachable only through this one go back to the caller's
	// group; resolve them before the edges are gone.
	nested, err := s.channelSvc.ReachingInto(r.Context(), ch.ID)
	if err != nil {
		slog.Warn("could not resolve nested channels", "channel_id", ch.ID, "error", err)
	}
	if err := s.channelSvc.Delete(r.Context(), ch.ID); err != nil {
		channelError(w, err)
		return
	}
	if s.authzStore != nil {
		s.releaseNestedChannels(r, ch.ID, nested)
		if err := s.authzStore.DeleteResourceGroup(r.Context(), "channel", ch.ID); err != nil {
			slog.Warn("resource group cleanup failed", "resource_type", "channel", "resource_id", ch.ID, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// releaseNestedChannels re-homes the custom channels that were pulled under the
// deleted grouping channel, so they do not stay assigned to a group the caller
// may not own. A channel still reached through another bridge is left alone:
// that edge still carries its ownership.
func (s *Server) releaseNestedChannels(r *http.Request, deletedID string, nested []string) {
	if len(nested) == 0 {
		return
	}
	groupID, ok := s.callerGroupID(r)
	if !ok {
		return
	}
	for _, id := range nested {
		if id == deletedID {
			continue
		}
		remaining, err := s.channelSvc.BridgesInto(r.Context(), id)
		if err != nil || len(remaining) > 0 {
			continue
		}
		if err := s.authzStore.SetResourceGroup(r.Context(), "channel", id, groupID, false); err != nil {
			slog.Warn("resource group reset failed", "resource_type", "channel", "resource_id", id, "error", err)
		}
	}
}

// callerGroupID returns the group new resources created by this caller should
// land in: their first group, or empty when they belong to none.
func (s *Server) callerGroupID(r *http.Request) (string, bool) {
	groups := s.callerGroupIDs(r)
	if len(groups) == 0 {
		return "", false
	}
	return groups[0], true
}

// channelError maps the channel store's answers onto status codes: 404 when the
// subject is gone, 409 when it exists but refuses the operation — a provisioned
// channel renamed, a channel still in use, a bridge that would close a cycle.
func channelError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, channels.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, channels.ErrInUse),
		errors.Is(err, channels.ErrNotCustom),
		errors.Is(err, channels.ErrNoCustomEndpoint),
		errors.Is(err, channels.ErrSelfEdge),
		errors.Is(err, channels.ErrCycle),
		errors.Is(err, channels.ErrBridgeExists):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// ---------------------------------------------------------------------------
// Timeline
// ---------------------------------------------------------------------------

// handleChannelEvents returns a channel's timeline: what was born in it, plus
// what was transferred into it, newest first. The envelope matches the events
// endpoint, so one row type serves both views.
func (s *Server) channelEvents(w http.ResponseWriter, r *http.Request, id string) {
	ch, ok := s.channelFromPath(w, r, id)
	if !ok {
		return
	}
	if !s.requireChannelRead(w, r, ch) {
		return
	}
	q := r.URL.Query()

	limit := defaultEventLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = n
	}
	if limit > maxEventLimit {
		limit = maxEventLimit
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid offset")
			return
		}
		offset = n
	}
	since, err := channelSince(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := q.Get("status")
	if status != "" && !validActivityStatuses[status] {
		writeError(w, http.StatusBadRequest, "invalid status: expected matched, unmatched or error")
		return
	}

	events, total, err := s.channelSvc.Events(r.Context(), ch.ID, channels.EventQuery{
		Since:   since,
		Limit:   limit,
		Offset:  offset,
		Agent:   q.Get("agent"),
		Status:  status,
		Subject: q.Get("subject"),
	})
	if err != nil {
		channelError(w, err)
		return
	}

	entries := make([]activityEntry, 0, len(events))
	if len(events) > 0 {
		ids := make([]string, len(events))
		for i, e := range events {
			ids[i] = e.ID
		}
		tasks, err := s.eventQueue.TasksForEvents(r.Context(), ids)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list event tasks")
			return
		}
		tasksByEvent := make(map[string][]eventqueue.Task, len(events))
		for _, t := range tasks {
			tasksByEvent[t.EventID] = append(tasksByEvent[t.EventID], t)
		}
		for _, e := range events {
			entries = append(entries, buildActivityEntry(e, tasksByEvent[e.ID], s.invocations))
		}
	}
	writeJSON(w, http.StatusOK, eventsEnvelope{Events: entries, Total: total})
}

// ---------------------------------------------------------------------------
// Bridges
// ---------------------------------------------------------------------------

type bridgeCreateRequest struct {
	// To is the far endpoint: an agent inbox, or the grouping channel events
	// should be transferred into.
	To string `json:"to"`
	// Name optionally labels the edge; it defaults to "<from> → <to>".
	Name string `json:"name,omitempty"`
}

// handleAttachBridge records that events arriving at the path channel are
// transferred to the target. Triggers are not touched: a trigger decides which
// inbox a connector's events land in, a bridge moves an event again from the
// channel it reached.
func (s *Server) attachBridge(w http.ResponseWriter, r *http.Request, id string) {
	from, ok := s.channelFromPath(w, r, id)
	if !ok {
		return
	}
	if !s.requireChannelWrite(w, r, from) {
		return
	}
	var req bridgeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.To) == "" {
		writeError(w, http.StatusBadRequest, "to is required")
		return
	}
	to, err := s.channelSvc.Channel(r.Context(), req.To)
	if err != nil {
		channelError(w, err)
		return
	}
	if !s.requireChannelWrite(w, r, to) {
		return
	}

	bridge, err := s.channelSvc.Attach(r.Context(), from.ID, to.ID, req.Name)
	if err != nil {
		channelError(w, err)
		return
	}
	// The edge belongs to the group that owns its target, so grouping a stream
	// under one of your own channels never needs a second grant.
	s.adoptBridgeGroup(r, bridge)
	writeJSON(w, http.StatusCreated, bridge)
}

// adoptBridgeGroup records a bridge as a resource of the target channel's group.
func (s *Server) adoptBridgeGroup(r *http.Request, bridge *channels.Bridge) {
	if s.authzStore == nil {
		return
	}
	to, err := s.channelSvc.Channel(r.Context(), bridge.ToChannel)
	if err != nil {
		return
	}
	resourceType, name := channelAuthzTarget(to)
	groupID := ""
	if existing, err := s.authzStore.GetResourceGroup(r.Context(), resourceType, name); err == nil && existing != nil {
		groupID = existing.GroupID
	}
	if groupID == "" {
		var ok bool
		if groupID, ok = s.callerGroupID(r); !ok {
			return
		}
	}
	if err := s.authzStore.SetResourceGroup(r.Context(), "channel", bridge.ID, groupID, false); err != nil {
		slog.Warn("resource group write failed", "resource_type", "channel", "resource_id", bridge.ID, "error", err)
	}
}

// handleDetachBridge removes a subscription edge. The path channel must be one
// of the bridge's endpoints, so an id from the wrong side cannot delete it.
func (s *Server) detachBridge(w http.ResponseWriter, r *http.Request, id, bridgeID string) {
	if s.channelSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "channels not configured")
		return
	}
	if bridgeID == "" {
		writeError(w, http.StatusBadRequest, "missing bridge id")
		return
	}

	existing, err := s.channelSvc.Bridge(r.Context(), bridgeID)
	if err != nil {
		channelError(w, err)
		return
	}
	if existing.FromChannel != id && existing.ToChannel != id {
		writeError(w, http.StatusNotFound, "bridge not found on this channel")
		return
	}
	for _, endpoint := range []string{existing.FromChannel, existing.ToChannel} {
		ch, err := s.channelSvc.Channel(r.Context(), endpoint)
		if err != nil {
			channelError(w, err)
			return
		}
		if !s.requireChannelWrite(w, r, ch) {
			return
		}
	}
	if err := s.channelSvc.Detach(r.Context(), bridgeID); err != nil {
		channelError(w, err)
		return
	}
	if s.authzStore != nil {
		if err := s.authzStore.DeleteResourceGroup(r.Context(), "channel", bridgeID); err != nil {
			slog.Warn("resource group cleanup failed", "resource_type", "channel", "resource_id", bridgeID, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

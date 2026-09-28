package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/DominikPinsel/ainsel/services/hub/internal/types"
	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
	"github.com/gorilla/websocket"
)

// wsMessage is the envelope sent over WebSocket connections.
type wsMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// wsClient is a connection plus the identity it was established with.
//
// The identity has to be captured at upgrade time: the read loop outlives the
// originating request, and a later broadcast has no other way to know who is
// on the other end. Without it every message went to every connection, so one
// user's chat was streamed to another user's browser.
type wsClient struct {
	conn   *websocket.Conn
	userID string // OIDC subject; "" when the hub runs without authentication
	admin  bool
}

// wsHub manages active WebSocket connections.
type wsHub struct {
	mu      sync.Mutex
	clients map[*wsClient]struct{}
}

func newWsHub() *wsHub {
	return &wsHub{clients: make(map[*wsClient]struct{})}
}

func (h *wsHub) add(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

func (h *wsHub) remove(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
}

// snapshot copies the client set so writes never happen under the lock.
func (h *wsHub) snapshot() []*wsClient {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		out = append(out, c)
	}
	return out
}

// broadcast delivers to every connection. Used for aggregate telemetry
// (stats, errors, events), which does not belong to a single user.
func (h *wsHub) broadcast(msg wsMessage) {
	h.deliver(h.snapshot(), msg)
}

// broadcastToUser delivers only to connections permitted to see data owned by
// owner. An owner of "" means the record has no owner, so it is delivered to
// everyone rather than silently dropped.
func (h *wsHub) broadcastToUser(owner string, msg wsMessage) {
	h.deliver(h.selectRecipients(owner), msg)
}

// selectRecipients returns the connections permitted to see data owned by
// owner: the owner themself, any admin, and any anonymous connection.
//
// Anonymous connections only exist when the hub runs without authentication,
// in which case there is no identity to scope by and every session is shared —
// the same assumption requireRead makes when no authz checker is configured.
//
// Split out from broadcastToUser so the decision can be tested without a live
// WebSocket connection.
func (h *wsHub) selectRecipients(owner string) []*wsClient {
	clients := h.snapshot()
	if owner == "" {
		return clients
	}
	selected := make([]*wsClient, 0, len(clients))
	for _, c := range clients {
		if c.admin || c.userID == "" || c.userID == owner {
			selected = append(selected, c)
		}
	}
	return selected
}

func (h *wsHub) deliver(clients []*wsClient, msg wsMessage) {
	for _, c := range clients {
		if err := c.conn.WriteJSON(msg); err != nil {
			slog.Error("websocket write error", "error", err)
			_ = c.conn.Close()
			h.remove(c)
		}
	}
}

// SetWSAllowedOrigins configures Origins permitted to open the WebSocket in
// addition to same-origin requests. Entries may be full origins
// ("https://app.example.com") or bare hosts ("localhost:5173").
func (s *Server) SetWSAllowedOrigins(origins []string) {
	s.wsAllowedOrigins = origins
}

// wsOriginAllowed validates the Origin header on a WebSocket handshake.
//
// This replaces a CheckOrigin that returned true unconditionally, which
// disabled the same-origin check entirely. The hub authenticates with bearer
// tokens and sets no cookies, and a browser cannot attach an Authorization
// header to a WebSocket handshake, so cross-site hijacking is not reachable
// today. The check is enforced anyway because it is the only thing standing
// between a cross-site page and this stream the moment cookie-based sessions
// are introduced — and a `return true` gives no warning when that happens.
//
// A missing Origin means a non-browser client (curl, a native app, the agent
// sidecar), which has no origin to validate and is allowed through.
func (s *Server) wsOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, allowed := range s.wsAllowedOrigins {
		a := strings.TrimSpace(allowed)
		if a == "" {
			continue
		}
		if !strings.Contains(a, "://") {
			a = "//" + a // normalise a bare host into a host-only URL
		}
		au, err := url.Parse(a)
		if err != nil || au.Host == "" {
			continue
		}
		if !strings.EqualFold(au.Host, u.Host) {
			continue
		}
		// Honour the scheme when the operator pinned one, so an https-only
		// entry does not also admit http.
		if au.Scheme != "" && !strings.EqualFold(au.Scheme, u.Scheme) {
			continue
		}
		return true
	}
	return false
}

func (s *Server) handleWs(w http.ResponseWriter, r *http.Request) {
	// Built per request so CheckOrigin can close over this Server's config.
	upgrader := websocket.Upgrader{CheckOrigin: s.wsOriginAllowed}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade error", "error", err)
		return
	}

	client := &wsClient{conn: conn, admin: s.callerIsAdmin(r)}
	if u, ok := oidc.FromContext(r.Context()); ok {
		client.userID = u.Sub
	}

	s.wsHub.add(client)

	// Send initial stats snapshot
	stats := s.GetStats(r.Context())
	if err := conn.WriteJSON(wsMessage{Type: "stats", Data: stats}); err != nil {
		slog.Error("websocket initial stats write error", "error", err)
		_ = conn.Close()
		s.wsHub.remove(client)
		return
	}

	// Read loop — handles pings / client disconnects
	go func() {
		defer func() {
			_ = conn.Close()
			s.wsHub.remove(client)
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					slog.Error("websocket read error", "error", err)
				}
				return
			}
		}
	}()
}

// BroadcastStats calls GetStats and broadcasts a "stats" message to all clients.
func (s *Server) BroadcastStats(ctx context.Context) {
	stats := s.GetStats(ctx)
	s.wsHub.broadcast(wsMessage{Type: "stats", Data: stats})
}

// BroadcastError broadcasts an "error" message to all clients.
func (s *Server) BroadcastError(e types.ErrorEntry) {
	s.wsHub.broadcast(wsMessage{Type: "error", Data: e})
}

// BroadcastEvent broadcasts an "event" message to all clients.
func (s *Server) BroadcastEvent(e types.ActivityEntry) {
	s.wsHub.broadcast(wsMessage{Type: "event", Data: e})
}

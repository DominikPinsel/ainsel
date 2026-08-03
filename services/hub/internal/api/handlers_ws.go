package api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/DominikPinsel/ainsel/services/hub/internal/types"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsMessage is the envelope sent over WebSocket connections.
type wsMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// wsHub manages active WebSocket connections.
type wsHub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

func newWsHub() *wsHub {
	return &wsHub{clients: make(map[*websocket.Conn]struct{})}
}

func (h *wsHub) add(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = struct{}{}
}

func (h *wsHub) remove(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
}

func (h *wsHub) broadcast(msg wsMessage) {
	h.mu.Lock()
	conns := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		conns = append(conns, c)
	}
	h.mu.Unlock()

	for _, c := range conns {
		if err := c.WriteJSON(msg); err != nil {
			slog.Error("websocket write error", "error", err)
			_ = c.Close()
			h.remove(c)
		}
	}
}

func (s *Server) handleWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade error", "error", err)
		return
	}

	s.wsHub.add(conn)

	// Send initial stats snapshot
	stats := s.GetStats(r.Context())
	if err := conn.WriteJSON(wsMessage{Type: "stats", Data: stats}); err != nil {
		slog.Error("websocket initial stats write error", "error", err)
		_ = conn.Close()
		s.wsHub.remove(conn)
		return
	}

	// Read loop — handles pings / client disconnects
	go func() {
		defer func() {
			_ = conn.Close()
			s.wsHub.remove(conn)
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

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWsSelectRecipients pins down the per-connection scoping that replaced a
// broadcast to every client. Before the fix, addChatMessage sent each message
// to all connections, so any authenticated user watching /api/v1/ws received
// every other tenant's conversation in real time.
func TestWsSelectRecipients(t *testing.T) {
	h := newWsHub()
	alice := &wsClient{userID: "alice"}
	bob := &wsClient{userID: "bob"}
	root := &wsClient{userID: "root", admin: true}
	for _, c := range []*wsClient{alice, bob, root} {
		h.add(c)
	}

	contains := func(set []*wsClient, want *wsClient) bool {
		for _, c := range set {
			if c == want {
				return true
			}
		}
		return false
	}

	t.Run("owner message reaches owner and admins only", func(t *testing.T) {
		got := h.selectRecipients("alice")
		if !contains(got, alice) {
			t.Error("alice did not receive her own message")
		}
		if !contains(got, root) {
			t.Error("admin did not receive the message")
		}
		if contains(got, bob) {
			t.Error("bob received alice's message — cross-tenant leak")
		}
	})

	t.Run("unowned message reaches everyone", func(t *testing.T) {
		// A record with no owner has nobody to scope to; dropping it silently
		// would be worse than delivering it, so it goes to all.
		got := h.selectRecipients("")
		if len(got) != 3 {
			t.Errorf("got %d recipients, want 3", len(got))
		}
	})

	t.Run("anonymous hub delivers to all", func(t *testing.T) {
		// No authentication configured: every connection is anonymous and
		// scoping is meaningless, so dev deployments keep working.
		dev := newWsHub()
		c1 := &wsClient{userID: ""}
		c2 := &wsClient{userID: ""}
		dev.add(c1)
		dev.add(c2)
		if got := dev.selectRecipients("alice"); len(got) != 2 {
			t.Errorf("got %d recipients, want 2 (anonymous connections receive everything)", len(got))
		}
	})
}

// TestWsOriginAllowed covers the same-origin check that replaced a
// CheckOrigin returning true unconditionally.
func TestWsOriginAllowed(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		host    string
		allowed []string
		want    bool
	}{
		{"no origin is a non-browser client", "", "hub.example.com", nil, true},
		{"same origin", "https://hub.example.com", "hub.example.com", nil, true},
		{"same origin with port", "https://hub.example.com:8443", "hub.example.com:8443", nil, true},
		{"cross origin is refused", "https://evil.example.com", "hub.example.com", nil, false},
		{"malformed origin is refused", "://bad", "hub.example.com", nil, false},
		{"allowlisted full origin", "https://app.example.com", "hub.example.com", []string{"https://app.example.com"}, true},
		{"allowlisted bare host", "http://localhost:5173", "hub.example.com", []string{"localhost:5173"}, true},
		{"scheme pinned in allowlist is honoured", "http://app.example.com", "hub.example.com", []string{"https://app.example.com"}, false},
		{"other origin still refused with allowlist", "https://evil.example.com", "hub.example.com", []string{"https://app.example.com"}, false},
		{"empty allowlist entries ignored", "https://evil.example.com", "hub.example.com", []string{"", "  "}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{wsAllowedOrigins: tc.allowed}
			r := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if got := s.wsOriginAllowed(r); got != tc.want {
				t.Errorf("wsOriginAllowed(origin=%q, host=%q, allowlist=%v) = %v, want %v",
					tc.origin, tc.host, tc.allowed, got, tc.want)
			}
		})
	}
}

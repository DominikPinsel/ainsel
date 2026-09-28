package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// channelTestHub serves a small fixture of the hub's channel API and records
// the requests it received, so the tests can assert both the routing and the
// name→id resolution the tools perform.
type channelTestHub struct {
	t        *testing.T
	requests []string
	server   *httptest.Server
}

func newChannelTestHub(t *testing.T) *channelTestHub {
	t.Helper()
	h := &channelTestHub{t: t}
	h.server = httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(h.server.Close)
	return h
}

func (h *channelTestHub) serve(w http.ResponseWriter, r *http.Request) {
	h.requests = append(h.requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	switch {
	case r.URL.Path == "/api/v1/channels" && r.Method == http.MethodGet:
		if q := r.URL.Query().Get("kind"); q != "" {
			if q != "connector" {
				writeErrorJSON(w, http.StatusBadRequest, "invalid kind filter")
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"id": "ch-forgejo", "kind": "connector", "name": "Forgejo", "entityRef": "forgejo"}},
				"total": 1, "page": 1, "pageSize": 50, "totalPages": 1,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "ch-forgejo", "kind": "connector", "name": "Forgejo", "entityRef": "forgejo"},
				{"id": "ch-inbox", "kind": "agent", "name": "forgejo", "entityRef": "forgejo"},
				{"id": "ch-group", "kind": "custom", "name": "Nightly bundle", "entityRef": ""},
			},
			"total": 3, "page": 1, "pageSize": 50, "totalPages": 1,
			"window": "24h0m0s",
		})
	case r.URL.Path == "/api/v1/channels" && r.Method == http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if name, _ := body["name"].(string); strings.TrimSpace(name) == "" {
			writeErrorJSON(w, http.StatusBadRequest, "name is required")
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "ch-created", "kind": "custom", "name": body["name"]})
	case r.URL.Path == "/api/v1/channel-subscriptions":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"source": "trigger", "refId": "tg-1", "name": "on issue", "fromChannel": "ch-forgejo", "toChannel": "ch-inbox"},
				{"source": "bridge", "refId": "br-1", "name": "everything", "fromChannel": "ch-forgejo", "toChannel": "ch-group"},
			},
			"total": 2,
		})
	case strings.HasSuffix(r.URL.Path, "/events"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events": []map[string]any{{"id": "evt-1", "connector": "forgejo", "status": "matched"}},
			"total":  1,
		})
	case strings.HasSuffix(r.URL.Path, "/bridges") && r.Method == http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "br-new", "name": body["name"], "fromChannel": "ch-forgejo", "toChannel": body["to"],
		})
	case strings.Contains(r.URL.Path, "/bridges/br-1") && r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(r.URL.Path, "/api/v1/channels/") && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       r.URL.Path[len("/api/v1/channels/"):],
			"incoming": []map[string]any{},
			"outgoing": []map[string]any{
				{"source": "bridge", "refId": "br-1", "name": "everything", "toChannel": "ch-group"},
				{"source": "trigger", "refId": "tg-1", "name": "on issue", "toChannel": "ch-inbox"},
			},
		})
	case strings.HasPrefix(r.URL.Path, "/api/v1/channels/") && r.Method == http.MethodPut:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "ch-group", "kind": "custom", "name": body["name"]})
	case strings.HasPrefix(r.URL.Path, "/api/v1/channels/") && r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		writeErrorJSON(w, http.StatusNotFound, "unexpected path "+r.URL.Path)
	}
}

func writeErrorJSON(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func callTool(t *testing.T, fn func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := fn(context.Background(), req)
	if err != nil {
		t.Fatalf("tool returned go error: %v", err)
	}
	return res
}

func resultText(res *mcp.CallToolResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	return res.Content[0].(mcp.TextContent).Text
}

func TestListChannels(t *testing.T) {
	hub := newChannelTestHub(t)
	ct := &ChannelTools{HubURL: hub.server.URL, HTTPClient: hub.server.Client()}

	res := callTool(t, ct.ListChannels, map[string]any{"pageSize": float64(50)})
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(res))
	}
	if !strings.Contains(resultText(res), "ch-forgejo") {
		t.Fatalf("unexpected payload: %s", resultText(res))
	}
	if !strings.Contains(hub.requests[len(hub.requests)-1], "pageSize=50") {
		t.Errorf("pagination not forwarded: %v", hub.requests)
	}

	res = callTool(t, ct.ListChannels, map[string]any{"kind": "connector"})
	if res.IsError {
		t.Fatalf("filtered list error: %s", resultText(res))
	}
}

func TestGetChannelResolvesNameToID(t *testing.T) {
	hub := newChannelTestHub(t)
	ct := &ChannelTools{HubURL: hub.server.URL, HTTPClient: hub.server.Client()}

	// A unique display name is resolved through the list before the detail fetch.
	res := callTool(t, ct.GetChannel, map[string]any{"name": "Nightly bundle"})
	if res.IsError {
		t.Fatalf("get by name: %s", resultText(res))
	}
	last := hub.requests[len(hub.requests)-1]
	if !strings.Contains(last, "/api/v1/channels/ch-group?") {
		t.Fatalf("expected the resolved id to be fetched, requests: %v", hub.requests)
	}

	// The qualified form reaches the connector stream specifically.
	res = callTool(t, ct.GetChannel, map[string]any{"name": "connector:Forgejo"})
	if res.IsError {
		t.Fatalf("qualified name: %s", resultText(res))
	}
	if last := hub.requests[len(hub.requests)-1]; !strings.Contains(last, "/api/v1/channels/ch-forgejo?") {
		t.Fatalf("expected the connector channel, requests: %v", hub.requests)
	}
}

func TestGetChannelRefusesAmbiguousName(t *testing.T) {
	hub := newChannelTestHub(t)
	ct := &ChannelTools{HubURL: hub.server.URL, HTTPClient: hub.server.Client()}

	// "forgejo" names both a connector stream and an agent inbox. Guessing would
	// hand the caller the wrong stream, so the tool reports the candidates.
	res := callTool(t, ct.GetChannel, map[string]any{"name": "forgejo"})
	if !res.IsError {
		t.Fatalf("expected an ambiguity error, got: %s", resultText(res))
	}
	text := resultText(res)
	if !strings.Contains(text, "connector:forgejo") || !strings.Contains(text, "agent:forgejo") {
		t.Fatalf("ambiguity error should list the candidates: %s", text)
	}

	// The qualified form resolves.
	res = callTool(t, ct.GetChannel, map[string]any{"name": "agent:forgejo"})
	if res.IsError {
		t.Fatalf("qualified name failed: %s", resultText(res))
	}

	// An explicit id skips the lookup entirely.
	hub.requests = hub.requests[:0]
	res = callTool(t, ct.GetChannel, map[string]any{"name": "ch-group"})
	if res.IsError {
		t.Fatalf("by id: %s", resultText(res))
	}
	if len(hub.requests) != 1 {
		t.Errorf("a channel id should need exactly one request, got %v", hub.requests)
	}
}

func TestGetChannelUnknownName(t *testing.T) {
	hub := newChannelTestHub(t)
	ct := &ChannelTools{HubURL: hub.server.URL, HTTPClient: hub.server.Client()}
	res := callTool(t, ct.GetChannel, map[string]any{"name": "nope"})
	if !res.IsError || !strings.Contains(resultText(res), "no channel named") {
		t.Fatalf("expected a clear miss, got %q", resultText(res))
	}
}

func TestListChannelSubscriptions(t *testing.T) {
	hub := newChannelTestHub(t)
	ct := &ChannelTools{HubURL: hub.server.URL, HTTPClient: hub.server.Client()}
	res := callTool(t, ct.ListChannelSubscriptions, nil)
	if res.IsError {
		t.Fatalf("subscriptions: %s", resultText(res))
	}
	text := resultText(res)
	if !strings.Contains(text, "trigger") || !strings.Contains(text, "bridge") {
		t.Fatalf("both edge kinds should be reported: %s", text)
	}
}

func TestGetChannelEvents(t *testing.T) {
	hub := newChannelTestHub(t)
	ct := &ChannelTools{HubURL: hub.server.URL, HTTPClient: hub.server.Client()}
	res := callTool(t, ct.GetChannelEvents, map[string]any{
		"name": "Nightly bundle", "limit": float64(25), "subject": "forgejo.issue_open",
	})
	if res.IsError {
		t.Fatalf("events: %s", resultText(res))
	}
	last := hub.requests[len(hub.requests)-1]
	if !strings.Contains(last, "/events?") || !strings.Contains(last, "limit=25") || !strings.Contains(last, "subject=forgejo") {
		t.Fatalf("filters not forwarded: %s", last)
	}
}

func TestCreateAndDeleteChannel(t *testing.T) {
	hub := newChannelTestHub(t)
	ct := &ChannelTools{HubURL: hub.server.URL, HTTPClient: hub.server.Client()}

	res := callTool(t, ct.CreateChannel, map[string]any{"name": "Bundle", "description": "d", "groupId": "g1"})
	if res.IsError {
		t.Fatalf("create: %s", resultText(res))
	}
	if !strings.Contains(resultText(res), "ch-created") {
		t.Fatalf("create response: %s", resultText(res))
	}

	// A blank name is refused client-side, without a hub round trip.
	before := len(hub.requests)
	res = callTool(t, ct.CreateChannel, map[string]any{"name": "   "})
	if !res.IsError || len(hub.requests) != before {
		t.Fatalf("blank name should fail locally, requests: %v", hub.requests)
	}

	res = callTool(t, ct.DeleteChannel, map[string]any{"name": "Nightly bundle"})
	if res.IsError {
		t.Fatalf("delete: %s", resultText(res))
	}

	res = callTool(t, ct.UpdateChannel, map[string]any{"name": "ch-group"})
	if !res.IsError {
		t.Fatal("an update with nothing to change should be refused")
	}
	res = callTool(t, ct.UpdateChannel, map[string]any{"name": "ch-group", "displayName": "Renamed"})
	if res.IsError {
		t.Fatalf("update: %s", resultText(res))
	}
}

func TestAttachAndDetachBridge(t *testing.T) {
	hub := newChannelTestHub(t)
	ct := &ChannelTools{HubURL: hub.server.URL, HTTPClient: hub.server.Client()}

	res := callTool(t, ct.AttachChannelBridge, map[string]any{"from": "connector:forgejo", "to": "custom:Nightly bundle"})
	if res.IsError {
		t.Fatalf("attach: %s", resultText(res))
	}
	// The POST must carry the resolved target id, not the label the caller used.
	var posted bool
	for _, r := range hub.requests {
		if strings.HasPrefix(r, "POST /api/v1/channels/ch-forgejo/bridges") {
			posted = true
		}
	}
	if !posted {
		t.Fatalf("bridge not posted to the resolved source channel: %v", hub.requests)
	}

	// Detach finds the bridge id from the source channel's own edges.
	res = callTool(t, ct.DetachChannelBridge, map[string]any{"from": "ch-forgejo", "to": "ch-group"})
	if res.IsError {
		t.Fatalf("detach: %s", resultText(res))
	}
	var deleted bool
	for _, r := range hub.requests {
		if strings.HasPrefix(r, "DELETE /api/v1/channels/ch-forgejo/bridges/br-1") {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("expected the bridge id to be used, requests: %v", hub.requests)
	}

	// A trigger edge is not a bridge: detaching a connector→agent pair that has
	// no bridge must report that rather than deleting the trigger.
	res = callTool(t, ct.DetachChannelBridge, map[string]any{"from": "ch-forgejo", "to": "ch-inbox"})
	if !res.IsError || !strings.Contains(resultText(res), "no bridge") {
		t.Fatalf("expected 'no bridge' for a trigger edge, got %q", resultText(res))
	}
}

func TestChannelToolsRegistered(t *testing.T) {
	// Every tool the package advertises must be wired into the server, or an
	// agent sees it in docs and gets "unknown tool" at runtime.
	hub := newChannelTestHub(t)
	ct := &ChannelTools{HubURL: hub.server.URL, HTTPClient: hub.server.Client()}
	names := []string{
		ct.ListChannelsTool().Name,
		ct.GetChannelTool().Name,
		ct.ListChannelSubscriptionsTool().Name,
		ct.GetChannelEventsTool().Name,
		ct.CreateChannelTool().Name,
		ct.UpdateChannelTool().Name,
		ct.DeleteChannelTool().Name,
		ct.AttachChannelBridgeTool().Name,
		ct.DetachChannelBridgeTool().Name,
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("duplicate tool name %q", n)
		}
		seen[n] = true
		if !strings.HasPrefix(n, "channel") && !strings.Contains(n, "channel") {
			t.Errorf("tool %q should be namespaced under channels", n)
		}
	}
}

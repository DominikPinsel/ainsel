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

func TestGetStreamInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/queue/info" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"stream":    map[string]any{"name": "EVENTS", "messages": 42},
			"consumers": []any{},
		})
	}))
	defer srv.Close()

	et := NewEventTools(srv.URL)
	result, err := et.GetStreamInfo(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
}

func TestListRecentEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/queue/recent" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"subject": "forgejo.push", "data": `{"type":"push"}`, "sequence": 1, "timestamp": "2024-01-01T00:00:00Z"},
		})
	}))
	defer srv.Close()

	et := NewEventTools(srv.URL)
	result, err := et.ListRecentEvents(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
}

func recentEventsHub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/queue/recent" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":        "evt-1",
				"connector": "forgejo",
				"headers": map[string]any{
					"X-Gitea-Event":    "issue_comment",
					"X-Gitea-Delivery": "abc-123",
					"Content-Type":     "application/json",
					"Authorization":    "Bearer super-secret",
					"X-Forwarded-For":  "10.0.0.1",
					"Accept-Encoding":  "gzip",
				},
				"data":        `{"action":"created","repository":{"full_name":"org/repo"},"sender":{"login":"alice"},"issue":{"number":42,"title":"Something broke"},"commits":[{"id":1},{"id":2}],"huge":"lots of extra payload fields that should be dropped"}`,
				"raw":         `{"action":"created","duplicate":"of the entire body"}`,
				"received_at": "2024-01-01T00:00:00Z",
			},
		})
	}))
}

func TestListRecentEvents_SummarizedByDefault(t *testing.T) {
	srv := recentEventsHub(t)
	defer srv.Close()

	et := NewEventTools(srv.URL)
	result, err := et.ListRecentEvents(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
	text := result.Content[0].(mcp.TextContent).Text

	// Duplicates and noise must be gone.
	for _, banned := range []string{`"raw"`, "duplicate", "Authorization", "X-Forwarded-For", "huge"} {
		if strings.Contains(text, banned) {
			t.Errorf("summarized response should not contain %q; got: %s", banned, text)
		}
	}
	// Identity and summary fields must survive.
	for _, want := range []string{"evt-1", "forgejo", "issue_comment", `"action":"created"`, "org/repo", "alice", `"number":42`, `"commitCount":2`, `"payloadBytes"`} {
		if !strings.Contains(text, want) {
			t.Errorf("summarized response missing %q; got: %s", want, text)
		}
	}
}

func TestListRecentEvents_FullOptIn(t *testing.T) {
	srv := recentEventsHub(t)
	defer srv.Close()

	et := NewEventTools(srv.URL)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"full": true}
	result, err := et.ListRecentEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"raw"`) {
		t.Errorf("full=true should keep raw payload; got: %s", text)
	}
	if !strings.Contains(text, "Authorization") {
		t.Errorf("full=true should keep all headers; got: %s", text)
	}
}

func TestSummarizeEvent_NonObjectData(t *testing.T) {
	evt := map[string]any{
		"id":        "evt-2",
		"connector": "github",
		"data":      `["not","an","object"]`,
		"raw":       `["not","an","object"]`,
	}
	out := summarizeEvent(evt)
	if _, hasRaw := out["raw"]; hasRaw {
		t.Error("raw must be dropped")
	}
	if _, hasData := out["data"]; hasData {
		t.Error("full data must be dropped")
	}
	if out["payloadBytes"] != len(`["not","an","object"]`) {
		t.Errorf("unexpected payloadBytes: %v", out["payloadBytes"])
	}
	if _, hasSummary := out["summary"]; hasSummary {
		t.Error("non-object data should not produce a summary")
	}
}

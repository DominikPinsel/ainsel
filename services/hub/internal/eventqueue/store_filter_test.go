package eventqueue

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestParseSubjectFilter(t *testing.T) {
	tests := []struct {
		pattern string
		want    SubjectFilter
	}{
		{"", SubjectFilter{}},
		{">", SubjectFilter{}},
		{"*", SubjectFilter{}},
		{"forgejo", SubjectFilter{Connector: "forgejo"}},
		{"forgejo.push", SubjectFilter{Connector: "forgejo", EventType: "push"}},
		{"Forgejo.Push", SubjectFilter{Connector: "forgejo", EventType: "push"}},
		{"forgejo.*", SubjectFilter{Connector: "forgejo"}},
		{"forgejo.>", SubjectFilter{Connector: "forgejo"}},
		{"*.push", SubjectFilter{EventType: "push"}},
		{">.push", SubjectFilter{EventType: "push"}},
		{"  forgejo.push  ", SubjectFilter{Connector: "forgejo", EventType: "push"}},
		// Subjects have exactly two levels; deeper non-wildcard tokens never match.
		{"forgejo.push.extra", SubjectFilter{None: true}},
		{"forgejo..push", SubjectFilter{None: true}},
		{"forgejo.push.>", SubjectFilter{Connector: "forgejo", EventType: "push"}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.pattern), func(t *testing.T) {
			got := ParseSubjectFilter(tt.pattern)
			if got != tt.want {
				t.Errorf("ParseSubjectFilter(%q) = %+v, want %+v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestSubjectFilterMatches(t *testing.T) {
	tests := []struct {
		name      string
		filter    SubjectFilter
		connector string
		eventType string
		want      bool
	}{
		{"zero matches all", SubjectFilter{}, "forgejo", "push", true},
		{"connector match", SubjectFilter{Connector: "forgejo"}, "Forgejo", "anything", true},
		{"connector mismatch", SubjectFilter{Connector: "github"}, "forgejo", "push", false},
		{"type match", SubjectFilter{EventType: "push"}, "forgejo", "Push", true},
		{"type mismatch", SubjectFilter{EventType: "push"}, "forgejo", "issues", false},
		{"both match", SubjectFilter{Connector: "forgejo", EventType: "push"}, "forgejo", "push", true},
		{"none never matches", SubjectFilter{None: true}, "forgejo", "push", false},
		{"empty event type with type constraint", SubjectFilter{EventType: "push"}, "forgejo", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Matches(tt.connector, tt.eventType); got != tt.want {
				t.Errorf("Matches(%q, %q) = %v, want %v", tt.connector, tt.eventType, got, tt.want)
			}
		})
	}
}

// TestStore_RecentEventsSubjectFilter verifies that the SQL-side filtering
// agrees with ParseSubjectFilter semantics. Requires TEST_DB_URL.
func TestStore_RecentEventsSubjectFilter(t *testing.T) {
	pool := testPool(t)
	s := NewStore(pool)
	ctx := context.Background()

	base := time.Now().UTC().Format("20060102150405")
	events := []struct {
		id        string
		connector string
		headers   string
	}{
		{fmt.Sprintf("test-sf-%s-1", base), "forgejo", `{"X-Gitea-Event": "push"}`},
		{fmt.Sprintf("test-sf-%s-2", base), "forgejo", `{"X-Gitea-Event": "issue_comment"}`},
		{fmt.Sprintf("test-sf-%s-3", base), "github", `{"X-GitHub-Event": "push"}`},
		{fmt.Sprintf("test-sf-%s-4", base), "other", `{"type": "push"}`},
		{fmt.Sprintf("test-sf-%s-5", base), "forgejo", `{}`},
	}
	for _, e := range events {
		if err := s.InsertEvent(ctx, Event{
			ID: e.id, Connector: e.connector,
			Headers: []byte(e.headers), Data: []byte(`{}`),
		}); err != nil {
			t.Fatalf("insert event: %v", err)
		}
	}
	t.Cleanup(func() {
		for _, e := range events {
			_, _ = pool.Exec(ctx, "DELETE FROM events WHERE id = $1", e.id)
		}
	})

	tests := []struct {
		filter string
		want   int
	}{
		{"", 5},
		{"forgejo.push", 1},
		{"Forgejo.Push", 1},
		{"forgejo.*", 3},
		{"forgejo.>", 3},
		{"*.push", 3},
		{"github.issue_comment", 0},
		{"forgejo.push.extra", 0},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("filter=%q", tt.filter), func(t *testing.T) {
			got, err := s.RecentEvents(ctx, 100, "", tt.filter, time.Time{})
			if err != nil {
				t.Fatalf("RecentEvents: %v", err)
			}
			matched := 0
			for _, evt := range got {
				if len(evt.ID) > 8 && evt.ID[:8] == "test-sf-" {
					matched++
				}
			}
			if matched != tt.want {
				t.Errorf("filter %q matched %d test events, want %d", tt.filter, matched, tt.want)
			}
		})
	}
}

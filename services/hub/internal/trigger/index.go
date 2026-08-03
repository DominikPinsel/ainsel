package trigger

import (
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

// MatchResult holds information about a trigger that matched an event.
type MatchResult struct {
	TriggerName string
	AgentRef    string
}

// Index is a thread-safe in-memory index of Trigger objects that supports
// matching incoming events against registered triggers.
type Index struct {
	mu       sync.RWMutex
	triggers map[string]*triggers.Trigger
}

// NewIndex creates a new empty trigger index.
func NewIndex() *Index {
	return &Index{triggers: make(map[string]*triggers.Trigger)}
}

// Update adds or replaces a trigger in the index.
func (idx *Index) Update(t *triggers.Trigger) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.triggers[t.ID] = &triggers.Trigger{
		ID:             t.ID,
		DisplayName:    t.DisplayName,
		AgentRef:       t.AgentRef,
		ConnectorRef:   t.ConnectorRef,
		Filters:        t.Filters,
		AgentValid:     t.AgentValid,
		ConnectorValid: t.ConnectorValid,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}
}

// Delete removes a trigger from the index by its id.
func (idx *Index) Delete(id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.triggers, id)
}

// Keys returns the IDs of all triggers currently in the index.
// Used by the sync loop to detect and remove stale entries that no longer
// exist in the database.
func (idx *Index) Keys() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	keys := make([]string, 0, len(idx.triggers))
	for k := range idx.triggers {
		keys = append(keys, k)
	}
	return keys
}

// Match returns all triggers that match the given event. When multiple
// triggers match the same event, callers should use DeduplicateByAgent to
// ensure each agent receives the event at most once.
func (idx *Index) Match(evt *ainselapishared.Event) []MatchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Build payload once per event; reused across all trigger evaluations.
	payload := buildFilterPayload(evt)

	var results []MatchResult
	for _, t := range idx.triggers {
		if !isValid(t) {
			slog.Info("trigger not valid", "trigger", t.ID)
			continue
		}
		if t.ConnectorRef != evt.Connector {
			slog.Info("trigger connector mismatch", "trigger", t.ID, "triggerConnector", t.ConnectorRef, "eventConnector", evt.Connector)
			continue
		}
		if len(t.Filters) > 0 {
			if !ainselapishared.MatchFilters(t.Filters, payload) {
				slog.Info("trigger filter did not match", "trigger", t.ID, "filters", t.Filters)
				continue
			}
		}
		results = append(results, MatchResult{
			TriggerName: t.ID,
			AgentRef:    t.AgentRef,
		})
	}
	return results
}

// buildFilterPayload merges event body fields and headers into a single map
// for filter evaluation. Body fields are at the top level; headers are nested
// under the "headers" key, allowing filters like {"field": "headers.X-GitHub-Event"}.
// A canonical "type" key is also derived from the webhook event-type header
// (any header ending in "-Event") so that triggers can match on event type
// with a filter like {field: "type", op: "eq", value: "push"}.
func buildFilterPayload(evt *ainselapishared.Event) map[string]any {
	payload := map[string]any{}
	if len(evt.Data) > 0 {
		_ = json.Unmarshal(evt.Data, &payload)
	}
	if len(evt.Headers) > 0 {
		hmap := make(map[string]any, len(evt.Headers))
		for k, v := range evt.Headers {
			hmap[k] = v
		}
		payload["headers"] = hmap

		if t := CanonicalEventType(evt.Headers); t != "" {
			payload["type"] = t
		}
	}
	return payload
}

// CanonicalEventType extracts the event type from HTTP webhook headers.
// It scans for any header ending in "-Event" (e.g. X-Forgejo-Event,
// X-Gitea-Event, X-Github-Event, X-GitLab-Event) so the system works with
// any webhook source without hardcoding provider-specific header names.
// If no "-Event" header is found it falls back to a generic "type" header.
func CanonicalEventType(headers map[string]string) string {
	for h, v := range headers {
		if strings.HasSuffix(h, "-Event") && v != "" {
			return v
		}
	}
	return headers["type"]
}

// DeduplicateByAgent removes duplicate agent routings from a match result
// slice. When multiple triggers route to the same agent, only the first match
// (sorted by trigger name for determinism) is kept. This prevents an agent
// from receiving the same event twice, which would cause duplicate work
// (e.g. creating two PRs for the same issue).
//
// The function sorts matches by trigger name first so that deduplication is
// deterministic regardless of map iteration order. When a Priority field is
// added to TriggerSpec (see ainsel/ainsel-api-shared#25), the sort should
// be changed to sort by priority instead.
func DeduplicateByAgent(matches []MatchResult) []MatchResult {
	if len(matches) <= 1 {
		return matches
	}

	// Sort by trigger name for deterministic ordering.
	// When TriggerSpec gains a Priority field, sort by priority (ascending)
	// so that lower-numbered (higher-priority) triggers win.
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].TriggerName < matches[j].TriggerName
	})

	seen := make(map[string]bool, len(matches))
	deduped := make([]MatchResult, 0, len(matches))
	for _, m := range matches {
		if seen[m.AgentRef] {
			continue
		}
		seen[m.AgentRef] = true
		deduped = append(deduped, m)
	}
	return deduped
}

// isValid checks that a trigger has both AgentValid and ConnectorValid set.
func isValid(t *triggers.Trigger) bool {
	return t.AgentValid && t.ConnectorValid
}

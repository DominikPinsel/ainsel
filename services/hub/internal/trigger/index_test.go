package trigger

import (
	"encoding/json"
	"testing"

	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

func makeTrigger(id, agentRef, connectorRef string, filters []ainselapishared.Filter) *triggers.Trigger {
	return &triggers.Trigger{
		ID:             id,
		AgentRef:       agentRef,
		ConnectorRef:   connectorRef,
		Filters:        filters,
		AgentValid:     true,
		ConnectorValid: true,
	}
}

func makeEvent(connector string, data map[string]any) *ainselapishared.Event {
	d, _ := json.Marshal(data)
	return &ainselapishared.Event{
		Connector: connector,
		Data:      d,
	}
}

func TestExactMatch(t *testing.T) {
	idx := NewIndex()
	idx.Update(makeTrigger("t1", "agent-a", "forgejo", nil))

	results := idx.Match(makeEvent("forgejo", nil))
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].TriggerName != "t1" || results[0].AgentRef != "agent-a" {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}

func TestConnectorMismatch(t *testing.T) {
	idx := NewIndex()
	idx.Update(makeTrigger("t1", "agent-a", "forgejo", nil))

	results := idx.Match(makeEvent("github", nil))
	if len(results) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(results))
	}
}

func TestFilterMatch(t *testing.T) {
	idx := NewIndex()
	filters := []ainselapishared.Filter{
		{Field: "comment.body", Op: "contains", Value: "@dev-agent"},
	}
	idx.Update(makeTrigger("t1", "agent-a", "forgejo", filters))

	data := map[string]any{
		"comment": map[string]any{
			"body": "Hey @dev-agent please review this",
		},
	}
	results := idx.Match(makeEvent("forgejo", data))
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
}

func TestFilterMismatch(t *testing.T) {
	idx := NewIndex()
	filters := []ainselapishared.Filter{
		{Field: "comment.body", Op: "contains", Value: "@dev-agent"},
	}
	idx.Update(makeTrigger("t1", "agent-a", "forgejo", filters))

	data := map[string]any{
		"comment": map[string]any{
			"body": "Just a regular comment",
		},
	}
	results := idx.Match(makeEvent("forgejo", data))
	if len(results) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(results))
	}
}

func TestInvalidTriggerSkipped(t *testing.T) {
	idx := NewIndex()
	tr := makeTrigger("t1", "agent-a", "forgejo", nil)
	tr.AgentValid = false
	tr.ConnectorValid = false
	idx.Update(tr)

	results := idx.Match(makeEvent("forgejo", nil))
	if len(results) != 0 {
		t.Fatalf("expected 0 matches (invalid trigger), got %d", len(results))
	}
}

func TestMultipleMatches(t *testing.T) {
	idx := NewIndex()
	idx.Update(makeTrigger("t1", "agent-a", "forgejo", nil))
	idx.Update(makeTrigger("t2", "agent-b", "forgejo", nil))

	results := idx.Match(makeEvent("forgejo", nil))
	if len(results) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(results))
	}

	names := map[string]bool{}
	for _, r := range results {
		names[r.TriggerName] = true
	}
	if !names["t1"] || !names["t2"] {
		t.Fatalf("expected both t1 and t2, got %+v", results)
	}
}

func TestUpdateAndDelete(t *testing.T) {
	idx := NewIndex()
	idx.Update(makeTrigger("t1", "agent-a", "forgejo", nil))

	// Verify initial match
	results := idx.Match(makeEvent("forgejo", nil))
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}

	// Update trigger to different connector
	idx.Update(makeTrigger("t1", "agent-a", "github", nil))
	results = idx.Match(makeEvent("forgejo", nil))
	if len(results) != 0 {
		t.Fatalf("expected 0 matches after update, got %d", len(results))
	}

	// Verify new connector matches
	results = idx.Match(makeEvent("github", nil))
	if len(results) != 1 {
		t.Fatalf("expected 1 match for github, got %d", len(results))
	}

	// Delete trigger
	idx.Delete("t1")
	results = idx.Match(makeEvent("github", nil))
	if len(results) != 0 {
		t.Fatalf("expected 0 matches after delete, got %d", len(results))
	}
}

func TestMatchWithHeaderFilter(t *testing.T) {
	idx := NewIndex()
	trigger := makeTrigger("t1", "agent1", "conn1", []ainselapishared.Filter{
		{Field: "headers.X-GitHub-Event", Op: "eq", Value: "issues"},
	})
	idx.Update(trigger)

	evt := &ainselapishared.Event{
		Connector: "conn1",
		Headers:   map[string]string{"X-GitHub-Event": "issues"},
		Data:      ainselapishared.RawJSON(`{"action":"opened"}`),
	}
	results := idx.Match(evt)
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
}

func TestMatchHeaderFilterMismatch(t *testing.T) {
	idx := NewIndex()
	trigger := makeTrigger("t1", "agent1", "conn1", []ainselapishared.Filter{
		{Field: "headers.X-GitHub-Event", Op: "eq", Value: "pull_request"},
	})
	idx.Update(trigger)

	evt := &ainselapishared.Event{
		Connector: "conn1",
		Headers:   map[string]string{"X-GitHub-Event": "issues"},
		Data:      ainselapishared.RawJSON(`{"action":"opened"}`),
	}
	results := idx.Match(evt)
	if len(results) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(results))
	}
}

func TestDeduplicateByAgent_NoDuplicates(t *testing.T) {
	matches := []MatchResult{
		{TriggerName: "t1", AgentRef: "agent-a"},
		{TriggerName: "t2", AgentRef: "agent-b"},
	}
	deduped := DeduplicateByAgent(matches)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 results (no duplicates), got %d", len(deduped))
	}
}

func TestDeduplicateByAgent_SameAgentDifferentTriggers(t *testing.T) {
	// This is the core bug scenario: two triggers routing to the same agent.
	// Only the first (alphabetically by trigger name) should be kept.
	matches := []MatchResult{
		{TriggerName: "dev-issues", AgentRef: "developer"},
		{TriggerName: "dev-all-issues", AgentRef: "developer"},
	}
	deduped := DeduplicateByAgent(matches)

	if len(deduped) != 1 {
		t.Fatalf("expected 1 result after dedup, got %d", len(deduped))
	}
	// Alphabetically, "dev-all-issues" < "dev-issues", so it wins.
	if deduped[0].TriggerName != "dev-all-issues" {
		t.Fatalf("expected trigger dev-all-issues, got %s", deduped[0].TriggerName)
	}
}

func TestDeduplicateByAgent_MixedAgents(t *testing.T) {
	// Two matches for the same agent + one for a different agent.
	// Only one per agent should remain.
	matches := []MatchResult{
		{TriggerName: "t1", AgentRef: "developer"},
		{TriggerName: "t2", AgentRef: "reviewer"},
		{TriggerName: "t3", AgentRef: "developer"},
	}
	deduped := DeduplicateByAgent(matches)

	if len(deduped) != 2 {
		t.Fatalf("expected 2 results after dedup, got %d", len(deduped))
	}

	agents := map[string]bool{}
	for _, m := range deduped {
		agents[m.AgentRef] = true
	}
	if !agents["developer"] || !agents["reviewer"] {
		t.Fatalf("expected developer and reviewer, got %v", deduped)
	}
}

func TestDeduplicateByAgent_Empty(t *testing.T) {
	deduped := DeduplicateByAgent(nil)
	if len(deduped) != 0 {
		t.Fatalf("expected 0 results for nil, got %d", len(deduped))
	}

	deduped = DeduplicateByAgent([]MatchResult{})
	if len(deduped) != 0 {
		t.Fatalf("expected 0 results for empty, got %d", len(deduped))
	}
}

func TestDeduplicateByAgent_SingleMatch(t *testing.T) {
	matches := []MatchResult{
		{TriggerName: "t1", AgentRef: "developer"},
	}
	deduped := DeduplicateByAgent(matches)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 result, got %d", len(deduped))
	}
	if deduped[0].TriggerName != "t1" {
		t.Fatalf("expected trigger t1, got %s", deduped[0].TriggerName)
	}
}

func TestDeduplicateByAgent_DeterministicOrdering(t *testing.T) {
	// Verify that dedup is deterministic regardless of input order.
	// Three triggers for the same agent — should always keep alphabetically first.
	matches1 := []MatchResult{
		{TriggerName: "z-trigger", AgentRef: "developer"},
		{TriggerName: "a-trigger", AgentRef: "developer"},
		{TriggerName: "m-trigger", AgentRef: "developer"},
	}
	deduped1 := DeduplicateByAgent(matches1)

	matches2 := []MatchResult{
		{TriggerName: "m-trigger", AgentRef: "developer"},
		{TriggerName: "z-trigger", AgentRef: "developer"},
		{TriggerName: "a-trigger", AgentRef: "developer"},
	}
	deduped2 := DeduplicateByAgent(matches2)

	if len(deduped1) != 1 || len(deduped2) != 1 {
		t.Fatalf("expected 1 result each, got %d and %d", len(deduped1), len(deduped2))
	}
	if deduped1[0].TriggerName != "a-trigger" {
		t.Fatalf("expected a-trigger, got %s", deduped1[0].TriggerName)
	}
	if deduped2[0].TriggerName != "a-trigger" {
		t.Fatalf("expected a-trigger, got %s", deduped2[0].TriggerName)
	}
}

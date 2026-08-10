package invocations

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestStore_RecordAssignsIDAndDefaults(t *testing.T) {
	s := NewStore(10)
	rec := s.Record(Invocation{
		AgentName:   "dev-agent",
		TriggerName: "issue-assigned",
	})
	if rec.ID == "" {
		t.Fatal("expected ID to be generated")
	}
	if rec.Status != StatusRunning {
		t.Errorf("expected status %q, got %q", StatusRunning, rec.Status)
	}
	if rec.StartTime.IsZero() {
		t.Error("expected StartTime to be set")
	}
	if rec.EndTime != nil {
		t.Errorf("expected EndTime nil for running invocation, got %v", rec.EndTime)
	}
}

func TestStore_GetReturnsCopy(t *testing.T) {
	s := NewStore(10)
	rec := s.Record(Invocation{AgentName: "a1"})
	got, ok := s.Get(rec.ID)
	if !ok {
		t.Fatal("expected to find invocation")
	}
	if got.AgentName != "a1" {
		t.Errorf("expected agent a1, got %q", got.AgentName)
	}
	// Mutating the returned copy should not affect the store.
	got.AgentName = "mutated"
	got2, _ := s.Get(rec.ID)
	if got2.AgentName != "a1" {
		t.Errorf("store record was mutated externally: got %q", got2.AgentName)
	}
}

func TestStore_CompleteSetsTerminalFields(t *testing.T) {
	s := NewStore(10)
	rec := s.Record(Invocation{AgentName: "a1"})
	// Sleep briefly to ensure measurable duration.
	time.Sleep(2 * time.Millisecond)

	ok := s.Complete(rec.ID, StatusSuccess, "", time.Time{})
	if !ok {
		t.Fatal("expected Complete to succeed")
	}
	got, _ := s.Get(rec.ID)
	if got.Status != StatusSuccess {
		t.Errorf("expected status %q, got %q", StatusSuccess, got.Status)
	}
	if got.EndTime == nil {
		t.Fatal("expected EndTime to be set")
	}
	if got.DurationMs == nil || *got.DurationMs < 0 {
		t.Errorf("expected non-negative duration, got %v", got.DurationMs)
	}
	if !got.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestStore_CompleteFailureRecordsError(t *testing.T) {
	s := NewStore(10)
	rec := s.Record(Invocation{AgentName: "a1"})
	end := time.Now().UTC()
	if !s.Complete(rec.ID, StatusFailure, "boom", end) {
		t.Fatal("expected Complete to succeed")
	}
	got, _ := s.Get(rec.ID)
	if got.Error != "boom" {
		t.Errorf("expected error %q, got %q", "boom", got.Error)
	}
	if got.Status != StatusFailure {
		t.Errorf("expected status failure, got %q", got.Status)
	}
}

func TestStore_CompleteUnknownIDReturnsFalse(t *testing.T) {
	s := NewStore(10)
	if s.Complete("inv-deadbeef", StatusSuccess, "", time.Time{}) {
		t.Error("expected Complete to return false for unknown ID")
	}
}

func TestStore_RingBufferEvictsOldest(t *testing.T) {
	s := NewStore(3)
	r1 := s.Record(Invocation{AgentName: "a1"})
	r2 := s.Record(Invocation{AgentName: "a2"})
	r3 := s.Record(Invocation{AgentName: "a3"})
	if s.Len() != 3 {
		t.Fatalf("expected 3, got %d", s.Len())
	}

	// Recording a fourth should evict r1.
	r4 := s.Record(Invocation{AgentName: "a4"})
	if s.Len() != 3 {
		t.Fatalf("expected 3 after eviction, got %d", s.Len())
	}
	if _, ok := s.Get(r1.ID); ok {
		t.Error("expected r1 to be evicted")
	}
	for _, id := range []string{r2.ID, r3.ID, r4.ID} {
		if _, ok := s.Get(id); !ok {
			t.Errorf("expected %s to be present", id)
		}
	}
}

func TestStore_ListNewestFirst(t *testing.T) {
	s := NewStore(10)
	for i := 0; i < 5; i++ {
		s.Record(Invocation{
			AgentName: fmt.Sprintf("a%d", i),
			StartTime: time.Now().Add(time.Duration(i) * time.Millisecond).UTC(),
		})
	}
	list := s.List(ListOptions{})
	if len(list) != 5 {
		t.Fatalf("expected 5, got %d", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].StartTime.Before(list[i].StartTime) {
			t.Errorf("list not newest-first at index %d", i)
		}
	}
}

func TestStore_ListFiltersAndLimit(t *testing.T) {
	s := NewStore(10)
	now := time.Now().UTC()
	a := s.Record(Invocation{AgentName: "agent-a", StartTime: now.Add(-30 * time.Minute), Status: StatusRunning})
	b := s.Record(Invocation{AgentName: "agent-b", StartTime: now.Add(-20 * time.Minute), Status: StatusRunning})
	c := s.Record(Invocation{AgentName: "agent-a", StartTime: now.Add(-10 * time.Minute), Status: StatusRunning})
	s.Complete(b.ID, StatusFailure, "x", now)

	// Filter by agent.
	gotA := s.List(ListOptions{AgentName: "agent-a"})
	if len(gotA) != 2 {
		t.Errorf("expected 2 for agent-a, got %d", len(gotA))
	}

	// Filter by status (b is failure now).
	gotF := s.List(ListOptions{Status: StatusFailure})
	if len(gotF) != 1 || gotF[0].ID != b.ID {
		t.Errorf("expected only b for status=failure, got %+v", gotF)
	}

	// Filter by since (only c is within last 15m).
	gotS := s.List(ListOptions{Since: now.Add(-15 * time.Minute)})
	if len(gotS) != 1 || gotS[0].ID != c.ID {
		t.Errorf("expected only c for since=-15m, got %+v", gotS)
	}

	// Limit.
	gotL := s.List(ListOptions{Limit: 2})
	if len(gotL) != 2 {
		t.Errorf("expected limit 2, got %d", len(gotL))
	}

	// silence "declared and not used" for a in case all checks above pass.
	_ = a
}

func TestStore_ListWithTotalReportsPreLimitCount(t *testing.T) {
	s := NewStore(10)
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		s.Record(Invocation{AgentName: "agent-a", StartTime: now.Add(time.Duration(-i) * time.Minute)})
	}
	s.Record(Invocation{AgentName: "agent-b", StartTime: now})

	// Without a limit, items and total agree.
	items, total := s.ListWithTotal(ListOptions{})
	if len(items) != 6 || total != 6 {
		t.Errorf("expected 6/6 without limit, got %d/%d", len(items), total)
	}

	// With a limit, items are capped but total still reflects all matches.
	items, total = s.ListWithTotal(ListOptions{Limit: 2})
	if len(items) != 2 {
		t.Errorf("expected 2 items with limit=2, got %d", len(items))
	}
	if total != 6 {
		t.Errorf("expected total 6 with limit=2, got %d", total)
	}

	// Filters are applied before the limit: only agent-a matches.
	items, total = s.ListWithTotal(ListOptions{AgentName: "agent-a", Limit: 2})
	if len(items) != 2 {
		t.Errorf("expected 2 items for agent-a with limit=2, got %d", len(items))
	}
	if total != 5 {
		t.Errorf("expected total 5 for agent-a, got %d", total)
	}
}

func TestStore_ListFilterByTriggerName(t *testing.T) {
	s := NewStore(10)
	now := time.Now().UTC()
	s.Record(Invocation{AgentName: "agent-a", TriggerName: "trigger-1", StartTime: now.Add(-2 * time.Minute)})
	s.Record(Invocation{AgentName: "agent-b", TriggerName: "trigger-2", StartTime: now.Add(-1 * time.Minute)})
	s.Record(Invocation{AgentName: "agent-a", TriggerName: "trigger-1", StartTime: now})

	got := s.List(ListOptions{TriggerName: "trigger-1"})
	if len(got) != 2 {
		t.Errorf("expected 2 for trigger-1, got %d", len(got))
	}
	for _, inv := range got {
		if inv.TriggerName != "trigger-1" {
			t.Errorf("expected trigger-1, got %q", inv.TriggerName)
		}
	}

	gotMiss := s.List(ListOptions{TriggerName: "missing"})
	if len(gotMiss) != 0 {
		t.Errorf("expected 0 for missing trigger, got %d", len(gotMiss))
	}
}

func TestStore_ListFilterByUntil(t *testing.T) {
	s := NewStore(10)
	now := time.Now().UTC()
	s.Record(Invocation{AgentName: "a", StartTime: now.Add(-30 * time.Minute)})
	s.Record(Invocation{AgentName: "b", StartTime: now.Add(-10 * time.Minute)})
	s.Record(Invocation{AgentName: "c", StartTime: now.Add(-1 * time.Minute)})

	// until 15 minutes ago should include a (30m ago) and exclude b (10m ago) and c (1m ago)
	got := s.List(ListOptions{Until: now.Add(-15 * time.Minute)})
	if len(got) != 1 {
		t.Fatalf("expected 1 for until=-15m, got %d", len(got))
	}
	if got[0].AgentName != "a" {
		t.Errorf("expected agent a, got %q", got[0].AgentName)
	}
}

func TestStore_ListFilterBySinceAndUntil(t *testing.T) {
	s := NewStore(10)
	now := time.Now().UTC()
	s.Record(Invocation{AgentName: "old", StartTime: now.Add(-60 * time.Minute)})
	s.Record(Invocation{AgentName: "mid", StartTime: now.Add(-20 * time.Minute)})
	s.Record(Invocation{AgentName: "recent", StartTime: now.Add(-5 * time.Minute)})

	// between 30m ago and 10m ago should match only "mid"
	got := s.List(ListOptions{
		Since: now.Add(-30 * time.Minute),
		Until: now.Add(-10 * time.Minute),
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 for since/until window, got %d", len(got))
	}
	if got[0].AgentName != "mid" {
		t.Errorf("expected agent mid, got %q", got[0].AgentName)
	}
}

func TestStore_ListCombinedFilters(t *testing.T) {
	s := NewStore(20)
	now := time.Now().UTC()

	// Create invocations with different combinations.
	s.Record(Invocation{AgentName: "agent-a", TriggerName: "t1", Status: StatusSuccess, StartTime: now.Add(-60 * time.Minute)})
	b := s.Record(Invocation{AgentName: "agent-b", TriggerName: "t1", Status: StatusFailure, StartTime: now.Add(-30 * time.Minute)})
	s.Record(Invocation{AgentName: "agent-a", TriggerName: "t2", Status: StatusSuccess, StartTime: now.Add(-15 * time.Minute)})
	s.Record(Invocation{AgentName: "agent-b", TriggerName: "t2", Status: StatusRunning, StartTime: now.Add(-5 * time.Minute)})

	// Filter: trigger=t1 + status=failure → only b
	got := s.List(ListOptions{TriggerName: "t1", Status: StatusFailure})
	if len(got) != 1 || got[0].ID != b.ID {
		t.Errorf("expected only b for trigger=t1+status=failure, got %+v", got)
	}

	// Filter: agent=agent-a + since 20m ago → only one a with t2
	got2 := s.List(ListOptions{AgentName: "agent-a", Since: now.Add(-20 * time.Minute)})
	if len(got2) != 1 {
		t.Errorf("expected 1 for agent-a+since=-20m, got %d", len(got2))
	}
}

func TestStore_ConcurrentRecordAndComplete(t *testing.T) {
	s := NewStore(1000)
	var wg sync.WaitGroup
	const n = 200
	ids := make(chan string, n)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			rec := s.Record(Invocation{AgentName: "x"})
			ids <- rec.ID
		}
		close(ids)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for id := range ids {
			s.Complete(id, StatusSuccess, "", time.Time{})
		}
	}()

	wg.Wait()

	if s.Len() != n {
		t.Errorf("expected %d records, got %d", n, s.Len())
	}
	for _, inv := range s.List(ListOptions{}) {
		if inv.Status != StatusSuccess {
			t.Errorf("expected all success, found %q", inv.Status)
		}
	}
}

func TestStore_ListFilterByEventID(t *testing.T) {
	s := NewStore(10)
	s.Record(Invocation{AgentName: "a", EventID: "evt-1"})
	s.Record(Invocation{AgentName: "b", EventID: "evt-2"})
	s.Record(Invocation{AgentName: "c", EventID: "evt-1"})

	got := s.List(ListOptions{EventID: "evt-1"})
	if len(got) != 2 {
		t.Fatalf("expected 2 invocations for evt-1, got %d", len(got))
	}
	for _, inv := range got {
		if inv.EventID != "evt-1" {
			t.Errorf("unexpected eventID %q", inv.EventID)
		}
	}

	gotMiss := s.List(ListOptions{EventID: "missing"})
	if len(gotMiss) != 0 {
		t.Errorf("expected 0 for missing event, got %d", len(gotMiss))
	}
}

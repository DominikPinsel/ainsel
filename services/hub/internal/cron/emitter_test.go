package cron

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DominikPinsel/ainsel/services/hub/internal/invocations"
	"github.com/DominikPinsel/ainsel/services/hub/internal/triggers"
	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

type capturedFire struct {
	agentRef     string
	triggerName  string
	invocationID string
	event        ainselapishared.Event
}

func TestUpsertAndDelete(t *testing.T) {
	e := New(nil, invocations.NewStore(10))
	ct := &triggers.CronTrigger{
		ID:        "daily",
		DisplayName: "Daily",
		AgentRef:  "bot",
		Schedule:  "0 9 * * *",
		Prompt:    "hi",
		Enabled:   true,
	}
	e.Upsert(ct)
	if e.EntryCount() != 1 {
		t.Fatalf("EntryCount = %d, want 1", e.EntryCount())
	}
	e.Delete("daily")
	if e.EntryCount() != 0 {
		t.Fatalf("EntryCount after delete = %d, want 0", e.EntryCount())
	}
}

func TestInvalidScheduleSkipped(t *testing.T) {
	e := New(nil, invocations.NewStore(10))
	e.Upsert(&triggers.CronTrigger{
		ID:       "bad",
		Schedule: "not a cron",
	})
	if e.EntryCount() != 0 {
		t.Fatalf("invalid schedule should not be scheduled; EntryCount=%d", e.EntryCount())
	}
}

func TestTickFiresDueEntry(t *testing.T) {
	invStore := invocations.NewStore(10)
	e := New(nil, invStore)

	var captured []capturedFire
	e.SetFire(func(agentRef, triggerName, invocationID string, event ainselapishared.Event) error {
		captured = append(captured, capturedFire{agentRef, triggerName, invocationID, event})
		return nil
	})

	ct := &triggers.CronTrigger{
		ID:       "every-min",
		AgentRef: "bot",
		Schedule: "* * * * *",
		Prompt:   "do the thing",
		Enabled:  true,
	}
	e.Upsert(ct)

	// Force the entry's lastFired into the past so it is "due" now.
	e.mu.Lock()
	if en, ok := e.entries["every-min"]; ok {
		en.lastFired = time.Now().Add(-2 * time.Minute)
	}
	e.mu.Unlock()

	e.tick(time.Now())

	if len(captured) != 1 {
		t.Fatalf("expected 1 fire, got %d", len(captured))
	}
	c := captured[0]
	if c.agentRef != "bot" {
		t.Errorf("agentRef = %q, want bot", c.agentRef)
	}
	if c.triggerName != "every-min" {
		t.Errorf("triggerName = %q, want every-min", c.triggerName)
	}
	if c.invocationID == "" {
		t.Error("invocationID should be set when invStore present")
	}
	if c.event.Connector != ConnectorName {
		t.Errorf("event connector = %q, want %q", c.event.Connector, ConnectorName)
	}
	var data map[string]string
	if err := json.Unmarshal(c.event.Data, &data); err != nil {
		t.Fatalf("unmarshal event data: %v", err)
	}
	if data["prompt"] != "do the thing" {
		t.Errorf("prompt = %q, want %q", data["prompt"], "do the thing")
	}
	if data["cronTrigger"] != "every-min" {
		t.Errorf("cronTrigger = %q, want every-min", data["cronTrigger"])
	}
	if c.event.Headers["type"] != ConnectorName {
		t.Errorf("header type = %q, want %q", c.event.Headers["type"], ConnectorName)
	}

	// An invocation should have been recorded in running state.
	rec, ok := invStore.Get(c.invocationID)
	if !ok {
		t.Fatalf("invocation %s not recorded", c.invocationID)
	}
	if rec.Status != invocations.StatusRunning {
		t.Errorf("invocation status = %q, want running", rec.Status)
	}
	if rec.AgentName != "bot" || rec.TriggerName != "every-min" {
		t.Errorf("invocation agent/trigger = %q/%q, want bot/every-min", rec.AgentName, rec.TriggerName)
	}
}

func TestDisabledEntryDoesNotFire(t *testing.T) {
	e := New(nil, invocations.NewStore(10))
	var captured []capturedFire
	e.SetFire(func(agentRef, triggerName, invocationID string, event ainselapishared.Event) error {
		captured = append(captured, capturedFire{})
		return nil
	})

	e.Upsert(&triggers.CronTrigger{
		ID:       "off",
		AgentRef: "bot",
		Schedule: "* * * * *",
		Prompt:   "x",
		Enabled:  false,
	})
	e.mu.Lock()
	if en, ok := e.entries["off"]; ok {
		en.lastFired = time.Now().Add(-2 * time.Minute)
	}
	e.mu.Unlock()
	e.tick(time.Now())
	if len(captured) != 0 {
		t.Fatalf("disabled entry should not fire; got %d", len(captured))
	}
}

func TestFireFailureMarksInvocationFailed(t *testing.T) {
	invStore := invocations.NewStore(10)
	e := New(nil, invStore)
	e.SetFire(func(agentRef, triggerName, invocationID string, event ainselapishared.Event) error {
		return errPublish
	})

	e.Upsert(&triggers.CronTrigger{
		ID:       "f",
		AgentRef: "bot",
		Schedule: "* * * * *",
		Prompt:   "x",
		Enabled:  true,
	})
	e.mu.Lock()
	if en, ok := e.entries["f"]; ok {
		en.lastFired = time.Now().Add(-2 * time.Minute)
	}
	e.mu.Unlock()
	e.tick(time.Now())

	// The single in-flight invocation should now be failure.
	running := 0
	failed := 0
	for _, inv := range invStore.List(invocations.ListOptions{}) {
		switch inv.Status {
		case invocations.StatusRunning:
			running++
		case invocations.StatusFailure:
			failed++
		}
	}
	if running != 0 || failed != 1 {
		t.Errorf("after failed publish: running=%d failed=%d, want running=0 failed=1", running, failed)
	}
}

// errPublish is a sentinel error for the failing-publish test.
var errPublish = errPublishErr{}

type errPublishErr struct{}

func (errPublishErr) Error() string { return "publish failed" }

func TestNewEntryDoesNotFireImmediately(t *testing.T) {
	invStore := invocations.NewStore(10)
	e := New(nil, invStore)

	var captured []capturedFire
	e.SetFire(func(agentRef, triggerName, invocationID string, event ainselapishared.Event) error {
		captured = append(captured, capturedFire{agentRef, triggerName, invocationID, event})
		return nil
	})

	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	e.SetNow(func() time.Time { return now })

	e.Upsert(&triggers.CronTrigger{
		ID:       "hourly",
		AgentRef: "bot",
		Schedule: "0 * * * *",
		Prompt:   "check PRs",
		Enabled:  true,
	})

	// A brand-new entry should not fire on the first tick — Next(now) is in the future.
	e.tick(now)
	if len(captured) != 0 {
		t.Fatalf("new entry should not fire immediately; got %d fires", len(captured))
	}

	// After advancing to the next hour the trigger should fire exactly once.
	e.tick(now.Add(1*time.Hour + 1*time.Second))
	if len(captured) != 1 {
		t.Fatalf("expected 1 fire after 1 hour, got %d", len(captured))
	}
	if captured[0].event.Timestamp != now.Add(1*time.Hour) {
		t.Errorf("fire_time = %v, want %v", captured[0].event.Timestamp, now.Add(1*time.Hour))
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	e := New(nil, invocations.NewStore(10))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = e.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit after context cancel")
	}
}

package ainselapishared

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	evt := Event{
		ID:        "evt_test123",
		Version:   "1",
		Connector: "my-forgejo",
		Timestamp: now,
		Headers: map[string]string{
			"X-GitHub-Event": "issues",
		},
		Data: json.RawMessage(`{"comment":{"id":1,"body":"hello"}}`),
	}

	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != evt.ID {
		t.Errorf("ID: got %q, want %q", got.ID, evt.ID)
	}
	if got.Connector != evt.Connector {
		t.Errorf("Connector: got %q, want %q", got.Connector, evt.Connector)
	}
	if got.Headers["X-GitHub-Event"] != "issues" {
		t.Errorf("Headers[X-GitHub-Event]: got %q, want %q", got.Headers["X-GitHub-Event"], "issues")
	}
	if !got.Timestamp.Equal(now) {
		t.Errorf("Timestamp: got %v, want %v", got.Timestamp, now)
	}
}

func TestEventWithNilData(t *testing.T) {
	evt := Event{
		ID:      "evt_nil",
		Version: "1",
	}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal with nil data: %v", err)
	}
	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal with nil data: %v", err)
	}
	if got.Data != nil {
		t.Errorf("expected nil Data, got %s", string(got.Data))
	}
}

func TestIssueCommentDataRoundTrip(t *testing.T) {
	data := IssueCommentData{
		Comment: CommentPayload{
			ID:        1234,
			Body:      "@dev-agent please implement this",
			CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
		},
		Issue: IssuePayload{
			Number:    42,
			Title:     "Add feature X",
			Body:      "We need feature X",
			State:     "open",
			Labels:    []string{"enhancement"},
			Assignees: []string{"dev-agent"},
		},
	}

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got IssueCommentData
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Comment.ID != 1234 {
		t.Errorf("Comment.ID: got %d, want 1234", got.Comment.ID)
	}
	if got.Issue.Number != 42 {
		t.Errorf("Issue.Number: got %d, want 42", got.Issue.Number)
	}
	if len(got.Issue.Assignees) != 1 || got.Issue.Assignees[0] != "dev-agent" {
		t.Errorf("Issue.Assignees: got %v, want [dev-agent]", got.Issue.Assignees)
	}
}

func TestPullRequestDataRoundTrip(t *testing.T) {
	data := PullRequestData{
		PullRequest: PullRequestPayload{
			Number: 10,
			Title:  "feat: add login",
			Body:   "Implements login flow",
			State:  "open",
			Merged: false,
			Head:   "feature/login",
			Base:   "main",
		},
	}

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got PullRequestData
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.PullRequest.Number != 10 {
		t.Errorf("PR.Number: got %d, want 10", got.PullRequest.Number)
	}
	if got.PullRequest.Head != "feature/login" {
		t.Errorf("PR.Head: got %q, want %q", got.PullRequest.Head, "feature/login")
	}
}

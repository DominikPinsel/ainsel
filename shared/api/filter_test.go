package ainselapishared

import (
	"encoding/json"
	"testing"
)

func TestFilterEq(t *testing.T) {
	payload := jsonPayload(`{"action": "opened"}`)
	f := Filter{Field: "action", Op: "eq", Value: "opened"}
	if !f.Match(payload) {
		t.Fatal("expected match")
	}
	f.Value = "closed"
	if f.Match(payload) {
		t.Fatal("expected no match")
	}
}

func TestFilterNeq(t *testing.T) {
	payload := jsonPayload(`{"action": "closed"}`)
	f := Filter{Field: "action", Op: "neq", Value: "opened"}
	if !f.Match(payload) {
		t.Fatal("expected match")
	}
}

func TestFilterPrefix(t *testing.T) {
	payload := jsonPayload(`{"ref": "refs/heads/feature/login"}`)
	f := Filter{Field: "ref", Op: "prefix", Value: "refs/heads/feature/"}
	if !f.Match(payload) {
		t.Fatal("expected match")
	}
}

func TestFilterSuffix(t *testing.T) {
	payload := jsonPayload(`{"filename": "README.md"}`)
	f := Filter{Field: "filename", Op: "suffix", Value: ".md"}
	if !f.Match(payload) {
		t.Fatal("expected match")
	}
}

func TestFilterContains(t *testing.T) {
	payload := jsonPayload(`{"comment": {"body": "Hey @dev-agent please look at this"}}`)
	f := Filter{Field: "comment.body", Op: "contains", Value: "@dev-agent"}
	if !f.Match(payload) {
		t.Fatal("expected match")
	}
}

func TestFilterIn(t *testing.T) {
	payload := jsonPayload(`{"action": "opened"}`)
	f := Filter{Field: "action", Op: "in", Values: []string{"opened", "synchronize"}}
	if !f.Match(payload) {
		t.Fatal("expected match")
	}
	payload = jsonPayload(`{"action": "closed"}`)
	if f.Match(payload) {
		t.Fatal("expected no match")
	}
}

func TestFilterNotIn(t *testing.T) {
	payload := jsonPayload(`{"action": "opened"}`)
	f := Filter{Field: "action", Op: "not-in", Values: []string{"closed", "deleted"}}
	if !f.Match(payload) {
		t.Fatal("expected match for value not in list")
	}
	f.Values = []string{"opened", "closed"}
	if f.Match(payload) {
		t.Fatal("expected no match for value in list")
	}
}

func TestFilterContainsArray(t *testing.T) {
	payload := jsonPayload(`{"issue": {"labels": ["bug", "enhancement", "refined"]}}`)
	f := Filter{Field: "issue.labels", Op: "contains", Value: "refined"}
	if !f.Match(payload) {
		t.Fatal("expected match for label in array")
	}
	f.Value = "security"
	if f.Match(payload) {
		t.Fatal("expected no match for label not in array")
	}
}

func TestFilterNotContainsArray(t *testing.T) {
	payload := jsonPayload(`{"issue": {"labels": ["bug", "enhancement"]}}`)
	f := Filter{Field: "issue.labels", Op: "not-contains", Value: "refined"}
	if !f.Match(payload) {
		t.Fatal("expected match for label not in array")
	}
	f.Value = "bug"
	if f.Match(payload) {
		t.Fatal("expected no match for label in array")
	}
}

func TestFilterContainsArrayAlternateOp(t *testing.T) {
	// Test "contains not" as alternative syntax for not-contains
	payload := jsonPayload(`{"issue": {"labels": ["bug", "enhancement"]}}`)
	f := Filter{Field: "issue.labels", Op: "contains not", Value: "refined"}
	if !f.Match(payload) {
		t.Fatal("expected match for 'contains not' operator")
	}
}

func TestFilterInArray(t *testing.T) {
	payload := jsonPayload(`{"issue": {"labels": ["bug", "enhancement", "refined"]}}`)
	f := Filter{Field: "issue.labels", Op: "in", Values: []string{"refined", "reviewed"}}
	if !f.Match(payload) {
		t.Fatal("expected match for label in values list")
	}
	f.Values = []string{"security", "critical"}
	if f.Match(payload) {
		t.Fatal("expected no match for label not in values list")
	}
}

func TestFilterNotInArray(t *testing.T) {
	payload := jsonPayload(`{"issue": {"labels": ["bug", "enhancement"]}}`)
	f := Filter{Field: "issue.labels", Op: "not-in", Values: []string{"refined", "reviewed"}}
	if !f.Match(payload) {
		t.Fatal("expected match when no labels match values list")
	}
	f.Values = []string{"bug", "security"}
	if f.Match(payload) {
		t.Fatal("expected no match when a label matches values list")
	}
}

func TestFilterEmptyArray(t *testing.T) {
	payload := jsonPayload(`{"issue": {"labels": []}}`)
	f := Filter{Field: "issue.labels", Op: "contains", Value: "refined"}
	if f.Match(payload) {
		t.Fatal("expected no match on empty array")
	}
	f.Op = "not-contains"
	if !f.Match(payload) {
		t.Fatal("expected match for not-contains on empty array")
	}
}

func TestFilterRegex(t *testing.T) {
	payload := jsonPayload(`{"ref": "refs/heads/fix/issue-42"}`)
	f := Filter{Field: "ref", Op: "regex", Value: `refs/heads/(fix|feature)/.*`}
	if !f.Match(payload) {
		t.Fatal("expected match")
	}
}

func TestFilterNestedField(t *testing.T) {
	payload := jsonPayload(`{"repository": {"full_name": "dpinsel/my-app"}}`)
	f := Filter{Field: "repository.full_name", Op: "eq", Value: "dpinsel/my-app"}
	if !f.Match(payload) {
		t.Fatal("expected match")
	}
}

func TestFilterMissingField(t *testing.T) {
	payload := jsonPayload(`{"action": "opened"}`)
	f := Filter{Field: "nonexistent.field", Op: "eq", Value: "foo"}
	if f.Match(payload) {
		t.Fatal("expected no match on missing field")
	}
}

func TestFilterBoolField(t *testing.T) {
	payload := jsonPayload(`{"pull_request": {"merged": true}}`)
	f := Filter{Field: "pull_request.merged", Op: "eq", Value: "true"}
	if !f.Match(payload) {
		t.Fatal("expected match on bool field")
	}
}

func TestFilterUnknownOp(t *testing.T) {
	payload := jsonPayload(`{"action": "opened"}`)
	f := Filter{Field: "action", Op: "unknown_op", Value: "opened"}
	if f.Match(payload) {
		t.Fatal("expected no match for unknown op")
	}
}

func TestMatchFiltersAND(t *testing.T) {
	payload := jsonPayload(`{"action": "assigned", "issue": {"assignee": {"login": "dev-agent"}}}`)
	filters := []Filter{
		{Field: "action", Op: "eq", Value: "assigned"},
		{Field: "issue.assignee.login", Op: "eq", Value: "dev-agent"},
	}
	if !MatchFilters(filters, payload) {
		t.Fatal("expected all filters to match")
	}
}

func TestMatchFiltersANDFailsOnOne(t *testing.T) {
	payload := jsonPayload(`{"action": "assigned", "issue": {"assignee": {"login": "other-user"}}}`)
	filters := []Filter{
		{Field: "action", Op: "eq", Value: "assigned"},
		{Field: "issue.assignee.login", Op: "eq", Value: "dev-agent"},
	}
	if MatchFilters(filters, payload) {
		t.Fatal("expected no match when one filter fails")
	}
}

func TestMatchFiltersEmpty(t *testing.T) {
	payload := jsonPayload(`{"action": "opened"}`)
	if !MatchFilters(nil, payload) {
		t.Fatal("expected match with no filters")
	}
}

func jsonPayload(s string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		panic(err)
	}
	return m
}

// TestTriggerDirectorAutomatedLabel documents the filter config for the
// t-director-automated-label trigger. The payload shape matches what the
// normalizer emits for issue.label.added events after the label field was
// added to IssueData.
func TestTriggerDirectorAutomatedLabel(t *testing.T) {
	// Payload: issue.label.added — "automated" was just applied to issue #181.
	payload := jsonPayload(`{
		"label": "automated",
		"issue": {
			"number": 181,
			"title": "Improve agent onboarding docs",
			"body": "",
			"state": "open",
			"labels": ["automated"],
			"assignees": []
		}
	}`)

	f := Filter{Field: "label", Op: "eq", Value: "automated"}
	if !f.Match(payload) {
		t.Fatal("expected trigger to fire when automated label is added")
	}

	// A different label was added to the same issue — trigger must NOT fire.
	otherPayload := jsonPayload(`{
		"label": "bug",
		"issue": {
			"number": 181,
			"labels": ["automated", "bug"],
			"assignees": []
		}
	}`)
	if f.Match(otherPayload) {
		t.Fatal("expected no match when a different label is added to issue that already has automated")
	}
}

// TestTriggerReviewerReviewLabel documents the filter config for the
// t-reviewer-review-label trigger. The payload shape matches what the
// normalizer emits for pull_request.label.added events.
func TestTriggerReviewerReviewLabel(t *testing.T) {
	// Payload: pull_request.label.added — "ainsel/review" was just applied to PR #182.
	payload := jsonPayload(`{
		"label": "ainsel/review",
		"pull_request": {
			"number": 182,
			"title": "fix: prefer repo-level labels",
			"body": "",
			"state": "open",
			"merged": false,
			"head": "fix/labels",
			"base": "main",
			"labels": ["ainsel/review"]
		}
	}`)

	f := Filter{Field: "label", Op: "eq", Value: "ainsel/review"}
	if !f.Match(payload) {
		t.Fatal("expected trigger to fire when ainsel/review label is added to PR")
	}

	// A different label was added — trigger must NOT fire.
	otherPayload := jsonPayload(`{
		"label": "bug",
		"pull_request": {
			"number": 182,
			"labels": ["ainsel/review", "bug"]
		}
	}`)
	if f.Match(otherPayload) {
		t.Fatal("expected no match when a different label is added to PR that already has ainsel/review")
	}
}

func TestFilterNullValues(t *testing.T) {
	// Test not-contains on null (should return true - null doesn't contain anything)
	payload := jsonPayload(`{"issue": {"labels": null}}`)
	f := Filter{Field: "issue.labels", Op: "not-contains", Value: "refined"}
	if !f.Match(payload) {
		t.Fatal("expected not-contains on null to return true")
	}

	// Test contains on null (should return false - null doesn't contain anything)
	f.Op = "contains"
	if f.Match(payload) {
		t.Fatal("expected contains on null to return false")
	}

	// Test not-contains on missing field (should return false)
	f.Op = "not-contains"
	f.Field = "issue.nonexistent"
	if f.Match(payload) {
		t.Fatal("expected not-contains on missing field to return false")
	}
}

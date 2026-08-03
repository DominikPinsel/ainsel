# Event Schema Reference

## Canonical Event Format

Every event in the Ainsel platform uses this canonical format, regardless of the source connector.

### Event Structure

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique event identifier |
| `version` | string | Schema version |
| `source` | string | Source system (e.g., `"forgejo"`) |
| `connector` | string | Connector instance name |
| `type` | string | Event type (e.g., `"issue.opened"`) |
| `timestamp` | time.Time | When the event occurred |
| `subject` | EventSubject | Target entity of the event |
| `actor` | EventActor | Who triggered the event |
| `action` | string | Action performed |
| `data` | RawJSON | Type-specific payload |
| `raw` | string | Original raw webhook body (optional) |

### EventSubject

| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | Entity kind: `"issue"`, `"pull_request"`, `"push"`, `"repository"` |
| `owner` | string | Repository owner |
| `repo` | string | Repository name |
| `number` | int | Issue/PR number (0 for push/repo events) |

### EventActor

| Field | Type | Description |
|-------|------|-------------|
| `login` | string | Username |
| `display_name` | string | Display name |
| `is_bot` | bool | Whether the actor is a bot account |

## Event Types

### Issue Events

| Type | Data Payload | Description |
|------|-------------|-------------|
| `issue.opened` | `IssueData` | New issue created |
| `issue.assigned` | `IssueData` | Issue assigned to someone |
| `issue.closed` | `IssueData` | Issue closed |
| `issue.reopened` | `IssueData` | Issue reopened |
| `issue.label.added` | `IssueData` | Label added to issue |
| `issue.comment.created` | `IssueCommentData` | Comment added to issue |

### Pull Request Events

| Type | Data Payload | Description |
|------|-------------|-------------|
| `pull_request.opened` | `PullRequestData` | New PR created |
| `pull_request.closed` | `PullRequestData` | PR closed or merged |
| `pull_request.comment.created` | `PullRequestCommentData` | Comment on PR |
| `pull_request.review.submitted` | `PullRequestReviewData` | Review submitted |

### Other Events

| Type | Data Payload | Description |
|------|-------------|-------------|
| `push` | `PushData` | Push to repository |
| `repository.created` | - | New repository created |

## Data Payloads

### IssuePayload

```json
{
  "issue": {
    "number": 42,
    "title": "Bug in login page",
    "body": "Steps to reproduce...",
    "state": "open",
    "labels": ["bug", "priority:high"],
    "assignees": ["alice"]
  }
}
```

### PullRequestPayload

```json
{
  "pull_request": {
    "number": 15,
    "title": "Fix login bug",
    "body": "Fixes #42",
    "state": "open",
    "merged": false,
    "head": "fix/login-bug",
    "base": "main"
  }
}
```

### CommentPayload

```json
{
  "comment": {
    "id": 101,
    "body": "Looks good to me!",
    "created_at": "2026-04-24T10:00:00Z"
  }
}
```

### PushData

```json
{
  "ref": "refs/heads/main",
  "before": "abc123",
  "after": "def456",
  "commits": [
    {
      "id": "def456",
      "message": "fix: resolve login issue",
      "author": "alice"
    }
  ]
}
```

## Filter Examples

Filters operate on the `data` field using dotted paths:

```yaml
# Match issues with "bug" label
filters:
  - field: "issue.labels"
    op: contains
    value: "bug"

# Match PRs targeting main branch
filters:
  - field: "pull_request.base"
    op: eq
    value: "main"

# Match comments containing "/review"
filters:
  - field: "comment.body"
    op: contains
    value: "/review"
```

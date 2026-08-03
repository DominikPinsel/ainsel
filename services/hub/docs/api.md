# Ainsel Hub Backend REST API Reference

Base URL: `/api/v1`

## Pagination

Collection endpoints (`/agents`, `/connectors`, `/triggers`, `/github-apps`,
`/invocations`) return paginated responses so the API never streams every
record back in a single payload.

**Query parameters:**

| Param | Description |
|-------|-------------|
| `page` | 1-based page number. Defaults to `1`. Values `< 1` are coerced to `1`. |
| `pageSize` | Items per page. Defaults to `50`. Values `< 1` are coerced to `1`, values above `200` are clamped to `200`. |

Non-numeric `page` or `pageSize` values return `400`.

**Response envelope:**

```json
{
  "items": [...],
  "total": 123,
  "page": 1,
  "pageSize": 50,
  "totalPages": 3
}
```

- `total` is the total number of matching records *after* any filters are
  applied, so the frontend can render "Page 1 of N" without a second call.
- `totalPages` is `ceil(total / pageSize)`, or `0` when `total == 0`.
- `items` is always present, even when empty.
- The `/invocations` endpoint preserves its legacy `invocations` key (rather
  than `items`) for backward compatibility but otherwise carries the same
  pagination fields.

Pages are ordered deterministically by resource name (Kubernetes-backed
endpoints) or by start time (invocations) so a given `page` returns the same
slice across calls.

---

## Health Check

### GET /health

Returns the health status of the hub backend.

**Response:** `200 OK`
```json
{"status": "ok"}
```

---

## Agents

### GET /api/v1/agents

List all Agent resources in the configured namespace.

Supports [pagination](#pagination) via `page` / `pageSize`.

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "a-1a2b3c4d",
      "name": "Code Reviewer",
      "forgejo": {"username": "code-reviewer", "email": "..."},
      "runtime": {"image": "registry/agent:v1", "provider": "claude"},
      "llm": {"model": "claude-sonnet-4-20250514"},
      "status": {"ready": true, "replicas": 1}
    }
  ],
  "total": 1,
  "page": 1,
  "pageSize": 50,
  "totalPages": 1
}
```

### POST /api/v1/agents

Create a new Agent resource.

**Request Body:** Agent spec (JSON)
**Response:** `201 Created`

### GET /api/v1/agents/:name

Get a specific Agent by name.

**Response:** `200 OK` or `404 Not Found`

### PUT /api/v1/agents/:name

Update an existing Agent.

**Request Body:** Agent spec (JSON)
**Response:** `200 OK` or `404 Not Found`

### DELETE /api/v1/agents/:name

Delete an Agent.

**Response:** `200 OK` or `404 Not Found`

---

## Agent Images

An AgentImage describes a container image that can back one or more Agents. It
also carries the list of tools the image exposes (populated by the tool-sync
job — see [POST `.../sync`](#post-apiv1agent-imagesnamesync) below). The hub
validates `agents.imageRef.name` and `agents.enabledTools` against this list so
agents cannot reference a missing image or an unknown tool.

### GET /api/v1/agent-images

List all AgentImage resources in the configured namespace. Supports
[pagination](#pagination) via `page` / `pageSize`.

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "img-1a2b3c4d",
      "displayName": "Claude Dev Image",
      "description": "Standard Claude development image",
      "imageURL": "registry.example.com/claude-dev:v1",
      "tools": [
        {
          "name": "git",
          "kind": "shell",
          "description": "git CLI",
          "examples": [
            {"title": "status", "snippet": "git status"}
          ]
        }
      ],
      "status": {
        "phase": "Ready",
        "lastSync": "2026-05-10T15:30:00Z",
        "syncError": "",
        "orphanTools": []
      }
    }
  ],
  "total": 1,
  "page": 1,
  "pageSize": 50,
  "totalPages": 1
}
```

`status.phase` is one of `Pending`, `Syncing`, `Ready`, `SyncFailed`.
`tools[].kind` is one of `container`, `shell`, `claude-cli`.

### POST /api/v1/agent-images

Create a new AgentImage. The image starts in `Pending` and has no tools until
a sync job runs successfully (see [POST `.../sync`](#post-apiv1agent-imagesnamesync)).

**Request Body:**
```json
{
  "displayName": "Claude Dev Image",
  "description": "Standard Claude development image",
  "imageURL": "registry.example.com/claude-dev:v1"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `displayName` | yes | Human-readable name shown in the dashboard. |
| `description` | no | Free-form description. |
| `imageURL` | yes | Fully-qualified container image reference (registry/name:tag). |

**Response:** `201 Created` with the created AgentImage. `400 Bad Request`
when `displayName` or `imageURL` is missing.

### GET /api/v1/agent-images/:name

Get a specific AgentImage by name.

**Response:** `200 OK` or `404 Not Found`.

### PUT /api/v1/agent-images/:name

Update an existing AgentImage. All fields are optional — only fields present
in the request body are mutated. The `tools` field is a full replacement: a
non-nil empty array clears the tool list, while omitting `tools` leaves the
current list untouched.

**Request Body:**
```json
{
  "displayName": "Claude Dev Image (v2)",
  "description": "Updated description",
  "imageURL": "registry.example.com/claude-dev:v2",
  "tools": [
    {
      "name": "git",
      "kind": "shell",
      "description": "git CLI",
      "examples": [{"title": "status", "snippet": "git status"}]
    }
  ]
}
```

**Response:** `200 OK` with the updated AgentImage.

**Tool-removal protection:** If the request would remove a tool that is still
listed in some Agent's `enabledTools` (and that Agent's `imageRef` points at
this image), the request is rejected with `409 Conflict`:

```json
{
  "error": "tool referenced by agent(s)",
  "affectedAgents": ["a-1a2b3c4d"],
  "removedTools": ["git"]
}
```

Resolve by updating the affected agents to remove the tool from their
`enabledTools` first, then retry.

Returns `404 Not Found` when the AgentImage does not exist, `400 Bad Request`
on invalid JSON.

### DELETE /api/v1/agent-images/:name

Delete an AgentImage.

**Response:** `204 No Content` on success, with no response body.

**Reference protection:** If any Agent in the namespace has `imageRef.name`
pointing at this image, the delete is rejected with `409 Conflict`:

```json
{
  "error": "agent image referenced by agent(s)",
  "affectedAgents": ["a-1a2b3c4d", "a-5e6f7g8h"]
}
```

Resolve by deleting or repointing the listed agents first, then retry.

Returns `404 Not Found` when the AgentImage does not exist, and
`500 Internal Server Error` for any other backend failure (network, RBAC,
missing CRD).

### POST /api/v1/agent-images/:name/sync

Trigger an asynchronous tool-sync job for an AgentImage. The hub creates a
short-lived Kubernetes Job that runs `agent --list-tools` against the
image; when the Job completes, the controller writes the discovered tools
back into `.spec.tools` and transitions `.status.phase` from `Syncing` to
`Ready` (or `SyncFailed` if the Job errored or timed out).

**Request Body:** none.

**Response:**
- `202 Accepted` — Job created, phase advanced to `Syncing`. No body.
- `404 Not Found` — AgentImage does not exist.
- `409 Conflict` — a sync Job for this image is already running. Wait for it
  to finish before retrying.
- `500 Internal Server Error` — Job creation or status update failed.

Sync Jobs have an `activeDeadlineSeconds` of 60 and a `ttlSecondsAfterFinished`
of 300, so they self-clean up shortly after completion.

---

## Connectors

### GET /api/v1/connectors

List all connector resources. Supports
[pagination](#pagination) via `page` / `pageSize`.

**Response:** `200 OK`

### POST /api/v1/connectors

Create a new WebhookConnector resource.

**Request Body:** WebhookConnector spec (JSON)
**Response:** `201 Created`

### GET /api/v1/connectors/:name

Get a specific WebhookConnector by name.

### PUT /api/v1/connectors/:name

Update a WebhookConnector.

### DELETE /api/v1/connectors/:name

Delete a WebhookConnector.

---

## Triggers

### GET /api/v1/triggers

List Trigger resources. Supports [pagination](#pagination) via `page` /
`pageSize`. Pagination is applied after filtering, so `total` reflects the
filtered count.

**Query parameters:**

| Param | Description |
|-------|-------------|
| `agent` | Filter by `spec.agentRef` (exact, case-sensitive match) |
| `connector` | Filter by `spec.connectorRef` (exact, case-sensitive match) |
| `eventType` | Filter by `spec.eventType` (exact, case-sensitive match, e.g. `issue.opened`) |
| `page` | See [pagination](#pagination) |
| `pageSize` | See [pagination](#pagination) |

Filters compose with AND semantics; an unset filter matches everything.
An empty query string returns every trigger in the namespace (paginated).

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "t-1a2b3c4d",
      "name": "develop-on-assign",
      "agentRef": "dev-agent",
      "connectorRef": "forgejo-dev",
      "eventType": "issue.assigned",
      "ignoreBotEvents": true,
      "status": {"agentValid": true, "connectorValid": true}
    }
  ],
  "total": 1,
  "page": 1,
  "pageSize": 50,
  "totalPages": 1
}
```

**Examples:**

```
GET /api/v1/triggers?agent=dev-agent
GET /api/v1/triggers?connector=forgejo-dev&eventType=issue.opened&page=2&pageSize=20
GET /api/v1/triggers?agent=reviewer&connector=forgejo-dev&eventType=issue.opened
```

### POST /api/v1/triggers

Create a new Trigger resource.

**Request Body:** Trigger spec (JSON)
**Response:** `201 Created`

### GET /api/v1/triggers/:name

Get a specific Trigger by name.

### PUT /api/v1/triggers/:name

Update a Trigger.

### DELETE /api/v1/triggers/:name

Delete a Trigger.

---

## Invocations

The hub records every event dispatch to an agent in an in-memory ring buffer
(default capacity 1000, configurable via `HUB_INVOCATION_BUFFER_SIZE`). Each
record tracks who was invoked, with which trigger, when it started/completed,
and what the outcome was. Agents report completion by publishing a
`hub.invocation.completed` event on NATS with the `X-Invocation-ID` header
they received in the dispatched message.

### GET /api/v1/invocations

List recent invocations newest-first. Supports [pagination](#pagination) via
`page` / `pageSize`. The `limit` parameter from earlier revisions has been
superseded by `pageSize` — older clients that send `limit` will fall through
to the default pageSize.

**Query parameters:**

| Param | Description |
|-------|-------------|
| `agent` | Filter by agent name |
| `status` | Filter by status: `running`, `success`, `failure`, `timeout` |
| `since` | RFC3339 timestamp; only invocations started at or after this time |
| `page` | See [pagination](#pagination) |
| `pageSize` | See [pagination](#pagination) |

**Response:** `200 OK`
```json
{
  "invocations": [
    {
      "id": "inv-1a2b3c4d",
      "agentName": "dev-agent",
      "triggerName": "issue-assigned",
      "eventId": "evt-abc",
      "eventType": "issue.assigned",
      "eventSource": "forgejo",
      "connector": "my-forgejo",
      "startTime": "2026-05-10T15:30:00Z",
      "endTime": "2026-05-10T15:30:42Z",
      "durationMs": 42000,
      "status": "success"
    }
  ],
  "total": 1,
  "capacity": 1000,
  "page": 1,
  "pageSize": 50,
  "totalPages": 1
}
```

The `invocations` key (instead of `items`) is kept for backward compatibility
with frontend code that pre-dates the unified pagination envelope.

### GET /api/v1/invocations/:id

Get a single invocation by ID.

**Response:** `200 OK` or `404 Not Found`

When the invocation store is not configured, both endpoints return `503`.

---

## Observability Metrics

The hub exposes a small read-only API over Prometheus so the frontend
dashboard can render hub-internal counters, time series, and per-agent token
usage without talking to Prometheus directly. All endpoints return `503 Service
Unavailable` with `{"error": "metrics backend not configured"}` when
`HUB_PROMETHEUS_URL` is unset.

Responses are cached server-side for ~30 seconds (one Prometheus scrape
interval) so a busy dashboard does not generate one query-per-poll.

### Canonical paths

| Path | Returns |
|------|---------|
| `GET /api/v1/observability/metrics/summary` | Current value of each hub counter |
| `GET /api/v1/observability/metrics/timeseries?metric=<name>&range=<1h\|6h\|24h\|7d>` | Per-second rate of one counter over a range |
| `GET /api/v1/observability/metrics/agents` | Per-agent token usage, invocations, and (if available) cost |

### Deprecated aliases

The same handlers also respond at `/api/v1/metrics/{summary,timeseries,agents}`
to preserve compatibility with the deployed `ainsel-hub-frontend` bundle prior
to the dashboard rewrite (frontend PR 50). Alias responses include:

- `Deprecation: true`
- `Link: </api/v1/observability/metrics/...>; rel="successor-version"`

New frontend code should use the canonical `/api/v1/observability/...` paths.
The aliases will be removed once the dashboard rewrite is deployed.

### GET /api/v1/observability/metrics/summary

**Query parameters:**

| Param | Description |
|-------|-------------|
| `range` | Optional. One of `1h`, `6h`, `24h`, `7d`. When supplied, counters are windowed using `increase()` over the given range. When omitted, returns all-time cumulative counter values (legacy behavior). |

**Response:** `200 OK`
```json
{
  "eventsConsumed": 42,
  "triggersMatched": 30,
  "eventsRouted": 29,
  "routingErrors": 1,
  "updatedAt": "2026-05-10T15:30:00Z"
}
```

Counters that have never been observed (e.g. fresh hub) return `0` rather
than an error so the dashboard renders a clean zero state. Returns `400`
when `range` is outside the supported set.

### GET /api/v1/observability/metrics/tokens/summary

**Query parameters:**

| Param | Description |
|-------|-------------|
| `range` | Optional. One of `1h`, `6h`, `24h`, `7d`. Defaults to `24h` when omitted. |

**Response:** `200 OK`
```json
{
  "inputTokens": 800000,
  "outputTokens": 400000,
  "totalTokens": 1200000,
  "previousTotalTokens": 950000,
  "updatedAt": "2026-05-10T15:30:00Z"
}
```

`previousTotalTokens` contains the token total for the equivalent prior
window (used by the frontend for trend indicators). Returns `400` when
`range` is outside the supported set.

### GET /api/v1/observability/metrics/timeseries

**Query parameters:**

| Param | Description |
|-------|-------------|
| `metric` | One of `events_consumed`, `triggers_matched`, `events_routed`, `routing_errors`. Defaults to `events_consumed` so the dashboard's "no metric picked yet" view renders. |
| `range` | One of `1h`, `6h`, `24h`, `7d`. Defaults to `1h`. |

**Response:** `200 OK`
```json
{
  "metric": "events_consumed",
  "range": "1h",
  "step": "30s",
  "points": [
    {"timestamp": "2026-05-10T14:30:00Z", "value": 0.5},
    {"timestamp": "2026-05-10T14:30:30Z", "value": 0.7}
  ]
}
```

Points are spaced so each chart has roughly 60-180 samples regardless of
range. For counter metrics the `points[].value` is the per-second rate over
the range's natural rate window (`1m` for `1h`, `5m` for `6h`, etc.), not the
raw counter. Returns `400` when `metric` is unknown, or when `range` is
outside the supported set.

### GET /api/v1/observability/metrics/agents

**Response:** `200 OK`
```json
{
  "agents": [
    {
      "agent": "dev",
      "inputTokens": 1500,
      "outputTokens": 300,
      "totalTokens": 1800,
      "invocations": 5
    }
  ],
  "updatedAt": "2026-05-10T15:30:00Z"
}
```

`invocations` falls back across `claude_invocations_total` and
`agent_invocations_total` so agents on either runtime surface the right
number. Agents are sorted alphabetically for stable diffing across polls.

---

## Observability Logs

The hub exposes a thin proxy over Loki so the frontend can render agent and
hub logs without talking to Loki directly.

### GET /api/observability/logs

Also available at `GET /api/v1/observability/logs`.

**Query parameters:**

| Param | Description |
|-------|-------------|
| `app` | Application label (e.g. `hub-backend`, `dev-agent`). Builds the selector `{namespace="<HUB_LOKI_NAMESPACE>", app="<app>"}`. Mutually exclusive with `query`. |
| `query` | Free-form LogQL passed through to Loki untouched. Mutually exclusive with `app`. |
| `range` | Lookback window: `1h`, `6h`, or `24h`. Defaults to `1h`. |
| `limit` | Max log lines to return. Defaults to 500, capped at 1000. |

When neither `app` nor `query` is supplied, the selector defaults to
agent logs only: `{namespace="<HUB_LOKI_NAMESPACE>",app=~".+",app!="<hub-app>"}`.
Without this scoping every stream in Loki matches (argocd, kube-system,
etc.) and the dashboard becomes useless. Power users who want the
truly-everything view can pass `query={app=~".+"}` explicitly.

**Response:** `200 OK`
```json
{
  "logs": [
    {
      "timestamp": "2026-05-10T15:30:00Z",
      "message": "starting hub",
      "labels": {"app": "hub-backend", "namespace": "ainsel"}
    }
  ],
  "total": 1,
  "query": "{namespace=\"ainsel\",app=\"hub-backend\"}"
}
```

Lines are returned newest-first. Returns `503` when `HUB_LOKI_URL` is not
configured and `502` when Loki itself errors.

---

## Error Responses

All error responses follow the format:

```json
{"error": "error message"}
```

| Status | Meaning |
|--------|---------|
| `400` | Bad request (invalid JSON, missing fields) |
| `404` | Resource not found |
| `409` | Conflict (resource already exists, or referenced by another resource — e.g. deleting an AgentImage that an Agent still uses) |
| `500` | Internal server error |

Successful mutations return `200 OK` (update) or `201 Created` (create) with a
JSON body. Successful `DELETE` returns `204 No Content` with an empty body.
Asynchronous trigger endpoints (e.g. `POST /agent-images/:name/sync`) return
`202 Accepted` with no body.

## Ingress

When deployed via the ainsel-chart, the API is exposed at:
```
https://ainsel.example.com/ainsel/api/v1/...
```

The nginx ingress rewrites `/ainsel/api` to `/api`.

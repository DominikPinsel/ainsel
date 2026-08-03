# Hub REST API Reference

The hub service (see [`services/hub/`](../services/hub/)) exposes a REST API for managing Ainsel resources. Most endpoints operate on CRDs in the configured Kubernetes namespace; others surface observability data, invocation history, and operator state.

**Base URL:** `/api/v1`
**Ingress:** `https://ainsel.example.com/ainsel/api/v1`

All list endpoints accept the standard pagination query parameters:

| Query | Default | Max | Notes |
|-------|---------|-----|-------|
| `page` | `1` | — | 1-indexed page number; non-numeric values return `400`. |
| `pageSize` | `50` | `200` | Values above the max are clamped down. |

List responses include `total`, `page`, `pageSize`, and `totalPages` alongside the per-collection `items`.

## Health Check

### GET /health

```bash
curl https://ainsel.example.com/ainsel/api/health
```

**Response:** `200 OK`
```json
{"status": "ok"}
```

---

## Agents

The Agents endpoints expose a simplified projection of the `Agent` CRD. The full CRD shape is described in [`crd-reference.md`](crd-reference.md).

### GET /api/v1/agents

List all Agents in the configured namespace, sorted by resource name.

**Query parameters:** `page`, `pageSize` (see top of file).

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "a-3f9a2b",
      "name": "Code Reviewer",
      "description": "...",
      "imageRef": {"name": "img-claude-coder"},
      "runtime": {"provider": "ollama-cloud"},
      "llm": {"model": "glm-5.1:cloud", "maxTurns": 25},
      "persona": {"inline": "..."},
      "enabledTools": ["read", "edit"],
      "scaling": {"minReplicas": 0, "maxReplicas": 3},
      "memory": {"enabled": true, "provider": "example"},
      "status": {"ready": true, "replicas": 1}
    }
  ],
  "total": 1, "page": 1, "pageSize": 50, "totalPages": 1
}
```

### POST /api/v1/agents

Create a new Agent. The hub generates the resource name (`a-<short id>`); the request supplies the human display name.

**Request body:**
```json
{
  "name": "Code Reviewer",
  "description": "...",
  "imageRef": {"name": "img-claude-coder"},
  "runtime": {"provider": "ollama-cloud"},
  "llm": {"model": "glm-5.1:cloud", "maxTurns": 25, "temperature": 0.2},
  "persona": {"inline": "..."},
  "enabledTools": ["read", "edit"],
  "scaling": {"minReplicas": 0, "maxReplicas": 3, "cooldownPeriod": 300, "lagThreshold": 5},
  "memory": {"enabled": true, "provider": "example"},
  "ollamaCloud": {"apiKey": "<consumed-once>"}
}
```

`imageRef.name` is required and must reference an existing `AgentImage`. Any names in `enabledTools` not declared by that image are rejected. The `ollamaCloud.apiKey`, when present, is stored in a Secret named `<agent>-ollama-key` and never echoed back.

**Response:** `201 Created` with the same shape as `GET /api/v1/agents/{name}`. `400` on validation errors, `500` on Kubernetes failures.

### GET /api/v1/agents/{name}

Fetch one Agent by resource name.

**Response:** `200 OK` (same shape as the list item) or `404 Not Found`.

### PUT /api/v1/agents/{name}

Update an Agent. Body fields are all optional; only fields that are present are applied. When `imageRef` or `enabledTools` changes, the new combination is re-validated against the referenced `AgentImage`.

**Response:** `200 OK` with the updated agent, `400` on invalid body, `404` if missing, `500` on K8s failures.

### DELETE /api/v1/agents/{name}

Delete an Agent.

**Response:** `204 No Content` or `404 Not Found`.

---

## Agent Images

`AgentImage` resources catalog container images and the tools they advertise. The hub also runs a sync Job that boots the image with `agent --list-tools` to discover its current tool set.

Each `AgentImage` may define environment variables (`env`) to inject into every agent pod that uses the image. Env vars have an optional `secret` flag:

- When `secret` is `true`, the API **never** returns the value — it is always returned as `""` in GET/list responses to prevent leaking sensitive data.
- On update, sending `value: ""` for a secret env var means "keep the existing value" (the value is not overwritten with an empty string). To replace a secret, send a non-empty value.

### GET /api/v1/agent-images

List all `AgentImage` resources, sorted by name. Supports `page`/`pageSize`.

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "img-7a2c1f",
      "displayName": "Claude Coder",
      "description": "...",
      "imageURL": "registry.example.com/ainsel/claude-coder:1.2.3",
      "tools": [
        {"name": "read", "kind": "file", "description": "...", "examples": [{"title": "...", "snippet": "..."}]}
      ],
      "status": {"phase": "Ready", "lastSync": "2026-05-20T10:00:00Z", "syncError": "", "orphanTools": []}
    }
  ],
  "total": 1, "page": 1, "pageSize": 50, "totalPages": 1
}
```

### POST /api/v1/agent-images

Create an `AgentImage`. `displayName` and `imageURL` are required. The new image starts in phase `Pending`; tools are populated after the first successful sync.

**Request body:**
```json
{"displayName": "Claude Coder", "description": "...", "imageURL": "registry.example.com/ainsel/claude-coder:1.2.3"}
```

**Response:** `201 Created` with the created image, `400` if required fields are missing, `500` on K8s failures.

### GET /api/v1/agent-images/{name}

Fetch one `AgentImage` by name.

**Response:** `200 OK` or `404 Not Found`.

### PUT /api/v1/agent-images/{name}

Update an `AgentImage`. All body fields are optional. Setting `tools` to a non-nil array replaces the tool list; an empty array clears it.

For secret env vars, sending `value: ""` preserves the existing stored value (no overwrite). Send a non-empty `value` to replace the secret.

If any tool is removed and an existing Agent has it in `enabledTools`, the request fails with `409 Conflict` and the response body lists the affected agents and the removed tools.

**Response:** `200 OK`, `400`, `404`, `409`, or `500`.

### DELETE /api/v1/agent-images/{name}

Delete an `AgentImage`. Returns `409 Conflict` (with `affectedAgents`) if any Agent still references it via `imageRef.name`.

**Response:** `204 No Content`, `404 Not Found`, `409 Conflict`, or `500`.

### POST /api/v1/agent-images/{name}/sync

Trigger a tool sync for an `AgentImage`. The hub schedules a one-off Job that runs `agent --list-tools` against the image; the controller updates the image's `tools` and `status.phase` when the Job completes. Returns `409 Conflict` if a sync Job for this image is still active.

**Response:** `202 Accepted` (no body), `404 Not Found`, `409 Conflict`, or `500`.

---

## Connectors

Connectors are the resources that bridge Ainsel to upstream code-hosting platforms. The list/get endpoints return `WebhookConnector` resources in a simplified response.

### GET /api/v1/connectors

List all connectors across both kinds, sorted by ID. Supports `page`/`pageSize`.

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "c-1a2b3c",
      "name": "Forgejo Self-Hosted",
      "type": "forgejo",
      "url": "https://forgejo.example.com",
      "organization": "AInsel",
      "botIdentity": {"username": "ainsel-bot"},
      "webhookEndpoint": "https://.../webhook/c-1a2b3c",
      "status": {"ready": true, "installed": false, "webhookRegistered": true}
    }
  ],
  "total": 1, "page": 1, "pageSize": 50, "totalPages": 1
}
```

### POST /api/v1/connectors

Create a connector. `name` and `type` are always required; the remaining required fields depend on `type`.

**Request body (forgejo):**
```json
{
  "name": "Forgejo Self-Hosted",
  "type": "forgejo",
  "url": "https://forgejo.example.com",
  "token": "<webhook bootstrap token>",
  "organization": "AInsel",
  "botIdentity": {"username": "ainsel-bot"},
  "botPassword": "<consumed-once>"
}
```

`botToken` (pre-minted) and `botPassword` (basic-auth mint) are mutually exclusive. The webhook HMAC is generated by the hub and returned exactly once on this response as `webhookSecretValue`.

**Response:** `201 Created` with the connector (forgejo create additionally includes `webhookSecretValue`), `400` on validation errors, `502` if minting the bot token via Forgejo basic auth fails, `500` on K8s failures.

### GET /api/v1/connectors/{name}

Fetch one connector by name.

**Response:** `200 OK` or `404 Not Found`.

### PUT /api/v1/connectors/{name}

Update a connector. Body fields are all optional; only present fields are applied. `botToken`/`botPassword` may be used to rotate the bot access token.

**Response:** `200 OK`, `400`, `404`, `502` (Forgejo mint failure), or `500`.

### DELETE /api/v1/connectors/{name}

Delete a connector. Associated credentials and webhook secrets are deleted as well.

**Response:** `204 No Content` or `404 Not Found`.

---

## Triggers

### GET /api/v1/triggers

List all `Trigger` resources, sorted by name. Supports `page`/`pageSize` plus optional filters.

**Query parameters:** `agent`, `connector`, `eventType` (each is an exact match against the trigger spec; unset filters match everything).

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "t-9c1d2e",
      "name": "Review issues",
      "agentRef": "a-3f9a2b",
      "connectorRef": "c-1a2b3c",
      "eventType": "issue.opened",
      "ignoreBotEvents": true,
      "filters": [{"field": "repo", "op": "eq", "value": "AInsel/ainsel"}],
      "status": {"agentValid": true, "connectorValid": true}
    }
  ],
  "total": 1, "page": 1, "pageSize": 50, "totalPages": 1
}
```

### POST /api/v1/triggers

Create a Trigger.

**Request body:**
```json
{
  "name": "Review issues",
  "agentRef": "a-3f9a2b",
  "connectorRef": "c-1a2b3c",
  "eventType": "issue.opened",
  "ignoreBotEvents": true,
  "filters": [{"field": "repo", "op": "eq", "value": "AInsel/ainsel"}]
}
```

**Response:** `201 Created` with the trigger, `400` on invalid JSON, `500` on K8s failures.

### GET /api/v1/triggers/{name}

**Response:** `200 OK` or `404 Not Found`.

### PUT /api/v1/triggers/{name}

Update a Trigger. All body fields are optional.

**Response:** `200 OK`, `400`, `404`, or `500`.

### DELETE /api/v1/triggers/{name}

**Response:** `204 No Content` or `404 Not Found`.

#### Filter operators

Each filter in the `filters` array specifies a `field`, an `op`, and either a `value` (string) or `values` (string array). Filters are combined with AND logic.

| Operator | `value` / `values` | Description |
|----------|--------------------|-------------|
| `eq` | `value` | Exact string match |
| `neq` | `value` | Not equal |
| `contains` | `value` | Field contains the value as a substring |
| `not-contains` | `value` | Field does not contain the value |
| `prefix` | `value` | Field starts with the value |
| `suffix` | `value` | Field ends with the value |
| `in` | `values` | Field value is one of the entries in `values` |
| `not-in` | `values` | Field value is not in `values` |
| `regex` | `value` | Field matches the value as a regular expression |

Example with `in`:

```json
{"field": "labels", "op": "in", "values": ["bug", "urgent"]}
```

---

## Cron Triggers

Manage `CronTrigger` resources — scheduled prompts delivered to an agent on a
cron schedule. See the [CRD reference](crd-reference.md#crontrigger) for the
schedule syntax.

### GET /api/v1/cron-triggers

List all `CronTrigger` resources, sorted by name. Supports `page`/`pageSize`
plus an optional `agent` filter (exact match against `spec.agentRef`).

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "c-9c1d2e",
      "name": "Daily standup summary",
      "agentRef": "standup-bot",
      "schedule": "0 9 * * 1-5",
      "prompt": "Summarize open PRs and stale issues.",
      "enabled": true,
      "status": {"agentValid": true, "scheduleValid": true, "nextRun": "2026-01-05T09:00:00Z"}
    }
  ],
  "total": 1, "page": 1, "pageSize": 50, "totalPages": 1
}
```

### POST /api/v1/cron-triggers

Create a CronTrigger.

**Request body:**
```json
{
  "name": "Daily standup summary",
  "agentRef": "standup-bot",
  "schedule": "0 9 * * 1-5",
  "prompt": "Summarize open PRs and stale issues.",
  "enabled": true
}
```

`agentRef`, `schedule`, and `prompt` are required; `enabled` defaults to `true`.

**Response:** `201 Created`, `400` on invalid JSON or missing fields, `500` on K8s failures.

### GET /api/v1/cron-triggers/{name}

**Response:** `200 OK` or `404 Not Found`.

### PUT /api/v1/cron-triggers/{name}

Update a CronTrigger. All body fields are optional; omitted fields are unchanged.

**Response:** `200 OK`, `400`, `404`, or `500`.

### DELETE /api/v1/cron-triggers/{name}

**Response:** `204 No Content` or `404 Not Found`.

---

## Invocations

Invocations record one dispatch of an event to an agent. They are kept in an in-process ring buffer by the hub; the endpoint returns `503 Service Unavailable` when invocation history is not configured.

### GET /api/v1/invocations

List recent invocations, newest first. Supports `page`/`pageSize`.

**Query parameters:** `agent`, `status` (one of `running`, `success`, `failure`, `timeout`), `trigger`, `event`, `since` / `until` (RFC3339 timestamp), `limit`.

**Response:** `200 OK`
```json
{
  "invocations": [
    {
      "id": "inv-1a2b3c4d",
      "agentName": "a-3f9a2b",
      "triggerName": "t-9c1d2e",
      "eventId": "evt-abc123",
      "eventType": "issue.opened",
      "eventSource": "forgejo",
      "connector": "c-1a2b3c",
      "startTime": "2026-05-20T10:00:00Z",
      "endTime": "2026-05-20T10:00:14Z",
      "durationMs": 14000,
      "status": "success"
    }
  ],
  "total": 1, "capacity": 1000,
  "page": 1, "pageSize": 50, "totalPages": 1
}
```

**Status codes:** `200`, `400` (invalid `page`/`pageSize`), `503` (invocation store not configured).

### GET /api/v1/invocations/{id}

Fetch one invocation by ID.

**Response:** `200 OK`, `400` if the ID is missing, `404 Not Found`, or `503`.

---

## Chat Sessions

Chat sessions let operators converse with an agent directly from the hub UI — no webhook or trigger required. Sessions are stored in the hub's database (table `chat_sessions` and `chat_messages`) and are per-user.

### GET /api/v1/chat/sessions

List chat sessions for the authenticated user. Supports `?page=` and `?pageSize=` (defaults: `1` / `20`). Optional `?agent=<name>` filters by agent.

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "sess-abc123",
      "agentName": "code-reviewer",
      "userId": "user-1",
      "createdAt": "2026-06-22T00:00:00Z",
      "updatedAt": "2026-06-22T00:05:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "pageSize": 20
}
```

### POST /api/v1/chat/sessions

Create a new chat session.

**Request body:**
```json
{ "agentName": "code-reviewer" }
```

**Response:** `201 Created` with the session object (including an empty `messages` array). `400` if `agentName` is missing. `503` if chat is not configured.

### GET /api/v1/chat/sessions/{id}

Fetch a chat session with its full message history.

**Response:** `200 OK`
```json
{
  "id": "sess-abc123",
  "agentName": "code-reviewer",
  "userId": "user-1",
  "createdAt": "2026-06-22T00:00:00Z",
  "updatedAt": "2026-06-22T00:05:00Z",
  "messages": [
    {
      "id": 1,
      "sessionId": "sess-abc123",
      "role": "user",
      "content": "Hello",
      "tokens": 5,
      "createdAt": "2026-06-22T00:00:00Z"
    },
    {
      "id": 2,
      "sessionId": "sess-abc123",
      "role": "assistant",
      "content": "Hi there!",
      "tokens": 10,
      "createdAt": "2026-06-22T00:00:01Z"
    }
  ]
}
```

`400` if the ID is missing, `404 Not Found`.

### DELETE /api/v1/chat/sessions/{id}

Delete a chat session and all its messages.

**Response:** `204 No Content`, `400` if the ID is missing, `404 Not Found`.

### POST /api/v1/chat/sessions/{id}/messages

Send a message to the agent in an existing session. The hub forwards the message to the agent runtime via the event queue and returns immediately.

**Request body:**
```json
{ "content": "Review this PR for me" }
```

**Response:** `201 Created` with the created user message. `400` if `content` is missing/empty. `404` if the session doesn't exist.

---

## MCP Servers

Internal MCP-server registry. Endpoints return `503` when the MCP service is not wired (typical for dev clusters without the registry enabled).

### GET /api/v1/mcp-servers

List all registered MCP servers.

**Response:** `200 OK` returns a JSON array (not a paginated envelope) of:
```json
[
  {
    "name": "fs",
    "displayName": "Filesystem MCP",
    "description": "...",
    "image": {"repository": "ghcr.io/example/fs-mcp", "tag": "1.0.0"},
    "transport": "sse",
    "port": 8080,
    "path": "/sse",
    "env": [{"name": "FOO", "value": "bar"}],
    "envFrom": [{"secretRef": {"name": "fs-secrets"}}],
    "resources": {"requests": {"cpu": "50m", "memory": "64Mi"}, "limits": {"cpu": "200m", "memory": "128Mi"}},
    "managedBy": "user",
    "endpoint": "http://fs.ainsel.svc:8080/sse",
    "status": {"phase": "Ready", "message": ""},
    "createdAt": "2026-05-20T09:00:00Z",
    "updatedAt": "2026-05-20T10:00:00Z"
  }
]
```

### POST /api/v1/mcp-servers

Register a new MCP server. `managedBy` must be empty or `"user"` (the hub forces it to `"user"` server-side); hub-managed entries cannot be created through this API.

**Request body:** same as the list item shape minus `endpoint`, `status`, `createdAt`, `updatedAt`.

**Response:** `201 Created`, `400` on invalid body or bad `managedBy`, `409 Conflict` if the name exists, `500` on internal failures, or `503` if the MCP service is not configured.

### GET /api/v1/mcp-servers/{name}

**Response:** `200 OK`, `400` if the name is missing or contains a slash, `404 Not Found`, `500`, or `503`.

### PUT /api/v1/mcp-servers/{name}

Update an MCP server. `managedBy` on the stored record is preserved; the hub does not allow promoting a user-managed entry to hub-managed (or vice versa) through this endpoint.

**Response:** `200 OK`, `400`, `404`, `500`, or `503`.

### DELETE /api/v1/mcp-servers/{name}

Delete an MCP server. Returns `409 Conflict` for hub-managed entries.

**Response:** `204 No Content`, `404 Not Found`, `409 Conflict`, `500`, or `503`.

---

## Personas

Personas live in the hub's database (tables `personas` and `persona_versions`). Each persona has an opaque ULID identifier, a current version, and a full edit history. When a persona is created or updated, the hub renders a ConfigMap named `persona-<id>` into the hub's own namespace with a single `persona.md` data key holding the current text. The ConfigMap also carries:

- `labels.ainsel.dev/managed-by: hub`
- `labels.ainsel.dev/resource: persona`
- `annotations.ainsel.dev/persona-name: <name>`
- `annotations.ainsel.dev/persona-version: "<version-number>"`

The runtime mounts `persona.md` at `/etc/agent/persona.md` (consumer added in a follow-up project).

Validation: `name` is non-empty, unique across personas, and ≤ 200 chars. `description` ≤ 2000 chars. `text` is non-empty and ≤ 100 000 chars.

### GET /api/v1/personas

List all personas (metadata only — no `text` body).

Supports the standard `?page=` and `?pageSize=` query params. `page` defaults
to `1`, `pageSize` defaults to `50` and is clamped to `200`. Invalid values
respond `400`.

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "01HX8YTNRD9Q3K5R6Z3SD9TXC7",
      "name": "code-reviewer",
      "description": "Reviews pull requests",
      "currentVersion": 3,
      "createdAt": "2026-05-20T09:00:00Z",
      "updatedAt": "2026-05-20T10:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "pageSize": 50,
  "totalPages": 1
}
```

### POST /api/v1/personas

Create a new persona. The hub generates the ULID and inserts the initial version (1).

**Request body:**
```json
{
  "name": "code-reviewer",
  "description": "Reviews pull requests",
  "text": "You are a code reviewer..."
}
```

**Response:** `201 Created` with the full persona including `text` and `currentVersion: 1`. `400` on validation failures, `409` if the `name` is already in use, `500` on backend failures.

### GET /api/v1/personas/{id}

Fetch one persona, including the current `text`.

**Response:** `200 OK` (full Persona) or `404 Not Found`.

### PUT /api/v1/personas/{id}

Apply a partial update. Any subset of `{name, description, text}` is accepted. If `text` differs from the current text, a new `persona_versions` row is inserted and `currentVersion` is bumped. If `text` is omitted or unchanged, only metadata is updated and `currentVersion` stays the same.

**Request body:**
```json
{"text": "You are a thorough code reviewer..."}
```

**Response:** `200 OK` with the updated persona, `400` on validation failure, `404` if missing, `409` on name collision, `500` on backend failure.

### DELETE /api/v1/personas/{id}

Delete a persona. Cascades to its version history and removes the rendered ConfigMap.

The hub refuses the delete with `409 Conflict` if any Agent CR references the persona by ID (`spec.persona.id`). The response body lists the referrers so the caller can act on them:

```json
{
  "error": "persona in use",
  "referrers": [{"agentName": "code-reviewer-agent"}]
}
```

**Response:** `204 No Content`, `404 Not Found`, `409 Conflict` (with `referrers`), or `500`.

### GET /api/v1/personas/{id}/versions

List every stored version, newest first. Metadata only — no `text`.

Supports the standard `?page=` / `?pageSize=` query params, same defaults and
bounds as `GET /api/v1/personas`.

**Response:** `200 OK`
```json
{
  "items": [
    {"personaId": "01HX...", "versionNumber": 3, "createdAt": "..."},
    {"personaId": "01HX...", "versionNumber": 2, "createdAt": "..."},
    {"personaId": "01HX...", "versionNumber": 1, "createdAt": "..."}
  ],
  "total": 3,
  "page": 1,
  "pageSize": 50,
  "totalPages": 1
}
```

### GET /api/v1/personas/{id}/versions/{n}

Fetch one specific historical version with its `text`.

**Response:** `200 OK` (full Version) or `404 Not Found`.

### POST /api/v1/personas/{id}/rollback

Copy the text of an older version into a new current version (incrementing `currentVersion`). The historical row is left untouched; rollback creates a new row pointing at the old text.

**Request body:**
```json
{"toVersion": 2}
```

**Response:** `200 OK` with the updated persona, `400` if `toVersion` is missing / non-positive, `404` if the persona or target version doesn't exist, `500` on backend failure.

---

## Skills

Skills are reusable prompt fragments stored in the hub's database (table `skills`). A skill has a name, a description, and a body (free-form text, typically markdown instructions). Personas reference skills by name; the runtime injects the body at invocation time.

Validation: `name` is non-empty, unique, and ≤ 200 chars. `description` ≤ 2000 chars. `body` ≤ 100 000 chars.

### GET /api/v1/skills

List all skills (metadata only — no `body`). Supports `?page=` and `?pageSize=` (defaults: `1` / `50`, max `200`).

**Response:** `200 OK`
```json
{
  "items": [
    {
      "id": "01HX8YTNRD9Q3K5R6Z3SD9TXC7",
      "name": "pr-review",
      "description": "Reviews pull requests",
      "createdAt": "2026-06-01T00:00:00Z",
      "updatedAt": "2026-06-15T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "pageSize": 50
}
```

### POST /api/v1/skills

Create a new skill.

**Request body:**
```json
{
  "name": "pr-review",
  "description": "Reviews pull requests",
  "body": "## Instructions\n\nFocus on..."
}
```

**Response:** `201 Created` with the full skill (including `body`). `400` on validation failure. `409` if a skill with that name already exists.

### GET /api/v1/skills/{id}

Fetch one skill by ID, including the full `body`.

**Response:** `200 OK`, `400` if the ID is missing, `404 Not Found`.

### PUT /api/v1/skills/{id}

Update a skill. All fields are optional; only provided fields are updated.

**Request body:**
```json
{ "description": "Updated description", "body": "## Updated..." }
```

**Response:** `200 OK` with the updated skill, `400` on validation failure, `404 Not Found`.

### DELETE /api/v1/skills/{id}

Delete a skill.

**Response:** `204 No Content`, `400` if the ID is missing, `404 Not Found`.

---

## Stats

### GET /api/v1/stats

Aggregate dashboard tile. Returns resource counts (total + healthy) for agents, connectors, and triggers, plus the last-hour error count from Loki and the lifetime token total from Prometheus.

**Response:** `200 OK`
```json
{
  "agents":     {"total": 5, "healthy": 4},
  "connectors": {"total": 2, "healthy": 2},
  "triggers":   {"total": 12, "healthy": 11},
  "errors":     {"lastHour": 3},
  "tokens":     {"inputTokens": 120000, "outputTokens": 45000}
}
```

When Loki or Prometheus is not configured, the affected fields are silently left at zero rather than failing the request.

---

## Activity & Errors

These endpoints stream log-derived activity off Loki. They return `503 Service Unavailable` when the log backend is not configured.

### GET /api/v1/events

List recent activity events (log_type=`activity_event`).

**Query parameters:** `limit` (default `50`), `status`, `eventType`, `connector`, `since` (RFC3339).

**Response:** `200 OK`
```json
{
  "events": [
    {
      "id": "evt-1716198000000000000",
      "timestamp": "2026-05-20T10:00:00Z",
      "eventType": "issue.opened",
      "connector": "c-1a2b3c",
      "actor": "alice",
      "subject": "AInsel/ainsel#42",
      "action": "opened",
      "status": "matched",
      "matches": [{"triggerName": "t-9c1d2e", "agentName": "a-3f9a2b"}]
    }
  ],
  "total": 1
}
```

**Status codes:** `200`, `502` (Loki query failed), `503` (log backend not configured).

### GET /api/v1/errors

List recent error events (log_type=`error_event`).

**Query parameters:** `limit` (default `50`), `severity`, `source`, `since` (RFC3339).

**Response:** `200 OK`
```json
{
  "errors": [
    {
      "id": "err-1716198000000000000",
      "timestamp": "2026-05-20T10:00:00Z",
      "severity": "error",
      "source": "hub",
      "message": "...",
      "details": {"...": "..."}
    }
  ],
  "total": 1
}
```

**Status codes:** `200`, `502`, `503`.

---

## Observability — Metrics

Hub-internal Prometheus counters and per-agent token usage. Responses are cached server-side for ~30 s. All endpoints return `503` when the Prometheus backend is not configured and `502` when the upstream query fails.

### GET /api/v1/observability/metrics/summary

Current values of the four hub counters (`hub_events_consumed_total`, `hub_triggers_matched_total`, `hub_events_routed_total`, `hub_routing_errors_total`).

**Response:** `200 OK`
```json
{
  "eventsConsumed": 12345,
  "triggersMatched": 4567,
  "eventsRouted": 4500,
  "routingErrors": 12,
  "updatedAt": "2026-05-20T10:00:00Z"
}
```

### GET /api/v1/observability/metrics/timeseries

Per-second rate of a hub counter over a chosen window.

**Query parameters:**
- `metric` — one of `events_consumed`, `triggers_matched`, `events_routed`, `routing_errors` (default `events_consumed`).
- `range` — one of `1h`, `6h`, `24h`, `7d` (default `1h`).

**Response:** `200 OK`
```json
{
  "metric": "events_consumed",
  "range": "1h",
  "step": "30s",
  "points": [{"timestamp": "2026-05-20T09:00:00Z", "value": 0.42}]
}
```

**Status codes:** `200`, `400` on unknown metric or range, `502`, `503`.

### GET /api/v1/observability/metrics/agents

Per-agent token consumption and invocation count.

**Response:** `200 OK`
```json
{
  "agents": [
    {"agent": "a-3f9a2b", "inputTokens": 12000, "outputTokens": 4500, "totalTokens": 16500, "invocations": 42}
  ],
  "updatedAt": "2026-05-20T10:00:00Z"
}
```

### GET /api/v1/observability/metrics/tokens/summary

24-hour token totals (fixed window, regardless of any dashboard range selector) with previous-period comparison.

**Response:** `200 OK`
```json
{
  "inputTokens": 8400,
  "outputTokens": 3200,
  "totalTokens": 11600,
  "previousTotalTokens": 9100,
  "updatedAt": "2026-05-20T10:00:00Z"
}
```

### GET /api/v1/observability/metrics/tokens/timeseries

24-hour token-usage sparkline stepped at 30-minute intervals.

**Query parameters:** `range` — `24h` only (other values return `400`).

**Response:** `200 OK`
```json
{
  "range": "24h",
  "step": "30m0s",
  "points": [{"timestamp": "2026-05-19T10:00:00Z", "value": 510}]
}
```

### GET /api/v1/observability/metrics/tokens/by-subject

One row per `(agent, repo, eventType, model)` tuple over the requested range.

**Query parameters:** `range` — one of `1h`, `6h`, `24h`, `7d` (default `24h`).

**Response:** `200 OK`
```json
{
  "range": "24h",
  "rows": [
    {"agent": "a-3f9a2b", "repo": "AInsel/ainsel", "eventType": "issue.opened", "model": "gpt-4", "inputTokens": 2400, "outputTokens": 900, "totalTokens": 3300}
  ],
  "updatedAt": "2026-05-20T10:00:00Z"
}
```

### Deprecated metric aliases

The following routes are accepted as deprecated aliases of their `/observability/` siblings. Each response includes `Deprecation: true` and a `Link: <successor>; rel="successor-version"` header. They will be removed once all deployed frontends call the canonical paths.

| Alias | Successor |
|-------|-----------|
| `GET /api/v1/metrics/summary` | `GET /api/v1/observability/metrics/summary` |
| `GET /api/v1/metrics/timeseries` | `GET /api/v1/observability/metrics/timeseries` |
| `GET /api/v1/metrics/agents` | `GET /api/v1/observability/metrics/agents` |

---

## Observability — Logs

### GET /api/v1/observability/logs

Tail recent log lines from Loki. Also accessible (with the same handler) at `GET /api/observability/logs`.

**Query parameters:**
- `query` — free-form LogQL. Forwarded to Loki untouched.
- `app` — convenience selector. Builds `{namespace="<loki ns>", app="<app>"}`. Ignored when `query` is set.
- `range` — one of `1h`, `6h`, `24h` (default `1h`).
- `limit` — positive integer, default `500`, capped at `1000`.

When neither `query` nor `app` is set the handler builds a default selector that scopes to the hub's Loki namespace, requires a non-empty `app` label, and excludes the hub's own app — i.e. "all agent pods".

**Response:** `200 OK`
```json
{
  "logs": [
    {"timestamp": "2026-05-20T10:00:00.123Z", "message": "...", "labels": {"app": "ainsel-agent-foo"}}
  ],
  "total": 1,
  "query": "{namespace=\"ainsel\",app=~\".+\",app!=\"ainsel-hub\"}"
}
```

**Status codes:** `200`, `400` on invalid `limit`/`range`, `502` when the Loki query fails, `503` when no log backend is configured.

---

## Observability — Conversations

### GET /api/v1/observability/conversations

Return agent conversation messages captured from agent turns and stored in the `task_conversations` table. These are the messages the hub's event detail view uses to render the communication for an event/invocation (user prompt, assistant thinking/text, tool calls, and tool results).

**Query parameters:**
- `agent` — filter by agent name.
- `invocation` — filter by invocation ID.
- `correlation` — filter by correlation ID.
- `limit` — max messages, default `100`, capped at `500`.

**Response:** `200 OK`
```json
{
  "messages": [
    {
      "id": 12,
      "invocationId": "inv-1a2b3c4d",
      "correlationId": "corr-9f8e7d",
      "agentName": "a-3f9a2b",
      "role": "assistant",
      "content": "[{\"type\":\"text\",\"text\":\"Looking into this issue...\"}]",
      "model": "glm-5.1:cloud",
      "inputTokens": 2400,
      "outputTokens": 900,
      "stopReason": "end_turn",
      "createdAt": "2026-05-20T10:00:05Z"
    }
  ],
  "total": 1
}
```

Each message has the fields `id` (number), `invocationId`, `correlationId`, `agentName`, `role` (`user` | `assistant` | `toolResult`), `content` (a JSON **string** — see encoding note below), `model`, `inputTokens`, `outputTokens`, `stopReason`, and `createdAt` (RFC3339).

**`content` encoding:** `content` is always a JSON-encoded string. For `user` and `assistant` messages it decodes to an array of blocks: `{"type":"text","text"}`, `{"type":"thinking","text"}`, and `{"type":"toolCall","id","name","arguments"}`. For `toolResult` messages it decodes to a single object: `{"toolCallId","isError","content"}`.

**Status codes:** `200`, `405` (wrong method), `502` (query failure), `503` (log backend not configured).

---

## Tokens

### GET /api/v1/tokens

Per-`(agent, repository, issueNumber, model)` token consumption from Prometheus. Returns `503` when no Prometheus backend is configured.

**Query parameters:** `agent`, `repository`, `issueNumber` (each adds an exact-match Prometheus label filter).

**Response:** `200 OK`
```json
{
  "tokens": [
    {"agent": "a-3f9a2b", "repository": "AInsel/ainsel", "issueNumber": "42", "model": "glm-5.1:cloud", "inputTokens": 2400, "outputTokens": 900}
  ],
  "total": {"inputTokens": 2400, "outputTokens": 900}
}
```

**Status codes:** `200`, `502` on Prometheus query failure, `503` when no metrics backend is configured.

---

## WebSocket

### GET /api/v1/ws

Upgrade to a WebSocket. The hub pushes a stream of JSON envelopes:

```json
{"type": "stats", "data": { ... same shape as /api/v1/stats ... }}
{"type": "event", "data": { ... ActivityEntry ... }}
{"type": "error", "data": { ... ErrorEntry ... }}
```

Immediately after the upgrade the server writes one `stats` snapshot. Subsequent messages are broadcast by hub-internal publishers (stats poller, event consumer, error consumer). Incoming client messages are read solely to detect disconnects; they are not interpreted.

**Status codes:** `101 Switching Protocols` on success, `400` if the upgrade handshake fails.

---

## Me

### GET /api/v1/me

Echo the authenticated user as forwarded by the ingress middleware (Authelia or equivalent). The hub reads the `Remote-User`, `Remote-Name`, `Remote-Email`, and `Remote-Groups` headers; when the user header is missing, `username` defaults to `"anonymous"`.

**Response:** `200 OK`
```json
{"username": "alice", "name": "Alice Doe", "email": "alice@example.com", "groups": "admins,operators"}
```

---

## Error Format

All non-WebSocket error responses return:

```json
{"error": "descriptive error message"}
```

| HTTP Status | Meaning |
|-------------|---------|
| `400` | Bad request (invalid JSON, missing required fields, unsupported query parameter values). |
| `401` / `403` | Issued by the ingress layer, not by the hub itself. |
| `404` | Resource not found. |
| `405` | Method not allowed on this route. |
| `409` | Conflict (resource already exists; deletion blocked by references; sync already running). |
| `500` | Internal server error. |
| `502` | Upstream backend (Loki, Prometheus, Forgejo) returned an error or was unreachable. |
| `503` | A required dependency is not configured (Loki, Prometheus, MCP service, invocation store). |

## Authentication

The API supports two authentication methods. See
[Access Control](access-control.md) for the full authorization model
(groups, roles, visibility rules).

### OIDC (interactive)

Browser-based authentication via Zitadel. API requests carry a standard
`Authorization: Bearer <jwt>` header. The OIDC middleware validates the
token against the configured issuer and audience.

### User tokens (programmatic)

Personal access tokens prefixed with `ainsel_`. Created via
`POST /api/v1/user-tokens`. A token grants the same permissions as the
owning user.

```
Authorization: Bearer ainsel_abc123...
```

### Internal service auth

Service-to-service calls use a shared secret via the `X-Internal-Token`
header, scoped to `/api/internal/*` endpoints only.

---

Source of truth: [`services/hub/internal/api/server.go`](../services/hub/internal/api/server.go).

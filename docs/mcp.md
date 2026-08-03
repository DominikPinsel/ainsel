# AInsel MCP Server

> **Intended use:** connect the AInsel MCP server to a **local agent** —
> Claude Code, Claude Desktop, an IDE plugin, or any other MCP-capable
> client running on your own machine — and use that agent to **control
> AInsel and ask it questions**: inspect triggers and connectors, list
> and update agents, read personas and skills, watch invocations, costs,
> logs, and platform health. The MCP server is the control surface for
> the platform; your local agent is the operator behind it.

The server lives in [`services/mcp/`](../services/mcp/). It speaks the
[Model Context Protocol](https://modelcontextprotocol.io) streamable
HTTP transport on `/mcp` and exposes a single, uniform tool surface that
proxies to the hub backend. It stores no state of its own — every tool
call is a typed read or write against the hub's REST API (and, for
observability, against Loki / Prometheus through the hub).

## Why a local agent?

AInsel's normal event loop is fully automated: webhooks arrive, the hub
matches triggers, agents run, artifacts appear back in the source
system. Administrators don't need to drive that loop — they configure it
once. The MCP server is for the **operations** side: the moments when a
human (or a human-with-an-agent) wants to look at the running system and
change it.

You _could_ read the same data from the hub REST API or the operations
console by hand. But the platform has dozens of resources (agents,
triggers, cron triggers, connectors, personas, skills, agent images,
MCP servers, GitHub apps, invocations, token usage, errors, logs,
metrics) and the relationships between them are what matter — "which
triggers fire for this agent?", "what did the last invocation cost and
did it error?", "is this connector healthy?". An agent with the full
tool surface can answer those in one turn by composing a few tool calls,
and can also make changes (update an agent's model, create a trigger,
edit a persona) without you reaching for `kubectl` or the web UI.

That is the intended workflow: **you point a local MCP client at the
AInsel MCP server, and the agent becomes your operations assistant for
the platform.**

## Connecting a local agent

The server is a standard MCP streamable HTTP endpoint. Any MCP-capable
client works. The two most common are Claude Code and Claude Desktop.

### Claude Code

Add the server to your Claude Code MCP configuration (project
`.mcp.json` or user config):

```json
{
  "mcpServers": {
    "ainsel": {
      "type": "http",
      "url": "https://your-ainsel-domain.example.com/mcp"
    }
  }
}
```

Claude Code performs OAuth Dynamic Client Registration against the
server's `/.well-known/openid-configuration` and runs a PKCE
authorization-code flow with the configured OIDC provider. After a
one-time browser login you get a short-lived token that is refreshed
automatically. No static token to manage. See
[Auth](#authentication) below for what the server does under the hood.

### Claude Desktop

Claude Desktop supports streamable HTTP servers in its MCP config
(`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "ainsel": {
      "type": "http",
      "url": "https://your-ainsel-domain.example.com/mcp"
    }
  }
}
```

Restart Claude Desktop and the `ainsel` tools appear as
`mcp__ainsel__<tool>` (for example `mcp__ainsel__list_triggers`).

### Any other MCP client

Point the client at the streamable HTTP endpoint:

```
URL:     https://your-ainsel-domain.example.com/mcp
Method:  POST
Auth:    Bearer <OIDC access token>   (or an ainsel_ user token, see below)
```

The server advertises tool capabilities and lists all tools on the
initial handshake. Each tool call is one `tools/call` request with a
JSON `arguments` object.

## Authentication

The `/mcp` endpoint is protected. Two token types are accepted:

1. **OIDC access token (JWT).** The default path for interactive local
   agents. The server validates the JWT against the configured OIDC
   issuer's JWKS and requires the Zitadel project ID in the `aud`
   claim. This is what Claude Code and Claude Desktop obtain via the
   OAuth flow. The server exposes a synthetic
   `/.well-known/openid-configuration` and `/.well-known/oauth-authorization-server`
   document plus a `/oauth/register` (DCR) endpoint that hands out a
   pre-registered PKCE client id, so clients that follow RFC 8414 /
   RFC 7591 (Claude Code among them) work with no extra wiring.

2. **`ainsel_` user token.** A long-lived personal token prefixed with
   `ainsel_`, issued from the operations console. The server forwards
   these to the hub's internal validate endpoint (cached for 60s) and
   resolves them to a user identity. Use this for headless clients or
   scripts that can't run a browser OAuth flow.

Requests with no `Authorization` header, or with an invalid token,
get `401` with a `WWW-Authenticate: Bearer` challenge. The health
endpoint (`/health`) is unauthenticated.

Required server-side env vars (set by the Helm chart):

| Var | Purpose |
|-----|---------|
| `OIDC_ISSUER` | OIDC issuer URL (e.g. your Zitadel) |
| `OIDC_PROJECT_ID` | Project ID; required value in token `aud` |
| `OIDC_CLIENT_ID` | Pre-registered PKCE client id returned by DCR |
| `MCP_RESOURCE_URL` | Externally-reachable server URL, used as the synthetic issuer |
| `INTERNAL_VALIDATE_SECRET` | Shared secret with the hub for `ainsel_` token validation |

## What you can do with the agent

Once connected, the agent can both **ask questions about** the platform
and **control** it. Below is the full tool surface, grouped by area.
Read tools are safe to call freely; mutation tools change platform state
and should be used deliberately.

### Workflows & agents

| Tool | Mode | What it answers |
|------|------|-----------------|
| `summarize_workflows` | read | Agent-centric joined view: every agent with its triggers, connector, event type, filters, tools, MCPs. Orphaned triggers listed separately. |
| `list_agents` | read | All agents and their status. |
| `get_agent` | read | One agent: config, persona, pod state. |
| `update_agent` | write | Update an agent's LLM config (`model`, `max_turns`, `temperature`). Omitted fields unchanged. |

### Triggers

| Tool | Mode | What it answers |
|------|------|-----------------|
| `list_triggers` | read | All triggers with agent/connector bindings and filters. |
| `get_trigger` | read | One trigger: filters and routing. |
| `create_trigger` | write | Create a trigger binding a connector + agent with optional event filters. |
| `update_trigger` | write | Update a trigger (name, agentRef, connectorRef, filters). Pass `[]` to clear filters. |
| `delete_trigger` | write | Delete a trigger by name. |

Filters are a JSON array of `{field, op, value}` objects. The `type`
field is derived from the `X-Forgejo-Event` / `X-Github-Event` header,
e.g. `[{"field":"type","op":"eq","value":"issues"},{"field":"action","op":"eq","value":"opened"}]`.

### Cron triggers

| Tool | Mode | What it answers |
|------|------|-----------------|
| `list_cron_triggers` | read | All cron triggers: schedule, agent binding, status. |
| `get_cron_trigger` | read | One cron trigger: schedule, prompt, agent binding, validation status. |
| `create_cron_trigger` | write | Create a cron trigger (5-field cron expression, prompt delivered verbatim to the agent on each fire). |
| `update_cron_trigger` | write | Update a cron trigger (empty string clears `prompt`/`agentRef`). |
| `delete_cron_trigger` | write | Delete a cron trigger by name. |

Schedule is a standard 5-field cron expression in the hub's local time
(default UTC). Numbers only — named days (`sun`/`mon`) are not
supported. Example: `"0 9 * * 1-5"` fires at 09:00 on weekdays.

### Connectors

| Tool | Mode | What it answers |
|------|------|-----------------|
| `list_connectors` | read | All connectors and their status. |
| `get_connector` | read | One connector: configuration and health. |

### Agent images (runtimes)

| Tool | Mode | What it answers |
|------|------|-----------------|
| `list_agent_images` | read | Agent runtimes available in the platform, with their tools and supported models. |
| `get_agent_image` | read | One agent image: tools, models, reference name. |
| `create_agent_image` | write | Register a new agent runtime (displayName, imageURL, optional description/env). |
| `update_agent_image` | write | Update an agent image (displayName, imageURL, description, env, tools). Send `[]` to clear env or tools. |
| `delete_agent_image` | write | Delete an agent image. Fails with a conflict if any Agent references it. |

### Personas

| Tool | Mode | What it answers |
|------|------|-----------------|
| `list_personas` | read | Personas registered with the hub (metadata only). |
| `get_persona` | read | Full persona by id, including the current version's text. |
| `list_persona_versions` | read | Version history for a persona, newest first (metadata only). |
| `get_persona_version` | read | A specific historical version, including its text. |
| `create_persona` | write | Create a persona (name, optional description, text). Returns the created persona with its id. |
| `update_persona` | write | Update a persona's name/description/text. Updating text creates a new version. |
| `delete_persona` | write | Delete a persona. Fails if active agents reference it. |

### Skills

| Tool | Mode | What it answers |
|------|------|-----------------|
| `list_skills` | read | Skills registered with the hub (metadata only). |
| `get_skill` | read | One skill by id (slug), including its Markdown body. |
| `create_skill` | write | Create a skill (id slug, name, optional description/body). |
| `update_skill` | write | Update a skill's name/description/body. Omitted fields preserved. |
| `delete_skill` | write | Delete a skill. Fails with 409 if referenced by any agent image. |

### MCP servers, GitHub apps

| Tool | Mode | What it answers |
|------|------|-----------------|
| `list_mcp_servers` | read | MCP servers in the registry: URLs and the tools each exposes. |
| `get_mcp_server` | read | One MCP server entry: URL, transport, tool names, non-secret auth config. |
| `list_github_apps` | read | GitHub App installations, install state, and which connectors use them. |
| `get_github_app` | read | One GitHub App: app ID, install state, referencing connectors. Does not expose private keys. |

### Invocations & activity

| Tool | Mode | What it answers |
|------|------|-----------------|
| `list_invocations` | read | Recent invocations (optional `agent`, `trigger`, `since`, `until`, `limit`). |
| `get_invocation` | read | One invocation by id: prompt, model output, tool calls, token usage, errors. |
| `summarize_agent_activity` | read | Per-agent rollup over a window (default 24h): invocation counts by status, token usage, cost, recent errors. |

### Cost & errors

| Tool | Mode | What it answers |
|------|------|-----------------|
| `get_token_usage` | read | Hub-aggregated cost: token counts per agent / repository / issue / model. |
| `get_stats` | read | Dashboard summary: agent/trigger/connector counts, healthy subset, last-hour errors, aggregate tokens. |
| `get_recent_errors` | read | Cross-agent error summary (optional `agent`, `since`, `limit`, `severity`, `source`). |

### Events (NATS)

| Tool | Mode | What it answers |
|------|------|-----------------|
| `get_stream_info` | read | PostgreSQL event queue stream and consumer stats (message count, lag, pending). |
| `list_recent_events` | read | Recent events on a stream, optionally filtered by subject pattern. |

### Observability

| Tool | Mode | What it answers |
|------|------|-----------------|
| `get_agent_logs` | read | Recent logs for a specific agent. |
| `query_logs` | read | Freeform LogQL query against Loki across the `ainsel` namespace. |
| `get_agent_metrics` | read | Key metrics for a specific agent: processing times, event counts, error rates. |
| `query_metrics` | read | Freeform PromQL query against Prometheus. |
| `get_platform_health` | read | Overview of the `ainsel` namespace: pod statuses, restarts, readiness, resource usage. |

## Default payload limits

MCP clients keep every tool result in conversation history, so large
responses quickly exhaust the context window. To keep sessions usable,
all read tools that return collections or logs apply **sensible default
limits** when the caller does not pass an explicit parameter:

| Tool | Default limit | Override parameter |
|------|--------------|-------------------|
| `list_agents` | `pageSize=50` | `page`, `pageSize` |
| `list_triggers` | `pageSize=50` | `page`, `pageSize` |
| `list_cron_triggers` | `pageSize=50` | `page`, `pageSize` |
| `list_connectors` | `pageSize=50` | `page`, `pageSize` |
| `list_agent_images` | `pageSize=50` | `page`, `pageSize` |
| `list_personas` | `pageSize=50` | `page`, `pageSize` |
| `list_skills` | `pageSize=50` | `page`, `pageSize` |
| `list_mcp_servers` | `pageSize=50` | `page`, `pageSize` |
| `list_invocations` | `limit=50` | `limit` |
| `get_recent_errors` | `limit=50` | `limit` |
| `get_token_usage` | 50 rows (local cap) | Narrow filters (`agent`, `repository`, `since`/`until`) |
| `get_agent_logs` / `query_logs` | `limit=100`, `since=1h` | `limit`, `since` |
| `list_recent_events` | `count=20` (max 100) | `count` |
| `summarize_agent_activity` | 1000 invocation cap | Default 24h window |
| `get_agent_metrics` | Extracted scalar values | `raw=true` for full Prometheus envelopes |
| `query_metrics` | 20 result series | `limit` |
| `get_platform_health` | Compact summary | `full=true` for complete pod detail |

### Truncation hints

When a response is truncated by a default limit, the tool adds metadata
to the response so agents know they can ask for more:

- **`truncated`**: boolean — `true` when the result was capped.
- **`hint`**: string — how to fetch more data (e.g. "pass limit=N" or "pass page=2").
- **`total`** / **`returned`**: numeric — how many items exist vs. how many were returned.

These fields are **additive** — existing response fields are never
renamed or removed. Where the hub already provides pagination metadata
(`total`, `page`, `pageSize`, `totalPages`), those fields are preserved
and `truncated`/`hint` are added alongside them.

### Bare array wrapping

If a hub endpoint returns a bare JSON array (no object wrapper), the
response is wrapped as `{"items": [...], "truncated": ..., "hint": ...}`
to provide a place for the truncation metadata.

## Example conversations

Once your local agent is connected, you can ask it things in plain
language and it will compose the right tool calls:

- *"What workflows do I have running?"* → `summarize_workflows`
- *"Show me the code-reviewer agent's persona."* → `get_agent` + `get_persona`
- *"Which triggers fire on new pull requests?"* → `list_triggers` (or `summarize_workflows`), filtered by the agent
- *"How much did agents cost today, and did anything error?"* → `get_token_usage` + `get_recent_errors`
- *"What did the last invocation of the triager do?"* → `list_invocations` + `get_invocation`
- *"Is the platform healthy?"* → `get_platform_health` + `get_stats`
- *"Update the code-reviewer to use qwen3.5:cloud and lower the temperature to 0.2."* → `update_agent`
- *"Add a cron trigger that asks the summarizer to write a daily standup at 9am on weekdays."* → `create_cron_trigger`
- *"Create a new trigger that sends issue comments to the triager."* → `create_trigger`
- *"Edit the reviewer persona to add a rule about not commenting on imports."* → `update_persona`

The read/mutate split matters: reads are cheap and side-effect free, so
encourage the agent to gather context before making changes. Mutation
tools act immediately against the hub (which then reconciles Kubernetes
state) — there is no pending-change staging yet, so review what the
agent proposes before it calls a write tool.

## Local development

```bash
cd services/mcp
make build
# or: go build -o bin/ainsel-mcp ./cmd/server

PORT=8080 \
OIDC_ISSUER=https://oidc.example.com \
OIDC_PROJECT_ID=<your-project-id> \
OIDC_CLIENT_ID=<your-pkce-client-id> \
MCP_RESOURCE_URL=http://localhost:8080 \
HUB_URL=http://localhost:8080 \
INTERNAL_VALIDATE_SECRET=<shared-secret> \
./bin/ainsel-mcp
```

The server only needs `HUB_URL` to reach the hub; everything else
(Loki, Prometheus, mem0) is accessed through the hub. Run
`make test` for the test suite.

## Reference

- [`services/mcp/README.md`](../services/mcp/README.md) — service-level README, env vars, build
- [`services/chat-mcp/README.md`](../services/chat-mcp/README.md) — the **chat sidecar** MCP (different scope: gives an _agent pod_ chat tools; this doc is about the **platform** MCP for operators)
- [Hub REST API](api-reference.md) — what the hub-backed tools proxy to
- [Event schema](event-schema.md) — payload shape returned by the NATS tools
- [Administrator guide](administrator-guide.md) — the "Talking to AInsel via MCP" section
- [Architecture](architecture.md) — where the MCP server sits in the platform
# MCP

Model Context Protocol server that exposes the AInsel platform to MCP
clients (Claude Desktop, IDE plugins, other agents) as a uniform tool
surface.

Speaks the MCP [streamable HTTP transport](https://modelcontextprotocol.io)
on `/mcp` and registers tools that proxy to the hub backend. (NATS,
Loki, and Prometheus are all reached through the hub, so the
server only needs `HUB_URL`.) The endpoint is protected by OIDC
JWT validation or `ainsel_` user-token validation; see
[`docs/mcp.md`](../../docs/mcp.md) for the full guide.

## Role in the platform

See [`docs/architecture.md`](../../docs/architecture.md).

Read-side companion to [`services/hub/`](../hub/): the hub exposes a
typed REST API for human consumers (`frontend/`); this service exposes
the same data plus observability and memory tools through the MCP
protocol for AI clients. It is a **client** of the hub — it stores no
state of its own.

## Tool groups

The server registers a single uniform tool surface. The canonical,
up-to-date reference — including args, read/write mode, and example
usage — is [`docs/mcp.md`](../../docs/mcp.md). Summary of what's
registered in [`internal/mcp/server.go`](internal/mcp/server.go):

- **Agents:** `list_agents`, `get_agent`, `update_agent` (LLM config).
- **Triggers:** `list_triggers`, `get_trigger`, `create_trigger`,
  `update_trigger`, `delete_trigger`.
- **Cron triggers:** `list_cron_triggers`, `get_cron_trigger`,
  `create_cron_trigger`, `update_cron_trigger`, `delete_cron_trigger`.
- **Connectors:** `list_connectors`, `get_connector`.
- **Workflows / activity:** `summarize_workflows`, `list_invocations`,
  `get_invocation`, `summarize_agent_activity`.
- **Agent images:** `list_agent_images`, `get_agent_image`,
  `create_agent_image`, `update_agent_image`, `delete_agent_image`.
- **Personas:** `list_personas`, `get_persona`, `list_persona_versions`,
  `get_persona_version`, `create_persona`, `update_persona`,
  `delete_persona`.
- **Skills:** `list_skills`, `get_skill`, `create_skill`,
  `update_skill`, `delete_skill`.
- **MCP servers / GitHub apps:** `list_mcp_servers`, `get_mcp_server`,
  `list_github_apps`, `get_github_app`.
- **Cost / errors:** `get_token_usage`, `get_stats`, `get_recent_errors`.
- **Events (NATS, via hub):** `get_stream_info`, `list_recent_events`.
- **Observability (via hub):** `get_agent_logs`, `query_logs` (Loki),
  `get_agent_metrics`, `query_metrics` (Prometheus).
- **Health:** `get_platform_health`.

> **Note:** Memory tools (`search_memory`, `list_memories`, `get_memory`) were
> removed in PR #372. Agents now use external MCP servers (configured as
> generic MCP entries in the frontend) for memory and other tools. The
> `MEM0_URL` env var is no longer set on the ainsel-mcp deployment.

## Intended use

The MCP server is meant to be connected to a **local agent** (Claude
Code, Claude Desktop, an IDE plugin) which then acts as an operations
assistant for the platform — asking questions about triggers,
connectors, agents, personas, invocations, costs, and health, and
making controlled changes. See [`docs/mcp.md`](../../docs/mcp.md) for
connection instructions, authentication, and example conversations.

## Local development

```bash
make build
# Or directly:
go build -o bin/ainsel-mcp ./cmd/server

# Run (OIDC_ISSUER and OIDC_PROJECT_ID are required; HUB_URL defaults
# to the in-cluster hub)
PORT=8080 \
OIDC_ISSUER=https://oidc.example.com \
OIDC_PROJECT_ID=<your-project-id> \
OIDC_CLIENT_ID=<your-pkce-client-id> \
MCP_RESOURCE_URL=http://localhost:8080 \
HUB_URL=http://localhost:8080 \
INTERNAL_VALIDATE_SECRET=<shared-secret> \
./bin/ainsel-mcp
```

Ports:

- `8080` — HTTP server hosting `/mcp` and `/health` (`PORT`)

Env vars (see [`internal/config/config.go`](internal/config/config.go)):
`PORT`, `OIDC_ISSUER` (required), `OIDC_PROJECT_ID` (required),
`OIDC_CLIENT_ID`, `MCP_RESOURCE_URL`, `HUB_URL`,
`INTERNAL_VALIDATE_SECRET`. The server only needs `HUB_URL` to reach the
hub; NATS, Loki, and Prometheus are all accessed through the hub.

## Testing

```bash
make test
# Or:
go test ./...
```

## Reference

- [MCP server guide](../../docs/mcp.md) — connecting a local agent, auth,
  full tool reference, example conversations
- [Hub REST API](../../docs/api-reference.md) — what the hub-backed tools
  proxy to
- [Event schema](../../docs/event-schema.md) — payload shape returned by
  the NATS tools

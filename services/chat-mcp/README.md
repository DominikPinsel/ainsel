# Chat MCP Sidecar

MCP server that exposes **chat tools** to the agent runtime. Runs as a
sidecar container in the agent pod, reachable via `localhost`. The agent's
`ainsel-mcp` pi extension connects to this server and registers its tools
as `mcp__chat__<tool>`, making them available to the LLM during
conversation turns.

## Role in the platform

See [`../../docs/architecture.md`](../../docs/architecture.md).

The chat sidecar is the **outbound** half of the chat feature: it gives
the agent tools to interact with chat sessions (send replies, read
history, list sessions). The **inbound** half — delivering a human's chat
message to the agent — goes through the hub → NATS → `ainsel-runner`
event loop, same as any other event.

The sidecar is **stateless**: every tool call proxies to the hub backend's
internal chat REST API (`/api/internal/chat/*`, authenticated with the
shared `X-Internal-Token`). The hub owns session storage, message history,
and WebSocket routing to the frontend.

```
┌──────────────────────────────────────────────────────────┐
│  Agent Pod                                                │
│                                                           │
│  ┌──────────────────┐     ┌─────────────────────┐        │
│  │  Pi runtime       │     │  Chat sidecar       │        │
│  │                   │     │  (this service)      │        │
│  │  ainsel-mcp ──────┼────►│  /mcp  (MCP server) │        │
│  │  (connects to     │     │                     │        │
│  │   localhost:8081) │     │  Tools:             │        │
│  │                   │     │   chat.list_sessions │        │
│  │  LLM calls tools  │     │   chat.get_history   │        │
│  │  during turn      │     │   chat.send_reply    │        │
│  │                   │     │   chat.send_status   │        │
│  └──────────────────┘     └────────┬────────────┘        │
│                                      │                     │
└──────────────────────────────────────┼─────────────────────┘
                                       │ HTTP proxy
                                       ▼
                              ┌──────────────────┐
                              │  Hub backend     │
                              │  /api/internal/* │
                              │  (sessions,      │
                              │   messages,      │
                              │   WebSocket)     │
                              └──────────────────┘
```

## Tools

| Tool | Args | Description |
|------|------|-------------|
| `chat.list_sessions` | — | List active chat sessions for this agent |
| `chat.get_history` | `session_id`, `limit?` | Fetch message history for a session |
| `chat.send_reply` | `session_id`, `content` | Send the agent's response to a session |
| `chat.send_status` | `session_id`, `content` | Send an intermediate status message |

## Local development

```bash
make build
# Or directly:
go build -o bin/ainsel-chat-mcp ./cmd/server

# Run (HUB_URL defaults to in-cluster service DNS)
PORT=8081 \
AGENT_NAME=developer \
HUB_URL=http://localhost:8080 \
./bin/ainsel-chat-mcp
```

Ports:

- `8081` — HTTP server hosting `/mcp` and `/health` (`PORT`)

Env vars:

| Var | Required | Default | Purpose |
|-----|----------|---------|---------|
| `PORT` | no | `8081` | HTTP listen port |
| `HUB_URL` | yes | `http://hub-backend.ainsel.svc.cluster.local:8080` | Hub backend REST API URL |
| `AGENT_NAME` | yes | — | Logical agent name (set by operator on the pod) |

## Testing

```bash
make test
# Or:
go test ./...
```

## Reference

- [Hub REST API](../../docs/api-reference.md) — chat endpoints (planned)
- [Event schema](../../docs/event-schema.md) — `chat.message` event type (planned)
- [Pi runtime](../../pi/) — `ainsel-mcp` extension that connects to this server
- [Platform MCP server](../mcp/) — the platform-wide MCP server (different scope)
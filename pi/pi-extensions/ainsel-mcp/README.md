# pi-ainsel-mcp

Pi extension that connects to MCP servers and registers their tools with
the running Pi instance.

## Environment

| Variable | Required | Notes |
|---|---|---|
| `MCP_SERVERS` | no | Comma-separated `name=url` pairs (e.g. `mem0=http://mcp-mem0:8080/mcp`). Empty/unset = no MCP tools registered. |
| `AGENT_NAME` | no | If set, auto-injected into tool calls whose schema declares a `user_id` property and the caller didn't supply one. |

## Naming convention

Each remote tool is registered as `mcp__<server>__<tool>` so the LLM sees a
collision-free, prefix-routable namespace. Matches the Go runtime
(`ainsel-ai-agent`).

## Failure modes

- Malformed `MCP_SERVERS` -> log + register zero MCP tools, agent still starts.
- Unreachable server -> log + skip that server, agent still starts.
- Tool call fails -> return error envelope (caught upstream by Pi's tool-call
  framework -- does not crash the agent).

## Bound startup

Catalog connection is bounded by a 10-second `AbortSignal.timeout` so a slow
upstream cannot block agent readiness.

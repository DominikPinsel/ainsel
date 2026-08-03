# pi-ainsel-runner

The pi extension that runs the ainsel agent's event loop. This replaces
the previous `runner.js` Node process — there is no longer a wrapper
spawning pi; pi is the only process in the pod, and this extension
drives it.

## What it does

On `session_start`:

1. Reads `AGENT_NAME`, `HUB_URL`, and `AGENT_TOKEN` from the environment.
2. Starts a Prometheus metrics server on `:9090/metrics`.
3. Starts a background HTTP long-poll loop against the hub.

Per task (sequential — pi has one in-flight turn at a time):

1. Fetches the next task via `GET /api/v1/agents/{name}/next-task?timeout=30s`.
2. Extracts an `EventContext` from the task headers and payload (see
   [Event Type Derivation](#event-type-derivation) below).
3. Sets `AINSEL_EVENT_*` env vars on the process so the `ainsel-tools`
   extension can populate the toolio request envelope's `context` field
   for tool calls.
4. Calls `pi.sendUserMessage(...)` with a structured `<event>...</event>`
   block + a "Handle the … on …" nudge.
5. Awaits `turn_end` so the turn completes.
6. ACKs (`POST .../tasks/{id}/ack`) or NACKs (`POST .../tasks/{id}/nack`)
   the task via the hub REST API.

## Task Headers

Each task delivered by the hub carries a `headers` map. The runner uses
these headers to determine the event type and build the agent prompt.

| Header | Purpose | Example Values |
|--------|---------|----------------|
| `type` | Canonical event type, set by the hub for **all** event sources | `issues`, `push`, `cron`, `chat.message` |
| `X-Trigger-Name` | Name of the trigger that matched | `develop-on-assign` |
| `X-Invocation-ID` | Unique invocation ID for tracking | `inv-1a2b3c4d` |

The original webhook headers (`X-Forgejo-Event`, `content-type`, etc.)
are also passed through for informational purposes, but the runner does
not inspect them — all platform normalization happens in the hub.

### Event Type Derivation

The runner reads the `type` header and combines it with the `action`
field from the webhook payload to produce a compound event type:

| `type` header | `action` from payload | Result |
|---------------|----------------------|--------|
| `issues` | `opened` | `issues.opened` |
| `pull_request` | `assigned` | `pull_request.assigned` |
| `push` | *(none)* | `push` |
| `cron` | *(none)* | `cron` |
| `chat.message` | *(none)* | `chat.message` |

### Special Event Types

| Type | Behaviour |
|------|-----------|
| `chat.message` | Builds a chat prompt; agent replies via `mcp__chat__send_reply` |
| `cron` | Builds a prompt from `data.prompt`; no forge target |

## Authentication

The runner authenticates all hub API calls with the `X-Internal-Token`
header, using the value from the `AGENT_TOKEN` environment variable.

## Environment

| Var | Required | Purpose |
| --- | --- | --- |
| `AGENT_NAME` | yes | Logical agent name (used in hub API paths) |
| `HUB_URL` | yes | Hub backend base URL (e.g. `http://ainsel-hub:8080`) |
| `AGENT_TOKEN` | yes | Shared secret for `X-Internal-Token` header |
| `OLLAMA_CLOUD_MODEL` | yes | Model identifier; reported in token metrics |
| `NAK_DELAY_MS` | no | NACK backoff in ms before the hub re-delivers a failed task (default `60000`) |
| `TURN_TIMEOUT_MS` | no | Maximum time in ms to wait for a single LLM turn before aborting and nacking (default `600000`, i.e. 10 minutes) |
| `TURN_SETTLE_TIMEOUT_MS` | no | Maximum time in ms to wait for an aborted turn to settle before proceeding (default `10000`, i.e. 10 seconds). Backstop only — after `ctx.abort()` the run should settle in milliseconds. |
| `TOOL_RESULT_MAX_CHARS` | no | Per-text-block character cap for toolResult transcript content (default `16000`). Oversized blocks are truncated with head+tail retention and a `[truncated N chars]` marker. Read lazily at call time. |

## Metrics

The runner exports Prometheus metrics on `:9090/metrics`:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `agent_tokens_used_total` | Counter | `agent`, `repo`, `org`, `event_type`, `token_type`, `model` | Total tokens consumed by the agent |

## Why an extension, not a separate Node process

Pi already provides:

- a long-lived process lifecycle (in `--mode rpc`)
- `pi.sendUserMessage(content)` to start a turn programmatically
- `session_start` / `session_shutdown` hooks for setup / teardown
- a structured logging convention (JSON to stdout)
- abort / signal propagation
- the ability for one extension (`ainsel-tools`) to register tools the
  LLM uses during a turn

A separate runner.js reimplemented the lifecycle and event loop poorly.
Following pi's
["External integrations (file watchers, webhooks, CI triggers)"](https://pi.dev/docs/latest/extensions#examples-reference)
example pattern keeps the entire runtime inside pi, where its lifecycle
primitives are first-class.

## Secret Redaction

Before any transcript content is sent to the hub, the runner redacts
all environment variable values that could be secrets. This is a
defense-in-depth control: even if an LLM echoes a credential or a tool
result contains one, the hub never receives it.

**How it works:**

1. On first use, the runner collects all `process.env` values.
2. Values are excluded from redaction if:
   - The variable name is in a denylist of known non-sensitive vars
     (`AGENT_NAME`, `HUB_URL`, `OLLAMA_CLOUD_MODEL`, `PATH`, `HOME`, etc.)
   - The variable name starts with `AINSEL_EVENT_` (per-task context vars)
   - The value is shorter than 8 characters (avoids corrupting common
     short strings like booleans, ports, or single words)
3. Every remaining value is replaced with `***` in the serialized
   `content` of each conversation payload. Both the raw value and its
   JSON-escaped form are replaced (covers secrets containing `"`, `\`, etc.).
4. The secret set is computed once per pod lifetime (memoized).

**Failure mode:** The control fails *closed* — a new secret env var is
automatically covered without code changes. The worst case is
over-redaction of a non-secret value (a cosmetic transcript blemish,
not a breach).

## Caveats

- **Tool-result transcript content is size-capped.** Each text block in a
  `toolResult` message is capped at `TOOL_RESULT_MAX_CHARS` (default 16 000)
  characters using head+tail retention (75% head / 25% tail) with a
  `... [truncated N chars] ...` marker. Non-text blocks (images, etc.) are
  reduced to `{ type }`. A per-message safety net (`max(128 000, 8 × block
  cap)`) bounds the final serialized string. Caps are in JS string length
  (UTF-16 code units); JSON escaping can expand the final byte size somewhat.
- **Sequential only.** The `AINSEL_EVENT_*` env-var injection is
  process-global; concurrent event handling would race. This is fine
  because pi has only one active conversation per session.
- **`session_start` runs the loop in the background**, not awaited, so
  pi can finish startup. Loop errors are logged but don't crash pi —
  if the hub becomes unreachable, the loop retries with backoff.
- **Conversation history is stripped between events.** The `context`
  hook trims messages before the latest `<event>` block so each turn
  starts fresh.

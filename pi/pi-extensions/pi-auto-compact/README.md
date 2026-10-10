# pi-ainsel-auto-compact

Automatic context compaction for ainsel agents — ships the upstream
[`pi-auto-compact`](https://github.com/Chasen-Liao/pi-auto-compact) pi
extension (MIT) in the ainsel runtime image.

## Why

Pi's built-in compaction only checks between tool batches, and once the
projected context nears the model's window the agent run degrades — or hard
stops with an overflow error mid-task. Ainsel agents are long-lived pods fed a
stream of tasks; a pod whose context is wedged keeps pulling events it cannot
process. This extension compacts *before* the prompt is sent, whenever the
projected context would cross the configured threshold, so the turn starts
with headroom instead.

## What it does

- **Preflight compaction** (`input` hook): for each idle prompt, if current
  tokens plus the incoming prompt cross the threshold (default **78%** of the
  model's context window), it runs one compaction and then lets the prompt
  through unchanged. Pi's own auto-compaction stays enabled as the safety
  net — this extension never disables anything.
- **Soft-failure tolerant** ("Nothing to compact" / "Already compacted")
  errors mean the context is already minimal and the prompt is sent anyway.
- **Fail-closed** on hard compaction errors: the prompt is **not** sent. In
  the ainsel runner semantics that surfaces as the turn timing out and the
  task being nacked for retry (retry gives compaction another chance; a
  transient summarizer error, for example, may succeed on the next attempt).
- **Hot-reloaded config**: re-read before every prompt, so changes to the
  config file apply without restarting the pod.

Deliberately *not* triggered mid-turn: calling `ctx.compact()` while an agent
turn is running aborts that turn and drops its work, so the `tool_call` /
`turn_end` hooks here are status-only (and no-ops in RPC mode anyway).

## Config

Read from pi's agent dir — `~/.pi/agent/pi-auto-compact.json` in the runtime
container (honors `PI_CODING_AGENT_DIR`, same directory the operator mounts
`models.json` into):

```json
{
  "threshold": 78,
  "alignPiThreshold": false
}
```

| Field               | Default | Effect                                                                 |
| ------------------- | ------- | ---------------------------------------------------------------------- |
| `threshold`         | `78`    | Preflight-compact when projected usage reaches this percent of the window. Clamped to `[30, 99)`. |
| `alignPiThreshold`  | `false` | Also point pi's own mid-run compaction threshold at `threshold` (via `reserveTokens` overrides in `~/.pi/agent/settings.json`). |

Keep `alignPiThreshold` at its default unless you know you want it: with the
default, this extension owns the *pre-prompt* compaction and pi's built-in
compaction remains the *mid-run* safety net. It is also the safer choice with
regard to writable state — enabling it writes per-model `reserveTokens`
overrides into `/home/agent/.pi/agent/settings.json`.

No config file? The extension runs with the defaults above. Invalid values are
ignored (upstream keeps its last-known good config), never failing the agent
at startup.

## RPC-mode notes

The ainsel runtime runs pi with `--mode rpc`, so:

- `ctx.hasUI` is false — the upstream status-bar widget, notifications, and
  the `/compact-threshold` slash command are inert. Configuration happens via
  the JSON file above.
- Compaction itself, the `input` preflight, and pi's own mid-run compaction
  work exactly as interactively; they operate on the session, not the UI.
- Compaction consumes LLM tokens (it summarizes the conversation with the
  agent's own model). Expect occasional compaction-invoked token spend on
  long-running agents — that is the feature, not a bug.

## Why a thin wrapper instead of vendored code

`index.ts` here is a re-export of the upstream package's default factory; the
upstream MIT source is not duplicated into this repo. That keeps upgrades to a
one-line dependency bump in `package.json`, and upstream fixes land on normal
`npm install` during image build. Behavior- and bug-wise we follow upstream.

`.npmrc` in this directory sets `legacy-peer-deps=true`: npm would otherwise
auto-install this extension's `@earendil-works/pi-coding-agent` peer — a
second, version-floating copy of pi inside the image. None is needed at
runtime because pi's extension loader aliases the package specifier to its own
entry point, so upstream imports resolve to the running host pi.

## Upstream tracking

- Code: <https://github.com/Chasen-Liao/pi-auto-compact>
- npm:  <https://www.npmjs.com/package/pi-auto-compact>
- Version pinned: `^1.3.1` (needs pi-coding-agent `>=0.99.0`; the runtime pins
  `PI_VERSION=latest` in `pi/Dockerfile`, currently 1.x)
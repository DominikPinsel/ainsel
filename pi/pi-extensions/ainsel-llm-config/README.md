# pi-ainsel-llm-config

> ⚠️ **This extension is a workaround. Delete it as soon as pi-coding-agent
> adds a native `temperature` slot to `models.json`.**

Pi extension that injects LLM sampling parameters into every provider
request via the `before_provider_request` hook.

## Why this exists

The Agent CRD accepts `spec.llm.temperature` and the hub frontend exposes a
field for it, but pi-coding-agent (we run 0.75.x) has no native way to set
sampling parameters from `models.json`. Verified by reading the upstream
schema in `packages/coding-agent/src/core/model-registry.ts`:
`ModelDefinitionSchema` / `ModelOverrideSchema` / `ProviderConfigSchema`
enumerate every accepted field and temperature is not among them. The only
documented mechanism is the `before_provider_request` extension hook, which
is exactly what `examples/extensions/provider-payload.ts` shows.

This extension is the smallest possible implementation of that hook,
wired to the agent operator's models.json output.

## When this extension goes away

When pi-coding-agent adds a native temperature slot to its config schema —
either on the `Model` definition or as a top-level provider-options block —
do the following and the workaround is gone:

1. Bump `PI_VERSION` in `pi/Dockerfile` to the release that includes native
   support.
2. Update `operators/agent/internal/controller/agent_controller.go` so the
   `reconcilePiModelsConfigMap` template emits the value in pi's native
   shape instead of under the `ainsel` root key.
3. Delete this directory (`pi/pi-extensions/ainsel-llm-config/`) — pi will
   handle the injection itself.

Track upstream: see the pi project issue tracker.

## Config

Read from `~/.pi/agent/models.json` (or `$PI_CODING_AGENT_DIR/models.json`):

```json
{
  "providers": { ... },
  "ainsel": {
    "temperature": 0.3
  }
}
```

| Field         | Range  | Effect                                  |
|---------------|--------|-----------------------------------------|
| `temperature` | 0..2   | Sets `temperature` on every request     |

Pi ignores unknown root keys (TypeBox doesn't enforce
`additionalProperties: false` on `ModelsConfigSchema`), so the `ainsel`
block round-trips through pi's validator unchanged. Missing / malformed /
out-of-range values are logged and skipped — the agent always starts.

## Why a dedicated extension

Kept separate from `ainsel-mcp` so its workaround nature is loud and the
deletion path is one `rm -rf` instead of code surgery. When this goes away,
nothing about MCP tool registration needs to change.

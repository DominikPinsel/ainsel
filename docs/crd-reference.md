# CRD Reference

> For the admin-facing model — how to use these CRDs to configure agents
> for your org — see the
> [administrator guide](administrator-guide.md). This document is the raw
> spec derived from `shared/api/api/v1alpha1/*.go`.
>
> **Note:** `Trigger` and `CronTrigger` were moved from Kubernetes CRDs to
> database tables.
> Their Go types still exist in `shared/api/api/v1alpha1/` as the canonical
> schema used by the hub REST API, and are documented below for schema
> completeness.

All CRDs use API group `ainsel.dev` version `v1alpha1`.

---

## Agent

Defines an AI agent with its LLM configuration, persona, runtime settings,
and scaling parameters.

The agent operator reconciles `Agent` resources into Kubernetes Deployments.
An `Agent` references an [`AgentImage`](#agentimage) via `spec.imageRef.name`
for its container image and tool catalog.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `displayName` | string | Yes | Human-readable agent name |
| `description` | string | No | Agent description |
| `imageRef.name` | string | Yes | Name of the `AgentImage` CR in the same namespace that provides the container image and tool catalog |
| `runtime.imagePullPolicy` | string | No | Kubernetes image pull policy (`Always`, `Never`, `IfNotPresent`) |
| `runtime.resources` | ResourceRequirements | No | CPU/memory requests and limits for the agent pod |
| `llm.model` | string | Yes | LLM model identifier (e.g. `glm-5.1:cloud`) |
| `llm.provider` | string | No | LLM provider backend. One of `ollama-cloud`, `opencode`, `custom` |
| `llm.maxTurns` | int | No | Maximum tool-use loop turns |
| `llm.temperature` | float64 | No | LLM sampling temperature |
| `llm.vision` | bool | No | Model accepts image input. When true the operator advertises `"input": ["text", "image"]` to pi, so screenshots and image attachments are sent instead of dropped. Default false |
| `persona.id` | string | Yes | ULID of a persona managed by the hub. The operator mounts a ConfigMap named `persona-<id>` (rendered by the hub) at `/etc/agent/persona.md` |
| `enabledTools[]` | []string | No | List of tool names to enable for this agent (e.g. `forgejo`, `git`, `shell`) |
| `skills.items[]` | []string | No | Agent-scoped skill selection (skill ids). Present = explicit override (`items: []` = no skills at all); absent = inherit the referenced image's `enabledSkills` |
| `mcp.servers[]` | []AgentMCPServer | No | Agent-scoped MCP servers (`name`, `url`, `tokenFromEnv`), resolved from the hub's MCP registry when the agent is written. Present = explicit override (`servers: []` = connect to none); absent = inherit the image's `mcpServers`. The operator injects `MCP_SERVERS` / `MCP_SERVER_TOKENS` from this list |
| `env[]` | []AgentEnvVar | No | Per-agent environment variables (`name`, `value`, `secret`) layered on top of the referenced image's `env`: a matching name overrides the image's value and secret flag, a new name is added. The operator writes the effective set to the `<agent>-image-env` Secret and injects each entry into the agent container |
| `scaling.replicas` | int32 | No | Container count while the agent scales statically (default 1). Once `scaling.minReplicas` is set this becomes the **ceiling** the agent may burst to under queue pressure — one container claims one task at a time, so it also caps concurrent tasks |
| `scaling.minReplicas` | int32 | No | Floor the agent falls back to when its queue is empty and drained. **Absent = static**: the operator ignores queue depth and keeps `replicas` running, which is how every agent behaved before this field existed. `0` = park the agent at zero containers until work arrives, paying one cold start per wake. Must not exceed `scaling.replicas` |
| `scaling.disabled` | bool | No | Set by the operator for a stopped agent: hold at zero containers even if the hub still counts queued work. Cleared on start |
| `memory.enabled` | bool | Yes | Enable shared memory |
| `memory.provider` | string | No | Memory provider |
| `ollamaCloud.apiKeySecretRef` | SecretKeySelector | No | Secret containing the Ollama Cloud API key (key: `api-key`). Used when `llm.provider` is `ollama-cloud` |
| `openCode.apiKeySecretRef` | SecretKeySelector | No | Secret containing the OpenCode API key (key: `api-key`). Used when `llm.provider` is `opencode` |
| `customProvider.url` | string | No | Base URL of a custom OpenAI-compatible LLM API (e.g. `https://api.openai.com/v1`). Required when `llm.provider` is `custom` (the `customProvider` block is optional, but `url` is mandatory when `customProvider` is specified) |
| `customProvider.apiKeySecretRef` | SecretKeySelector | No | Secret containing the custom provider API key (key: `api-key`) |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | `Ready`, `ConsumerReady`, `Degraded` |
| `replicas` | int32 | Containers running now (operator, from the Deployment) |
| `pendingTasks` | int32 | Tasks waiting for this agent — **written by the hub**, read by the operator |
| `activeTasks` | int32 | Tasks claimed and in flight — hub-written; scale-down waits for these to finish |
| `queueObservedAt` | Time | When the hub last *measured* this agent's queue — hub-written. If it goes stale the operator holds its container count instead of scaling down |
| `lastInvocation` | Time | Last queue activity (hub-written idle clock driving the quiet window). `nil` means the hub has never observed this agent, which is not the same as its queue being empty |
| `scaling` | AgentScalingStatus | The operator's decision: `desired` count, `mode` (`static` or `queue`), `reason` and `message` explaining it (`ScaledToZero`, `Draining`, `QueueSignalStale`, …) |
| `observedGeneration` | int64 | Last observed generation |

The hub and the operator both write this status, and neither clobbers the other's
fields: each patches only its own (`pendingTasks`, `activeTasks`, `queueObservedAt`,
`lastInvocation` for the hub; `replicas`, `scaling`, conditions for the operator). The
split is load-bearing — the operator scales on a drain signal it does not own, so it
must not be the one to clear it.

### Example

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: Agent
metadata:
  name: code-reviewer
  namespace: ainsel
spec:
  displayName: "Code Reviewer"
  description: "Reviews pull requests for code quality and correctness"
  imageRef:
    name: ainsel-ai-agent-latest
  runtime:
    imagePullPolicy: IfNotPresent
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 512Mi
  llm:
    model: glm-5.1:cloud
    provider: ollama-cloud
    maxTurns: 25
    temperature: 0.3
  persona:
    id: 01HQTS7XKZ4P9XRN3Q8X9XKXKW
  enabledTools:
    - forgejo
    - git
    - shell
  skills:
    items:
      - git-review
  mcp:
    servers:
      - name: memory-server
        url: http://memory-server.ainsel.svc.cluster.local:8080/mcp
  env:
    # Layered on top of the referenced image's env: LOG_LEVEL overrides the
    # image's value, FEATURE_FLAGS is added.
    - name: LOG_LEVEL
      value: debug
    - name: FEATURE_FLAGS
      value: beta
  scaling:
    # Static: the operator keeps two containers running regardless of queue depth.
    # Add `minReplicas: 0` to let this agent drain to nothing when idle and wake on
    # the next event instead, with 2 as its ceiling.
    replicas: 2
  memory:
    enabled: true
    provider: example
  ollamaCloud:
    apiKeySecretRef:
      name: ollama-cloud-apikey
      key: api-key
```

---

## AgentImage

Defines a container image for an agent along with its tool catalog,
environment variables, MCP server configuration, and sidecar containers.
Referenced by `Agent.spec.imageRef.name`.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `displayName` | string | Yes | Human-readable image name |
| `description` | string | No | Image description |
| `imageURL` | string | Yes | Container image URL (e.g. `localhost:30500/ainsel/ainsel-ai-agent:latest`) |
| `tools[]` | []AgentImageTool | No | Tools exposed by this image |
| `tools[].name` | string | Yes | Tool name |
| `tools[].kind` | string | Yes | Tool kind: `container`, `shell`, or `mcp` |
| `tools[].description` | string | No | Tool description |
| `tools[].mcpSource` | string | No | MCP source identifier (for `mcp` kind tools) |
| `tools[].disabled` | bool | No | Whether the tool is disabled (false = enabled) |
| `tools[].isNew` | bool | No | True if added by last refresh; cleared on next PUT |
| `tools[].examples[]` | []AgentImageToolExample | No | Usage examples for the tool |
| `tools[].examples[].title` | string | Yes | Example title |
| `tools[].examples[].snippet` | string | Yes | Example code snippet |
| `env[]` | []AgentImageEnvVar | No | Environment variables injected into every agent pod using this image. Operator-set vars take precedence. When `secret` is true, the hub API masks the value |
| `env[].name` | string | Yes | Environment variable name |
| `env[].value` | string | Yes | Environment variable value |
| `env[].secret` | bool | No | If true, the value is treated as sensitive and masked by the hub API |
| `mcpServers[]` | []AgentImageMCPServer | No | External MCP servers whose tools are imported into the image tool catalog via refresh |
| `mcpServers[].name` | string | Yes | MCP server name |
| `mcpServers[].url` | string | Yes | MCP server URL |
| `mcpServers[].tokenFromEnv` | string | No | Name of an agent-pod env var whose value is forwarded as the bearer token. Empty means anonymous. Must match `^[A-Z_][A-Z0-9_]*$` (max 64 chars) |
| `sidecars[]` | []AgentImageSidecar | No | Sidecar containers injected alongside the agent runtime in the same pod |
| `sidecars[].name` | string | Yes | Container name and MCP server name used in `MCP_SERVERS` |
| `sidecars[].image` | string | Yes | Container image URL |
| `sidecars[].port` | int32 | No | Container listen port (default: 8080) |
| `sidecars[].mcpPath` | string | No | When non-empty, the operator appends an `MCP_SERVERS` entry pointing to this sidecar (e.g. `/mcp`) |
| `sidecars[].env[]` | []AgentImageEnvVar | No | Environment variables for the sidecar container |
| `enabledSkills[]` | []string | No | List of skill names enabled for this image |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | `Ready` |

### Example

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: AgentImage
metadata:
  name: ainsel-ai-agent-latest
  namespace: ainsel
spec:
  displayName: "AInsel AI Agent (latest)"
  description: "Default agent image with forgejo, git, and shell tools"
  imageURL: localhost:30500/ainsel/ainsel-ai-agent:latest
  tools:
    - name: forgejo
      kind: container
      description: "Forgejo MCP server for issue/PR operations"
    - name: git
      kind: shell
      description: "Git operations via shell"
    - name: memory
      kind: mcp
      mcpSource: memory-server
      description: "Shared memory MCP server"
      examples:
        - title: Store a memory
          snippet: "memory_store(content=\"hello\", user_id=\"agent1\")"
  env:
    - name: LOG_LEVEL
      value: info
    - name: API_KEY
      value: secret-value
      secret: true
  mcpServers:
    - name: memory-server
      url: http://memory-mcp.ainsel.svc:8080/mcp
      tokenFromEnv: MEMORY_MCP_TOKEN
  sidecars:
    - name: sqlite-mcp
      image: localhost:30500/ainsel/sqlite-mcp:latest
      port: 9090
      mcpPath: /mcp
  enabledSkills:
    - code-review
    - pr-summary
```

---

## WebhookConnector

Configures a webhook receiver for an external source (e.g. Forgejo, GitHub),
including the webhook endpoint, HMAC verification, and container image.

> The previous `ForgejoConnector` CRD was replaced by this generic
> `WebhookConnector`. The old fields (`url`, `externalUrl`, `credentials`,
> `events`) no longer exist.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `displayName` | string | No | User-facing name |
| `webhookEndpoint` | string | Yes | URL to paste into the source webhook settings. Set by the hub when creating the connector; read-only thereafter |
| `signatureHeader` | string | Yes | HTTP header the sender puts the HMAC-SHA256 signature in (e.g. `X-Hub-Signature-256` for GitHub, `X-Forgejo-Signature` for Forgejo) |
| `webhookSecret.secretRef.name` | string | Yes | Name of the K8s Secret holding the MAC key |
| `webhookSecret.secretRef.key` | string | Yes | Key in the Secret (key: `secret`) |
| `image.repository` | string | Yes | Container image repository |
| `image.tag` | string | Yes | Container image tag |
| `disabled` | bool | No | Scales the Deployment to zero when true |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | `Ready`, `Disabled` |
| `observedGeneration` | int64 | Last observed generation |

### Example

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: WebhookConnector
metadata:
  name: c-0c4b01e3 # the connector's generated id — set by the hub, routes events
  namespace: ainsel
spec:
  displayName: "connector-forgejo-ainsel"
  webhookEndpoint: "https://ainsel.example.com/webhooks/c-0c4b01e3"
  signatureHeader: "X-Forgejo-Signature"
  webhookSecret:
    secretRef:
      name: connector-c-0c4b01e3-webhook-hmac
      key: secret
  image:
    repository: dpinsel/ainsel-webhook-receiver
    tag: latest
```

---

## Trigger

> **DB-backed (not a Kubernetes CRD).** `Trigger` was moved from a CRD to a
> database table. The Go type in
> `shared/api/api/v1alpha1/trigger_types.go` remains the canonical schema used
> by the hub REST API. It is documented here for schema completeness.

Routes events from a connector to an agent, with optional filters.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `displayName` | string | Yes | User-facing trigger name |
| `agentRef` | string | Yes | **Id** of the agent that should receive matching events (the `a-…` name of its `Agent` CR) — display names are not valid refs |
| `connectorRef` | string | Yes | **Id** of the connector that sources events (the `c-…` name of its `WebhookConnector` CR) — display names are not valid refs |
| `filters[]` | []Filter | No | Event filters to apply before delivering to the agent |

#### Filter

Each filter has three fields:

| Field | Type | Description |
|-------|------|-------------|
| `field` | string | Dot-separated path into the event payload (e.g. `repo`, `action`, `issue.labels`) |
| `op` | string | Comparison operator (see table below) |
| `value` | string | Comparison value (used by all operators except `in`/`not-in`) |
| `values` | []string | Comparison values for `in` and `not-in` operators |

**Supported operators:**

| Operator | Description |
|----------|-------------|
| `eq` | Exact string match |
| `neq` | Not equal |
| `contains` | Field contains the value as a substring |
| `not-contains` | Field does not contain the value |
| `prefix` | Field starts with the value |
| `suffix` | Field ends with the value |
| `in` | Field value is one of the entries in `values` |
| `not-in` | Field value is not in `values` |
| `regex` | Field matches the value as a regular expression |

Filters are combined with AND logic — all filters must match for the trigger to fire.

### Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | `AgentRefValid`, `ConnectorRefValid` |
| `observedGeneration` | int64 | Last observed generation |

### Example

```yaml
# DB-backed schema (not a Kubernetes CRD). Shown as YASL for illustration.
displayName: "Code Review on PR Open"
agentRef: a-3f9a2b
connectorRef: c-1a2b3c
filters:
  - field: type
    op: eq
    value: pull_request
```

---

## CronTrigger

> **DB-backed (not a Kubernetes CRD).** `CronTrigger` was moved from a CRD to
> a database table. The Go type in
> `shared/api/api/v1alpha1/crontrigger_types.go` remains the canonical schema
> used by the hub REST API. It is documented here for schema completeness.

Schedules a recurring prompt delivered to an agent on a cron schedule.
Unlike a webhook-driven `Trigger`, a `CronTrigger` has no connector — the
hub emits a synthetic event on the schedule (connector `cron`) and enqueues
the task directly for the trigger's agent.

### Spec

| Field | Type | Required | Description |
|--------|------|----------|-------------|
| `displayName` | string | Yes | User-facing cron trigger name |
| `agentRef` | string | Yes | Name of the Agent that should receive the scheduled prompt |
| `schedule` | string | Yes | Standard 5-field cron expression (minute hour day-of-month month day-of-week, in the hub's local time). E.g. `0 9 * * 1-5` fires at 09:00 on weekdays |
| `prompt` | string | Yes | Text delivered to the agent as the user message on each fire. Sent verbatim — no event template rendering |
| `enabled` | bool | No | Whether the schedule is active. Defaults to `true` when unset. Allows pausing without deleting |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | `AgentRefValid`, `ScheduleValid`, `Ready` |
| `lastRun` | Time | Last time the cron trigger fired and published an event |
| `nextRun` | Time | Next scheduled fire time computed by the hub |
| `observedGeneration` | int64 | Last observed generation |

### Example

```yaml
# DB-backed schema (not a Kubernetes CRD). Shown as YAML for illustration.
displayName: "Daily Code Review Summary"
agentRef: code-reviewer
schedule: "0 9 * * 1-5"
prompt: "Summarize all open pull requests and post a daily digest."
enabled: true
```